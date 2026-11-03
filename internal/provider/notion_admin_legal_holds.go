package provider

import (
	"context"
	"net/http"
	"net/url"
)

// Legal hold endpoints of the Admin API (organization-level, not scoped to
// one workspace). Scopes: legal-hold:read, legal-hold:write,
// legal-hold:write-high-impact (release), legal-hold:export.
//
// The reference types start_date, end_date, and created_time as plain
// numbers without a unit. Other Admin API endpoints document epoch
// milliseconds, so the provider passes the numbers through unchanged and
// documents them as epoch milliseconds.

// legalHoldInteractionTypes are the documented user_interaction_type values.
var legalHoldInteractionTypes = []string{"page.created", "page.edited", "page.viewed"}

type adminIDSet struct {
	IDs   []string `json:"ids"`
	Total float64  `json:"total"`
}

type adminLegalHold struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	Icon                string     `json:"icon"`
	CreatedBy           string     `json:"created_by"`
	CreatedTime         float64    `json:"created_time"`
	StartDate           float64    `json:"start_date"`
	EndDate             *float64   `json:"end_date"`
	Status              string     `json:"status"`
	UserInteractionType []string   `json:"user_interaction_type"`
	Users               adminIDSet `json:"users"`
	Workspaces          adminIDSet `json:"workspaces"`
}

type legalHoldCreate struct {
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Icon                string   `json:"icon,omitempty"`
	StartDate           int64    `json:"start_date"`
	EndDate             *int64   `json:"end_date,omitempty"`
	UserIDs             []string `json:"user_ids"`
	UserInteractionType []string `json:"user_interaction_type"`
}

// legalHoldUpdate sends only the fields that changed. A pointer to "" clears
// a description or icon.
type legalHoldUpdate struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

func legalHoldPath(id string, rest ...string) string {
	p := "/v1/legal_holds/" + url.PathEscape(id)
	for _, r := range rest {
		p += "/" + url.PathEscape(r)
	}
	return p
}

func (c *adminClient) ListLegalHolds(ctx context.Context, userID string) ([]adminLegalHold, error) {
	q := url.Values{}
	if userID != "" {
		q.Set("user_id", userID)
	}
	return listInto[adminLegalHold](ctx, c, adminListSpec{Path: "/v1/legal_holds", Query: q, ResultsKey: "legal_holds"})
}

func (c *adminClient) GetLegalHold(ctx context.Context, id string) (*adminLegalHold, error) {
	var h adminLegalHold
	if err := c.do(ctx, http.MethodGet, legalHoldPath(id), nil, nil, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (c *adminClient) CreateLegalHold(ctx context.Context, req legalHoldCreate) (*adminLegalHold, error) {
	var h adminLegalHold
	if err := c.do(withNoReplay(ctx), http.MethodPost, "/v1/legal_holds", nil, req, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (c *adminClient) UpdateLegalHold(ctx context.Context, id string, req legalHoldUpdate) (*adminLegalHold, error) {
	var h adminLegalHold
	if err := c.do(ctx, http.MethodPatch, legalHoldPath(id), nil, req, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (c *adminClient) AddLegalHoldUsers(ctx context.Context, id string, userIDs []string) error {
	return c.do(ctx, http.MethodPost, legalHoldPath(id, "users"), nil, map[string][]string{"user_ids": userIDs}, nil)
}

func (c *adminClient) RemoveLegalHoldUser(ctx context.Context, id, userID string) error {
	return c.do(ctx, http.MethodDelete, legalHoldPath(id, "users", userID), nil, nil, nil)
}

// ReleaseLegalHold ends a hold. This is a compliance action and needs the
// legal-hold:write-high-impact scope.
func (c *adminClient) ReleaseLegalHold(ctx context.Context, id string) (*adminLegalHold, error) {
	var h adminLegalHold
	if err := c.do(ctx, http.MethodPost, legalHoldPath(id, "release"), nil, nil, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (c *adminClient) ListLegalHoldUsers(ctx context.Context, id string) ([]string, error) {
	return listInto[string](ctx, c, adminListSpec{Path: legalHoldPath(id, "users"), ResultsKey: "user_ids"})
}

func (c *adminClient) ListLegalHoldWorkspaces(ctx context.Context, id string) ([]string, error) {
	return listInto[string](ctx, c, adminListSpec{Path: legalHoldPath(id, "workspaces"), ResultsKey: "workspace_ids"})
}

func (c *adminClient) ListLegalHoldPages(ctx context.Context, id, spaceID string) ([]string, error) {
	return listInto[string](ctx, c, adminListSpec{Path: legalHoldPath(id, "spaces", spaceID, "pages"), ResultsKey: "page_ids"})
}

// ExportLegalHold starts an export, or continues an existing one when
// exportID is set. The reference gives two body shapes: requesting_user_id
// with space_id, or requesting_user_id with legal_hold_export_id.
func (c *adminClient) ExportLegalHold(ctx context.Context, id, requestingUserID, spaceID, exportID string) (string, error) {
	body := map[string]string{"requesting_user_id": requestingUserID}
	if exportID != "" {
		body["legal_hold_export_id"] = exportID
	} else {
		body["space_id"] = spaceID
	}
	var out struct {
		LegalHoldExportID string `json:"legal_hold_export_id"`
	}
	if err := c.do(withNoReplay(ctx), http.MethodPost, legalHoldPath(id, "export"), nil, body, &out); err != nil {
		return "", err
	}
	return out.LegalHoldExportID, nil
}
