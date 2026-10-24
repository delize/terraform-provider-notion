package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Notion's page write endpoints accept notifications.mode (2026-10-08).
// "silent" skips page update, @mention, and Person property notifications,
// while automations, reminders, and webhooks still run. The SDK has no field
// for it, so when the provider's silent_notifications is on, this
// RoundTripper adds it to the request body of:
//
//   - POST  /v1/pages                 (Create a page)
//   - PATCH /v1/pages/{id}            (Update page)
//   - PATCH /v1/blocks/{id}/children  (Append block children)
//
// Those are the endpoints whose reference documents the parameter. Other
// writes such as /markdown, /move, and Update a block are left alone.
type notificationsTransport struct {
	next http.RoundTripper
}

func (t *notificationsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isNotifyingWrite(req) || req.Body == nil {
		return t.next.RoundTrip(req)
	}

	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}

	out := req.Clone(req.Context())
	newBody := withSilentNotifications(body)
	out.Body = io.NopCloser(bytes.NewReader(newBody))
	out.ContentLength = int64(len(newBody))
	out.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBody)), nil
	}
	return t.next.RoundTrip(out)
}

func isNotifyingWrite(req *http.Request) bool {
	if req.URL == nil {
		return false
	}
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "v1" {
		return false
	}
	switch req.Method {
	case http.MethodPost:
		return len(parts) == 2 && parts[1] == "pages"
	case http.MethodPatch:
		if len(parts) == 3 && parts[1] == "pages" && parts[2] != "" {
			return true
		}
		return len(parts) == 4 && parts[1] == "blocks" && parts[2] != "" && parts[3] == "children"
	}
	return false
}

// withSilentNotifications adds notifications.mode=silent to a JSON object
// body. A body that already sets notifications, or isn't a JSON object, is
// returned unchanged.
func withSilentNotifications(body []byte) []byte {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		return body
	}
	if _, ok := obj["notifications"]; ok {
		return body
	}
	obj["notifications"] = json.RawMessage(`{"mode":"silent"}`)
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// providerHTTPClients maps a token to the http.Client its provider
// configured, so the direct-HTTP helpers honor provider settings such as
// silent_notifications. Two provider blocks that share a token but differ
// in settings would share whichever registered last, which is an accepted
// edge case.
var providerHTTPClients sync.Map

func registerHTTPClient(token string, c *http.Client) {
	providerHTTPClients.Store(token, c)
}

func httpClientForToken(token string) *http.Client {
	if v, ok := providerHTTPClients.Load(token); ok {
		return v.(*http.Client)
	}
	return trashHTTPClient
}

// newProviderHTTPClient builds the provider's http.Client: retries always,
// plus silent notifications when requested.
func newProviderHTTPClient(silent bool) *http.Client {
	c := newRetryHTTPClient()
	if silent {
		c.Transport = &notificationsTransport{next: c.Transport}
	}
	return c
}
