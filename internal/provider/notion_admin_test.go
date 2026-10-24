package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeAdminAPI is an in-memory stand-in for the Notion Admin API endpoints
// this provider uses. It follows the documented paths, list envelope, and
// error body, and rejects requests without the bearer token or the Admin
// API Notion-Version.
type fakeAdminAPI struct {
	t        *testing.T
	token    string
	mu       sync.Mutex
	nextID   int
	groups   map[string]string            // group id -> name
	members  map[string]map[string]string // group id -> user id -> role
	requests []string                     // "METHOD path?query"
	pageSize int                          // forces pagination when > 0
	bodies   []map[string]interface{}     // decoded request bodies, in order

	// Routes for other Admin API areas, registered by their test files.
	// Each returns true when it handled the request.
	routes []func(w http.ResponseWriter, r *http.Request, parts []string, body map[string]interface{}) bool

	legalHolds map[string]*fakeLegalHold
	areaState  map[string]interface{} // free-form state for other areas
}

func newFakeAdminAPI(t *testing.T) (*fakeAdminAPI, *httptest.Server) {
	t.Helper()
	f := &fakeAdminAPI{
		t:       t,
		token:   "adm_test",
		groups:     map[string]string{},
		members:    map[string]map[string]string{},
		legalHolds: map[string]*fakeLegalHold{},
		areaState:  map[string]interface{}{},
	}
	srv := httptest.NewServer(f)
	prev := adminBaseURL
	adminBaseURL = srv.URL
	t.Cleanup(func() {
		adminBaseURL = prev
		srv.Close()
	})
	return f, srv
}

func (f *fakeAdminAPI) writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"object": "error", "status": status, "code": code, "message": msg})
}

func (f *fakeAdminAPI) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// page slices results with an integer cursor so pagination is exercised.
func (f *fakeAdminAPI) page(r *http.Request, typ string, all []interface{}) map[string]interface{} {
	size := len(all)
	if f.pageSize > 0 {
		size = f.pageSize
	}
	start := 0
	if c := r.URL.Query().Get("start_cursor"); c != "" {
		fmt.Sscanf(c, "%d", &start)
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	var next interface{}
	if end < len(all) {
		next = fmt.Sprintf("%d", end)
	}
	results := all[start:end]
	if results == nil {
		results = []interface{}{}
	}
	return map[string]interface{}{"object": "list", "type": typ, "results": results, "has_more": next != nil, "next_cursor": next}
}

func (f *fakeAdminAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())

	if r.Header.Get("Authorization") != "Bearer "+f.token {
		f.writeErr(w, 401, "unauthorized", "Unauthorized.")
		return
	}
	if r.Header.Get("Notion-Version") != adminAPIVersion {
		f.writeErr(w, 400, "validation_error", "bad Notion-Version "+r.Header.Get("Notion-Version"))
		return
	}

	var body map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	f.bodies = append(f.bodies, body)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	inSpaces := len(parts) >= 2 && parts[1] == "spaces"
	switch {
	case !inSpaces:
		// Other areas (legal holds, MCP connections, ...) are handled by
		// registered routes below.
		for _, route := range f.routes {
			if route(w, r, parts, body) {
				return
			}
		}
		f.writeErr(w, 404, "object_not_found", "no route "+r.Method+" "+r.URL.Path)
	case len(parts) == 2 && parts[1] == "spaces" && r.Method == http.MethodGet:
		f.writeJSON(w, f.page(r, "space", []interface{}{
			map[string]interface{}{"object": "space", "id": "11111111-1111-1111-1111-111111111111", "name": "Acme"},
			map[string]interface{}{"object": "space", "id": "22222222-2222-2222-2222-222222222222", "name": nil},
		}))
	case len(parts) == 4 && parts[3] == "users" && r.Method == http.MethodGet:
		f.writeJSON(w, f.page(r, "user", []interface{}{
			map[string]interface{}{"object": "user", "id": "u1", "name": "Ada", "avatar_url": nil, "type": "person", "membership_type": "owner", "person": map[string]string{"email": "ada@example.com"}},
			map[string]interface{}{"object": "user", "id": "b1", "name": "Bot", "avatar_url": nil, "type": "bot", "bot": map[string]string{}},
		}))
	case len(parts) == 4 && parts[3] == "groups":
		switch r.Method {
		case http.MethodGet:
			var all []interface{}
			for _, id := range sortedKeys(f.groups) {
				all = append(all, map[string]string{"object": "group", "id": id, "name": f.groups[id]})
			}
			f.writeJSON(w, f.page(r, "group", all))
		case http.MethodPost:
			name, _ := body["name"].(string)
			for _, existing := range f.groups {
				if existing == name {
					f.writeErr(w, 409, "conflict_error", "duplicate name")
					return
				}
			}
			f.nextID++
			id := fmt.Sprintf("group-%d", f.nextID)
			f.groups[id] = name
			f.members[id] = map[string]string{}
			f.writeJSON(w, map[string]string{"object": "group", "id": id, "name": name})
		}
	case len(parts) == 5 && parts[3] == "groups":
		id := parts[4]
		name, ok := f.groups[id]
		if !ok {
			f.writeErr(w, 404, "object_not_found", "Not found.")
			return
		}
		switch r.Method {
		case http.MethodGet:
			f.writeJSON(w, map[string]string{"object": "group", "id": id, "name": name})
		case http.MethodPatch:
			f.groups[id], _ = body["name"].(string)
			f.writeJSON(w, map[string]string{"object": "group", "id": id, "name": f.groups[id]})
		case http.MethodDelete:
			delete(f.groups, id)
			delete(f.members, id)
			f.writeJSON(w, map[string]string{"object": "group", "id": id, "name": name})
		}
	case len(parts) >= 6 && parts[3] == "groups" && parts[5] == "members":
		gid := parts[4]
		members, ok := f.members[gid]
		if !ok {
			f.writeErr(w, 404, "object_not_found", "Not found.")
			return
		}
		membership := func(uid, role string) map[string]interface{} {
			return map[string]interface{}{"object": "group_membership", "group_id": gid, "member": map[string]string{"type": "user", "user_id": uid}, "role": role}
		}
		if len(parts) == 6 {
			switch r.Method {
			case http.MethodGet:
				var all []interface{}
				for _, uid := range sortedKeys(members) {
					all = append(all, membership(uid, members[uid]))
				}
				f.writeJSON(w, f.page(r, "group_membership", all))
			case http.MethodPost:
				m, _ := body["member"].(map[string]interface{})
				uid, _ := m["user_id"].(string)
				role, _ := body["role"].(string)
				members[uid] = role
				f.writeJSON(w, membership(uid, role))
			}
			return
		}
		uid := parts[7]
		if _, ok := members[uid]; !ok {
			f.writeErr(w, 404, "object_not_found", "Not found.")
			return
		}
		switch r.Method {
		case http.MethodPatch:
			members[uid], _ = body["role"].(string)
			f.writeJSON(w, membership(uid, members[uid]))
		case http.MethodDelete:
			role := members[uid]
			delete(members, uid)
			f.writeJSON(w, membership(uid, role))
		}
	default:
		for _, route := range f.routes {
			if route(w, r, parts, body) {
				return
			}
		}
		f.writeErr(w, 404, "object_not_found", "no route "+r.Method+" "+r.URL.Path)
	}
}

// sortedKeys gives the fake stable list order, so paging across requests
// neither skips nor repeats items.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- adminClient unit tests ----

func TestAdminClient_PaginatesLists(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.pageSize = 1
	c := &adminClient{token: f.token}

	spaces, err := c.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 2 || spaces[1].Name != nil {
		t.Fatalf("spaces: %+v", spaces)
	}
	// Second request must carry the cursor from the first page.
	if !strings.Contains(f.requests[1], "start_cursor=1") {
		t.Errorf("cursor not forwarded: %v", f.requests)
	}
	if !strings.Contains(f.requests[0], "page_size=100") {
		t.Errorf("page_size not sent: %v", f.requests)
	}
}

func TestAdminClient_ErrorsAreTyped(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	c := &adminClient{token: f.token}
	ctx := context.Background()

	_, err := c.GetGroup(ctx, "space", "missing")
	if !isAdminNotFound(err) {
		t.Errorf("want not found, got %v", err)
	}
	if _, err := c.CreateGroup(ctx, "space", "Eng"); err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateGroup(ctx, "space", "Eng")
	if !isAdminConflict(err) || !strings.Contains(err.Error(), "conflict_error") {
		t.Errorf("want conflict, got %v", err)
	}

	bad := &adminClient{token: "wrong"}
	_, err = bad.ListWorkspaces(ctx)
	if err == nil || !strings.Contains(err.Error(), "401 unauthorized") {
		t.Errorf("want 401, got %v", err)
	}
}

func TestAdminClient_DeleteGroupTransferParam(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	c := &adminClient{token: f.token}
	ctx := context.Background()
	g, _ := c.CreateGroup(ctx, "space", "Eng")
	if err := c.DeleteGroup(ctx, "space", g.ID, "owner-1"); err != nil {
		t.Fatal(err)
	}
	last := f.requests[len(f.requests)-1]
	if !strings.HasPrefix(last, "DELETE /v1/spaces/space/groups/"+g.ID) || !strings.Contains(last, "transfer_to_owner_id=owner-1") {
		t.Errorf("delete request: %s", last)
	}
}

func TestAdminClient_FindGroupMemberNormalizesIDs(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.pageSize = 1
	c := &adminClient{token: f.token}
	ctx := context.Background()
	g, _ := c.CreateGroup(ctx, "space", "Eng")
	f.members[g.ID]["aaaaaaaa-0000-0000-0000-000000000001"] = "member"
	f.members[g.ID]["bbbbbbbb-0000-0000-0000-000000000002"] = "owner"

	m, err := c.FindGroupMember(ctx, "space", g.ID, "bbbbbbbb000000000000000000000002")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.Role != "owner" {
		t.Fatalf("membership: %+v", m)
	}
	none, err := c.FindGroupMember(ctx, "space", g.ID, "cccccccc000000000000000000000003")
	if err != nil || none != nil {
		t.Errorf("want nil, got %+v %v", none, err)
	}
}

func TestAdminClientFor(t *testing.T) {
	if _, err := adminClientFor(nil); err == nil || !strings.Contains(err.Error(), "NOTION_ADMIN_TOKEN") {
		t.Errorf("want admin token error, got %v", err)
	}
}

func TestMissingTokenTransport(t *testing.T) {
	client := &http.Client{Transport: missingTokenTransport{}}
	_, err := client.Get("https://api.notion.com/v1/users/me")
	if err == nil || !strings.Contains(err.Error(), "only sets admin_token") {
		t.Errorf("want missing token error, got %v", err)
	}
}

// ---- End-to-end through Terraform against the fake API ----

// adminOnlyProvider configures only admin_token. NOTION_TOKEN is cleared so
// the test proves an admin-only provider block works.
func adminOnlyProvider(t *testing.T, token string) string {
	t.Setenv("NOTION_TOKEN", "")
	t.Setenv("NOTION_ADMIN_TOKEN", "")
	return fmt.Sprintf(`
provider "notion" {
  admin_token = %q
}
`, token)
}

func TestUnitPermissionGroup_Lifecycle(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	prov := adminOnlyProvider(t, f.token)

	cfg := func(name, role string) string {
		return prov + fmt.Sprintf(`
resource "notion_permission_group" "eng" {
  workspace_id = "11111111-1111-1111-1111-111111111111"
  name         = %q
}

resource "notion_permission_group_member" "ada" {
  workspace_id = notion_permission_group.eng.workspace_id
  group_id     = notion_permission_group.eng.id
  user_id      = "u1"
  role         = %q
}
`, name, role)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if len(f.groups) != 0 {
				return fmt.Errorf("groups left after destroy: %v", f.groups)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg("Engineering", "member"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_permission_group.eng", "id", "group-1"),
					resource.TestCheckResourceAttr("notion_permission_group.eng", "name", "Engineering"),
					resource.TestCheckResourceAttr("notion_permission_group_member.ada", "id", "11111111-1111-1111-1111-111111111111/group-1/u1"),
					resource.TestCheckResourceAttr("notion_permission_group_member.ada", "role", "member"),
				),
			},
			// Rename in place and promote the member in place.
			{
				Config: cfg("Platform", "owner"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("notion_permission_group.eng", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("notion_permission_group_member.ada", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("notion_permission_group.eng", "id", "group-1"),
					func(*terraform.State) error {
						if f.groups["group-1"] != "Platform" || f.members["group-1"]["u1"] != "owner" {
							return fmt.Errorf("server state: groups=%v members=%v", f.groups, f.members)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "notion_permission_group.eng",
				ImportState:       true,
				ImportStateId:     "11111111-1111-1111-1111-111111111111/group-1",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "notion_permission_group_member.ada",
				ImportState:       true,
				ImportStateId:     "11111111-1111-1111-1111-111111111111/group-1/u1",
				ImportStateVerify: true,
			},
		},
	})
}

// A membership removed outside Terraform is dropped from state and re-added.
func TestUnitPermissionGroupMember_DriftRecreates(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	prov := adminOnlyProvider(t, f.token)
	cfg := prov + `
resource "notion_permission_group" "eng" {
  workspace_id = "11111111-1111-1111-1111-111111111111"
  name         = "Engineering"
}

resource "notion_permission_group_member" "ada" {
  workspace_id = notion_permission_group.eng.workspace_id
  group_id     = notion_permission_group.eng.id
  user_id      = "u1"
  role         = "member"
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.members["group-1"], "u1")
					f.mu.Unlock()
				},
				Config: cfg,
				Check: func(*terraform.State) error {
					if f.members["group-1"]["u1"] != "member" {
						return fmt.Errorf("membership not re-added: %v", f.members)
					}
					return nil
				},
			},
		},
	})
}

func TestUnitAdminDataSources(t *testing.T) {
	f, _ := newFakeAdminAPI(t)
	f.pageSize = 1
	f.groups["g-a"] = "Alpha"
	prov := adminOnlyProvider(t, f.token)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: prov + `
data "notion_workspaces" "all" {}

data "notion_permission_groups" "acme" {
  workspace_id = data.notion_workspaces.all.workspaces[0].id
}

data "notion_admin_users" "acme" {
  workspace_id = data.notion_workspaces.all.workspaces[0].id
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.notion_workspaces.all", "workspaces.#", "2"),
				resource.TestCheckResourceAttr("data.notion_workspaces.all", "workspaces.0.name", "Acme"),
				resource.TestCheckNoResourceAttr("data.notion_workspaces.all", "workspaces.1.name"),
				resource.TestCheckResourceAttr("data.notion_permission_groups.acme", "groups.#", "1"),
				resource.TestCheckResourceAttr("data.notion_permission_groups.acme", "groups.0.name", "Alpha"),
				resource.TestCheckResourceAttr("data.notion_admin_users.acme", "users.#", "2"),
				resource.TestCheckResourceAttr("data.notion_admin_users.acme", "users.0.email", "ada@example.com"),
				resource.TestCheckResourceAttr("data.notion_admin_users.acme", "users.0.membership_type", "owner"),
				resource.TestCheckResourceAttr("data.notion_admin_users.acme", "users.1.type", "bot"),
				resource.TestCheckNoResourceAttr("data.notion_admin_users.acme", "users.1.email"),
			),
		}},
	})
}

func TestUnitAdmin_Validation(t *testing.T) {
	prov := adminOnlyProvider(t, "adm_test")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: prov + `
resource "notion_permission_group_member" "x" {
  workspace_id = "w"
  group_id     = "g"
  user_id      = "u"
  role         = "admin"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`"admin" is not a group role`),
			},
			{
				Config: prov + `
resource "notion_permission_group" "x" {
  workspace_id = "w"
  name         = "   "
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`1 to 200 characters`),
			},
		},
	})
}

// Without admin_token, Admin API resources fail with guidance, while the
// regular token still configures the provider.
func TestUnitAdmin_MissingAdminToken(t *testing.T) {
	t.Setenv("NOTION_ADMIN_TOKEN", "")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
provider "notion" {
  token = "secret_unit"
}

data "notion_workspaces" "all" {}
`,
			ExpectError: regexp.MustCompile(`NOTION_ADMIN_TOKEN`),
		}},
	})
}

// With neither token, the provider still refuses to configure.
func TestUnitProvider_NoTokens(t *testing.T) {
	t.Setenv("NOTION_TOKEN", "")
	t.Setenv("NOTION_ADMIN_TOKEN", "")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      `data "notion_workspaces" "all" {}`,
			ExpectError: regexp.MustCompile(`Missing Notion API Token`),
		}},
	})
}
