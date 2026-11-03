package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseEntriesQuery(t *testing.T) {
	q, err := parseEntriesQuery("", "")
	if err != nil || q.Filter != nil || q.Sorts != nil {
		t.Fatalf("empty: %+v, %v", q, err)
	}

	q, err = parseEntriesQuery(
		`{"property":"Due","date":{"on_or_before":"today"}}`,
		`[{"property":"Due","direction":"ascending"}]`,
	)
	if err != nil || q.Filter == nil || q.Sorts == nil {
		t.Fatalf("valid: %+v, %v", q, err)
	}

	for _, bad := range []struct{ filter, sorts string }{
		{`[1,2]`, ""},    // filter must be an object
		{`not json`, ""}, // filter must parse
		{"", `{"a":1}`},  // sorts must be an array
		{"", `null`},     // null is not an array
		{`null`, ""},     // null is not an object
	} {
		if _, err := parseEntriesQuery(bad.filter, bad.sorts); err == nil {
			t.Errorf("expected error for filter=%q sorts=%q", bad.filter, bad.sorts)
		}
	}
}

// The filter and sorts reach Notion unchanged, alongside paging, and the
// 10K request_status signal still parses.
func TestQueryDatabaseRawAt_PassesFilterAndSorts(t *testing.T) {
	const filter = `{"property":"Status","status":{"equals":["Done","Blocked"]}}`
	const sorts = `[{"timestamp":"last_edited_time","direction":"descending"}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/databases/db1/query" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if v := r.Header.Get("Notion-Version"); v != legacySchemaAPIVersion {
			t.Errorf("Notion-Version: %q", v)
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("body: %s", b)
		}
		if string(body["filter"]) != filter || string(body["sorts"]) != sorts {
			t.Errorf("filter/sorts not passed through: %s", b)
		}
		if string(body["start_cursor"]) != `"cur"` || string(body["page_size"]) != "100" {
			t.Errorf("paging: %s", b)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"has_more":true,"next_cursor":"x","request_status":{"type":"incomplete","incomplete_reason":"query_result_limit_reached"}}`))
	}))
	t.Cleanup(srv.Close)

	q, err := parseEntriesQuery(filter, sorts)
	if err != nil {
		t.Fatal(err)
	}
	res, err := queryDatabaseRawAt(context.Background(), srv.URL, "tok", "db1", q, "cur")
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestStatus == nil || res.RequestStatus.Type != "incomplete" {
		t.Errorf("request_status: %+v", res.RequestStatus)
	}
}

func TestBuildEntriesQueryBody_OmitsUnsetFields(t *testing.T) {
	b, _ := json.Marshal(buildEntriesQueryBody(entriesQuery{}, ""))
	if string(b) != `{"page_size":100}` {
		t.Errorf("got %s", b)
	}
}
