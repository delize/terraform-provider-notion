package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &UserDataSource{}

type UserDataSource struct {
	client *notionapi.Client
}

type UserDataSourceModel struct {
	Email  types.String `tfsdk:"email"`
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	UserID types.String `tfsdk:"user_id"`
}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

func (d *UserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Notion user by email address.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Description: "The email address of the user.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the user (same as user_id).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the user.",
				Computed:    true,
			},
			"user_id": schema.StringAttribute{
				Description: "The Notion user ID.",
				Computed:    true,
			},
		},
	}
}

func (d *UserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*notionapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *notionapi.Client, got: %T.", req.ProviderData))
		return
	}
	d.client = client
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all users and filter by email
	var cursor notionapi.Cursor
	targetEmail := config.Email.ValueString()

	for {
		users, err := d.client.User.List(ctx, &notionapi.Pagination{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error listing users", err.Error())
			return
		}

		for _, user := range users.Results {
			if user.Person != nil && user.Person.Email == targetEmail {
				config.ID = types.StringValue(normalizeID(string(user.ID)))
				config.Name = types.StringValue(user.Name)
				config.UserID = types.StringValue(normalizeID(string(user.ID)))
				resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
				return
			}
		}

		if !users.HasMore {
			break
		}
		cursor = notionapi.Cursor(users.NextCursor)
	}

	resp.Diagnostics.AddError("User not found",
		fmt.Sprintf("No user found with email: %s", targetEmail))
}
