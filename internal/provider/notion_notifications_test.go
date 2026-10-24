package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureServer records the last request body it saw per method+path.
func captureServer(t *testing.T) (*httptest.Server, map[string]string) {
	t.Helper()
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen[r.Method+" "+r.URL.Path] = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func notificationsMode(t *testing.T, body string) string {
	t.Helper()
	var obj struct {
		Notifications *struct {
			Mode string `json:"mode"`
		} `json:"notifications"`
	}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		t.Fatalf("body is not JSON: %q", body)
	}
	if obj.Notifications == nil {
		return ""
	}
	return obj.Notifications.Mode
}

func TestNotificationsTransport_InjectsOnPageWrites(t *testing.T) {
	srv, seen := captureServer(t)
	client := &http.Client{Transport: &notificationsTransport{next: http.DefaultTransport}}

	do := func(method, path, body string) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		resp.Body.Close()
	}

	do(http.MethodPost, "/v1/pages", `{"parent":{"page_id":"p"}}`)
	do(http.MethodPatch, "/v1/pages/abc", `{"in_trash":true}`)
	do(http.MethodPatch, "/v1/pages/abc/markdown", `{"type":"insert_content"}`)
	do(http.MethodPost, "/v1/pages/abc/move", `{"parent":{"page_id":"q"}}`)
	do(http.MethodPatch, "/v1/databases/abc", `{"properties":{}}`)
	do(http.MethodPatch, "/v1/blocks/abc/children", `{"children":[]}`)
	do(http.MethodPatch, "/v1/blocks/abc", `{"paragraph":{}}`)

	cases := map[string]string{
		"POST /v1/pages":                "silent",
		"PATCH /v1/pages/abc":           "silent",
		"PATCH /v1/pages/abc/markdown":  "",
		"POST /v1/pages/abc/move":       "",
		"PATCH /v1/databases/abc":       "",
		"PATCH /v1/blocks/abc/children": "silent",
		"PATCH /v1/blocks/abc":          "",
	}
	for key, want := range cases {
		body, ok := seen[key]
		if !ok {
			t.Errorf("%s: request not seen", key)
			continue
		}
		if got := notificationsMode(t, body); got != want {
			t.Errorf("%s: notifications.mode = %q, want %q (body %s)", key, got, want, body)
		}
	}
	// The original fields survive the rewrite.
	if !strings.Contains(seen["PATCH /v1/pages/abc"], `"in_trash":true`) {
		t.Errorf("in_trash lost: %s", seen["PATCH /v1/pages/abc"])
	}
}

func TestWithSilentNotifications_RespectsExplicitValue(t *testing.T) {
	in := []byte(`{"notifications":{"mode":"default"}}`)
	if got := string(withSilentNotifications(in)); got != string(in) {
		t.Errorf("explicit notifications overwritten: %s", got)
	}
}

func TestWithSilentNotifications_NonObjectUnchanged(t *testing.T) {
	for _, in := range []string{`[]`, `not json`, `null`} {
		if got := string(withSilentNotifications([]byte(in))); got != in {
			t.Errorf("withSilentNotifications(%q) = %q", in, got)
		}
	}
}

// The retry transport replays the rewritten body, not the original.
func TestNotificationsTransport_RetryReplaysSilentBody(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: &notificationsTransport{next: newTestTransport()}}
	resp, err := client.Post(srv.URL+"/v1/pages", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(bodies) != 2 {
		t.Fatalf("attempts: %d, want 2", len(bodies))
	}
	for i, b := range bodies {
		if notificationsMode(t, b) != "silent" {
			t.Errorf("attempt %d body missing silent mode: %s", i+1, b)
		}
	}
}
