package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func agentWith(status string, reason *string) *agentObject {
	return &agentObject{ID: "a1", Status: status, PauseReason: reason}
}

func agentStrPtr(s string) *string { return &s }

func TestStatusChangeNeeded(t *testing.T) {
	cases := []struct {
		name     string
		want     bool
		agent    *agentObject
		send     bool
		warnText bool
	}{
		{"enable already active", true, agentWith("active", nil), false, false},
		{"enable after API disable", true, agentWith("disabled", agentStrPtr("disabled_from_api")), true, false},
		{"enable after settings disable", true, agentWith("disabled", agentStrPtr("disabled_from_agent_settings")), true, false},
		{"enable while credit paused", true, agentWith("disabled", agentStrPtr("credit_limit")), false, true},
		{"disable active", false, agentWith("active", nil), true, false},
		{"disable already paused", false, agentWith("disabled", agentStrPtr("run_limit")), false, false},
	}
	for _, c := range cases {
		send, why := statusChangeNeeded(c.want, c.agent)
		if send != c.send || (why != "") != c.warnText {
			t.Errorf("%s: send=%v why=%q", c.name, send, why)
		}
	}
}

func TestEnabledFromAgent(t *testing.T) {
	if got := enabledFromAgent(types.BoolNull(), agentWith("active", nil)); !got.IsNull() {
		t.Errorf("unmanaged enabled should stay null, got %v", got)
	}
	if got := enabledFromAgent(types.BoolValue(false), agentWith("active", nil)); !got.ValueBool() {
		t.Errorf("active agent should read as enabled")
	}
	if got := enabledFromAgent(types.BoolValue(true), agentWith("disabled", agentStrPtr("disabled_from_api"))); got.ValueBool() {
		t.Errorf("API-disabled agent should read as disabled")
	}
	// An automatic pause keeps the configured value, so it isn't drift.
	if got := enabledFromAgent(types.BoolValue(true), agentWith("disabled", agentStrPtr("credit_limit"))); !got.ValueBool() {
		t.Errorf("credit-limit pause should not flip enabled")
	}
}

const agentUnitProvider = `
provider "notion" {
  token = "unit-test-token"
}
`

func TestUnitAgentSettings_Validation(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  string
	}{
		{"personal agent", "agent_id = \"notion_ai\"\n  enabled = true", `notion_ai is the personal agent`},
		{"negative limit", "agent_id = \"a1\"\n  credit_limit = -1", `credit_limit must be zero or more`},
		{"both limits", "agent_id = \"a1\"\n  credit_limit = 10\n  unlimited_credits = true", `Set either credit_limit or unlimited_credits`},
		{"unlimited false", "agent_id = \"a1\"\n  unlimited_credits = false", `Set unlimited_credits to true`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      agentUnitProvider + "resource \"notion_agent_settings\" \"test\" {\n  " + c.body + "\n}\n",
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(c.err),
				}},
			})
		})
	}
}

func TestUnitAgentSettings_ValidPlans(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: agentUnitProvider + `
resource "notion_agent_settings" "test" {
  agent_id     = "1f0c7c06-781b-4987-9986-5c8dd3028013"
  enabled      = false
  credit_limit = 500
}`,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}

// TestAccAgentSettingsResource changes a real agent, so it only runs when
// NOTION_TEST_AGENT_ID names a throwaway Custom Agent. It disables the agent
// with a credit limit, then re-enables it and clears the limit. The token
// needs full access to that agent. Destroy leaves it enabled and uncapped.
func TestAccAgentSettingsResource(t *testing.T) {
	agentID := os.Getenv("NOTION_TEST_AGENT_ID")
	if agentID == "" {
		t.Skip("NOTION_TEST_AGENT_ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAgentSettingsConfig(agentID, "enabled = false\n  credit_limit = 1000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_agent_settings.test", "status", "disabled"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "pause_reason", "disabled_from_api"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "credit_limit", "1000"),
					resource.TestCheckResourceAttrSet("notion_agent_settings.test", "name"),
				),
			},
			{
				Config: testAccAgentSettingsConfig(agentID, "enabled = true\n  unlimited_credits = true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_agent_settings.test", "status", "active"),
					resource.TestCheckNoResourceAttr("notion_agent_settings.test", "pause_reason"),
				),
			},
			{
				ResourceName:            "notion_agent_settings.test",
				ImportState:             true,
				ImportStateId:           agentID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"enabled", "unlimited_credits"},
			},
		},
	})
}

func testAccAgentSettingsConfig(agentID, body string) string {
	return "\nresource \"notion_agent_settings\" \"test\" {\n  agent_id = \"" + agentID + "\"\n  " + body + "\n}\n"
}
