package identity

// Tenant-level roles (§5.4). Named constants because these five strings appear
// in the migrations' CHECK constraints, in JSON, and in the frontend — the
// naming contract is easier to keep when Go states it once.
const (
	TenantStaff      = "staff"
	TenantAdmin      = "admin"
	TenantSuperadmin = "superadmin"
)

// RoleLevel is a user's level in one module (§5.3).
//
// The levels are *ranked*, and each includes everything below it. That is what
// makes every permission check a comparison — `level >= min` — rather than set
// membership, and it is why RequireModule takes a minimum rather than a list.
type RoleLevel int

const (
	RoleNone RoleLevel = iota
	RoleViewer
	RoleUser
	RoleApprover
	RoleAdmin
)

// levelNames is indexed by rank, so String and ParseRoleLevel cannot disagree.
var levelNames = [...]string{"none", "viewer", "user", "approver", "admin"}

func (l RoleLevel) String() string {
	if l < RoleNone || int(l) >= len(levelNames) {
		return "unknown"
	}
	return levelNames[l]
}

// ParseRoleLevel converts the wire form to a rank. It reports whether the input
// was a level name at all, so a handler can reject `{"roleLevel":"managr"}` as
// malformed instead of silently treating a typo as `none` — the direction a
// silent default fails is "grants nothing", but a client that cannot tell a typo
// from a revocation will make one and not notice.
//
// `none` parses successfully even though `user_module_roles` cannot store it:
// it is the value the API accepts to mean "delete the row" (§5.3).
func ParseRoleLevel(s string) (RoleLevel, bool) {
	for rank, name := range levelNames {
		if s == name {
			return RoleLevel(rank), true
		}
	}
	return RoleNone, false
}

// TenantEntitled reports whether the caller's tenant is entitled to a module —
// layer 1 of §5.1, and the ceiling on everything else.
func (i *Identity) TenantEntitled(module string) bool {
	return i.EnabledModules[module]
}

// LevelFor is the caller's effective level in one module, and every permission
// check in this application goes through it (§5.4).
//
// The ordering is the whole design:
//
//	entitlement missing  → none   — checked FIRST; entitlement is the ceiling
//	tenant_role == admin → admin  — implicit, no user_module_roles rows needed
//	otherwise            → the explicit level, missing == none
//
// Entitlement before the admin shortcut means a tenant admin of a company
// without Finance still resolves to `none` in Finance: admin is the ceiling
// *within* what the tenant has bought, never above it (B8).
//
// A superadmin has no tenant and therefore no entitlements, so this returns
// `none` for every module — §5.5's "superadmins have no access to tenant
// business data" falls out of the same ordering rather than being a special
// case (B6).
func (i *Identity) LevelFor(module string) RoleLevel {
	if !i.TenantEntitled(module) {
		return RoleNone
	}
	if i.TenantRole == TenantAdmin {
		// Deliberately ignores any user_module_roles rows this user has. They
		// are left in place, not deleted, so demoting them to staff restores
		// the levels they held before promotion (§5.4).
		return RoleAdmin
	}
	level, _ := ParseRoleLevel(i.ModuleRoles[module])
	return level
}

// IsTenantAdmin reports whether the caller administers their own tenant's users
// and role matrix (§5.7's tenant plane). It is not a module role and is not
// ranked against one.
func (i *Identity) IsTenantAdmin() bool { return i.TenantRole == TenantAdmin }
