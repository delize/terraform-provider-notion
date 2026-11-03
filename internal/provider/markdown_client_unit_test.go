package provider

import "testing"

func TestPageMarkdownURL(t *testing.T) {
	cases := []struct {
		transcript bool
		version    string
		want       string
	}{
		{false, "", "https://api.notion.com/v1/pages/abc/markdown"},
		{true, "", "https://api.notion.com/v1/pages/abc/markdown?include_transcript=true"},
		{false, "v2", "https://api.notion.com/v1/pages/abc/markdown?markdown_version=v2"},
		{true, "v1", "https://api.notion.com/v1/pages/abc/markdown?include_transcript=true&markdown_version=v1"},
	}
	for _, c := range cases {
		if got := pageMarkdownURL("abc", c.transcript, c.version); got != c.want {
			t.Errorf("pageMarkdownURL(abc, %v, %q) = %q, want %q", c.transcript, c.version, got, c.want)
		}
	}
}
