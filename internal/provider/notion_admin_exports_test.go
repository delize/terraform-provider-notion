package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func (f *fakeAdminAPI) serveExports(w http.ResponseWriter, r *http.Request, parts []string, body map[string]interface{}) bool {
	if len(parts) < 4 || parts[1] != "spaces" || parts[3] != "exports" {
		return false
	}
	switch {
	case len(parts) == 4 && r.Method == http.MethodPost:
		f.areaState["export_request"] = body
		f.writeJSON(w, map[string]string{"export_job_id": "job-1", "status": "in_progress"})
	case len(parts) == 5 && r.Method == http.MethodGet && parts[4] == "job-1":
		f.writeJSON(w, map[string]interface{}{
			"export_job_id": "job-1", "status": "completed", "export_format": "markdown", "export_type": "space",
			"pages_exported": 1200.0, "created_time": 1.0, "last_edited_time": 2.0,
			"download_urls": []string{"https://files.example.com/a.zip"}, "url_expires_time": 3.0,
		})
	default:
		f.writeErr(w, 404, "object_not_found", "Not found.")
	}
	return true
}

func TestUnitWorkspaceExportActionAndStatus(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.routes = append(f.routes, f.serveExports)
	prov := adminOnlyProvider(t, f.token)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
action "notion_enqueue_workspace_export" "backup" {
  config {
    workspace_id            = "ws-1"
    export_type             = "markdown"
    on_behalf_of_user_email = "admin@example.com"
    include_contents        = "no_files"
    include_comments        = false
    teamspace_ids           = ["ts-1"]
  }
}

resource "terraform_data" "backup" {
  input = "2026-10"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_enqueue_workspace_export.backup]
    }
  }
}

data "notion_workspace_export_status" "backup" {
  workspace_id  = "ws-1"
  export_job_id = "job-1"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_workspace_export_status.backup", "status", "completed"),
				resource.TestCheckResourceAttr("data.notion_workspace_export_status.backup", "pages_exported", "1200"),
				resource.TestCheckResourceAttr("data.notion_workspace_export_status.backup", "download_urls.#", "1"),
				resource.TestCheckNoResourceAttr("data.notion_workspace_export_status.backup", "error_type"),
				func(*terraform.State) error {
					body, _ := f.areaState["export_request"].(map[string]interface{})
					if body["export_type"] != "markdown" || body["include_contents"] != "no_files" || body["include_comments"] != false {
						return fmt.Errorf("export request: %v", body)
					}
					if _, sent := body["locale"]; sent {
						return fmt.Errorf("unset options must be omitted: %v", body)
					}
					return nil
				},
			),
		}},
	})
}

func TestUnitWorkspaceExportAction_Validation(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
action "notion_enqueue_workspace_export" "x" {
  config {
    workspace_id            = "ws-1"
    export_type             = "pdf"
    on_behalf_of_user_email = "admin@example.com"
  }
}

resource "terraform_data" "t" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.notion_enqueue_workspace_export.x]
    }
  }
}`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`"pdf" is not valid for export_type`),
		}},
	})
}
