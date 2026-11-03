package provider

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func marshalQuery(t *testing.T, cfg AgentsDataSourceModel) string {
	t.Helper()
	body, err := buildAgentsQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func TestBuildAgentsQuery(t *testing.T) {
	base := AgentsDataSourceModel{
		Query:          types.StringNull(),
		AgentType:      types.StringNull(),
		Filter:         types.StringNull(),
		IncludeDeleted: types.BoolNull(),
		Limit:          types.Int64Null(),
	}

	if got := marshalQuery(t, base); got != `{}` {
		t.Errorf("empty: %s", got)
	}

	one := base
	one.Query = types.StringValue("triage")
	one.AgentType = types.StringValue("custom_agent")
	if got, want := marshalQuery(t, one), `{"filter":{"property":"agent_type","string":{"equals":"custom_agent"}},"query":"triage"}`; got != want {
		t.Errorf("single condition:\n got %s\nwant %s", got, want)
	}

	two := base
	two.AgentType = types.StringValue("custom_agent")
	two.Statuses = []types.String{types.StringValue("active"), types.StringValue("disabled")}
	two.IncludeDeleted = types.BoolValue(true)
	if got, want := marshalQuery(t, two), `{"filter":{"and":[{"property":"agent_type","string":{"equals":"custom_agent"}},{"property":"status","status":{"in":["active","disabled"]}}]},"include_deleted":true}`; got != want {
		t.Errorf("two conditions:\n got %s\nwant %s", got, want)
	}

	raw := base
	raw.Filter = types.StringValue(`{ "property": "model_mode", "string": { "equals": "auto" } }`)
	if got, want := marshalQuery(t, raw), `{"filter":{"property":"model_mode","string":{"equals":"auto"}}}`; got != want {
		t.Errorf("raw filter:\n got %s\nwant %s", got, want)
	}
}

func TestUnitAgentsDataSource_Validation(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  string
	}{
		{"filter conflicts", "agent_type = \"custom_agent\"\n  filter = jsonencode({ property = \"model_mode\", string = { equals = \"auto\" } })", `Use either filter or the agent_type`},
		{"bad agent_type", "agent_type = \"robot\"", `"robot" is not one of custom_agent`},
		{"bad status", "statuses = [\"paused\"]", `"paused" is not one of active, disabled, deleted`},
		{"bad json", "filter = \"{not json\"", `filter must be a JSON object`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      agentUnitProvider + "data \"notion_agents\" \"test\" {\n  " + c.body + "\n}\n",
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(c.err),
				}},
			})
		})
	}
}

func TestUnitAgentInsights_IncompleteWindow(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: agentUnitProvider + `
data "notion_agent_insights" "test" {
  agent_id   = "a1"
  start_time = 1790000000
}`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Set start_time and end_time together`),
		}},
	})
}

// TestAccAgentDataSources reads the agent named by NOTION_TEST_AGENT_ID
// through all three data sources. Read-only, but still gated on the agent
// variable so CI skips it unless a test agent is configured.
func TestAccAgentDataSources(t *testing.T) {
	agentID := os.Getenv("NOTION_TEST_AGENT_ID")
	if agentID == "" {
		t.Skip("NOTION_TEST_AGENT_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "notion_agent" "test" {
  agent_id = "` + agentID + `"
}

data "notion_agents" "test" {
  agent_type = data.notion_agent.test.agent_type
}

data "notion_agent_insights" "test" {
  agent_id = "` + agentID + `"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_agent.test", "id", agentID),
				resource.TestCheckResourceAttrSet("data.notion_agent.test", "name"),
				resource.TestCheckResourceAttrSet("data.notion_agent.test", "raw_json"),
				resource.TestCheckResourceAttrSet("data.notion_agents.test", "agents.#"),
				resource.TestCheckResourceAttrSet("data.notion_agent_insights.test", "runs_completed"),
			),
		}},
	})
}
