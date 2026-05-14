package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jomei/notionapi"
)

var _ datasource.DataSource = &UsersDataSource{}

type UsersDataSource struct {
	client *notionapi.Client
}

type UsersDataSourceModel struct {
	Users []UserDataModel `tfsdk:"users"`
}

type UserDataModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Email     types.String `tfsdk:"email"`
	AvatarURL types.String `tfsdk:"avatar_url"`
}

func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

func (d *UsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *UsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List all users in the Notion workspace (people and bots) the integration has access to.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				Description: "All users returned by the Notion API.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The Notion user ID.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The user's display name.",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: `The user type ("person" or "bot").`,
							Computed:    true,
						},
						"email": schema.StringAttribute{
							Description: "Email address for person-type users (empty for bots).",
							Computed:    true,
						},
						"avatar_url": schema.StringAttribute{
							Description: "URL of the user's avatar image, if set.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *UsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UsersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state UsersDataSourceModel
	var cursor notionapi.Cursor

	for {
		page, err := d.client.User.List(ctx, &notionapi.Pagination{
			StartCursor: cursor,
			PageSize:    100,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error listing users", err.Error())
			return
		}

		for _, user := range page.Results {
			model := UserDataModel{
				ID:        types.StringValue(normalizeID(string(user.ID))),
				Name:      types.StringValue(user.Name),
				Type:      types.StringValue(string(user.Type)),
				AvatarURL: types.StringValue(user.AvatarURL),
			}
			if user.Person != nil {
				model.Email = types.StringValue(user.Person.Email)
			} else {
				model.Email = types.StringValue("")
			}
			state.Users = append(state.Users, model)
		}

		if !page.HasMore {
			break
		}
		cursor = notionapi.Cursor(page.NextCursor)
	}

	if state.Users == nil {
		state.Users = []UserDataModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
