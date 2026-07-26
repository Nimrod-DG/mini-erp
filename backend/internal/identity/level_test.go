// Unit tests for §5.4's effective-level resolution. No database and no HTTP:
// LevelFor is a pure function and this is the only place its *internal ordering*
// is observable.
//
// That matters more than it looks. RequireModule happens to check entitlement
// itself before calling LevelFor — it has to, because the two failures carry
// different error codes — so every HTTP-level permission test passes even with
// the ordering inside LevelFor reversed. Verified by mutation: swapping the two
// branches leaves all of Group B green.
//
// From Phase 4 on, handlers call LevelFor directly for record-level rules with
// no middleware in front of them. These tests are what will catch the reversal
// then.
package identity

import "testing"

// entitled builds an Identity with the given entitlements and stored levels.
func entitled(tenantRole string, enabled []string, roles map[string]string) *Identity {
	set := make(map[string]bool, len(enabled))
	for _, code := range enabled {
		set[code] = true
	}
	if roles == nil {
		roles = map[string]string{}
	}
	return &Identity{TenantRole: tenantRole, EnabledModules: set, ModuleRoles: roles}
}

func TestLevelForChecksEntitlementBeforeTheAdminShortcut(t *testing.T) {
	// Bahari Logistics: entitled to Procurement and Inventory, not Finance.
	admin := entitled(TenantAdmin, []string{"procurement", "inventory"}, nil)

	if got := admin.LevelFor("procurement"); got != RoleAdmin {
		t.Errorf("procurement = %v, want admin — an admin is implicitly a module admin", got)
	}
	// The whole point. Admin is the ceiling *within* what the tenant has bought,
	// never above it: check the shortcut first and this returns `admin` in a
	// module the company never licensed.
	if got := admin.LevelFor("finance"); got != RoleNone {
		t.Errorf("finance = %v, want none — entitlement is checked FIRST and beats "+
			"the admin shortcut (§5.4, B8)", got)
	}
}

// The same ordering, with stored rows present. A staff member promoted to admin
// keeps their rows; an entitlement the tenant lost must still win over both.
func TestLevelForEntitlementBeatsAStoredLevelToo(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tenantRole string
	}{
		{"staff", TenantStaff},
		{"admin", TenantAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := entitled(tc.tenantRole, []string{"procurement"},
				map[string]string{"finance": "admin"})

			if got := user.LevelFor("finance"); got != RoleNone {
				t.Errorf("finance = %v, want none — a role level in an unentitled "+
					"module is not access to it", got)
			}
		})
	}
}

func TestLevelForStaffReadsTheStoredLevel(t *testing.T) {
	staff := entitled(TenantStaff, []string{"procurement", "inventory", "finance"},
		map[string]string{"procurement": "approver", "inventory": "viewer"})

	for module, want := range map[string]RoleLevel{
		"procurement": RoleApprover,
		"inventory":   RoleViewer,
		// Entitled, but no row: the level is the absence of a row (§5.3, B4).
		"finance": RoleNone,
		// Not a module at all.
		"manufacturing": RoleNone,
	} {
		if got := staff.LevelFor(module); got != want {
			t.Errorf("LevelFor(%q) = %v, want %v", module, got, want)
		}
	}
}

// An admin's stored rows are ignored while they are an admin and left in place,
// so a later demotion restores them (§5.4). This is the read half of that.
func TestLevelForAdminIgnoresStoredRowsWithoutLoweringAccess(t *testing.T) {
	admin := entitled(TenantAdmin, []string{"procurement"},
		map[string]string{"procurement": "viewer"})

	if got := admin.LevelFor("procurement"); got != RoleAdmin {
		t.Errorf("procurement = %v, want admin — a stored `viewer` must not cap an admin", got)
	}
	if admin.ModuleRoles["procurement"] != "viewer" {
		t.Error("the stored row was mutated; demotion must be able to restore it")
	}
}

// A superadmin has no tenant, so no entitlements, so no module access anywhere.
// §5.5 falls out of the same ordering rather than needing a special case (B6).
func TestLevelForSuperadminHasNothing(t *testing.T) {
	super := entitled(TenantSuperadmin, nil, nil)

	for _, module := range []string{"procurement", "inventory", "finance"} {
		if got := super.LevelFor(module); got != RoleNone {
			t.Errorf("LevelFor(%q) = %v, want none for a platform superadmin", module, got)
		}
	}
	if super.IsTenantAdmin() {
		t.Error("a superadmin is not a tenant admin; they administer no workspace's users")
	}
}

func TestRoleLevelOrderingAndNames(t *testing.T) {
	// Ranked, so every permission check is a comparison (§5.3).
	ranked := []RoleLevel{RoleNone, RoleViewer, RoleUser, RoleApprover, RoleAdmin}
	for i := 1; i < len(ranked); i++ {
		if ranked[i-1] >= ranked[i] {
			t.Fatalf("%v is not ranked below %v", ranked[i-1], ranked[i])
		}
	}

	// The wire names are a naming contract shared with the migrations, the JSON,
	// and the frontend. A rename here is a five-file change.
	for level, want := range map[RoleLevel]string{
		RoleNone: "none", RoleViewer: "viewer", RoleUser: "user",
		RoleApprover: "approver", RoleAdmin: "admin",
	} {
		if got := level.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
		parsed, ok := ParseRoleLevel(want)
		if !ok || parsed != level {
			t.Errorf("ParseRoleLevel(%q) = %v, %t, want %v, true", want, parsed, ok, level)
		}
	}

	// A typo is not silently `none`. The direction a silent default fails is
	// "grants nothing", but a client that cannot tell a typo from a revocation
	// will make one and not notice.
	if _, ok := ParseRoleLevel("managr"); ok {
		t.Error("ParseRoleLevel accepted a typo")
	}
	if _, ok := ParseRoleLevel(""); ok {
		t.Error("ParseRoleLevel accepted an empty string")
	}
}
