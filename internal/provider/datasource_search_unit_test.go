package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestBuildSearchBody(t *testing.T) {
	cases := []struct {
		name          string
		query, object string
		inTrash       bool
		cursor        string
		want          string
	}{
		{"empty", "", "", false, "", `{"page_size":100}`},
		{"object only", "q", "page", false, "", `{"filter":{"property":"object","value":"page"},"page_size":100,"query":"q"}`},
		{"trash only", "", "", true, "", `{"filter":{"in_trash":true},"page_size":100}`},
		{"trash with object and cursor", "", "database", true, "c1", `{"filter":{"in_trash":true,"property":"object","value":"database"},"page_size":100,"start_cursor":"c1"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := json.Marshal(buildSearchBody(c.query, c.object, c.inTrash, c.cursor))
			var gotV, wantV interface{}
			_ = json.Unmarshal(got, &gotV)
			_ = json.Unmarshal([]byte(c.want), &wantV)
			if !reflect.DeepEqual(gotV, wantV) {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestDoSearch_DecodesPagesAndDatabases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if v := r.Header.Get("Notion-Version"); v != legacySchemaAPIVersion {
			t.Errorf("Notion-Version: %q", v)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"filter":{"in_trash":true},"page_size":100}` {
			t.Errorf("body: %s", b)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","results":[
			{"object":"page","id":"11111111-1111-1111-1111-111111111111","in_trash":true,"parent":{"type":"workspace","workspace":true},
			 "properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Old page"}]}}},
			{"object":"database","id":"22222222-2222-2222-2222-222222222222","title":[{"plain_text":"Old DB"}],"parent":{"type":"page_id","page_id":"33333333-3333-3333-3333-333333333333"}}
		],"has_more":false,"next_cursor":null}`))
	}))
	t.Cleanup(srv.Close)

	page, err := doSearch(context.Background(), srv.URL, "tok", buildSearchBody("", "", true, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 2 {
		t.Fatalf("results: %d", len(page.Results))
	}
	p := searchResultFor(page.Results[0])
	if p.Title.ValueString() != "Old page" || p.Object.ValueString() != "page" {
		t.Errorf("page result: %+v", p)
	}
	db := searchResultFor(page.Results[1])
	if db.Title.ValueString() != "Old DB" || db.ParentID.ValueString() != "33333333333333333333333333333333" {
		t.Errorf("database result: %+v", db)
	}
}

func TestDoSearch_SurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"object":"error","code":"validation_error","message":"bad filter"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := doSearch(context.Background(), srv.URL, "tok", buildSearchBody("", "", true, "")); err == nil {
		t.Fatal("expected error")
	}
}
