package provider

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// retryTransport is an http.RoundTripper that retries transient failures
// before they reach the Notion API client.
//
// Motivation
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
//   - 2xx responses with an HTML body (Cloudflare "200 OK" maintenance
//     pages — rare but observed in production).
//
// What we do NOT retry
//
//   - 4xx other than 429. These are client errors and almost always
//     permanent; retrying would just waste time and surface a confusing
//     latency profile.
//   - 429. The jomei SDK already handles these with Retry-After semantics;
//     adding a second retry loop here would interact badly with the SDK's.
//   - Requests whose body cannot be replayed (no GetBody set). For
//     idempotent GET/HEAD/DELETE there's no body to replay; for
//     POST/PATCH the SDK constructs requests with bytes.Buffer which
//     causes net/http to set GetBody automatically, so this should be
//     non-issue in practice.
//
// Backoff
//
// Exponential with jitter, capped at maxDelay. If the response carries a
// Retry-After header, we honour it (capped to maxDelay).
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
			next:       http.DefaultTransport,
			maxRetries: 5,
			baseDelay:  500 * time.Millisecond,
			maxDelay:   30 * time.Second,
		},
		// Generous per-attempt timeout. The retry loop is bounded by
		// maxRetries × maxDelay anyway, so this just keeps a single
		// hung connection from holding things up forever.
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
			// Network/transport error — retry.
			lastErr, lastResp = err, nil
			continue
		}

		if !shouldRetryResponse(resp) {
			return resp, nil
		}

		// Mark for retry; the next iteration drains & closes the body.
		lastErr, lastResp = nil, resp
	}

	// Out of retries — surface the last thing we saw. Either it's a
	// retryable response we couldn't get past or a network error.
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

// shouldRetryResponse returns true if a response represents a transient
// failure that's worth retrying. See type comment for the full policy.
func shouldRetryResponse(resp *http.Response) bool {
	// 5xx is always transient enough to be worth a retry.
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}

	// Cloudflare and other edges occasionally serve HTML maintenance
	// pages with a 2xx status. Detect those: a Notion API success is
	// always application/json, so any 2xx HTML body is wrong.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ct := resp.Header.Get("Content-Type")
		if ct != "" && strings.HasPrefix(strings.ToLower(ct), "text/html") {
			return true
		}
	}

	return false
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
