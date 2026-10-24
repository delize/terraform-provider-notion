package provider

import "testing"

func TestPageMarkdownURL(t *testing.T) {
	if got, want := pageMarkdownURL("abc", false), "https://api.notion.com/v1/pages/abc/markdown"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := pageMarkdownURL("abc", true), "https://api.notion.com/v1/pages/abc/markdown?include_transcript=true"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
