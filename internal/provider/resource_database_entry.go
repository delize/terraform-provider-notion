package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jomei/notionapi"
)

var (
	_ resource.Resource                = &DatabaseEntryResource{}
	_ resource.ResourceWithImportState = &DatabaseEntryResource{}
)

type DatabaseEntryResource struct {
	client *notionapi.Client
}

type DatabaseEntryResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Database              types.String `tfsdk:"database"`
	Title                 types.String `tfsdk:"title"`
	URL                   types.String `tfsdk:"url"`
	RichTextProperties    types.Map    `tfsdk:"rich_text_properties"`
	NumberProperties      types.Map    `tfsdk:"number_properties"`
	CheckboxProperties    types.Map    `tfsdk:"checkbox_properties"`
	SelectProperties      types.Map    `tfsdk:"select_properties"`
	StatusProperties      types.Map    `tfsdk:"status_properties"`
	URLProperties         types.Map    `tfsdk:"url_properties"`
	EmailProperties       types.Map    `tfsdk:"email_properties"`
	PhoneNumberProperties types.Map    `tfsdk:"phone_number_properties"`
	DateProperties        types.Map    `tfsdk:"date_properties"`
}

func NewDatabaseEntryResource() resource.Resource {
	return &DatabaseEntryResource{}
}

func (r *DatabaseEntryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_entry"
}

func (r *DatabaseEntryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an entry (page) in a Notion database.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the database entry.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database": schema.StringAttribute{
				Description: "The ID of the parent database.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				Description: "The title of the entry.",
				Required:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the entry.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"rich_text_properties": schema.MapAttribute{
				Description: "Map of rich text property name to string value.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"number_properties": schema.MapAttribute{
				Description: "Map of number property name to numeric value.",
				Optional:    true,
				ElementType: types.Float64Type,
			},
			"checkbox_properties": schema.MapAttribute{
				Description: "Map of checkbox property name to boolean value.",
				Optional:    true,
				ElementType: types.BoolType,
			},
			"select_properties": schema.MapAttribute{
				Description: "Map of select property name to option name.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"status_properties": schema.MapAttribute{
				Description: "Map of status property name to status name.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"url_properties": schema.MapAttribute{
				Description: "Map of URL property name to URL value.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"email_properties": schema.MapAttribute{
				Description: "Map of email property name to email value.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"phone_number_properties": schema.MapAttribute{
				Description: "Map of phone number property name to phone number value.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"date_properties": schema.MapAttribute{
				Description: "Map of date property name to ISO 8601 date string.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *DatabaseEntryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	r.client = client
}

// findTitlePropertyName retrieves the database and returns the name of the title property.
func (r *DatabaseEntryResource) findTitlePropertyName(ctx context.Context, databaseID string) (string, error) {
	db, err := r.client.Database.Get(ctx, notionapi.DatabaseID(databaseID))
	if err != nil {
		return "", err
	}
	for name, prop := range db.Properties {
		if prop.GetType() == notionapi.PropertyConfigTypeTitle {
			return name, nil
		}
	}
	return "Name", nil
}

func (r *DatabaseEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titlePropName, err := r.findTitlePropertyName(ctx, plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	properties := buildEntryProperties(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	properties[titlePropName] = notionapi.TitleProperty{
		Type:  notionapi.PropertyTypeTitle,
		Title: plainToRichText(plan.Title.ValueString()),
	}

	params := &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:       notionapi.ParentTypeDatabaseID,
			DatabaseID: notionapi.DatabaseID(plan.Database.ValueString()),
		},
		Properties: properties,
	}

	page, err := r.client.Page.Create(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error creating database entry", err.Error())
		return
	}

	plan.ID = types.StringValue(normalizeID(string(page.ID)))
	plan.URL = types.StringValue(page.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	page, err := r.client.Page.Get(ctx, notionapi.PageID(state.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading database entry", err.Error())
		return
	}

	if page.Archived {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(normalizeID(string(page.ID)))
	state.URL = types.StringValue(page.URL)

	if page.Parent.Type == notionapi.ParentTypeDatabaseID {
		state.Database = types.StringValue(normalizeID(string(page.Parent.DatabaseID)))
	}

	for _, prop := range page.Properties {
		if tp, ok := prop.(*notionapi.TitleProperty); ok {
			state.Title = types.StringValue(richTextToPlain(tp.Title))
			break
		}
	}

	readEntryProperties(page, &state, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabaseEntryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	titlePropName, err := r.findTitlePropertyName(ctx, plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	properties := buildEntryProperties(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	properties[titlePropName] = notionapi.TitleProperty{
		Type:  notionapi.PropertyTypeTitle,
		Title: plainToRichText(plan.Title.ValueString()),
	}

	clearRemovedProperties(&state, &plan, properties)

	params := &notionapi.PageUpdateRequest{
		Properties: properties,
	}

	page, err := r.client.Page.Update(ctx, notionapi.PageID(plan.ID.ValueString()), params)
	if err != nil {
		resp.Diagnostics.AddError("Error updating database entry", err.Error())
		return
	}

	plan.URL = types.StringValue(page.URL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	archiveReq := &notionapi.PageUpdateRequest{
		Archived:   true,
		Properties: notionapi.Properties{},
	}

	debugBody, _ := json.Marshal(archiveReq)
	tflog.Debug(ctx, "Delete: archive request body", map[string]interface{}{
		"page_id":        state.ID.ValueString(),
		"request_body":   string(debugBody),
		"properties_nil": archiveReq.Properties == nil,
	})

	_, err := r.client.Page.Update(ctx, notionapi.PageID(state.ID.ValueString()), archiveReq)
	if err != nil {
		tflog.Error(ctx, "Delete: archive failed", map[string]interface{}{
			"error":        err.Error(),
			"request_body": string(debugBody),
		})
		resp.Diagnostics.AddError("Error archiving database entry", err.Error())
		return
	}
}

func (r *DatabaseEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildEntryProperties constructs notionapi.Properties from all typed map fields in the plan.
func buildEntryProperties(ctx context.Context, plan *DatabaseEntryResourceModel, diags *diag.Diagnostics) notionapi.Properties {
	props := make(notionapi.Properties)

	if !plan.RichTextProperties.IsNull() && !plan.RichTextProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.RichTextProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.RichTextProperty{
				Type:     notionapi.PropertyTypeRichText,
				RichText: plainToRichText(val),
			}
		}
	}

	if !plan.NumberProperties.IsNull() && !plan.NumberProperties.IsUnknown() {
		var vals map[string]float64
		diags.Append(plan.NumberProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.NumberProperty{
				Type:   notionapi.PropertyTypeNumber,
				Number: val,
			}
		}
	}

	if !plan.CheckboxProperties.IsNull() && !plan.CheckboxProperties.IsUnknown() {
		var vals map[string]bool
		diags.Append(plan.CheckboxProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.CheckboxProperty{
				Type:     notionapi.PropertyTypeCheckbox,
				Checkbox: val,
			}
		}
	}

	if !plan.SelectProperties.IsNull() && !plan.SelectProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.SelectProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.SelectProperty{
				Type:   notionapi.PropertyTypeSelect,
				Select: notionapi.Option{Name: val},
			}
		}
	}

	if !plan.StatusProperties.IsNull() && !plan.StatusProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.StatusProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.StatusProperty{
				Type:   notionapi.PropertyTypeStatus,
				Status: notionapi.Option{Name: val},
			}
		}
	}

	if !plan.URLProperties.IsNull() && !plan.URLProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.URLProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.URLProperty{
				Type: notionapi.PropertyTypeURL,
				URL:  val,
			}
		}
	}

	if !plan.EmailProperties.IsNull() && !plan.EmailProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.EmailProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.EmailProperty{
				Type:  notionapi.PropertyTypeEmail,
				Email: val,
			}
		}
	}

	if !plan.PhoneNumberProperties.IsNull() && !plan.PhoneNumberProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.PhoneNumberProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			props[name] = notionapi.PhoneNumberProperty{
				Type:        notionapi.PropertyTypePhoneNumber,
				PhoneNumber: val,
			}
		}
	}

	if !plan.DateProperties.IsNull() && !plan.DateProperties.IsUnknown() {
		var vals map[string]string
		diags.Append(plan.DateProperties.ElementsAs(ctx, &vals, false)...)
		for name, val := range vals {
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				t, err = time.Parse("2006-01-02", val)
				if err != nil {
					diags.AddError("Invalid date value",
						fmt.Sprintf("Property %q: %q is not a valid ISO 8601 date or datetime.", name, val))
					continue
				}
			}
			d := notionapi.Date(t)
			props[name] = notionapi.DateProperty{
				Type: notionapi.PropertyTypeDate,
				Date: &notionapi.DateObject{Start: &d},
			}
		}
	}

	return props
}

// readEntryProperties reads API response properties back into the matching state maps.
// Only properties whose keys are already managed (present in the current state maps) are read.
func readEntryProperties(page *notionapi.Page, state *DatabaseEntryResourceModel, diags *diag.Diagnostics) {
	if !state.RichTextProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.RichTextProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if rtp, ok := prop.(*notionapi.RichTextProperty); ok {
					vals[name] = types.StringValue(richTextToPlain(rtp.RichText))
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.RichTextProperties = m
	}

	if !state.NumberProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.NumberProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if np, ok := prop.(*notionapi.NumberProperty); ok {
					vals[name] = types.Float64Value(np.Number)
				}
			}
		}
		m, d := types.MapValue(types.Float64Type, vals)
		diags.Append(d...)
		state.NumberProperties = m
	}

	if !state.CheckboxProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.CheckboxProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if cp, ok := prop.(*notionapi.CheckboxProperty); ok {
					vals[name] = types.BoolValue(cp.Checkbox)
				}
			}
		}
		m, d := types.MapValue(types.BoolType, vals)
		diags.Append(d...)
		state.CheckboxProperties = m
	}

	if !state.SelectProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.SelectProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if sp, ok := prop.(*notionapi.SelectProperty); ok {
					vals[name] = types.StringValue(sp.Select.Name)
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.SelectProperties = m
	}

	if !state.StatusProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.StatusProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if sp, ok := prop.(*notionapi.StatusProperty); ok {
					vals[name] = types.StringValue(sp.Status.Name)
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.StatusProperties = m
	}

	if !state.URLProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.URLProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if up, ok := prop.(*notionapi.URLProperty); ok {
					vals[name] = types.StringValue(up.URL)
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.URLProperties = m
	}

	if !state.EmailProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.EmailProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if ep, ok := prop.(*notionapi.EmailProperty); ok {
					vals[name] = types.StringValue(ep.Email)
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.EmailProperties = m
	}

	if !state.PhoneNumberProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.PhoneNumberProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if pp, ok := prop.(*notionapi.PhoneNumberProperty); ok {
					vals[name] = types.StringValue(pp.PhoneNumber)
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.PhoneNumberProperties = m
	}

	if !state.DateProperties.IsNull() {
		vals := make(map[string]attr.Value)
		for name := range state.DateProperties.Elements() {
			if prop, ok := page.Properties[name]; ok {
				if dp, ok := prop.(*notionapi.DateProperty); ok {
					if dp.Date != nil && dp.Date.Start != nil {
						vals[name] = types.StringValue(formatNotionDate(dp.Date.Start))
					}
				}
			}
		}
		m, d := types.MapValue(types.StringType, vals)
		diags.Append(d...)
		state.DateProperties = m
	}
}

// removedKeys returns keys present in stateMap but absent from planMap.
func removedKeys(stateMap, planMap types.Map) []string {
	if stateMap.IsNull() || stateMap.IsUnknown() {
		return nil
	}
	stateElems := stateMap.Elements()
	planElems := map[string]attr.Value{}
	if !planMap.IsNull() && !planMap.IsUnknown() {
		planElems = planMap.Elements()
	}
	var removed []string
	for key := range stateElems {
		if _, ok := planElems[key]; !ok {
			removed = append(removed, key)
		}
	}
	return removed
}

// clearRemovedProperties sends empty/default values for properties that were
// in the prior state but are no longer in the plan, so they get cleared in Notion.
func clearRemovedProperties(state, plan *DatabaseEntryResourceModel, props notionapi.Properties) {
	for _, name := range removedKeys(state.RichTextProperties, plan.RichTextProperties) {
		props[name] = notionapi.RichTextProperty{
			Type:     notionapi.PropertyTypeRichText,
			RichText: []notionapi.RichText{},
		}
	}
	for _, name := range removedKeys(state.NumberProperties, plan.NumberProperties) {
		props[name] = notionapi.NumberProperty{
			Type:   notionapi.PropertyTypeNumber,
			Number: 0,
		}
	}
	for _, name := range removedKeys(state.CheckboxProperties, plan.CheckboxProperties) {
		props[name] = notionapi.CheckboxProperty{
			Type:     notionapi.PropertyTypeCheckbox,
			Checkbox: false,
		}
	}
	for _, name := range removedKeys(state.SelectProperties, plan.SelectProperties) {
		props[name] = notionapi.SelectProperty{
			Type:   notionapi.PropertyTypeSelect,
			Select: notionapi.Option{},
		}
	}
	for _, name := range removedKeys(state.StatusProperties, plan.StatusProperties) {
		props[name] = notionapi.StatusProperty{
			Type:   notionapi.PropertyTypeStatus,
			Status: notionapi.Option{},
		}
	}
	for _, name := range removedKeys(state.URLProperties, plan.URLProperties) {
		props[name] = notionapi.URLProperty{
			Type: notionapi.PropertyTypeURL,
			URL:  "",
		}
	}
	for _, name := range removedKeys(state.EmailProperties, plan.EmailProperties) {
		props[name] = notionapi.EmailProperty{
			Type:  notionapi.PropertyTypeEmail,
			Email: "",
		}
	}
	for _, name := range removedKeys(state.PhoneNumberProperties, plan.PhoneNumberProperties) {
		props[name] = notionapi.PhoneNumberProperty{
			Type:        notionapi.PropertyTypePhoneNumber,
			PhoneNumber: "",
		}
	}
	for _, name := range removedKeys(state.DateProperties, plan.DateProperties) {
		props[name] = notionapi.DateProperty{
			Type: notionapi.PropertyTypeDate,
			Date: nil,
		}
	}
}

// formatNotionDate formats a Notion Date as date-only (2006-01-02) when the time
// component is midnight UTC, otherwise as full RFC3339.
func formatNotionDate(d *notionapi.Date) string {
	t := time.Time(*d)
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}
