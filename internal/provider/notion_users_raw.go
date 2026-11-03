package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// The jomei/notionapi SDK's Person type has only `email`. User objects carry
// `person.email_verified` since 2026-07-15, so the user data sources list
// users over direct HTTP. The field is a backwards-compatible addition that
// appears on every API version, so this stays on 2022-06-28 like the SDK.

type rawListedUser struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Person    *struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
	} `json:"person"`
}

func (u rawListedUser) email() string {
	if u.Person == nil {
		return ""
	}
	return u.Person.Email
}

func (u rawListedUser) emailVerified() *bool {
	if u.Person == nil {
		return nil
	}
	return u.Person.EmailVerified
}

type rawUsersPage struct {
	Results    []rawListedUser `json:"results"`
	HasMore    bool            `json:"has_more"`
	NextCursor *string         `json:"next_cursor"`
}

// listUsersRaw calls visit for each page of GET /v1/users until visit
// returns false or the list ends.
func listUsersRaw(ctx context.Context, token string, visit func([]rawListedUser) bool) error {
	return listUsersRawFrom(ctx, notionAPIBaseURL, token, visit)
}

func listUsersRawFrom(ctx context.Context, baseURL, token string, visit func([]rawListedUser) bool) error {
	cursor := ""
	for {
		q := url.Values{"page_size": {"100"}}
		if cursor != "" {
			q.Set("start_cursor", cursor)
		}
		body, err := doNotionGet(ctx, baseURL+"/users?"+q.Encode(), token, legacySchemaAPIVersion)
		if err != nil {
			return fmt.Errorf("listing users: %w", err)
		}
		var page rawUsersPage
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("failed to parse users response: %w", err)
		}
		if !visit(page.Results) {
			return nil
		}
		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			return nil
		}
		cursor = *page.NextCursor
	}
}

// doNotionGet performs a GET through the shared retry helper and returns the
// body, or an error carrying the status and body for non-2xx responses.
func doNotionGet(ctx context.Context, fullURL, token, notionVersion string) ([]byte, error) {
	resp, err := doNotionRequestWithVersion(ctx, http.MethodGet, fullURL, token, notionVersion, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
