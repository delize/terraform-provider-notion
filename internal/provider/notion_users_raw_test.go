package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListUsersRaw_PaginatesAndParsesEmailVerified(t *testing.T) {
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users" {
			t.Errorf("path: got %s", r.URL.Path)
		}
		if v := r.Header.Get("Notion-Version"); v != legacySchemaAPIVersion {
			t.Errorf("Notion-Version: got %q", v)
		}
		cursors = append(cursors, r.URL.Query().Get("start_cursor"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start_cursor") == "" {
			_, _ = w.Write([]byte(`{"results":[
				{"id":"u-1","type":"person","name":"Ada","person":{"email":"ada@example.com","email_verified":true}},
				{"id":"b-1","type":"bot","name":"Bot","bot":{}}
			],"has_more":true,"next_cursor":"opaque-cursor"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[
			{"id":"u-2","type":"person","name":"Bo","person":{"email":"bo@example.com"}}
		],"has_more":false,"next_cursor":null}`))
	}))
	t.Cleanup(srv.Close)

	var got []UserDataModel
	err := listUsersRawFrom(context.Background(), srv.URL, "tok", func(users []rawListedUser) bool {
		for _, u := range users {
			got = append(got, userDataModelFor(u))
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(cursors) != 2 || cursors[1] != "opaque-cursor" {
		t.Errorf("cursors: %v", cursors)
	}
	if len(got) != 3 {
		t.Fatalf("users: got %d, want 3", len(got))
	}
	if !got[0].EmailVerified.ValueBool() || got[0].Email.ValueString() != "ada@example.com" {
		t.Errorf("person: %+v", got[0])
	}
	if !got[1].EmailVerified.IsNull() || got[1].Email.ValueString() != "" {
		t.Errorf("bot should have null email_verified and empty email: %+v", got[1])
	}
	if !got[2].EmailVerified.IsNull() {
		t.Errorf("missing email_verified should be null: %+v", got[2])
	}
}

func TestListUsersRaw_StopsWhenVisitReturnsFalse(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"u","type":"person","person":{"email":"a@b.c"}}],"has_more":true,"next_cursor":"next"}`))
	}))
	t.Cleanup(srv.Close)

	err := listUsersRawFrom(context.Background(), srv.URL, "tok", func([]rawListedUser) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls: got %d, want 1", calls)
	}
}
