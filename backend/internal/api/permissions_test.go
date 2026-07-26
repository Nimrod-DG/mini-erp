// Group B — Permissions (§12.3). Every test here drives the real chain built by
// api.New, so what is asserted is what ships.
//
// The three that catch real design errors are B7, B8, and B10: the implicit
// admin who correctly has no rows, the entitlement ceiling that beats the admin
// shortcut, and two concurrent demotions of the last two admins.
package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// probe issues a GET against the route guarded by RequireModule(module, level).
func probe(t *testing.T, h *testsupport.Harness, token, module string, level identity.RoleLevel) *http.Response {
	t.Helper()
	return h.Get(t, testsupport.ProbePath(module, level), token)
}

// --------------------------------------------------------------------------
// B1, B2 — the two refusals, and why they are two.
// --------------------------------------------------------------------------

// B1: the tenant never bought the module. The fix belongs to the superadmin, so
// the code has to say so.
func TestB1ModuleNotEnabledWhenTenantLacksEntitlement(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "No Finance Ltd")
	tenant.DisableModule(t, "finance")

	// A user with the *highest* level in finance. The level is real and stored;
	// the entitlement is gone, and entitlement is the ceiling (§5.4).
	user := tenant.NewUser(t, map[string]string{"finance": "admin"})

	body := testsupport.AssertErrorCode(t,
		probe(t, h, user.FirebaseUID, "finance", identity.RoleViewer),
		http.StatusForbidden, "module_not_enabled")

	if body.Details["module"] != "finance" {
		t.Errorf("details.module = %v, want finance", body.Details["module"])
	}
}

// B2: the tenant has the module and this user was not given enough. The fix
// belongs to the tenant admin, and `required`/`actual` is what the console needs
// to say which dropdown to change.
func TestB2InsufficientModuleRoleCarriesRequiredAndActual(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Under-levelled Ltd")
	user := tenant.NewUser(t, map[string]string{"procurement": "user"})

	body := testsupport.AssertErrorCode(t,
		probe(t, h, user.FirebaseUID, "procurement", identity.RoleApprover),
		http.StatusForbidden, "insufficient_module_role")

	for field, want := range map[string]string{
		"module":   "procurement",
		"required": "approver",
		"actual":   "user",
	} {
		if got := body.Details[field]; got != want {
			t.Errorf("details.%s = %v, want %q", field, got, want)
		}
	}
}

// --------------------------------------------------------------------------
// B3 — the levels are ranked, so checks are comparisons (§5.3).
// --------------------------------------------------------------------------

func TestB3LevelOrdering(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Ordering Ltd")

	admin := tenant.NewUser(t, map[string]string{"inventory": "admin"})
	viewer := tenant.NewUser(t, map[string]string{"inventory": "viewer"})

	for _, tc := range []struct {
		name  string
		user  *testsupport.UserFixture
		level identity.RoleLevel
		want  int
	}{
		// Each level includes everything below it.
		{"admin satisfies viewer", admin, identity.RoleViewer, http.StatusOK},
		{"admin satisfies approver", admin, identity.RoleApprover, http.StatusOK},
		{"admin satisfies admin", admin, identity.RoleAdmin, http.StatusOK},
		{"viewer satisfies viewer", viewer, identity.RoleViewer, http.StatusOK},
		// And nothing above it.
		{"viewer does not satisfy approver", viewer, identity.RoleApprover, http.StatusForbidden},
		{"viewer does not satisfy user", viewer, identity.RoleUser, http.StatusForbidden},
		{"viewer does not satisfy admin", viewer, identity.RoleAdmin, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testsupport.AssertStatus(t,
				probe(t, h, tc.user.FirebaseUID, "inventory", tc.level), tc.want)
		})
	}
}

// --------------------------------------------------------------------------
// B4 — a missing row is `none`, and `none` reaches nothing.
// --------------------------------------------------------------------------

func TestB4MissingRoleRowIsNone(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Sparse Ltd")

	// A staff user with a level in procurement and no row at all in inventory.
	user := tenant.NewUser(t, map[string]string{"procurement": "viewer"})

	// The tenant *is* entitled to inventory, so this is under-levelling and not
	// a missing entitlement — the distinction B1 and B2 exist to keep.
	body := testsupport.AssertErrorCode(t,
		probe(t, h, user.FirebaseUID, "inventory", identity.RoleViewer),
		http.StatusForbidden, "insufficient_module_role")

	if body.Details["actual"] != "none" {
		t.Errorf("details.actual = %v, want none — a missing row must resolve to none",
			body.Details["actual"])
	}

	// And nothing was quietly written to make that true.
	var stored []string
	if err := h.DB.Owner.Raw(`
		SELECT role_level FROM user_module_roles
		WHERE user_id = ? AND module_code = 'inventory'`, user.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read stored roles: %v", err)
	}
	if len(stored) != 0 {
		t.Errorf("user_module_roles has %v for inventory — `none` must never be stored (§5.3)", stored)
	}
}

// --------------------------------------------------------------------------
// B5 — an entitlement change lands on the next request.
// --------------------------------------------------------------------------

// This is the "done when" item: toggling a module off in the admin UI makes
// that tenant's next request return 403, with no restart and no cache to
// invalidate. It goes through the real superadmin endpoint rather than a fixture
// UPDATE, because the claim is about the two planes meeting (§5.7).
func TestB5EntitlementToggleTakesEffectOnTheNextRequest(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)
	tenant := h.DB.NewTenant(t, "Toggle Ltd")
	user := tenant.NewUser(t, map[string]string{"procurement": "approver"})

	path := testsupport.ProbePath("procurement", identity.RoleApprover)
	toggle := fmt.Sprintf("/api/admin/tenants/%s/modules/procurement", tenant.ID)

	// Before.
	testsupport.AssertStatus(t, h.Get(t, path, user.FirebaseUID), http.StatusOK)

	// The superadmin revokes the entitlement. No restart, no invalidation call.
	testsupport.AssertStatus(t,
		h.Put(t, toggle, super.FirebaseUID, map[string]any{"enabled": false}), http.StatusOK)

	// The very next request from the same user, with the same token.
	testsupport.AssertErrorCode(t,
		h.Get(t, path, user.FirebaseUID), http.StatusForbidden, "module_not_enabled")

	// The user's stored level was not touched — the two planes are independent,
	// so re-enabling restores access rather than requiring the levels again.
	testsupport.AssertStatus(t,
		h.Put(t, toggle, super.FirebaseUID, map[string]any{"enabled": true}), http.StatusOK)
	testsupport.AssertStatus(t, h.Get(t, path, user.FirebaseUID), http.StatusOK)
}

// --------------------------------------------------------------------------
// B6 — superadmins have no access to tenant business data (§5.5).
// --------------------------------------------------------------------------

func TestB6SuperadminCannotReachTenantEndpoints(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)
	tenant := h.DB.NewTenant(t, "Off Limits Ltd")

	// Every module, at the lowest level there is. A superadmin has no tenant,
	// so no module is enabled for them and there is nothing to be a viewer of.
	for _, module := range []string{"procurement", "inventory", "finance"} {
		t.Run(module, func(t *testing.T) {
			testsupport.AssertErrorCode(t,
				probe(t, h, super.FirebaseUID, module, identity.RoleViewer),
				http.StatusForbidden, "module_not_enabled")
		})
	}

	// The tenant plane is closed to them too: managing one workspace's users is
	// not a platform-administration task (§5.7).
	t.Run("tenant user management", func(t *testing.T) {
		testsupport.AssertErrorCode(t,
			h.Get(t, "/api/tenant/users", super.FirebaseUID), http.StatusForbidden, "forbidden")
	})

	// And a route with no RequireModule on it at all still cannot read tenant
	// data, because TenantTx opens no transaction for a tenantless identity —
	// the handler finds nil rather than a handle RLS silently empties.
	t.Run("un-gated tenant-scoped route", func(t *testing.T) {
		resp := h.Get(t, testsupport.TenantTxPath, super.FirebaseUID)
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("a superadmin read tenant data at %s", testsupport.TenantTxPath)
		}
	})

	// Sanity: the tenant's own user reaches what the superadmin could not, so
	// the refusals above are about the caller and not about a broken route.
	testsupport.AssertStatus(t,
		probe(t, h, tenant.User.FirebaseUID, "procurement", identity.RoleViewer), http.StatusOK)
}

// --------------------------------------------------------------------------
// B7 — the implicit admin. The design error this catches is seeding rows.
// --------------------------------------------------------------------------

func TestB7TenantAdminResolvesToAdminEverywhereWithNoRows(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Owner Operator Ltd")
	admin := tenant.NewAdmin(t)

	// The premise: this user has no user_module_roles rows whatsoever. If the
	// fixture or a handler had seeded them, the rest of this test would pass
	// for the wrong reason.
	var count int64
	if err := h.DB.Owner.Raw(`
		SELECT count(*) FROM user_module_roles WHERE user_id = ?`, admin.ID).
		Scan(&count).Error; err != nil {
		t.Fatalf("count stored roles: %v", err)
	}
	if count != 0 {
		t.Fatalf("the tenant admin has %d user_module_roles rows; §5.4 says none", count)
	}

	// And yet they hold `admin` in all three entitled modules, at every level.
	for _, module := range []string{"procurement", "inventory", "finance"} {
		for _, level := range []identity.RoleLevel{
			identity.RoleViewer, identity.RoleUser, identity.RoleApprover, identity.RoleAdmin,
		} {
			t.Run(fmt.Sprintf("%s/%s", module, level), func(t *testing.T) {
				testsupport.AssertStatus(t,
					probe(t, h, admin.FirebaseUID, module, level), http.StatusOK)
			})
		}
	}
}

// A staff user promoted to admin keeps their old rows, so demotion restores the
// levels they held before — the reason §5.4 says to ignore the rows rather than
// delete them.
func TestB7DemotionRestoresThePreviousLevels(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Promotion Ltd")
	other := tenant.NewAdmin(t) // so the last-admin rule is not what fires
	user := tenant.NewUser(t, map[string]string{"procurement": "viewer"})

	approver := testsupport.ProbePath("procurement", identity.RoleApprover)

	// As staff: viewer is not enough.
	testsupport.AssertStatus(t, h.Get(t, approver, user.FirebaseUID), http.StatusForbidden)

	// Promoted: implicitly admin everywhere, without touching their rows.
	testsupport.AssertStatus(t,
		h.Patch(t, "/api/tenant/users/"+user.ID.String(), other.FirebaseUID,
			map[string]any{"tenantRole": "admin"}), http.StatusOK)
	testsupport.AssertStatus(t, h.Get(t, approver, user.FirebaseUID), http.StatusOK)

	// Demoted: back to the viewer row that was never removed.
	testsupport.AssertStatus(t,
		h.Patch(t, "/api/tenant/users/"+user.ID.String(), other.FirebaseUID,
			map[string]any{"tenantRole": "staff"}), http.StatusOK)
	testsupport.AssertStatus(t, h.Get(t, approver, user.FirebaseUID), http.StatusForbidden)
	testsupport.AssertStatus(t,
		h.Get(t, testsupport.ProbePath("procurement", identity.RoleViewer), user.FirebaseUID),
		http.StatusOK)
}

// --------------------------------------------------------------------------
// B8 — entitlement beats the admin shortcut. Acceptance step 5, seed user Agus.
// --------------------------------------------------------------------------

// The ordering inside LevelFor is the whole of this test: check entitlement
// first, and a tenant admin of a company without Finance resolves to `none` in
// Finance. Check the admin shortcut first, and they resolve to `admin` in a
// module their employer never bought.
func TestB8EntitlementBeatsTheAdminShortcut(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Bahari Logistics")
	tenant.DisableModule(t, "finance")
	admin := tenant.NewAdmin(t)

	// Admin everywhere the tenant is entitled...
	for _, module := range []string{"procurement", "inventory"} {
		testsupport.AssertStatus(t,
			probe(t, h, admin.FirebaseUID, module, identity.RoleAdmin), http.StatusOK)
	}

	// ...and nothing at all in Finance, at the lowest level asked for.
	testsupport.AssertErrorCode(t,
		probe(t, h, admin.FirebaseUID, "finance", identity.RoleViewer),
		http.StatusForbidden, "module_not_enabled")

	// The nav is driven off /api/me, so Finance must be absent there too — this
	// admin sees no Finance item, exactly like their staff (§5.6).
	body := testsupport.Decode[struct {
		ModuleRoles map[string]string `json:"moduleRoles"`
	}](t, h.Get(t, "/api/me", admin.FirebaseUID))
	if _, present := body.ModuleRoles["finance"]; present {
		t.Error("/api/me offers finance to the admin of a tenant without the entitlement")
	}

	// A tenant admin cannot grant what their tenant does not have, either: the
	// ceiling holds on the write path as well as the read path (§5.7).
	staff := tenant.NewUser(t, nil)
	testsupport.AssertErrorCode(t,
		h.Put(t, "/api/tenant/users/"+staff.ID.String()+"/modules/finance",
			admin.FirebaseUID, map[string]any{"roleLevel": "admin"}),
		http.StatusForbidden, "module_not_enabled")
}

// --------------------------------------------------------------------------
// B13 — /api/me reports the EFFECTIVE level, not the stored one.
// --------------------------------------------------------------------------

// The bug this exists for, found by Phase 7's acceptance run and worth stating
// plainly: `/api/me` used to return `user_module_roles` as stored, and a tenant
// admin correctly has no rows there (§5.4, B7). So Rina — the seed's Nusantara
// admin, the person acceptance step 10 is about — signed in to a sidebar with no
// modules in it, no bottom tab bar, and no way to reach a single screen, while
// every endpoint behind those links answered her requests perfectly.
//
// It went unnoticed for four phases because the one account anybody signed in
// with by hand had explicit role rows, which makes the stored and effective maps
// identical. B7 could not catch it either: B7 asks whether an implicit admin
// gets *past the gate*, and she always did. The gap was only ever in what the
// shell was told.
//
// So this test asserts the two maps for the same person and requires them to
// differ in exactly the way §5.4 says they should.
func TestB13MeReportsTheEffectiveLevelForATenantAdmin(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Implicit Admin Ltd")
	admin := tenant.NewAdmin(t)

	// The premise. If this ever fails, the fixture has started seeding rows for
	// admins and the rest of the test is measuring nothing.
	var stored int
	tenant.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM user_module_roles WHERE user_id = ?`,
			admin.ID).Row().Scan(&stored)
	})
	if stored != 0 {
		t.Fatalf("the fixture gave the admin %d role rows; §5.4 says none", stored)
	}

	body := testsupport.Decode[struct {
		ModuleRoles map[string]string `json:"moduleRoles"`
	}](t, h.Get(t, "/api/me", admin.FirebaseUID))

	for _, module := range []string{"procurement", "inventory", "finance"} {
		if body.ModuleRoles[module] != "admin" {
			t.Errorf("moduleRoles[%s] = %q, want admin — the nav is driven off this "+
				"map, so an empty one is an admin who can reach nothing",
				module, body.ModuleRoles[module])
		}
	}

	// And a staff user's map is still what they were actually given: the fix
	// must not have turned the effective map into "admin for everybody".
	staff := tenant.NewUser(t, map[string]string{"inventory": "viewer"})
	body = testsupport.Decode[struct {
		ModuleRoles map[string]string `json:"moduleRoles"`
	}](t, h.Get(t, "/api/me", staff.FirebaseUID))

	if body.ModuleRoles["inventory"] != "viewer" {
		t.Errorf("staff moduleRoles[inventory] = %q, want viewer",
			body.ModuleRoles["inventory"])
	}
	for _, module := range []string{"procurement", "finance"} {
		if level, present := body.ModuleRoles[module]; present {
			t.Errorf("moduleRoles[%s] = %q, want absent — a missing row is `none`, "+
				"and `none` is the absence of a key (§5.3)", module, level)
		}
	}
}

// --------------------------------------------------------------------------
// Isolation, through the permission gate.
// --------------------------------------------------------------------------

// Two tenants, because a single-tenant test cannot detect an isolation failure
// (§12.2). Same module, same level, different data.
func TestGatedRouteStillScopesToTheCallersTenant(t *testing.T) {
	h := testsupport.NewHarness(t)
	a := h.DB.NewTenant(t, "Gated A")
	b := h.DB.NewTenant(t, "Gated B")

	codeOf := func(f *testsupport.TenantFixture) string {
		var code string
		f.Must(t, func(tx *gorm.DB) error {
			return tx.Raw(`SELECT code FROM warehouses`).Row().Scan(&code)
		})
		return code
	}
	wantA, wantB := codeOf(a), codeOf(b)

	for _, tc := range []struct {
		name       string
		user       *testsupport.UserFixture
		want, deny string
	}{
		{"tenant A", a.User, wantA, wantB},
		{"tenant B", b.User, wantB, wantA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := probe(t, h, tc.user.FirebaseUID, "inventory", identity.RoleViewer)
			testsupport.AssertStatus(t, resp, http.StatusOK)
			codes := testsupport.Decode[[]string](t, resp)

			if len(codes) != 1 || codes[0] != tc.want {
				t.Fatalf("warehouses = %v, want exactly [%s]", codes, tc.want)
			}
			if codes[0] == tc.deny {
				t.Fatalf("saw the other tenant's warehouse %q", tc.deny)
			}
		})
	}
}
