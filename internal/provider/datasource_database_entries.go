package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	var cursor notionapi.Cursor

	for {
		result, err := d.client.Database.Query(ctx, notionapi.DatabaseID(config.Database.ValueString()), &notionapi.DatabaseQueryRequest{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error querying database", err.Error())
			return
		}

		for i := range result.Results {
			page := &result.Results[i]
			entry := DatabaseEntryDataModel{
				ID:  types.StringValue(normalizeID(string(page.ID))),
				URL: types.StringValue(page.URL),
			}

			// Extract all properties as string values
			props := make(map[string]string)
			for name, prop := range page.Properties {
				val := propertyToString(prop)
				props[name] = val
				if _, ok := prop.(*notionapi.TitleProperty); ok {
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
		cursor = result.NextCursor
	}

	config.Entries = entries
	if config.Entries == nil {
		config.Entries = []DatabaseEntryDataModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// propertyToString converts a Notion property value to its string representation.
func propertyToString(prop notionapi.Property) string {
	switch p := prop.(type) {
	case *notionapi.TitleProperty:
		return richTextToPlain(p.Title)
	case *notionapi.RichTextProperty:
		return richTextToPlain(p.RichText)
	case *notionapi.TextProperty:
		return richTextToPlain(p.Text)
	case *notionapi.NumberProperty:
		return fmt.Sprintf("%g", p.Number)
	case *notionapi.SelectProperty:
		return p.Select.Name
	case *notionapi.MultiSelectProperty:
		names := make([]string, len(p.MultiSelect))
		for i, opt := range p.MultiSelect {
			names[i] = opt.Name
		}
		return strings.Join(names, ", ")
	case *notionapi.DateProperty:
		if p.Date != nil && p.Date.Start != nil {
			return time.Time(*p.Date.Start).Format(time.RFC3339)
		}
		return ""
	case *notionapi.CheckboxProperty:
		if p.Checkbox {
			return "true"
		}
		return "false"
	case *notionapi.URLProperty:
		return p.URL
	case *notionapi.EmailProperty:
		return p.Email
	case *notionapi.PhoneNumberProperty:
		return p.PhoneNumber
	case *notionapi.PeopleProperty:
		names := make([]string, len(p.People))
		for i, user := range p.People {
			names[i] = user.Name
		}
		return strings.Join(names, ", ")
	case *notionapi.RelationProperty:
		ids := make([]string, len(p.Relation))
		for i, rel := range p.Relation {
			ids[i] = string(rel.ID)
		}
		return strings.Join(ids, ", ")
	case *notionapi.CreatedTimeProperty:
		return p.CreatedTime.Format(time.RFC3339)
	case *notionapi.CreatedByProperty:
		return p.CreatedBy.Name
	case *notionapi.LastEditedTimeProperty:
		return p.LastEditedTime.Format(time.RFC3339)
	case *notionapi.LastEditedByProperty:
		return p.LastEditedBy.Name
	case *notionapi.StatusProperty:
		return p.Status.Name
	case *notionapi.UniqueIDProperty:
		return p.UniqueID.String()
	case *notionapi.FormulaProperty:
		switch p.Formula.Type {
		case "string":
			return p.Formula.String
		case "number":
			return fmt.Sprintf("%g", p.Formula.Number)
		case "boolean":
			return fmt.Sprintf("%t", p.Formula.Boolean)
		case "date":
			if p.Formula.Date != nil && p.Formula.Date.Start != nil {
				return time.Time(*p.Formula.Date.Start).Format(time.RFC3339)
			}
		}
		return ""
	case *notionapi.RollupProperty:
		switch p.Rollup.Type {
		case "number":
			return fmt.Sprintf("%g", p.Rollup.Number)
		case "date":
			if p.Rollup.Date != nil && p.Rollup.Date.Start != nil {
				return time.Time(*p.Rollup.Date.Start).Format(time.RFC3339)
			}
		}
		return ""
	case *notionapi.FilesProperty:
		names := make([]string, len(p.Files))
		for i, f := range p.Files {
			names[i] = f.Name
		}
		return strings.Join(names, ", ")
	default:
		return ""
	}
}
