package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// logAttr logs an attribute value without asserting on it. Used for fields
// Notion may or may not return depending on plan or token capabilities, so
// CI output still shows what came back.
func logAttr(t *testing.T, resourceName, attr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			t.Logf("%s not in state", resourceName)
			return nil
		}
		t.Logf("%s.%s = %q", resourceName, attr, rs.Primary.Attributes[attr])
		return nil
	}
}
