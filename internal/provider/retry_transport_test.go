package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestTransport returns a retryTransport tuned for fast unit tests:
// short delays so the suite doesn't spend whole seconds in time.Sleep.
func newTestTransport() *retryTransport {
	return &retryTransport{
		next:       http.DefaultTransport,
		maxRetries: 4,
		baseDelay:  1 * time.Millisecond,
		maxDelay:   10 * time.Millisecond,
	}
}

// TestRetryTransport_RetriesOn5xxThenSucceeds verifies the core fix: a
// 5xx response (the case that was surfacing as "invalid character '<'"
// to the SDK) is retried until the upstream returns 200.
func TestRetryTransport_RetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			// Cloudflare-style HTML body on a 503 — exactly the shape
			// that was breaking the SDK's JSON decoder.
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("<html><body>503 Service Unavailable</body></html>"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: newTestTransport()}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
	if got, want := attempts.Load(), int32(3); got != want {
		t.Errorf("attempts: got %d, want %d", got, want)
	}
}

// TestRetryTransport_RetriesOnHTMLBodyWith2xx covers the edge case where
// Cloudflare or a maintenance page returns 200 with HTML, which would
// also break the SDK's success-path JSON decoding.
func TestRetryTransport_RetriesOnHTMLBodyWith2xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 2 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<!DOCTYPE html><html>maintenance</html>"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: newTestTransport()}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q, want application/json prefix", ct)
	}
	if got, want := attempts.Load(), int32(2); got != want {
		t.Errorf("attempts: got %d, want %d", got, want)
	}
}

// TestRetryTransport_DoesNotRetry4xx ensures we don't waste attempts on
// client errors that aren't going to fix themselves.
func TestRetryTransport_DoesNotRetry4xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, `{"code":"unauthorized"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: newTestTransport()}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusUnauthorized; got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
	if got, want := attempts.Load(), int32(1); got != want {
		t.Errorf("attempts: got %d, want %d (4xx must not retry)", got, want)
	}
}

// TestRetryTransport_DoesNotRetry429 — the jomei SDK has its own 429
// retry loop; double-retrying here would interact badly with it.
func TestRetryTransport_DoesNotRetry429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"code":"rate_limited"}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: newTestTransport()}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusTooManyRequests; got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
	if got, want := attempts.Load(), int32(1); got != want {
		t.Errorf("attempts: got %d, want %d (429 must not retry; SDK handles it)", got, want)
	}
}

// TestRetryTransport_GivesUpAfterMaxRetries verifies the loop is bounded
// — if the upstream keeps failing, we eventually return what we have.
func TestRetryTransport_GivesUpAfterMaxRetries(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	rt := newTestTransport()
	client := &http.Client{Transport: rt}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusBadGateway; got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
	// initial attempt + maxRetries = maxRetries+1 total
	if got, want := attempts.Load(), int32(rt.maxRetries+1); got != want {
		t.Errorf("attempts: got %d, want %d", got, want)
	}
}

// TestRetryTransport_ContextCancel verifies the retry loop exits
// promptly when the caller's context is cancelled.
func TestRetryTransport_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "still failing", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the first retry sleep starts.
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	client := &http.Client{
		Transport: &retryTransport{
			next:       http.DefaultTransport,
			maxRetries: 10,
			baseDelay:  50 * time.Millisecond, // long enough for cancel to land
			maxDelay:   1 * time.Second,
		},
	}
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context cancellation, got: %v", err)
	}
}

// TestRetryTransport_ReplaysRequestBody — POST/PATCH/PUT must be safely
// retryable when GetBody is set (which is the default when net/http
// builds a request from bytes.Buffer / strings.Reader, which is what the
// notionapi SDK and the trash shim both use).
func TestRetryTransport_ReplaysRequestBody(t *testing.T) {
	var (
		attempts atomic.Int32
		bodies   []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		// Read body so we can compare across attempts.
		buf := make([]byte, 1024)
		nbytes, _ := r.Body.Read(buf)
		bodies = append(bodies, string(buf[:nbytes]))

		if n < 3 {
			http.Error(w, "transient", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: newTestTransport()}
	resp, err := client.Post(srv.URL, "application/json", strings.NewReader(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
	if got, want := attempts.Load(), int32(3); got != want {
		t.Fatalf("attempts: got %d, want %d", got, want)
	}
	want := `{"hello":"world"}`
	for i, b := range bodies {
		if b != want {
			t.Errorf("attempt %d body: got %q, want %q", i+1, b, want)
		}
	}
}

// TestShouldRetryResponse covers the policy matrix directly without
// spinning up an http server for every case.
func TestShouldRetryResponse(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		bodyType  string
		wantRetry bool
	}{
		{"500 InternalServerError", 500, "application/json", true},
		{"502 BadGateway", 502, "text/html", true},
		{"503 ServiceUnavailable", 503, "text/html", true},
		{"504 GatewayTimeout", 504, "application/json", true},
		{"200 with HTML body", 200, "text/html; charset=utf-8", true},
		{"200 with JSON body", 200, "application/json", false},
		{"204 NoContent", 204, "", false},
		{"400 BadRequest", 400, "application/json", false},
		{"401 Unauthorized", 401, "application/json", false},
		{"404 NotFound", 404, "application/json", false},
		{"429 TooManyRequests", 429, "application/json", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: c.status,
				Header:     http.Header{},
			}
			if c.bodyType != "" {
				resp.Header.Set("Content-Type", c.bodyType)
			}
			if got := shouldRetryResponse(resp); got != c.wantRetry {
				t.Errorf("shouldRetryResponse(%d, %q): got %v, want %v",
					c.status, c.bodyType, got, c.wantRetry)
			}
		})
	}
}

// TestComputeDelay_HonoursRetryAfter — if the upstream sends Retry-After,
// the transport sleeps for that duration (capped to maxDelay).
func TestComputeDelay_HonoursRetryAfter(t *testing.T) {
	rt := &retryTransport{
		baseDelay: 100 * time.Millisecond,
		maxDelay:  5 * time.Second,
	}
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "2")

	d := rt.computeDelay(1, resp)
	if d != 2*time.Second {
		t.Errorf("got %v, want 2s", d)
	}
}

// TestComputeDelay_CapsRetryAfterAtMaxDelay — an upstream telling us to
// wait an hour shouldn't actually make us wait an hour.
func TestComputeDelay_CapsRetryAfterAtMaxDelay(t *testing.T) {
	rt := &retryTransport{
		baseDelay: 100 * time.Millisecond,
		maxDelay:  3 * time.Second,
	}
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "3600")

	d := rt.computeDelay(1, resp)
	if d != 3*time.Second {
		t.Errorf("got %v, want 3s (maxDelay cap)", d)
	}
}

// TestComputeDelay_ExponentialBackoff — without Retry-After, attempt N
// produces base * 2^(N-1) (modulo ±20% jitter and the maxDelay cap).
func TestComputeDelay_ExponentialBackoff(t *testing.T) {
	rt := &retryTransport{
		baseDelay: 100 * time.Millisecond,
		maxDelay:  10 * time.Second,
	}
	// Run multiple iterations to make sure jitter doesn't push us
	// outside the expected band.
	for i := 0; i < 50; i++ {
		d := rt.computeDelay(3, nil) // expected: 400ms ± 20%
		lower := 320 * time.Millisecond
		upper := 480 * time.Millisecond
		if d < lower || d > upper {
			t.Fatalf("attempt 3 delay outside [%v, %v]: got %v", lower, upper, d)
		}
	}
}

// TestComputeDelay_CapsExponentialAtMaxDelay — even with a huge attempt
// number, we never sleep longer than maxDelay (plus jitter, which is a
// fraction of the delay so still bounded).
func TestComputeDelay_CapsExponentialAtMaxDelay(t *testing.T) {
	rt := &retryTransport{
		baseDelay: 100 * time.Millisecond,
		maxDelay:  1 * time.Second,
	}
	for attempt := 1; attempt <= 20; attempt++ {
		d := rt.computeDelay(attempt, nil)
		// 1.2 × maxDelay is the loosest upper bound (max possible
		// jitter on the cap).
		if d > time.Duration(float64(rt.maxDelay)*1.21) {
			t.Errorf("attempt %d delay exceeded cap: got %v", attempt, d)
		}
	}
}

// TestRetryTransport_RetriesOnNetworkError ensures we also retry when
// the transport itself returns an error (the upstream isn't reachable).
func TestRetryTransport_RetriesOnNetworkError(t *testing.T) {
	var attempts atomic.Int32
	failingNext := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		n := attempts.Add(1)
		if n < 2 {
			return nil, fmt.Errorf("simulated dial failure")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     http.Header{},
		}, nil
	})

	rt := newTestTransport()
	rt.next = failingNext

	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
	if got, want := attempts.Load(), int32(2); got != want {
		t.Errorf("attempts: got %d, want %d", got, want)
	}
}

// roundTripperFunc is the test-only equivalent of http.HandlerFunc — it
// adapts a plain function into a RoundTripper for tests that need to
// simulate transport-level behaviour without a real server.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
