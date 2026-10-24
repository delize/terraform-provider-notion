package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestListCustomEmojis_PaginatesWithNameFilter(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom_emojis" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if v := r.Header.Get("Notion-Version"); v != notionTrashAPIVersion {
			t.Errorf("Notion-Version: %q", v)
		}
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start_cursor") == "" {
			_, _ = w.Write([]byte(`{"object":"list","type":"custom_emoji","results":[{"id":"e1","name":"party parrot","url":"https://example.com/1.png"}],"has_more":true,"next_cursor":"c2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","type":"custom_emoji","results":[{"id":"e2","name":"party parrot","url":"https://example.com/2.png"}],"has_more":false,"next_cursor":null}`))
	}))
	t.Cleanup(srv.Close)

	got, err := listCustomEmojis(context.Background(), srv.URL, "tok", "party parrot")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].ID != "e2" {
		t.Fatalf("emojis: %+v", got)
	}
	if queries[0] != "name=party+parrot&page_size=100" {
		t.Errorf("first query: %s", queries[0])
	}
	if queries[1] != "name=party+parrot&page_size=100&start_cursor=c2" {
		t.Errorf("second query: %s", queries[1])
	}
}

// TestAccCustomEmojisDataSource lists custom emojis. Most test workspaces
// have none, so only a successful read is asserted.
func TestAccCustomEmojisDataSource(t *testing.T) {
	if os.Getenv("NOTION_TOKEN") == "" {
		t.Skip("NOTION_TOKEN not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "notion_custom_emojis" "all" {}`,
				Check:  resource.TestMatchResourceAttr("data.notion_custom_emojis.all", "emojis.#", regexp.MustCompile(`^[0-9]+$`)),
			},
		},
	})
}
