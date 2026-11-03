package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A create with markdown must send the typed property maps in the same
// POST /v1/pages body, not only the title.
func TestUnitDatabaseEntry_MarkdownCreateSendsProperties(t *testing.T) {
	f := newFakeMarkdownPages(t)
	r := newEntryTestResource(t, f)
	ctx := context.Background()

	planned := entryModel("", "Row", "## Notes", "v2")
	planned.ID = types.StringUnknown()
	planned.URL = types.StringUnknown()
	planned.SelectProperties = types.MapValueMust(types.StringType, map[string]attr.Value{"Stage": types.StringValue("Draft")})
	planned.NumberProperties = types.MapValueMust(types.Float64Type, map[string]attr.Value{"Points": types.Float64Value(5)})

	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	null := tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)
	plan := tfsdk.Plan{Schema: sr.Schema, Raw: null}
	if d := plan.Set(ctx, &planned); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema, Raw: null}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.creates) != 1 {
		t.Fatalf("made %d page creates, want 1", len(f.creates))
	}
	body := f.creates[0]
	if body["markdown"] != "## Notes" || body["markdown_version"] != "v2" {
		t.Errorf("create body markdown %v version %v, want the planned markdown and v2", body["markdown"], body["markdown_version"])
	}
	got, _ := json.Marshal(body["properties"])
	var props map[string]map[string]interface{}
	_ = json.Unmarshal(got, &props)
	if _, ok := props["title"]; !ok {
		t.Errorf("properties missing the title: %s", got)
	}
	if sel, _ := props["Stage"]["select"].(map[string]interface{}); sel["name"] != "Draft" {
		t.Errorf("Stage select not sent: %s", got)
	}
	if props["Points"]["number"] != float64(5) {
		t.Errorf("Points number not sent: %s", got)
	}

	var state DatabaseEntryResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatal(d)
	}
	if state.ID.ValueString() == "" {
		t.Errorf("entry not saved to state")
	}
}
