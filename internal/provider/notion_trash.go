package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jomei/notionapi"
)

// The jomei/notionapi SDK pins Notion-Version "2022-06-28" and exposes only
// the deprecated `archived` field. Notion API version 2026-03-11 removed
// `archived` entirely in favor of `in_trash`. In current Notion, sending
// `archived: true` no longer fully moves items to trash — they remain
// visible in the sidebar. This file shims around the SDK with direct HTTP
// calls that use `in_trash` and a recent Notion-Version header, so deletes
// behave correctly until the upstream SDK catches up.

// notionAPIBaseURL is a variable so unit tests can point the direct HTTP
// helpers at a local fake.
var notionAPIBaseURL = "https://api.notion.com/v1"

const (
	notionTrashAPIVersion = "2026-03-11"
	// Mirrors the SDK's default (notionapi.Client.maxRetries = 3) so the
	// shim's rate-limit behavior matches the rest of the provider.
	notionTrashMaxRetries = 3
)

// trashHTTPClient is a package-level http.Client whose transport retries
// on 5xx and edge HTML errors (see retry_transport.go). doNotionRequest
// uses this so the trash shim gets the same treatment as the rest of the
// provider, on top of its own existing 429 retry loop below.
var trashHTTPClient = newRetryHTTPClient()

// clientTokens maps API client pointers to their bearer tokens. The
// provider's Configure stores the token here; the trash shim looks it up.
// This avoids changing every resource's Configure signature to plumb the
// token alongside the existing *notionapi.Client.
var clientTokens sync.Map

// registerClientToken records the token used to construct a client.
func registerClientToken(client *notionapi.Client, token string) {
	clientTokens.Store(client, token)
}

// tokenForClient returns the token for a given client, if registered.
func tokenForClient(client *notionapi.Client) (string, error) {
	v, ok := clientTokens.Load(client)
	if !ok {
		return "", fmt.Errorf("no Notion API token registered for client (provider Configure may not have run)")
	}
	return v.(string), nil
}

// doNotionRequest performs an HTTP request against the Notion API using the
// shim's default Notion-Version. See doNotionRequestWithVersion.
func doNotionRequest(ctx context.Context, method, url, token string, reqBody []byte) (*http.Response, error) {
	return doNotionRequestWithVersion(ctx, method, url, token, notionTrashAPIVersion, reqBody)
}

// doNotionRequestWithVersion performs an HTTP request against the Notion API
// with 429-retry semantics matching the upstream SDK: retry up to
// notionTrashMaxRetries times, honoring the Retry-After header. 5xx and edge
// HTML responses are retried one layer down by retryTransport. The caller
// owns closing the returned response body.
//
// Every direct-HTTP path in the provider should go through here. Notion now
// enforces a workspace-wide rate limit shared by all connections
// (2026-06-16) on top of the per-connection limit (2026-09-09), so a bare
// http.DefaultClient call is far more likely to surface a 429 to the user.
//
// reqBody is passed by value (not as a Reader) so each retry attempt can
// construct a fresh body without having to rewind a stream.
func doNotionRequestWithVersion(ctx context.Context, method, url, token, notionVersion string, reqBody []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		var body io.Reader
		if reqBody != nil {
			body = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Notion-Version", notionVersion)
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := httpClientForToken(token).Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		// Drain and close the 429 body before retrying.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if attempt+1 >= notionTrashMaxRetries {
			return nil, fmt.Errorf("notion API rate-limited %s %s after %d attempts", method, url, notionTrashMaxRetries)
		}

		retryAfter := 1
		if hdr := resp.Header.Get("Retry-After"); hdr != "" {
			if n, parseErr := strconv.Atoi(hdr); parseErr == nil && n > 0 {
				retryAfter = n
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(retryAfter) * time.Second):
		}
	}
}

// trashObject moves a Notion page or database to trash via the modern
// in_trash field. objectKind must be "pages" or "databases".
func trashObject(ctx context.Context, token, objectKind, id string) error {
	url := fmt.Sprintf("%s/%s/%s", notionAPIBaseURL, objectKind, id)
	body, err := json.Marshal(map[string]bool{"in_trash": true})
	if err != nil {
		return err
	}

	resp, err := doNotionRequest(ctx, http.MethodPatch, url, token, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API %d trashing %s/%s: %s", resp.StatusCode, objectKind, id, string(respBody))
	}
	return nil
}

// isObjectTrashed returns whether the given page or database has been moved
// to trash. Used by acceptance tests' CheckDestroy to verify the delete
// actually took effect (not just that the API returned success).
func isObjectTrashed(ctx context.Context, token, objectKind, id string) (bool, error) {
	url := fmt.Sprintf("%s/%s/%s", notionAPIBaseURL, objectKind, id)

	resp, err := doNotionRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("notion API %d fetching %s/%s: %s", resp.StatusCode, objectKind, id, string(body))
	}

	var result struct {
		InTrash  bool `json:"in_trash"`
		Archived bool `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.InTrash || result.Archived, nil
}
