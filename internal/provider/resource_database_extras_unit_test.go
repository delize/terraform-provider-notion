package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// containerFake records each request to /v1/{pages,databases}/{id} and
// answers with a fixed database body.
type containerFake struct {
	mu    sync.Mutex
	reqs  []string
	reply string
}

func newContainerFake(t *testing.T, reply string) *containerFake {
	t.Helper()
	f := &containerFake{reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, r.Method+" "+r.URL.Path+" "+string(b))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(f.reply))
	}))
	prev := notionAPIBaseURL
	notionAPIBaseURL = srv.URL + "/v1"
	t.Cleanup(func() {
		notionAPIBaseURL = prev
		srv.Close()
	})
	return f
}

const dbExtrasReply = `{"object":"database","id":"db-1","icon":null,"is_locked":false,"show_data_source_title":false}`

func TestDecodeContainerExtras_ShowDataSourceTitle(t *testing.T) {
	for body, want := range map[string]string{
		`{"show_data_source_title":true}`:  "true",
		`{"show_data_source_title":false}`: "false",
		`{"object":"page"}`:                "nil",
	} {
		e, err := decodeContainerExtras([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		got := "nil"
		if e.ShowDataSourceTitle != nil {
			got = map[bool]string{true: "true", false: "false"}[*e.ShowDataSourceTitle]
		}
		if got != want {
			t.Errorf("%s: got %s, want %s", body, got, want)
		}
	}
}

// show_data_source_title rides on the same PATCH as icon and is_locked.
// Set on its own it still turns the fallback GET into a PATCH, and pages
// never send it.
func TestPatchContainerExtras_ShowDataSourceTitle(t *testing.T) {
	f := newContainerFake(t, dbExtrasReply)
	cfg := DatabaseResourceModel{ShowDSTitle: types.BoolValue(false), IsLocked: types.BoolNull()}
	e, err := patchContainerExtras(t.Context(), "tok", "databases", "db-1", nil, false, cfg.IsLocked, cfg.extraFields())
	if err != nil {
		t.Fatal(err)
	}
	if e.ShowDataSourceTitle == nil || *e.ShowDataSourceTitle {
		t.Errorf("decoded show_data_source_title = %v, want false", e.ShowDataSourceTitle)
	}

	unset := DatabaseResourceModel{ShowDSTitle: types.BoolNull(), IsLocked: types.BoolValue(true)}
	if _, err := patchContainerExtras(t.Context(), "tok", "databases", "db-1", nil, false, unset.IsLocked, unset.extraFields()); err != nil {
		t.Fatal(err)
	}
	if _, err := patchContainerExtras(t.Context(), "tok", "pages", "p-1", nil, false, types.BoolValue(true), nil); err != nil {
		t.Fatal(err)
	}

	want := []string{
		`PATCH /v1/databases/db-1 {"show_data_source_title":false}`,
		`PATCH /v1/databases/db-1 {"is_locked":true}`,
		`PATCH /v1/pages/p-1 {"is_locked":true}`,
	}
	if len(f.reqs) != len(want) {
		t.Fatalf("requests = %q, want %q", f.reqs, want)
	}
	for i := range want {
		if f.reqs[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, f.reqs[i], want[i])
		}
	}
}

func TestFinishExtras_ShowDataSourceTitle(t *testing.T) {
	tr, fa := true, false
	cases := []struct {
		name string
		plan types.Bool
		resp *bool
		want types.Bool
	}{
		{"unset on create takes Notion's default", types.BoolUnknown(), &tr, types.BoolValue(true)},
		{"configured false", types.BoolValue(false), &fa, types.BoolValue(false)},
		{"unset and missing from the response", types.BoolUnknown(), nil, types.BoolNull()},
		{"configured and missing from the response", types.BoolValue(true), nil, types.BoolValue(true)},
	}
	for _, c := range cases {
		plan := DatabaseResourceModel{Icon: types.StringValue(""), ShowDSTitle: c.plan}
		finishExtras(&plan, containerExtras{ShowDataSourceTitle: c.resp})
		if !plan.ShowDSTitle.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.name, plan.ShowDSTitle, c.want)
		}
	}
}

func TestDatabaseExtraFields(t *testing.T) {
	for _, v := range []types.Bool{types.BoolNull(), types.BoolUnknown()} {
		if got := (&DatabaseResourceModel{ShowDSTitle: v}).extraFields(); got != nil {
			t.Errorf("%s: got %v, want nil", v, got)
		}
	}
	got, _ := json.Marshal((&DatabaseResourceModel{ShowDSTitle: types.BoolValue(true)}).extraFields())
	if string(got) != `{"show_data_source_title":true}` {
		t.Errorf("got %s", got)
	}
}

func TestDecodeContainerExtras_DatabaseTypeAndDataSources(t *testing.T) {
	e, err := decodeContainerExtras([]byte(`{"database_type":"skills","data_sources":[
	  {"id":"c174b72c-d782-432f-8dc0-b647e1c96df6","name":"Tasks"},{"id":"aaaa-bbbb","name":"Other"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.DatabaseType == nil || *e.DatabaseType != "skills" {
		t.Errorf("database_type = %v, want skills", e.DatabaseType)
	}
	if len(e.DataSourceIDs) != 2 || e.DataSourceIDs[0] != "c174b72cd782432f8dc0b647e1c96df6" || e.DataSourceIDs[1] != "aaaabbbb" {
		t.Errorf("data_source_ids = %q", e.DataSourceIDs)
	}

	e, _ = decodeContainerExtras([]byte(`{"database_type":null,"data_sources":[]}`))
	if e.DatabaseType != nil || e.DataSourceIDs == nil || len(e.DataSourceIDs) != 0 {
		t.Errorf("regular database: %+v", e)
	}
	e, _ = decodeContainerExtras([]byte(`{"object":"page"}`))
	if e.DatabaseType != nil || e.DataSourceIDs != nil {
		t.Errorf("page: %+v", e)
	}
}

func TestDatabaseSetReadOnly(t *testing.T) {
	kind := "github_prs"
	var m DatabaseResourceModel
	m.setReadOnly(containerExtras{DatabaseType: &kind, DataSourceIDs: []string{"ds1", "ds2"}})
	if m.DatabaseType.ValueString() != "github_prs" {
		t.Errorf("database_type = %s", m.DatabaseType)
	}
	if got := m.DataSourceIDs.String(); got != `["ds1","ds2"]` {
		t.Errorf("data_source_ids = %s", got)
	}

	m.setReadOnly(containerExtras{DataSourceIDs: []string{}})
	if !m.DatabaseType.IsNull() || m.DataSourceIDs.IsNull() || len(m.DataSourceIDs.Elements()) != 0 {
		t.Errorf("regular database: %s %s", m.DatabaseType, m.DataSourceIDs)
	}
	m.setReadOnly(containerExtras{})
	if !m.DataSourceIDs.IsNull() {
		t.Errorf("missing data_sources should be null, got %s", m.DataSourceIDs)
	}
}

// A view can take its data_source_id from the database in the same config.
func TestUnitDatabaseDataSourceIDs_FeedView(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: iconUnitProvider + `
resource "notion_database" "t" {
  parent             = "00000000000000000000000000000000"
  title              = "x"
  title_column_title = "Name"
}

resource "notion_view" "v" {
  database_id    = notion_database.t.id
  data_source_id = notion_database.t.data_source_ids[0]
  name           = "Table"
  type           = "table"
}`,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}

func TestUnitDatabaseShowDataSourceTitle_ValidPlan(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: iconUnitProvider + `
resource "notion_database" "t" {
  parent                 = "00000000000000000000000000000000"
  title                  = "x"
  title_column_title     = "Name"
  show_data_source_title = false
}`,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}
