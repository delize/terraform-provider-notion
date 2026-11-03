package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Notion's insert_content takes `content` and a position object.
func TestInsertContentBody(t *testing.T) {
	got := insertContentBody("## Hi", "end")
	b, _ := json.Marshal(got)
	want := `{"insert_content":{"content":"## Hi","position":{"type":"end"}},"type":"insert_content"}`
	if string(b) != want {
		t.Fatalf("body = %s, want %s", b, want)
	}
}

// The first read creates a query, a later page reads it by query ID.
func TestQueryViewPaths(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"view_query","id":"q1","results":[],"has_more":false}`))
	}))
	t.Cleanup(srv.Close)
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() { notionAPIBaseURL = prev })

	if _, err := queryView(t.Context(), "t", "v1", "", "", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := queryView(t.Context(), "t", "v1", "q1", "c2", 5); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /v1/views/v1/queries", "GET /v1/views/v1/queries/q1?page_size=5&start_cursor=c2"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %v, want %v", seen, want)
	}
}
