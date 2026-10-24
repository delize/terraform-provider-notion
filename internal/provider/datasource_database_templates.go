package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// notion_database_templates wraps List data source templates (2026-01-15),
// GET /v1/data_sources/{id}/templates. It exists so users can find the
// template_id that notion_page accepts without copying URLs from Notion.
//
// The endpoint needs a data source ID, which only API versions from
// 2025-09-03 expose. Given a database_id, the data source is discovered from
// GET /v1/databases/{id} on 2026-03-11, the same way the view tests do.

var (
	_ datasource.DataSource                   = &DatabaseTemplatesDataSource{}
	_ datasource.DataSourceWithValidateConfig = &DatabaseTemplatesDataSource{}
)

type DatabaseTemplatesDataSource struct {
	client *notionapi.Client
}

type DatabaseTemplatesDataSourceModel struct {
	DatabaseID        types.String            `tfsdk:"database_id"`
	DataSourceID      types.String            `tfsdk:"data_source_id"`
	Name              types.String            `tfsdk:"name"`
	DefaultTemplateID types.String            `tfsdk:"default_template_id"`
	Templates         []DatabaseTemplateModel `tfsdk:"templates"`
}

type DatabaseTemplateModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	IsDefault types.Bool   `tfsdk:"is_default"`
}

func NewDatabaseTemplatesDataSource() datasource.DataSource {
	return &DatabaseTemplatesDataSource{}
}

func (d *DatabaseTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_templates"
}

func (d *DatabaseTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the page templates of a database's data source, for use as notion_page template_id.",
		Attributes: map[string]schema.Attribute{
			"database_id": schema.StringAttribute{
				Description: "The database to list templates for. Its data source is looked up automatically, which works when the database has exactly one. Set this or data_source_id.",
				Optional:    true,
			},
			"data_source_id": schema.StringAttribute{
				Description: "The data source to list templates for. Set this or database_id. When database_id is set, this reports the data source that was used.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Optional filter on template name, a case-insensitive substring match done by Notion.",
				Optional:    true,
			},
			"default_template_id": schema.StringAttribute{
				Description: "ID of the data source's default template, or null when none of the returned templates is the default.",
				Computed:    true,
			},
			"templates": schema.ListNestedAttribute{
				Description: "Templates the integration can access.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The template page ID.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The template name.",
							Computed:    true,
						},
						"is_default": schema.BoolAttribute{
							Description: "Whether this is the data source's default template.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *DatabaseTemplatesDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg DatabaseTemplatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.DatabaseID.IsUnknown() || cfg.DataSourceID.IsUnknown() {
		return
	}
	hasDB := !cfg.DatabaseID.IsNull() && cfg.DatabaseID.ValueString() != ""
	hasDS := !cfg.DataSourceID.IsNull() && cfg.DataSourceID.ValueString() != ""
	if hasDB == hasDS {
		resp.Diagnostics.AddAttributeError(path.Root("database_id"),
			"Set exactly one of database_id or data_source_id",
			"notion_database_templates needs either a database_id or a data_source_id, not both and not neither.")
	}
}

func (d *DatabaseTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DatabaseTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DatabaseTemplatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error listing templates", err.Error())
		return
	}

	dataSourceID := config.DataSourceID.ValueString()
	if dataSourceID == "" {
		dataSourceID, err = discoverDataSourceID(ctx, notionAPIBaseURL, token, normalizeID(config.DatabaseID.ValueString()))
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("database_id"), "Error finding the database's data source", err.Error())
			return
		}
	}

	templates, err := listDataSourceTemplates(ctx, notionAPIBaseURL, token, normalizeID(dataSourceID), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing templates", err.Error())
		return
	}

	config.DataSourceID = types.StringValue(normalizeID(dataSourceID))
	config.DefaultTemplateID = types.StringNull()
	config.Templates = make([]DatabaseTemplateModel, 0, len(templates))
	for _, tpl := range templates {
		id := normalizeID(tpl.ID)
		config.Templates = append(config.Templates, DatabaseTemplateModel{
			ID:        types.StringValue(id),
			Name:      types.StringValue(tpl.Name),
			IsDefault: types.BoolValue(tpl.IsDefault),
		})
		if tpl.IsDefault {
			config.DefaultTemplateID = types.StringValue(id)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// discoverDataSourceID returns the only data source of a database. A
// database with several data sources is an error that lists them, since
// picking one silently would return the wrong templates.
func discoverDataSourceID(ctx context.Context, baseURL, token, databaseID string) (string, error) {
	body, err := doNotionGet(ctx, fmt.Sprintf("%s/databases/%s", baseURL, databaseID), token, notionTrashAPIVersion)
	if err != nil {
		return "", err
	}
	var parsed struct {
		DataSources []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data_sources"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse database response: %w", err)
	}
	switch len(parsed.DataSources) {
	case 0:
		return "", fmt.Errorf("database %s returned no data sources", databaseID)
	case 1:
		return parsed.DataSources[0].ID, nil
	default:
		names := make([]string, 0, len(parsed.DataSources))
		for _, ds := range parsed.DataSources {
			names = append(names, fmt.Sprintf("%s (%s)", ds.Name, normalizeID(ds.ID)))
		}
		return "", fmt.Errorf("database %s has %d data sources, set data_source_id to one of: %s",
			databaseID, len(parsed.DataSources), strings.Join(names, ", "))
	}
}

type rawTemplate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

// listDataSourceTemplates pages through GET /v1/data_sources/{id}/templates
// on 2026-03-11.
func listDataSourceTemplates(ctx context.Context, baseURL, token, dataSourceID, name string) ([]rawTemplate, error) {
	var out []rawTemplate
	cursor := ""
	for {
		q := url.Values{"page_size": {"100"}}
		if name != "" {
			q.Set("name", name)
		}
		if cursor != "" {
			q.Set("start_cursor", cursor)
		}
		body, err := doNotionGet(ctx, fmt.Sprintf("%s/data_sources/%s/templates?%s", baseURL, dataSourceID, q.Encode()), token, notionTrashAPIVersion)
		if err != nil {
			return nil, err
		}
		var page struct {
			Templates  []rawTemplate `json:"templates"`
			HasMore    bool          `json:"has_more"`
			NextCursor *string       `json:"next_cursor"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to parse templates response: %w", err)
		}
		out = append(out, page.Templates...)
		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			return out, nil
		}
		cursor = *page.NextCursor
	}
}
