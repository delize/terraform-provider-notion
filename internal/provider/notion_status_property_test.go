package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const statusDBFixture = `{
  "object": "database",
  "properties": {
    "Workflow": {
      "id": "abc%3D",
      "name": "Workflow",
      "type": "status",
      "status": {
        "options": [
          {"id": "o1", "name": "Not started", "color": "default"},
          {"id": "o2", "name": "In progress", "color": "blue"},
          {"id": "o3", "name": "Done", "color": "green"}
        ],
        "groups": [
          {"id": "g1", "name": "To-do", "color": "gray", "option_ids": ["o1"]},
          {"id": "g2", "name": "In progress", "color": "blue", "option_ids": ["o2"]},
          {"id": "g3", "name": "Complete", "color": "green", "option_ids": ["o3"]}
        ],
        "default_option": "Not started"
      }
    },
    "Name": {"id": "title", "name": "Name", "type": "title", "title": {}}
  }
}`

func TestParseStatusProperty_ByName(t *testing.T) {
	st, found, err := parseStatusProperty([]byte(statusDBFixture), "", "Workflow")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if !st.IsStatus || st.ID != "abc%3D" {
		t.Errorf("unexpected identity: %+v", st)
	}
	if st.Options["In progress"] != "blue" || len(st.Options) != 3 {
		t.Errorf("options: %v", st.Options)
	}
	if st.Groups["Done"] != "Complete" || st.Groups["Not started"] != "To-do" {
		t.Errorf("groups: %v", st.Groups)
	}
	if st.DefaultOption == nil || *st.DefaultOption != "Not started" {
		t.Errorf("default_option: %v", st.DefaultOption)
	}
}

// A property renamed in the Notion UI is still found by its ID.
func TestParseStatusProperty_IDWinsOverName(t *testing.T) {
	st, found, err := parseStatusProperty([]byte(statusDBFixture), "abc%3D", "Old Name")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if st.Name != "Workflow" {
		t.Errorf("name: got %q, want Workflow", st.Name)
	}
}

func TestParseStatusProperty_NotFound(t *testing.T) {
	_, found, err := parseStatusProperty([]byte(statusDBFixture), "missing", "Missing")
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want not found", found, err)
	}
}

// A same-named property of another type is reported but not as status, so
// Read leaves the configured options alone.
func TestParseStatusProperty_WrongType(t *testing.T) {
	st, found, err := parseStatusProperty([]byte(statusDBFixture), "", "Name")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if st.IsStatus {
		t.Errorf("title property reported as status")
	}
}

func TestParseStatusProperty_NullDefaultOption(t *testing.T) {
	body := `{"properties":{"S":{"id":"x","name":"S","type":"status","status":{"options":[],"groups":[],"default_option":null}}}}`
	st, found, err := parseStatusProperty([]byte(body), "", "S")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if st.DefaultOption != nil {
		t.Errorf("default_option: got %q, want nil", *st.DefaultOption)
	}
}

func TestStatusGroupFor_CaseInsensitive(t *testing.T) {
	st := &statusPropertySchema{Groups: map[string]string{"High": "In progress"}}
	if g, ok := st.groupFor("high"); !ok || g != "In progress" {
		t.Errorf("groupFor(high) = %q, %v", g, ok)
	}
}

func TestBuildStatusOptions(t *testing.T) {
	ctx := context.Background()
	opts, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"Done": "green", "Blocked": "red"})
	groups, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"Done": "Complete"})

	got, diags := buildStatusOptions(ctx, opts, groups)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	want := []statusOptionRequest{
		{Name: "Blocked", Color: "red"},
		{Name: "Done", Color: "green", Group: "Complete"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// Null option_groups sends no group at all.
	got, _ = buildStatusOptions(ctx, opts, types.MapNull(types.StringType))
	for _, o := range got {
		if o.Group != "" {
			t.Errorf("unexpected group on %+v", o)
		}
	}
}
