package provider

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
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
		{"model_id with auto", "agent_id = \"a1\"\n  model_mode = \"auto\"\n  model_id = \"claude-sonnet-5\"", `model_id is only allowed with model_mode = "pinned"`},
		{"model_id without mode", "agent_id = \"a1\"\n  model_id = \"claude-sonnet-5\"", `model_id is only allowed with model_mode = "pinned"`},
		{"pinned without id", "agent_id = \"a1\"\n  model_mode = \"pinned\"", `model_mode = "pinned" needs model_id`},
		{"empty model_id", "agent_id = \"a1\"\n  model_mode = \"pinned\"\n  model_id = \"\"", `model_id can't be empty`},
		{"unknown mode", "agent_id = \"a1\"\n  model_mode = \"fastest\"", `model_mode must be "auto" or "pinned"`},
		{"empty name", "agent_id = \"a1\"\n  name = \"\"", `name can't be empty`},
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
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: agentUnitProvider + `
resource "notion_agent_settings" "test" {
  agent_id    = "1f0c7c06-781b-4987-9986-5c8dd3028013"
  name        = "Triage"
  description = ""
  model_mode  = "pinned"
  model_id    = "claude-sonnet-5"
}`,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}

// TestAccAgentSettingsResource changes a real agent, so it only runs when
// NOTION_TEST_AGENT_ID names a throwaway Custom Agent. It disables the agent
// with a credit limit and a test name and description, then re-enables it,
// clears the limit, and puts the original name and description back. The
// token needs full access to that agent and must be a personal access token,
// because a connection can't change a credit limit. Destroy leaves it enabled
// and uncapped. The model isn't touched, since changing it republishes the
// agent and the available model IDs vary by workspace.
func TestAccAgentSettingsResource(t *testing.T) {
	agentID := os.Getenv("NOTION_TEST_AGENT_ID")
	if agentID == "" {
		t.Skip("NOTION_TEST_AGENT_ID not set")
	}
	original, _, err := getAgent(context.Background(), os.Getenv("NOTION_TOKEN"), agentID)
	if err != nil {
		t.Fatalf("reading the test agent: %v", err)
	}
	originalDescription := ""
	if original.Description != nil {
		originalDescription = *original.Description
	}
	testName := original.Name + " (tf acc)"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAgentSettingsConfig(agentID, "enabled = false\n  credit_limit = 1000\n  name = "+hclQuote(testName)+"\n  description = \"Managed by the Terraform acceptance test\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_agent_settings.test", "status", "disabled"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "pause_reason", "disabled_from_api"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "credit_limit", "1000"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "name", testName),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "description", "Managed by the Terraform acceptance test"),
					resource.TestCheckResourceAttrSet("notion_agent_settings.test", "model_mode"),
				),
			},
			{
				// Clearing the description while the agent stays disabled
				// must not resend status, which Notion would reject.
				Config: testAccAgentSettingsConfig(agentID, "enabled = false\n  credit_limit = 1000\n  name = "+hclQuote(testName)+"\n  description = \"\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_agent_settings.test", "status", "disabled"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "description", ""),
				),
			},
			{
				Config: testAccAgentSettingsConfig(agentID, "enabled = true\n  unlimited_credits = true\n  name = "+hclQuote(original.Name)+"\n  description = "+hclQuote(originalDescription)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_agent_settings.test", "status", "active"),
					resource.TestCheckNoResourceAttr("notion_agent_settings.test", "pause_reason"),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "name", original.Name),
					resource.TestCheckResourceAttr("notion_agent_settings.test", "description", originalDescription),
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

// hclQuote quotes s as an HCL string literal, escaping template sequences
// so an agent name containing ${ isn't read as interpolation.
func hclQuote(s string) string {
	q := strconv.Quote(s)
	q = strings.ReplaceAll(q, "${", "$${")
	return strings.ReplaceAll(q, "%{", "%%{")
}
