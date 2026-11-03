package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Icons on pages and databases are a union on `type` (see the Emoji and icon
// reference). The jomei/notionapi SDK only models emoji, external, and file,
// so native icons (`type: "icon"`, 2026-03-25) and custom emoji
// (`type: "custom_emoji"`) go over direct HTTP on Notion-Version 2026-03-11,
// the version that documents them. is_locked (2025-09-13) rides along on the
// same requests.

const containerAPIVersion = "2026-03-11"

// nativeIconColors are the colors Notion accepts on a native icon.
var nativeIconColors = []string{"gray", "lightgray", "brown", "yellow", "orange", "green", "blue", "purple", "pink", "red"}

// iconFields is the set of icon attributes shared by notion_page and
// notion_database. Emoji is the legacy `icon` attribute.
type iconFields struct {
	Emoji         types.String
	Name          types.String
	Color         types.String
	URL           types.String
	CustomEmojiID types.String
}

func setString(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

// managed reports whether any of the non-emoji icon attributes is set.
func (f iconFields) managed() bool {
	return setString(f.Name) || setString(f.URL) || setString(f.CustomEmojiID)
}

// request returns the JSON icon object for the configured icon, or nil when
// no icon is configured.
func (f iconFields) request() map[string]interface{} {
	switch {
	case setString(f.Emoji):
		return map[string]interface{}{"type": "emoji", "emoji": f.Emoji.ValueString()}
	case setString(f.Name):
		icon := map[string]interface{}{"name": f.Name.ValueString()}
		if setString(f.Color) {
			icon["color"] = f.Color.ValueString()
		}
		return map[string]interface{}{"type": "icon", "icon": icon}
	case setString(f.URL):
		return map[string]interface{}{"type": "external", "external": map[string]string{"url": f.URL.ValueString()}}
	case setString(f.CustomEmojiID):
		return map[string]interface{}{"type": "custom_emoji", "custom_emoji": map[string]string{"id": f.CustomEmojiID.ValueString()}}
	}
	return nil
}

// parsedIcon is a page or database icon as Notion returns it.
type parsedIcon struct {
	Type          string
	Emoji         string
	Name          string
	Color         string
	URL           string
	CustomEmojiID string
}

func parseIcon(raw json.RawMessage) parsedIcon {
	if len(raw) == 0 || string(raw) == "null" {
		return parsedIcon{}
	}
	var v struct {
		Type  string `json:"type"`
		Emoji string `json:"emoji"`
		Icon  struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"icon"`
		External struct {
			URL string `json:"url"`
		} `json:"external"`
		File struct {
			URL string `json:"url"`
		} `json:"file"`
		CustomEmoji struct {
			ID string `json:"id"`
		} `json:"custom_emoji"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return parsedIcon{}
	}
	p := parsedIcon{Type: v.Type}
	switch v.Type {
	case "emoji":
		p.Emoji = v.Emoji
	case "icon":
		p.Name, p.Color = v.Icon.Name, v.Icon.Color
	case "external":
		p.URL = v.External.URL
	case "file":
		p.URL = v.File.URL
	case "custom_emoji":
		p.CustomEmojiID = v.CustomEmoji.ID
	}
	return p
}

// normalizeIconName folds case and treats spaces, hyphens, and underscores
// alike, which is how Notion matches icon picker names.
func normalizeIconName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer(" ", "_", "-", "_").Replace(s)
}

// reconcile maps a server icon onto the prior state. Only the non-emoji
// attributes the configuration manages are tracked, so an icon set in the
// Notion UI doesn't show up as drift on an attribute nobody configured.
// Emoji is always reported, matching the legacy `icon` attribute.
func (f iconFields) reconcile(p parsedIcon) iconFields {
	out := iconFields{
		Emoji:         types.StringValue(""),
		Name:          types.StringNull(),
		Color:         types.StringNull(),
		URL:           types.StringNull(),
		CustomEmojiID: types.StringNull(),
	}
	if p.Type == "emoji" {
		out.Emoji = types.StringValue(p.Emoji)
	}

	if !f.Name.IsNull() {
		if p.Type == "icon" {
			if normalizeIconName(f.Name.ValueString()) == normalizeIconName(p.Name) {
				out.Name = f.Name
			} else {
				// Picker aliases (for example "star circle" for "token") read
				// back as the internal name. That shows as a diff, and the docs
				// tell users to configure the internal name.
				out.Name = types.StringValue(p.Name)
			}
			switch {
			case !f.Color.IsNull():
				out.Color = types.StringValue(p.Color)
			case p.Color != "" && p.Color != "gray":
				// Unset color means gray. Surface anything else as drift.
				out.Color = types.StringValue(p.Color)
			}
		}
	}
	if !f.URL.IsNull() && p.Type == "external" {
		out.URL = types.StringValue(p.URL)
	}
	if !f.CustomEmojiID.IsNull() && p.Type == "custom_emoji" {
		if normalizeID(f.CustomEmojiID.ValueString()) == normalizeID(p.CustomEmojiID) {
			out.CustomEmojiID = f.CustomEmojiID
		} else {
			out.CustomEmojiID = types.StringValue(p.CustomEmojiID)
		}
	}
	return out
}

// isNotionIconSetURL reports whether u points at Notion's built-in icon set,
// such as https://www.notion.so/icons/pizza_blue.svg. Notion turns those into
// native icons (2026-09-23), so they would read back as a different type.
func isNotionIconSetURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	if host != "notion.so" && host != "notion.com" && host != "app.notion.com" {
		return false
	}
	return strings.HasPrefix(parsed.Path, "/icons/")
}

// validateIconFields checks the icon attributes of a configuration. The
// path names are the attribute names used by both resources.
func validateIconFields(f iconFields, diags *diag.Diagnostics) {
	var set []string
	if setString(f.Emoji) {
		set = append(set, "icon")
	}
	if setString(f.Name) {
		set = append(set, "icon_name")
	}
	if setString(f.URL) {
		set = append(set, "icon_url")
	}
	if setString(f.CustomEmojiID) {
		set = append(set, "icon_custom_emoji_id")
	}
	if len(set) > 1 {
		diags.AddAttributeError(path.Root(set[1]), "Conflicting icon attributes",
			fmt.Sprintf("Only one of icon, icon_name, icon_url, and icon_custom_emoji_id can be set. Found %s.", strings.Join(set, ", ")))
	}

	if setString(f.Color) {
		if !f.Name.IsUnknown() && !setString(f.Name) {
			diags.AddAttributeError(path.Root("icon_color"), "icon_color needs icon_name",
				"icon_color only applies to a native icon set with icon_name.")
		}
		if !slices.Contains(nativeIconColors, f.Color.ValueString()) {
			diags.AddAttributeError(path.Root("icon_color"), "Invalid icon color",
				fmt.Sprintf("%q is not a native icon color. Use one of: %s.", f.Color.ValueString(), strings.Join(nativeIconColors, ", ")))
		}
	}

	if setString(f.URL) {
		u := f.URL.ValueString()
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			diags.AddAttributeError(path.Root("icon_url"), "Invalid icon URL",
				fmt.Sprintf("%q is not an absolute http(s) URL.", u))
		} else if isNotionIconSetURL(u) {
			diags.AddAttributeError(path.Root("icon_url"), "Use icon_name for Notion icons",
				"Notion turns URLs from its own icon set into native icons, which read back as icon_name and icon_color, "+
					"so this would never stop showing a diff. Set icon_name (and icon_color) instead.")
		}
	}
}

// containerExtras is the slice of a page or database object this file reads.
type containerExtras struct {
	Icon     parsedIcon
	IsLocked bool
}

func decodeContainerExtras(body []byte) (containerExtras, error) {
	var v struct {
		Icon     json.RawMessage `json:"icon"`
		IsLocked bool            `json:"is_locked"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return containerExtras{}, fmt.Errorf("failed to parse response: %w", err)
	}
	return containerExtras{Icon: parseIcon(v.Icon), IsLocked: v.IsLocked}, nil
}

// containerRequest sends a request on Notion-Version 2026-03-11 and returns
// the response body. kind is "pages" or "databases".
func containerRequest(ctx context.Context, token, method, kind, id string, body map[string]interface{}) ([]byte, error) {
	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = b
	}
	u := fmt.Sprintf("%s/%s/%s", notionAPIBaseURL, kind, id)
	resp, err := doNotionRequestWithVersion(ctx, method, u, token, containerAPIVersion, reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion API %d on %s %s/%s: %s", resp.StatusCode, method, kind, id, string(respBody))
	}
	return respBody, nil
}

// patchContainerExtras sets icon and is_locked on a page or database. A nil
// icon with clearIcon true sends `"icon": null`. Returns the extras from the
// response. With nothing to change it falls back to a GET.
func patchContainerExtras(ctx context.Context, token, kind, id string, icon map[string]interface{}, clearIcon bool, isLocked types.Bool) (containerExtras, error) {
	body := map[string]interface{}{}
	switch {
	case icon != nil:
		body["icon"] = icon
	case clearIcon:
		body["icon"] = nil
	}
	if !isLocked.IsNull() && !isLocked.IsUnknown() {
		body["is_locked"] = isLocked.ValueBool()
	}

	method := http.MethodPatch
	if len(body) == 0 {
		method, body = http.MethodGet, nil
	}
	respBody, err := containerRequest(ctx, token, method, kind, id, body)
	if err != nil {
		return containerExtras{}, err
	}
	return decodeContainerExtras(respBody)
}
