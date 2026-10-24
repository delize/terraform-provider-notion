package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/jomei/notionapi"
)

// Admin API operations that change something once and leave no state to
// track (revoking a session, a token, or an MCP connection, deleting an
// agent, starting an export) are Terraform actions rather than resources.
// Actions need Terraform 1.14 or later. They run only when invoked, either
// with `terraform apply -invoke=action.<type>.<name>` or from a resource's
// lifecycle.action_trigger, never as a side effect of plan or destroy.
// Actions have no outputs, so anything the API returns (an export job ID, a
// revoked token count) is reported as a progress message.

// adminActionBase holds the shared Configure for admin actions.
type adminActionBase struct {
	client *notionapi.Client
}

func (a *adminActionBase) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := providerDataFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected Action Configure Type", err.Error())
		return
	}
	a.client = client
}

// admin returns the admin client or adds an error and returns nil.
func (a *adminActionBase) admin(resp *action.InvokeResponse) *adminClient {
	c, err := adminClientFor(a.client)
	if err != nil {
		resp.Diagnostics.AddError("Admin API token required", err.Error())
		return nil
	}
	return c
}

// progress sends a message when Terraform is listening for progress.
func progress(resp *action.InvokeResponse, format string, args ...interface{}) {
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf(format, args...)})
	}
}

// bracketValues adds values under key[] as the MCP connection and PAT list
// endpoints document ("Use bracket encoding for multiple values").
func bracketValues(q url.Values, key string, values []string) {
	for _, v := range values {
		q.Add(key+"[]", v)
	}
}
