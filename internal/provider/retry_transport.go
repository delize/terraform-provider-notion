package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// retryTransport is an http.RoundTripper that retries transient failures
// before they reach the Notion API client.
//
// # Motivation
//
// The jomei/notionapi SDK retries only on HTTP 429 (Too Many Requests). On
// any other non-2xx response it reads the body and attempts to JSON-decode
// it as a Notion API error. When Notion's edge (Cloudflare) serves an HTML
// error page for a transient 5xx, the body starts with "<" and JSON
// decoding fails with the unhelpful error:
//
//	invalid character '<' looking for beginning of value
//
// That error masks the real cause (a 502/503/504) and the operation is
// permanently failed for the caller, even though a retry would almost
// always succeed. This RoundTripper retries those cases at the HTTP layer
// — the SDK never sees them.
//
// What we retry
//
//   - Network/transport errors returned by the underlying RoundTripper
//     (DNS failure, connection refused, EOF mid-stream, etc.).
//   - 5xx responses (500, 502, 503, 504, …) regardless of body content.
//   - Any response with an HTML body: Cloudflare "200 OK" maintenance
//     pages, and the 4xx pages Notion's edge serves while throttling an
//     integration. The API itself only ever answers in JSON.
//
// What we do NOT retry
//
//   - A JSON 409 "conflict_error", even for requests marked withNoReplay.
//     Notion rolls those transactions back, so nothing was saved.
//   - JSON 4xx other than 409 conflicts and 429. These are client errors and almost always
//     permanent; retrying would just waste time and surface a confusing
//     latency profile.
//   - 429. The jomei SDK already handles these with Retry-After semantics;
//     adding a second retry loop here would interact badly with the SDK's.
//   - A JSON 504 with code "gateway_timeout" on POST or PATCH. Since
//     2026-09-24 Notion returns this (instead of a 500) when a markdown
//     write timed out, and says to check whether the write saved before
//     retrying. Replaying a non-idempotent write such as insert_content
//     could duplicate content, so the response goes back to the caller.
//     Edge HTML 504s are still retried, since those never reached the API.
//   - A request marked with withNoReplay once it reached the API. That
//     covers any JSON response and any transport error other than a failed
//     dial. Creates such as a legal hold or an export have no idempotency
//     key, so a resend after a server error could make a second one. Edge
//     HTML errors and dial failures are still retried, since the request
//     never got to Notion.
//   - Requests whose body cannot be replayed (no GetBody set). For
//     idempotent GET/HEAD/DELETE there's no body to replay; for
//     POST/PATCH the SDK constructs requests with bytes.Buffer which
//     causes net/http to set GetBody automatically, so this should be
//     non-issue in practice.
//
// # Backoff
//
// Exponential with jitter, capped at maxDelay. If the response carries a
// Retry-After header, we honour it (capped to maxDelay). 429s, where Notion
// sends Retry-After values up to 60s, are handled by the SDK and by
// doNotionRequestWithVersion rather than here, so this cap only applies to
// 5xx retries.
type retryTransport struct {
	next       http.RoundTripper // underlying transport; defaults to http.DefaultTransport
	maxRetries int               // total attempts = maxRetries + 1
	baseDelay  time.Duration     // initial backoff for the first retry
	maxDelay   time.Duration     // upper bound on any single sleep
}

// newRetryHTTPClient returns a *http.Client wired with retryTransport. Use
// this anywhere we'd otherwise reach for http.DefaultClient or pass a
// *http.Client to the notionapi SDK.
func newRetryHTTPClient() *http.Client {
	return &http.Client{
		Transport: &retryTransport{
			next: http.DefaultTransport,
			// Six retries ride out about 30s of edge errors (0.5s doubling
			// to 16s), well inside the 90s client timeout.
			maxRetries: 6,
			baseDelay:  500 * time.Millisecond,
			maxDelay:   30 * time.Second,
		},
		// http.Client.Timeout covers the whole call, so every attempt and
		// every backoff sleep share these 90 seconds. Retries stop when it
		// runs out, even if maxRetries hasn't been reached.
		Timeout: 90 * time.Second,
	}
}

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var (
		lastResp *http.Response
		lastErr  error
	)

	for attempt := 0; attempt <= rt.maxRetries; attempt++ {
		if attempt > 0 {
			delay := rt.computeDelay(attempt, lastResp)
			// Free any previous response before sleeping. We've already
			// decided to retry it.
			drainAndClose(lastResp)
			lastResp = nil

			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}

			// Replay the request body if possible. net/http sets GetBody
			// automatically when the request is constructed with a
			// *bytes.Buffer, *bytes.Reader, or *strings.Reader, which
			// covers everything the notionapi SDK and the trash shim do.
			if req.Body != nil {
				if req.GetBody == nil {
					// Non-replayable body — return whatever we last had.
					if lastErr != nil {
						return nil, lastErr
					}
					return nil, fmt.Errorf(
						"notion retry transport: request body is not replayable; cannot retry %s %s",
						req.Method, req.URL,
					)
				}
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("notion retry transport: GetBody failed: %w", err)
				}
				req.Body = body
			}
		}

		resp, err := rt.next.RoundTrip(req)
		if err != nil {
			if noReplay(req) && !isDialError(err) {
				return nil, err
			}
			// Network/transport error — retry.
			lastErr, lastResp = err, nil
			continue
		}

		// A conflict_error means Notion rolled the transaction back because
		// another write to the same object landed first. Nothing was saved,
		// so it's retried even for requests marked withNoReplay.
		if isConflictError(resp) {
			lastErr, lastResp = nil, resp
			continue
		}

		if isWriteGatewayTimeout(req, resp) || (noReplay(req) && isJSONResponse(resp)) || !shouldRetryResponse(resp) {
			return edgeErrorAsJSON(resp), nil
		}

		// Mark for retry; the next iteration drains & closes the body.
		lastErr, lastResp = nil, resp
	}

	// Out of retries — surface the last thing we saw. Either it's a
	// retryable response we couldn't get past or a network error.
	if lastResp != nil {
		return edgeErrorAsJSON(lastResp), nil
	}
	return nil, lastErr
}

type noReplayKey struct{}

// withNoReplay marks requests made with ctx as unsafe to resend once they
// reached Notion. Use it for POSTs that create something and have no
// idempotency key.
func withNoReplay(ctx context.Context) context.Context {
	return context.WithValue(ctx, noReplayKey{}, true)
}

func noReplay(req *http.Request) bool {
	v, _ := req.Context().Value(noReplayKey{}).(bool)
	return v
}

// isDialError reports whether err happened while connecting, before any of
// the request was sent.
func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// isJSONResponse reports whether resp came from the Notion API itself
// rather than an edge error page.
func isJSONResponse(resp *http.Response) bool {
	return strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "application/json")
}

// edgeErrorAsJSON replaces an HTML error page with a JSON Notion error that
// names the HTTP status. Without it the SDK fails to decode the HTML and
// reports "invalid character '<' looking for beginning of value", which
// hides what happened. A 2xx HTML page that survived every retry becomes a
// 502. JSON responses pass through untouched.
func edgeErrorAsJSON(resp *http.Response) *http.Response {
	html := strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/html")
	if !html || (resp.StatusCode < 400 && !shouldRetryResponse(resp)) {
		return resp
	}
	drainAndClose(resp)
	status := resp.StatusCode
	if status < 400 {
		status = http.StatusBadGateway
	}
	msg := fmt.Sprintf("Notion returned HTTP %d with an HTML page instead of a JSON response. "+
		"This is usually a temporary problem at Notion's edge, so try again.", resp.StatusCode)
	body := fmt.Sprintf(`{"object":"error","status":%d,"code":"notion_edge_error","message":%q}`, status, msg)
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	resp.Header = resp.Header.Clone()
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Length")
	resp.ContentLength = int64(len(body))
	resp.Body = io.NopCloser(strings.NewReader(body))
	return resp
}

// shouldRetryResponse returns true if a response represents a transient
// failure that's worth retrying. See type comment for the full policy.
func shouldRetryResponse(resp *http.Response) bool {
	// 5xx is always transient enough to be worth a retry.
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}

	// Notion's API always answers in JSON, so an HTML page never came from
	// the API itself. Notion's edge serves them as 2xx maintenance pages and
	// as 4xx pages while it throttles an integration. Retry any of them.
	if resp.StatusCode >= 200 {
		ct := resp.Header.Get("Content-Type")
		if ct != "" && strings.HasPrefix(strings.ToLower(ct), "text/html") {
			return true
		}
	}

	return false
}

// isConflictError reports whether resp is Notion's JSON 409
// "conflict_error", which Notion documents as a transaction that "could not
// be completed" and should be tried again. Terraform's parallel applies hit it
// when several blocks are written to the same page at once. The body is
// buffered and restored so the caller can still read the error.
func isConflictError(resp *http.Response) bool {
	if resp.StatusCode != http.StatusConflict || !isJSONResponse(resp) {
		return false
	}
	return peekErrorCode(resp) == "conflict_error"
}

// peekErrorCode reads the "code" field of a JSON error body and puts the body
// back, so the caller still sees it untouched.
func peekErrorCode(resp *http.Response) string {
	if resp.Body == nil {
		return ""
	}
	orig := resp.Body
	body, err := io.ReadAll(io.LimitReader(orig, 1<<16))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(body), orig), orig}
	if err != nil {
		return ""
	}
	var apiErr struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &apiErr) != nil {
		return ""
	}
	return apiErr.Code
}

// isWriteGatewayTimeout reports whether resp is Notion's JSON
// "gateway_timeout" 504 for a POST or PATCH. The write may have been saved,
// so it must not be blindly replayed (see the type comment). The body is
// buffered and restored so the caller can still read the error.
func isWriteGatewayTimeout(req *http.Request, resp *http.Response) bool {
	if resp.StatusCode != http.StatusGatewayTimeout {
		return false
	}
	if req.Method != http.MethodPost && req.Method != http.MethodPatch {
		return false
	}
	if !isJSONResponse(resp) {
		return false
	}
	return peekErrorCode(resp) == "gateway_timeout"
}

// computeDelay returns the backoff for a given retry attempt. If the
// response carries a Retry-After header (as seconds) we honour it, capped
// to maxDelay; otherwise we use exponential backoff with jitter.
//
// attempt is 1-indexed: the delay before retry #1 uses attempt=1.
func (rt *retryTransport) computeDelay(attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if hdr := resp.Header.Get("Retry-After"); hdr != "" {
			if secs, err := strconv.Atoi(hdr); err == nil && secs > 0 {
				d := time.Duration(secs) * time.Second
				if d > rt.maxDelay {
					d = rt.maxDelay
				}
				return d
			}
		}
	}

	// Exponential: base * 2^(attempt-1)
	d := rt.baseDelay << (attempt - 1)
	if d <= 0 || d > rt.maxDelay {
		d = rt.maxDelay
	}

	// ±20% jitter. Failing to read randomness just skips the jitter —
	// not worth surfacing as an error.
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		// Map uint64 to [-0.2, +0.2].
		n := binary.LittleEndian.Uint64(buf[:])
		j := (float64(n)/float64(^uint64(0)))*0.4 - 0.2
		d = time.Duration(float64(d) * (1.0 + j))
	}
	if d < 0 {
		d = 0
	}
	return d
}

// drainAndClose discards and closes a response body so the underlying
// connection can be reused on the next attempt.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
