package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

// databasePropertyBaseModel is the shared model for all database property resources.
type databasePropertyBaseModel struct {
	ID       types.String `tfsdk:"id"`
	Database types.String `tfsdk:"database"`
	Name     types.String `tfsdk:"name"`
}

// databasePropertyBaseSchema returns the common schema attributes for all database property resources.
func databasePropertyBaseSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The ID of the property.",
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
		"name": schema.StringAttribute{
			Description: "The name of the property.",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
	}
}

// readPropertyFromDatabase reads a property from a database and returns its ID and current name.
func readPropertyFromDatabase(ctx context.Context, client *notionapi.Client, databaseID string, propertyName string, propertyID string) (string, string, error) {
	db, err := client.Database.Get(ctx, notionapi.DatabaseID(databaseID))
	if err != nil {
		return "", "", fmt.Errorf("error reading database: %w", err)
	}

	// Try to find property by ID first, then by name
	for name, prop := range db.Properties {
		if string(prop.GetID()) == propertyID || name == propertyName {
			return string(prop.GetID()), name, nil
		}
	}

	return "", "", fmt.Errorf("property %q not found in database", propertyName)
}

// deletePropertyFromDatabase removes a property from a database by setting it to nil.
func deletePropertyFromDatabase(ctx context.Context, client *notionapi.Client, databaseID string, propertyName string) error {
	_, err := client.Database.Update(ctx, notionapi.DatabaseID(databaseID), &notionapi.DatabaseUpdateRequest{
		Properties: notionapi.PropertyConfigs{
			propertyName: nil,
		},
	})
	return err
}

// parseCompositeID splits a composite ID of the form "database_id/property_name".
func parseCompositeID(id string) (string, string, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected composite ID in format 'database_id/property_name', got: %s", id)
	}
	return parts[0], parts[1], nil
}
