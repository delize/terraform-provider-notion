package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &DatabaseEntriesDataSource{}

type DatabaseEntriesDataSource struct {
	client *notionapi.Client
}

type DatabaseEntriesDataSourceModel struct {
	Database types.String             `tfsdk:"database"`
	Entries  []DatabaseEntryDataModel `tfsdk:"entries"`
}

type DatabaseEntryDataModel struct {
	ID         types.String `tfsdk:"id"`
	Title      types.String `tfsdk:"title"`
	URL        types.String `tfsdk:"url"`
	Properties types.Map    `tfsdk:"properties"`
}

func NewDatabaseEntriesDataSource() datasource.DataSource {
	return &DatabaseEntriesDataSource{}
}

func (d *DatabaseEntriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_entries"
}

func (d *DatabaseEntriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Query all entries in a Notion database.",
		Attributes: map[string]schema.Attribute{
			"database": schema.StringAttribute{
				Description: "The ID of the database to query.",
				Required:    true,
			},
			"entries": schema.ListNestedAttribute{
				Description: "List of database entries.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The ID of the entry.",
							Computed:    true,
						},
						"title": schema.StringAttribute{
							Description: "The title of the entry.",
							Computed:    true,
						},
						"url": schema.StringAttribute{
							Description: "The URL of the entry.",
							Computed:    true,
						},
						"properties": schema.MapAttribute{
							Description: "A map of property names to their string values.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *DatabaseEntriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	d.client = client
}

func (d *DatabaseEntriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DatabaseEntriesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var entries []DatabaseEntryDataModel
	var startCursor string

	for {
		result, err := d.queryDatabaseRaw(ctx, config.Database.ValueString(), startCursor)
		if err != nil {
			resp.Diagnostics.AddError("Error querying database", err.Error())
			return
		}

		for _, page := range result.Results {
			entry := DatabaseEntryDataModel{
				ID:  types.StringValue(normalizeID(page.ID)),
				URL: types.StringValue(page.URL),
			}

			props := make(map[string]string)
			for name, prop := range page.Properties {
				val := rawPropertyToString(prop)
				props[name] = val
				if prop.Type == "title" {
					entry.Title = types.StringValue(val)
				}
			}

			if entry.Title.IsNull() {
				entry.Title = types.StringValue("")
			}

			propMap := make(map[string]types.String, len(props))
			for k, v := range props {
				propMap[k] = types.StringValue(v)
			}
			mapVal, diags := types.MapValueFrom(ctx, types.StringType, propMap)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			entry.Properties = mapVal

			entries = append(entries, entry)
		}

		if !result.HasMore {
			break
		}
		startCursor = result.NextCursor
	}

	config.Entries = entries
	if config.Entries == nil {
		config.Entries = []DatabaseEntryDataModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// Raw JSON types for manual parsing (bypasses SDK's strict type checking)

type rawQueryResponse struct {
	Results    []rawPage `json:"results"`
	HasMore    bool      `json:"has_more"`
	NextCursor string    `json:"next_cursor"`
}

type rawPage struct {
	ID         string                 `json:"id"`
	URL        string                 `json:"url"`
	Properties map[string]rawProperty `json:"properties"`
}

type rawProperty struct {
	Type        string          `json:"type"`
	Title       json.RawMessage `json:"title,omitempty"`
	RichText    json.RawMessage `json:"rich_text,omitempty"`
	Number      *float64        `json:"number,omitempty"`
	Select      *rawOption      `json:"select,omitempty"`
	MultiSelect []rawOption     `json:"multi_select,omitempty"`
	Date        *rawDate        `json:"date,omitempty"`
	Checkbox    *bool           `json:"checkbox,omitempty"`
	URL         *string         `json:"url,omitempty"`
	Email       *string         `json:"email,omitempty"`
	PhoneNumber *string         `json:"phone_number,omitempty"`
	People      []rawUser       `json:"people,omitempty"`
	Relation    []rawRelation   `json:"relation,omitempty"`
	Formula     *rawFormula     `json:"formula,omitempty"`
	Rollup      *rawRollup      `json:"rollup,omitempty"`
	Status      *rawOption      `json:"status,omitempty"`
	UniqueID    *rawUniqueID    `json:"unique_id,omitempty"`
	CreatedTime *string         `json:"created_time,omitempty"`
	CreatedBy   *rawUser        `json:"created_by,omitempty"`
	LastEditedTime *string      `json:"last_edited_time,omitempty"`
	LastEditedBy   *rawUser     `json:"last_edited_by,omitempty"`
	Files       []rawFile       `json:"files,omitempty"`
}

type rawOption struct {
	Name string `json:"name"`
}

type rawDate struct {
	Start string `json:"start"`
	End   string `json:"end,omitempty"`
}

type rawUser struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type rawRelation struct {
	ID string `json:"id"`
}

type rawFormula struct {
	Type    string   `json:"type"`
	String  string   `json:"string,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Date    *rawDate `json:"date,omitempty"`
}

type rawRollup struct {
	Type   string   `json:"type"`
	Number *float64 `json:"number,omitempty"`
	Date   *rawDate `json:"date,omitempty"`
}

type rawUniqueID struct {
	Prefix *string `json:"prefix,omitempty"`
	Number int     `json:"number"`
}

type rawFile struct {
	Name string `json:"name"`
}

type rawRichText struct {
	PlainText string `json:"plain_text"`
}

// queryDatabaseRaw queries the Notion API directly, bypassing the SDK's
// strict property type checking that fails on unsupported types like "place".
func (d *DatabaseEntriesDataSource) queryDatabaseRaw(ctx context.Context, databaseID string, startCursor string) (*rawQueryResponse, error) {
	body := map[string]interface{}{
		"page_size": 100,
	}
	if startCursor != "" {
		body["start_cursor"] = startCursor
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", databaseID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.client.Token.String()))
	httpReq.Header.Set("Notion-Version", "2022-06-28")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to query database: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Notion API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var result rawQueryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// rawPropertyToString converts a raw property to its string representation.
// Unknown property types are gracefully returned as empty strings.
func rawPropertyToString(prop rawProperty) string {
	switch prop.Type {
	case "title":
		return extractRichText(prop.Title)
	case "rich_text":
		return extractRichText(prop.RichText)
	case "number":
		if prop.Number != nil {
			return fmt.Sprintf("%g", *prop.Number)
		}
		return ""
	case "select":
		if prop.Select != nil {
			return prop.Select.Name
		}
		return ""
	case "multi_select":
		names := make([]string, len(prop.MultiSelect))
		for i, opt := range prop.MultiSelect {
			names[i] = opt.Name
		}
		return strings.Join(names, ", ")
	case "date":
		if prop.Date != nil {
			return prop.Date.Start
		}
		return ""
	case "checkbox":
		if prop.Checkbox != nil && *prop.Checkbox {
			return "true"
		}
		return "false"
	case "url":
		if prop.URL != nil {
			return *prop.URL
		}
		return ""
	case "email":
		if prop.Email != nil {
			return *prop.Email
		}
		return ""
	case "phone_number":
		if prop.PhoneNumber != nil {
			return *prop.PhoneNumber
		}
		return ""
	case "people":
		names := make([]string, len(prop.People))
		for i, user := range prop.People {
			names[i] = user.Name
		}
		return strings.Join(names, ", ")
	case "relation":
		ids := make([]string, len(prop.Relation))
		for i, rel := range prop.Relation {
			ids[i] = rel.ID
		}
		return strings.Join(ids, ", ")
	case "created_time":
		if prop.CreatedTime != nil {
			return *prop.CreatedTime
		}
		return ""
	case "created_by":
		if prop.CreatedBy != nil {
			return prop.CreatedBy.Name
		}
		return ""
	case "last_edited_time":
		if prop.LastEditedTime != nil {
			return *prop.LastEditedTime
		}
		return ""
	case "last_edited_by":
		if prop.LastEditedBy != nil {
			return prop.LastEditedBy.Name
		}
		return ""
	case "status":
		if prop.Status != nil {
			return prop.Status.Name
		}
		return ""
	case "unique_id":
		if prop.UniqueID != nil {
			if prop.UniqueID.Prefix != nil && *prop.UniqueID.Prefix != "" {
				return fmt.Sprintf("%s-%d", *prop.UniqueID.Prefix, prop.UniqueID.Number)
			}
			return fmt.Sprintf("%d", prop.UniqueID.Number)
		}
		return ""
	case "formula":
		if prop.Formula != nil {
			switch prop.Formula.Type {
			case "string":
				return prop.Formula.String
			case "number":
				if prop.Formula.Number != nil {
					return fmt.Sprintf("%g", *prop.Formula.Number)
				}
			case "boolean":
				if prop.Formula.Boolean != nil {
					return fmt.Sprintf("%t", *prop.Formula.Boolean)
				}
			case "date":
				if prop.Formula.Date != nil {
					return prop.Formula.Date.Start
				}
			}
		}
		return ""
	case "rollup":
		if prop.Rollup != nil {
			switch prop.Rollup.Type {
			case "number":
				if prop.Rollup.Number != nil {
					return fmt.Sprintf("%g", *prop.Rollup.Number)
				}
			case "date":
				if prop.Rollup.Date != nil {
					return prop.Rollup.Date.Start
				}
			}
		}
		return ""
	case "files":
		names := make([]string, len(prop.Files))
		for i, f := range prop.Files {
			names[i] = f.Name
		}
		return strings.Join(names, ", ")
	default:
		// Unknown property types (e.g. "place") are gracefully skipped
		return ""
	}
}

func extractRichText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var texts []rawRichText
	if err := json.Unmarshal(raw, &texts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, t := range texts {
		sb.WriteString(t.PlainText)
	}
	return sb.String()
}
