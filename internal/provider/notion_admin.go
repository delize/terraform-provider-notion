package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/jomei/notionapi"
)

// Notion's Admin API (reference added 2026-08-24) manages organization-level
// resources such as permission groups and workspace users. It differs from
// the regular API in three ways:
//
//   - Base URL https://api.notion.com/admin instead of /v1 at the root.
//   - Its own Notion-Version (2026-06-01), required on every request.
//   - An organization bot token, created in the organization console by an
//     owner and scoped per resource (for example permission-group:read).
//     Only Enterprise organizations get it.
//
// Requests go through doNotionRequestWithVersion, so they get the same 429
// handling and 5xx retry as the rest of the provider.

const (
	adminAPIBaseURL = "https://api.notion.com/admin"
	adminAPIVersion = "2026-06-01"
)

// adminBaseURL is a variable so unit tests can point the client at httptest.
var adminBaseURL = adminAPIBaseURL

type adminClient struct {
	token string
}

// adminAPIError is a non-2xx Admin API response. Status and Code come from
// the documented error body ({object, code, status, message}).
type adminAPIError struct {
	Status  int
	Code    string
	Message string
	Method  string
	Path    string
}

func (e *adminAPIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Code != "" {
		return fmt.Sprintf("notion admin API %d %s on %s %s: %s", e.Status, e.Code, e.Method, e.Path, msg)
	}
	return fmt.Sprintf("notion admin API %d on %s %s: %s", e.Status, e.Method, e.Path, msg)
}

func isAdminNotFound(err error) bool {
	var apiErr *adminAPIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// isAdminConflict reports a 409 conflict_error, which Notion returns for
// duplicate group names, changes to SCIM-managed groups, and deletes that
// need transfer_to_owner_id.
func isAdminConflict(err error) bool {
	var apiErr *adminAPIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict
}

// do sends one Admin API request. path starts with /v1. A nil out skips
// decoding the response.
func (c *adminClient) do(ctx context.Context, method, path string, query url.Values, body, out interface{}) error {
	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = b
	}

	u := adminBaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	resp, err := doNotionRequestWithVersion(ctx, method, u, c.token, adminAPIVersion, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &adminAPIError{Status: resp.StatusCode, Method: method, Path: path}
		var parsed struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &parsed) == nil {
			apiErr.Code, apiErr.Message = parsed.Code, parsed.Message
		} else {
			apiErr.Message = strings.TrimSpace(string(respBody))
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse admin API response for %s %s: %w", method, path, err)
	}
	return nil
}

// adminListSpec describes one list endpoint's pagination. The Admin API is
// not uniform across areas:
//
//   - Groups, users, workspaces, MCP connections, and PATs take
//     start_cursor and return results with has_more and next_cursor.
//   - Agent lists and agent permissions take cursor instead.
//   - Legal hold lists name the array after the resource (legal_holds,
//     user_ids, page_ids, workspace_ids), have no has_more, and omit
//     next_cursor on the last page.
//
// Every list stops when next_cursor is empty, and also when has_more is
// present and false.
type adminListSpec struct {
	Path        string
	Query       url.Values // extra filters, copied per request
	CursorParam string     // "start_cursor" when empty
	ResultsKey  string     // "results" when empty
	PageSize    int        // sent as page_size when > 0
}

// listPages walks every page of a list endpoint and calls each with every
// result. A false return from each stops early.
func (c *adminClient) listPages(ctx context.Context, spec adminListSpec, each func(json.RawMessage) (bool, error)) error {
	cursorParam := spec.CursorParam
	if cursorParam == "" {
		cursorParam = "start_cursor"
	}
	resultsKey := spec.ResultsKey
	if resultsKey == "" {
		resultsKey = "results"
	}

	cursor := ""
	for {
		q := url.Values{}
		for k, v := range spec.Query {
			q[k] = append([]string(nil), v...)
		}
		if spec.PageSize > 0 {
			q.Set("page_size", fmt.Sprint(spec.PageSize))
		}
		if cursor != "" {
			q.Set(cursorParam, cursor)
		}

		var page map[string]json.RawMessage
		if err := c.do(ctx, http.MethodGet, spec.Path, q, nil, &page); err != nil {
			return err
		}
		var results []json.RawMessage
		if raw, ok := page[resultsKey]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &results); err != nil {
				return fmt.Errorf("failed to parse %s from %s: %w", resultsKey, spec.Path, err)
			}
		}
		for _, raw := range results {
			more, err := each(raw)
			if err != nil {
				return err
			}
			if !more {
				return nil
			}
		}

		var next *string
		if raw, ok := page["next_cursor"]; ok {
			_ = json.Unmarshal(raw, &next)
		}
		if raw, ok := page["has_more"]; ok {
			var hasMore bool
			if json.Unmarshal(raw, &hasMore) == nil && !hasMore {
				return nil
			}
		}
		if next == nil || *next == "" {
			return nil
		}
		cursor = *next
	}
}

// listAll pages through a standard start_cursor list of 100 per page.
func (c *adminClient) listAll(ctx context.Context, path string, each func(json.RawMessage) (bool, error)) error {
	return c.listPages(ctx, adminListSpec{Path: path, PageSize: 100}, each)
}

// listInto collects every result of a list into out, decoding each element
// as T.
func listInto[T any](ctx context.Context, c *adminClient, spec adminListSpec) ([]T, error) {
	var out []T
	err := c.listPages(ctx, spec, func(raw json.RawMessage) (bool, error) {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return false, err
		}
		out = append(out, v)
		return true, nil
	})
	return out, err
}

func adminSpacePath(spaceID string, rest ...string) string {
	parts := []string{"/v1/spaces", url.PathEscape(spaceID)}
	for _, r := range rest {
		parts = append(parts, url.PathEscape(r))
	}
	return strings.Join(parts, "/")
}

// ---- Workspaces ----

type adminWorkspace struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

func (c *adminClient) ListWorkspaces(ctx context.Context) ([]adminWorkspace, error) {
	var out []adminWorkspace
	err := c.listAll(ctx, "/v1/spaces", func(raw json.RawMessage) (bool, error) {
		var w adminWorkspace
		if err := json.Unmarshal(raw, &w); err != nil {
			return false, err
		}
		out = append(out, w)
		return true, nil
	})
	return out, err
}

// ---- Users ----

type adminUser struct {
	ID             string  `json:"id"`
	Name           *string `json:"name"`
	Type           string  `json:"type"`
	MembershipType string  `json:"membership_type"`
	Person         *struct {
		Email string `json:"email"`
	} `json:"person"`
}

func (c *adminClient) ListUsers(ctx context.Context, spaceID string) ([]adminUser, error) {
	var out []adminUser
	err := c.listAll(ctx, adminSpacePath(spaceID, "users"), func(raw json.RawMessage) (bool, error) {
		var u adminUser
		if err := json.Unmarshal(raw, &u); err != nil {
			return false, err
		}
		out = append(out, u)
		return true, nil
	})
	return out, err
}

// ---- Permission groups ----

type adminGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *adminClient) ListGroups(ctx context.Context, spaceID string) ([]adminGroup, error) {
	var out []adminGroup
	err := c.listAll(ctx, adminSpacePath(spaceID, "groups"), func(raw json.RawMessage) (bool, error) {
		var g adminGroup
		if err := json.Unmarshal(raw, &g); err != nil {
			return false, err
		}
		out = append(out, g)
		return true, nil
	})
	return out, err
}

func (c *adminClient) GetGroup(ctx context.Context, spaceID, groupID string) (*adminGroup, error) {
	var g adminGroup
	if err := c.do(ctx, http.MethodGet, adminSpacePath(spaceID, "groups", groupID), nil, nil, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *adminClient) CreateGroup(ctx context.Context, spaceID, name string) (*adminGroup, error) {
	var g adminGroup
	if err := c.do(withNoReplay(ctx), http.MethodPost, adminSpacePath(spaceID, "groups"), nil, map[string]string{"name": name}, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *adminClient) RenameGroup(ctx context.Context, spaceID, groupID, name string) (*adminGroup, error) {
	var g adminGroup
	if err := c.do(ctx, http.MethodPatch, adminSpacePath(spaceID, "groups", groupID), nil, map[string]string{"name": name}, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// DeleteGroup deletes a group. transferToOwnerID is required by Notion only
// when deleting would orphan access, otherwise it may be empty.
func (c *adminClient) DeleteGroup(ctx context.Context, spaceID, groupID, transferToOwnerID string) error {
	var q url.Values
	if transferToOwnerID != "" {
		q = url.Values{"transfer_to_owner_id": {transferToOwnerID}}
	}
	return c.do(ctx, http.MethodDelete, adminSpacePath(spaceID, "groups", groupID), q, nil, nil)
}

// ---- Permission group members ----

type adminGroupMember struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

type adminGroupMembership struct {
	GroupID string           `json:"group_id"`
	Member  adminGroupMember `json:"member"`
	Role    string           `json:"role"`
}

// groupMemberRoles are the documented membership roles.
var groupMemberRoles = []string{"owner", "member"}

func (c *adminClient) AddGroupMember(ctx context.Context, spaceID, groupID, userID, role string) (*adminGroupMembership, error) {
	body := map[string]interface{}{
		"member": adminGroupMember{Type: "user", UserID: userID},
		"role":   role,
	}
	var m adminGroupMembership
	if err := c.do(ctx, http.MethodPost, adminSpacePath(spaceID, "groups", groupID, "members"), nil, body, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *adminClient) UpdateGroupMember(ctx context.Context, spaceID, groupID, userID, role string) (*adminGroupMembership, error) {
	var m adminGroupMembership
	path := adminSpacePath(spaceID, "groups", groupID, "members", "users", userID)
	if err := c.do(ctx, http.MethodPatch, path, nil, map[string]string{"role": role}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *adminClient) RemoveGroupMember(ctx context.Context, spaceID, groupID, userID string) error {
	path := adminSpacePath(spaceID, "groups", groupID, "members", "users", userID)
	return c.do(ctx, http.MethodDelete, path, nil, nil, nil)
}

// FindGroupMember returns a user's direct membership in a group, or nil when
// the user isn't a direct member. The API has no single-membership lookup,
// so this pages through List group members. IDs are compared without
// hyphens because Notion accepts both forms.
func (c *adminClient) FindGroupMember(ctx context.Context, spaceID, groupID, userID string) (*adminGroupMembership, error) {
	var found *adminGroupMembership
	want := normalizeID(userID)
	err := c.listAll(ctx, adminSpacePath(spaceID, "groups", groupID, "members"), func(raw json.RawMessage) (bool, error) {
		var m adminGroupMembership
		if err := json.Unmarshal(raw, &m); err != nil {
			return false, err
		}
		if normalizeID(m.Member.UserID) == want {
			found = &m
			return false, nil
		}
		return true, nil
	})
	return found, err
}

// ---- Provider plumbing ----

// adminTokens maps a configured *notionapi.Client to the admin token from
// the same provider block. Resources keep receiving *notionapi.Client as
// provider data, so existing resources don't change, and admin resources
// look their token up here.
var adminTokens sync.Map

func registerAdminToken(client *notionapi.Client, token string) {
	adminTokens.Store(client, token)
}

// adminClientFor returns the admin client for a provider block, or an error
// that tells the user how to set admin_token.
func adminClientFor(client *notionapi.Client) (*adminClient, error) {
	if client != nil {
		if v, ok := adminTokens.Load(client); ok && v.(string) != "" {
			return &adminClient{token: v.(string)}, nil
		}
	}
	return nil, errors.New("this resource uses the Notion Admin API, which needs an organization bot token. " +
		"Set admin_token in the provider block or the NOTION_ADMIN_TOKEN environment variable. " +
		"Admin API tokens are created by organization owners in the organization console and need an Enterprise plan")
}

// missingTokenTransport fails every request made with the regular API
// client when a provider block sets only admin_token. That keeps the
// regular resources unchanged while giving a clear error instead of a
// Notion 401.
type missingTokenTransport struct{}

func (missingTokenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no Notion API token is configured. This provider block only sets admin_token, " +
		"which covers the Admin API resources, data sources, and actions (permission groups, legal holds, agents, " +
		"MCP connections, personal access tokens, workspaces, exports). Set token or NOTION_TOKEN to use other resources")
}

// providerDataFrom extracts the *notionapi.Client every resource receives.
func providerDataFrom(data interface{}) (*notionapi.Client, error) {
	client, ok := data.(*notionapi.Client)
	if !ok {
		return nil, fmt.Errorf("expected *notionapi.Client, got: %T", data)
	}
	return client, nil
}
