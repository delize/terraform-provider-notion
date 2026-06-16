package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// The jomei/notionapi SDK doesn't expose the 2026-05-11 meeting notes query
// endpoint, so this data source goes direct to the API via doNotionRequest
// (same pattern as notion_trash.go and notion_comments.go).

var _ datasource.DataSource = &MeetingNotesDataSource{}

type MeetingNotesDataSource struct {
	client *notionapi.Client
}

type MeetingNotesDataSourceModel struct {
	Filter  types.String       `tfsdk:"filter"`
	Sort    types.String       `tfsdk:"sort"`
	Limit   types.Int64        `tfsdk:"limit"`
	Results []MeetingNoteModel `tfsdk:"results"`
	RawJSON types.String       `tfsdk:"raw_json"`
}

type MeetingNoteModel struct {
	ID             types.String `tfsdk:"id"`
	Object         types.String `tfsdk:"object"`
	CreatedTime    types.String `tfsdk:"created_time"`
	LastEditedTime types.String `tfsdk:"last_edited_time"`
	URL            types.String `tfsdk:"url"`
	Title          types.String `tfsdk:"title"`
}

func NewMeetingNotesDataSource() datasource.DataSource {
	return &MeetingNotesDataSource{}
}

func (d *MeetingNotesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_meeting_notes"
}

func (d *MeetingNotesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Query AI meeting notes for the integration's user via the 2026-05-11 " +
			"`POST /v1/blocks/meeting_notes/query` endpoint. Returns the common identifying " +
			"fields per result plus the raw JSON response for callers that need the full shape.",
		Attributes: map[string]schema.Attribute{
			"filter": schema.StringAttribute{
				Description: "Optional filter as a JSON object string (e.g. `jsonencode({attendees = [\"user-id\"]})`). " +
					"The `attendees` alias is normalized server-side so filters round-trip cleanly.",
				Optional: true,
			},
			"sort": schema.StringAttribute{
				Description: "Optional sort as a JSON object/array string.",
				Optional:    true,
			},
			"limit": schema.Int64Attribute{
				Description: "Optional maximum number of results to return.",
				Optional:    true,
			},
			"results": schema.ListNestedAttribute{
				Description: "Meeting notes returned by the query.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The meeting note's ID.",
							Computed:    true,
						},
						"object": schema.StringAttribute{
							Description: "The Notion object type string.",
							Computed:    true,
						},
						"created_time": schema.StringAttribute{
							Description: "ISO-8601 timestamp the meeting note was created.",
							Computed:    true,
						},
						"last_edited_time": schema.StringAttribute{
							Description: "ISO-8601 timestamp of last edit.",
							Computed:    true,
						},
						"url": schema.StringAttribute{
							Description: "Notion URL of the meeting note, if present in the response.",
							Computed:    true,
						},
						"title": schema.StringAttribute{
							Description: "Plain-text title of the meeting note, if present in the response.",
							Computed:    true,
						},
					},
				},
			},
			"raw_json": schema.StringAttribute{
				Description: "The full raw JSON response from the endpoint. Use `jsondecode` to access fields " +
					"not surfaced individually (e.g. attendees, transcripts).",
				Computed: true,
			},
		},
	}
}

func (d *MeetingNotesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// meetingNoteRaw is the subset of fields surfaced as typed attributes. Any
// other fields remain accessible via raw_json.
type meetingNoteRaw struct {
	ID             string          `json:"id"`
	Object         string          `json:"object"`
	CreatedTime    string          `json:"created_time"`
	LastEditedTime string          `json:"last_edited_time"`
	URL            string          `json:"url"`
	Title          json.RawMessage `json:"title"`
}

func (d *MeetingNotesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config MeetingNotesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{}
	if !config.Filter.IsNull() && config.Filter.ValueString() != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(config.Filter.ValueString()), &parsed); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("filter"),
				"Invalid filter JSON",
				fmt.Sprintf("filter must be a JSON object string: %s", err.Error()),
			)
			return
		}
		body["filter"] = parsed
	}
	if !config.Sort.IsNull() && config.Sort.ValueString() != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(config.Sort.ValueString()), &parsed); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("sort"),
				"Invalid sort JSON",
				fmt.Sprintf("sort must be JSON: %s", err.Error()),
			)
			return
		}
		body["sort"] = parsed
	}
	if !config.Limit.IsNull() {
		body["limit"] = config.Limit.ValueInt64()
	}

	token, err := tokenForClient(d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error querying meeting notes", err.Error())
		return
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("Error encoding meeting notes request", err.Error())
		return
	}

	httpResp, err := doNotionRequest(ctx, http.MethodPost, notionAPIBaseURL+"/blocks/meeting_notes/query", token, reqBody)
	if err != nil {
		resp.Diagnostics.AddError("Error querying meeting notes", err.Error())
		return
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		resp.Diagnostics.AddError("Error reading meeting notes response", err.Error())
		return
	}
	if httpResp.StatusCode >= 400 {
		resp.Diagnostics.AddError(
			"Notion API error querying meeting notes",
			fmt.Sprintf("status %d: %s", httpResp.StatusCode, string(respBody)),
		)
		return
	}

	var parsed struct {
		Results []meetingNoteRaw `json:"results"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		resp.Diagnostics.AddError("Error parsing meeting notes response", err.Error())
		return
	}

	state := MeetingNotesDataSourceModel{
		Filter:  config.Filter,
		Sort:    config.Sort,
		Limit:   config.Limit,
		RawJSON: types.StringValue(string(respBody)),
	}
	state.Results = make([]MeetingNoteModel, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		state.Results = append(state.Results, MeetingNoteModel{
			ID:             types.StringValue(normalizeID(r.ID)),
			Object:         types.StringValue(r.Object),
			CreatedTime:    types.StringValue(r.CreatedTime),
			LastEditedTime: types.StringValue(r.LastEditedTime),
			URL:            types.StringValue(r.URL),
			Title:          types.StringValue(extractPlainTitle(r.Title)),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// extractPlainTitle pulls a plain-text title from a Notion rich-text title
// field. Notion serializes titles as an array of rich-text segments; we
// concatenate the plain_text values. If the shape is unexpected, returns "".
func extractPlainTitle(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var segments []struct {
		PlainText string `json:"plain_text"`
	}
	if err := json.Unmarshal(raw, &segments); err != nil {
		return ""
	}
	out := ""
	for _, s := range segments {
		out += s.PlainText
	}
	return out
}
