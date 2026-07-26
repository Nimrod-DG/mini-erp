package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

type tenantBody struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Status      string   `json:"status"`
	Timezone    string   `json:"timezone"`
	UserCount   int      `json:"userCount"`
	AdminCount  int      `json:"adminCount"`
	ModuleCount int      `json:"moduleCount"`
	Enabled     []string `json:"enabledModules"`
	Modules     []struct {
		Code    string `json:"code"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	} `json:"modules"`
}

type createTenantBody struct {
	Tenant tenantBody `json:"tenant"`
	Admin  struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		FullName string `json:"fullName"`
	} `json:"admin"`
}

type tenantListBody struct {
	Data       []tenantBody `json:"data"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalItems int64        `json:"totalItems"`
	TotalPages int          `json:"totalPages"`
}

// newTenantRequest is a valid POST /api/admin/tenants body.
func newTenantRequest(slug string) map[string]any {
	return map[string]any{
		"name":     "Nusantara " + slug,
		"slug":     slug,
		"timezone": "Asia/Jakarta",
		"admin": map[string]any{
			"email":    slug + "-owner@example.test",
			"fullName": "Rina Wijaya",
			"password": "correct horse battery",
		},
	}
}

// --------------------------------------------------------------------------
// Who may reach the platform plane.
// --------------------------------------------------------------------------

// A tenant admin runs one workspace. The platform plane is a different job with
// a different pool (§5.7).
func TestOnlySuperadminsReachTheAdminPlane(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Ambitious Ltd")
	admin := tenant.NewAdmin(t)

	for _, tc := range []struct {
		name string
		resp *http.Response
	}{
		{"list tenants", h.Get(t, "/api/admin/tenants", admin.FirebaseUID)},
		{"read own tenant", h.Get(t, "/api/admin/tenants/"+tenant.ID.String(), admin.FirebaseUID)},
		{"create a tenant", h.Post(t, "/api/admin/tenants", admin.FirebaseUID, newTenantRequest("sneaky"))},
		{"grant an entitlement", h.Put(t,
			fmt.Sprintf("/api/admin/tenants/%s/modules/finance", tenant.ID),
			admin.FirebaseUID, map[string]any{"enabled": true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testsupport.AssertErrorCode(t, tc.resp, http.StatusForbidden, "forbidden")
		})
	}

	// Notably including granting themselves an entitlement they do not have —
	// the ceiling is not theirs to raise.
	if h.DB.Owner.Raw(`SELECT 1`).Error != nil {
		t.Fatal("sanity check failed")
	}
}

// --------------------------------------------------------------------------
// Tenant bootstrap — the one superadmin write scoped to a tenant (§5.7).
// --------------------------------------------------------------------------

func TestCreateTenantSeedsAdminAccountsAndEntitlements(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	resp := h.Post(t, "/api/admin/tenants", super.FirebaseUID, newTenantRequest("bootstrap-co"))
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	created := testsupport.Decode[createTenantBody](t, resp)

	if created.Tenant.Slug != "bootstrap-co" || created.Tenant.Status != "active" {
		t.Errorf("tenant = %+v", created.Tenant)
	}
	if created.Tenant.AdminCount != 1 || created.Tenant.UserCount != 1 {
		t.Errorf("userCount = %d, adminCount = %d, want 1 and 1",
			created.Tenant.UserCount, created.Tenant.AdminCount)
	}
	if created.Tenant.ModuleCount != 3 {
		t.Errorf("moduleCount = %d, want all three enabled by default", created.Tenant.ModuleCount)
	}

	// The first admin can sign in immediately and is implicitly admin in every
	// entitled module, with no user_module_roles rows (§5.4, B7).
	account, ok := h.Users.LastCreated()
	if !ok {
		t.Fatal("no provider account was created for the first admin")
	}
	for _, module := range []string{"procurement", "inventory", "finance"} {
		testsupport.AssertStatus(t,
			h.Get(t, testsupport.ProbePath(module, identity.RoleAdmin), account.UID),
			http.StatusOK)
	}

	var roleRows int
	if err := h.DB.Owner.Raw(`
		SELECT count(*) FROM user_module_roles WHERE user_id = ?`, created.Admin.ID).
		Scan(&roleRows).Error; err != nil {
		t.Fatalf("count role rows: %v", err)
	}
	if roleRows != 0 {
		t.Errorf("the first admin has %d role rows; §5.4 says none", roleRows)
	}

	// The chart of accounts was seeded through the SECURITY DEFINER function of
	// §4.2.1. Read back as the tenant on the app pool, with RLS in force.
	var codes []string
	h.DB.MustAsTenant(t, mustParseUUID(t, created.Tenant.ID), func(tx *gorm.DB) error {
		return tx.Raw(`SELECT code FROM accounts ORDER BY code`).Scan(&codes).Error
	})
	if len(codes) != 2 || codes[0] != "1300" || codes[1] != "2150" {
		t.Errorf("accounts = %v, want [1300 2150]", codes)
	}
}

// The phase brief's warning, made into a test: do NOT "solve" the seeding
// problem by granting erp_admin access to `accounts`. The privileged surface is
// two rows wide and named — a function — rather than a table-level grant, and
// this is what fails if someone widens it (§4.2.1, A11).
func TestAdminPoolStillCannotTouchAccountsDirectly(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	resp := h.Post(t, "/api/admin/tenants", super.FirebaseUID, newTenantRequest("revoke-co"))
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	created := testsupport.Decode[createTenantBody](t, resp)

	// The very tenant erp_admin just seeded accounts for, and it cannot read one.
	err := h.DB.Admin.Exec(`SELECT code FROM accounts WHERE tenant_id = ?`,
		created.Tenant.ID).Error
	if !testsupport.IsPGCode(err, testsupport.CodeInsufficientPrivilege) {
		t.Fatalf("erp_admin reading accounts gave %v, want a permission error (42501). "+
			"If this now succeeds, a grant was added and §4.2.1's narrow surface is gone", err)
	}
}

func TestCreateTenantHonoursAnExplicitModuleList(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	req := newTenantRequest("bahari-logistics")
	req["modules"] = []string{"procurement", "inventory"} // no finance

	resp := h.Post(t, "/api/admin/tenants", super.FirebaseUID, req)
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	created := testsupport.Decode[createTenantBody](t, resp)

	if created.Tenant.ModuleCount != 2 {
		t.Errorf("moduleCount = %d, want 2", created.Tenant.ModuleCount)
	}
	// A row per available module either way, so the toggle screen is complete.
	if len(created.Tenant.Modules) != 3 {
		t.Errorf("the matrix has %d rows, want one per catalogue module",
			len(created.Tenant.Modules))
	}

	// This is B8's premise arriving through the real endpoint: the admin of a
	// tenant without Finance resolves to `none` there.
	account, _ := h.Users.LastCreated()
	testsupport.AssertErrorCode(t,
		h.Get(t, testsupport.ProbePath("finance", identity.RoleViewer), account.UID),
		http.StatusForbidden, "module_not_enabled")
}

func TestCreateTenantValidation(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	// Take the slug and the email, so the duplicate cases below have something
	// to collide with.
	testsupport.AssertStatus(t,
		h.Post(t, "/api/admin/tenants", super.FirebaseUID, newTenantRequest("taken-co")),
		http.StatusCreated)
	createdSoFar := len(h.Users.Created)

	for _, tc := range []struct {
		name       string
		mutate     func(map[string]any)
		wantStatus int
		wantCode   string
	}{
		{"duplicate slug", func(r map[string]any) {
			r["slug"] = "taken-co"
		}, http.StatusConflict, "in_use"},
		{"duplicate email", func(r map[string]any) {
			r["admin"].(map[string]any)["email"] = "taken-co-owner@example.test"
		}, http.StatusConflict, "in_use"},
		{"slug with a slash", func(r map[string]any) {
			r["slug"] = "not/a/slug"
		}, http.StatusBadRequest, "malformed"},
		{"slug in capitals", func(r map[string]any) {
			r["slug"] = "SHOUTING"
		}, http.StatusBadRequest, "malformed"},
		{"no name", func(r map[string]any) {
			r["name"] = "  "
		}, http.StatusBadRequest, "malformed"},
		// An unloadable zone would not fail here — it would fail later, on a
		// date, in a report, for one tenant only (I7).
		{"invented timezone", func(r map[string]any) {
			r["timezone"] = "Asia/Atlantis"
		}, http.StatusBadRequest, "malformed"},
		{"unknown module", func(r map[string]any) {
			r["modules"] = []string{"procurement", "manufacturing"}
		}, http.StatusBadRequest, "malformed"},
		{"short password", func(r map[string]any) {
			r["admin"].(map[string]any)["password"] = "short"
		}, http.StatusBadRequest, "malformed"},
		{"not an email", func(r map[string]any) {
			r["admin"].(map[string]any)["email"] = "rina-at-example"
		}, http.StatusBadRequest, "malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := newTenantRequest("candidate-" + fmt.Sprint(len(tc.name)))
			req["admin"].(map[string]any)["email"] =
				fmt.Sprintf("%s@example.test", tc.name[:3])
			tc.mutate(req)
			testsupport.AssertErrorCode(t,
				h.Post(t, "/api/admin/tenants", super.FirebaseUID, req),
				tc.wantStatus, tc.wantCode)
		})
	}

	// No provider account was created for any of the rejected requests. A
	// refusal that still provisions an account burns the address for the retry.
	if len(h.Users.Created) != createdSoFar {
		t.Errorf("%d provider accounts created by rejected requests",
			len(h.Users.Created)-createdSoFar)
	}
}

// --------------------------------------------------------------------------
// Entitlements and suspension.
// --------------------------------------------------------------------------

func TestSetTenantModuleTogglesOneEntitlement(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)
	tenant := h.DB.NewTenant(t, "Toggling Ltd")

	path := fmt.Sprintf("/api/admin/tenants/%s/modules/finance", tenant.ID)

	resp := h.Put(t, path, super.FirebaseUID, map[string]any{"enabled": false})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	matrix := testsupport.Decode[[]struct {
		Code    string `json:"code"`
		Enabled bool   `json:"enabled"`
	}](t, resp)
	for _, m := range matrix {
		if m.Code == "finance" && m.Enabled {
			t.Error("finance is still enabled after being toggled off")
		}
	}

	// Idempotent: the same call twice is not an error.
	testsupport.AssertStatus(t,
		h.Put(t, path, super.FirebaseUID, map[string]any{"enabled": false}), http.StatusOK)

	// The two names in the URL are checked separately, so the console knows
	// which one was wrong.
	testsupport.AssertStatus(t, h.Put(t,
		fmt.Sprintf("/api/admin/tenants/%s/modules/manufacturing", tenant.ID),
		super.FirebaseUID, map[string]any{"enabled": true}), http.StatusNotFound)
	testsupport.AssertStatus(t, h.Put(t,
		"/api/admin/tenants/00000000-0000-0000-0000-000000000000/modules/finance",
		super.FirebaseUID, map[string]any{"enabled": true}), http.StatusNotFound)

	// A body without `enabled` is malformed rather than "false": defaulting a
	// missing boolean would let a typo revoke an entitlement.
	testsupport.AssertErrorCode(t,
		h.Put(t, path, super.FirebaseUID, map[string]any{}),
		http.StatusBadRequest, "malformed")
}

// Suspending blocks the tenant's users at identity resolution, so they get a
// clear 403 rather than an inexplicably empty application (§9.2).
func TestPatchTenantSuspendsAndReactivates(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)
	tenant := h.DB.NewTenant(t, "Unpaid Invoice Ltd")
	path := "/api/admin/tenants/" + tenant.ID.String()

	testsupport.AssertStatus(t, h.Get(t, "/api/me", tenant.User.FirebaseUID), http.StatusOK)

	testsupport.AssertStatus(t,
		h.Patch(t, path, super.FirebaseUID, map[string]any{"status": "suspended"}), http.StatusOK)
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/me", tenant.User.FirebaseUID), http.StatusForbidden, "tenant_suspended")

	testsupport.AssertStatus(t,
		h.Patch(t, path, super.FirebaseUID, map[string]any{"status": "active"}), http.StatusOK)
	testsupport.AssertStatus(t, h.Get(t, "/api/me", tenant.User.FirebaseUID), http.StatusOK)
}

func TestPatchTenantRenamesAndRetimes(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)
	tenant := h.DB.NewTenant(t, "Old Name Ltd")
	path := "/api/admin/tenants/" + tenant.ID.String()

	resp := h.Patch(t, path, super.FirebaseUID, map[string]any{
		"name":     "New Name Ltd",
		"timezone": "Asia/Makassar",
	})
	testsupport.AssertStatus(t, resp, http.StatusOK)
	body := testsupport.Decode[tenantBody](t, resp)

	if body.Name != "New Name Ltd" || body.Timezone != "Asia/Makassar" {
		t.Errorf("tenant = %+v", body)
	}
	// The slug is absent from the PATCH surface deliberately: it is in URLs.
	if body.Slug != tenant.Slug {
		t.Errorf("slug = %q, want it unchanged at %q", body.Slug, tenant.Slug)
	}

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"invented timezone", map[string]any{"timezone": "Mars/Olympus"}},
		{"invented status", map[string]any{"status": "deleted"}},
		{"blank name", map[string]any{"name": "   "}},
		{"nothing at all", map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testsupport.AssertErrorCode(t,
				h.Patch(t, path, super.FirebaseUID, tc.body),
				http.StatusBadRequest, "malformed")
		})
	}

	testsupport.AssertStatus(t, h.Patch(t,
		"/api/admin/tenants/00000000-0000-0000-0000-000000000000", super.FirebaseUID,
		map[string]any{"name": "Ghost"}), http.StatusNotFound)
}

// --------------------------------------------------------------------------
// The §9.0 list contract.
// --------------------------------------------------------------------------

func TestTenantListPaginatesAndCounts(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	// A distinctive name so the search term isolates these three from whatever
	// else the shared database holds.
	for i := range 3 {
		req := newTenantRequest(fmt.Sprintf("paginated-%d", i))
		req["name"] = fmt.Sprintf("Zanzibar Trading %d", i)
		testsupport.AssertStatus(t,
			h.Post(t, "/api/admin/tenants", super.FirebaseUID, req), http.StatusCreated)
	}

	resp := h.Get(t, "/api/admin/tenants?q=Zanzibar&pageSize=2&sort=name", super.FirebaseUID)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	page := testsupport.Decode[tenantListBody](t, resp)

	if page.TotalItems != 3 {
		t.Errorf("totalItems = %d, want 3", page.TotalItems)
	}
	if page.TotalPages != 2 {
		t.Errorf("totalPages = %d, want 2", page.TotalPages)
	}
	if len(page.Data) != 2 {
		t.Fatalf("page 1 has %d rows, want 2", len(page.Data))
	}
	if page.Data[0].Name != "Zanzibar Trading 0" {
		t.Errorf("first row = %q; sorting is server-side across the whole result set",
			page.Data[0].Name)
	}
	if len(page.Data[0].Enabled) != 3 {
		t.Errorf("enabledModules = %v, want all three", page.Data[0].Enabled)
	}

	// Page 2 holds the remainder.
	resp = h.Get(t, "/api/admin/tenants?q=Zanzibar&pageSize=2&sort=name&page=2", super.FirebaseUID)
	page = testsupport.Decode[tenantListBody](t, resp)
	if len(page.Data) != 1 || page.Data[0].Name != "Zanzibar Trading 2" {
		t.Errorf("page 2 = %+v", page.Data)
	}

	// Descending, via the `-` prefix.
	resp = h.Get(t, "/api/admin/tenants?q=Zanzibar&sort=-name", super.FirebaseUID)
	page = testsupport.Decode[tenantListBody](t, resp)
	if len(page.Data) != 3 || page.Data[0].Name != "Zanzibar Trading 2" {
		t.Errorf("descending sort = %+v", page.Data)
	}

	// An unknown sort field is an error, not a silent fallback: sorting by
	// something other than what was asked for misrepresents the data (§9.0).
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/admin/tenants?sort=whatever", super.FirebaseUID),
		http.StatusBadRequest, "malformed")

	// pageSize is capped rather than refused.
	resp = h.Get(t, "/api/admin/tenants?pageSize=5000", super.FirebaseUID)
	if got := testsupport.Decode[tenantListBody](t, resp).PageSize; got != 100 {
		t.Errorf("pageSize = %d, want it clamped to 100", got)
	}
}

func TestModuleCatalogue(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	resp := h.Get(t, "/api/admin/modules", super.FirebaseUID)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	modules := testsupport.Decode[[]struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		SortOrder int    `json:"sortOrder"`
	}](t, resp)

	// The three of the naming contract, in display order.
	want := []string{"procurement", "inventory", "finance"}
	if len(modules) != len(want) {
		t.Fatalf("modules = %+v, want %v", modules, want)
	}
	for i, code := range want {
		if modules[i].Code != code {
			t.Errorf("modules[%d] = %q, want %q", i, modules[i].Code, code)
		}
	}
}

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}
