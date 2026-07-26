// Package identity answers "who is this verified UID, and what may they see?"
// from the database, on every request (I9). Nothing here reads a Firebase
// custom claim or a client-supplied parameter.
package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The three ways resolution legitimately fails. Each one is reachable in normal
// operation, so none of them is a 500.
var (
	// ErrNoUser is the orphaned-Firebase-account case (§3.3): a token that
	// verifies perfectly and names nobody. 401.
	ErrNoUser = errors.New("identity: no user for firebase uid")
	// ErrInactive is a deactivated user. Users are deactivated, never deleted
	// (§6.9.4), so this is the permanent state of every departed employee. 401.
	ErrInactive = errors.New("identity: user is not active")
	// ErrTenantSuspended is a live user in a suspended tenant. 403 — they are
	// authenticated, and telling them so is the point (§9.x tenant_suspended).
	ErrTenantSuspended = errors.New("identity: tenant is suspended")
)

// Identity is the resolved caller. Every field comes from the database.
type Identity struct {
	UserID      uuid.UUID
	FirebaseUID string
	Email       string
	FullName    string
	TenantRole  string // staff | admin | superadmin

	// TenantID is uuid.Nil for a superadmin, and only for a superadmin — the
	// users_superadmin_has_no_tenant CHECK makes that biconditional.
	TenantID       uuid.UUID
	TenantName     string
	TenantSlug     string
	TenantStatus   string
	TenantTimezone string

	// EnabledModules is the set of module codes the tenant is entitled to —
	// layer 1 of §5.1, and the ceiling on every level below it.
	//
	// It is separate from ModuleRoles because the two failures must stay
	// distinguishable: a tenant admin holds no role rows at all, so an empty
	// ModuleRoles cannot tell "this tenant never bought Finance"
	// (module_not_enabled) from "this user was given nothing in Finance"
	// (insufficient_module_role). Only this set can.
	EnabledModules map[string]bool

	// ModuleRoles maps module code to role level for the modules the tenant is
	// entitled to. An absent module means level `none` — the level is the
	// absence of a row (§5.3) — so this map is never a source of `"none"`.
	//
	// It is a rendering input, and an input to LevelFor; it is not itself a
	// permission check. Read it through LevelFor, which applies the entitlement
	// ceiling and the implicit-admin rule that this map knows nothing about.
	ModuleRoles map[string]string
}

// IsSuperadmin reports whether the caller is a platform superadmin, who belongs
// to no tenant and holds no module roles.
func (i *Identity) IsSuperadmin() bool { return i.TenantRole == TenantSuperadmin }

// row is the shape of the one join that identity resolution costs. The tenant
// columns are nullable because a superadmin has no tenant.
type row struct {
	ID          uuid.UUID
	FirebaseUID string
	Email       string
	FullName    string
	TenantRole  string
	IsActive    bool

	TenantID       *uuid.UUID
	TenantName     *string
	TenantSlug     *string
	TenantStatus   *string
	TenantTimezone *string
}

// Resolve looks up the user behind a verified Firebase UID.
//
// It runs on the erp_app pool outside any tenant transaction, which is correct
// and necessary: the five platform tables carry no RLS precisely because they
// are read *before* tenant context exists (§6.8).
//
// The user row is fetched without an `is_active` filter and the flag checked
// here, rather than filtered in SQL. The outcome is identical — both are 401 —
// but a deactivated user and an orphaned account are different operational
// events, and only this shape can tell them apart in a log.
func Resolve(ctx context.Context, g *gorm.DB, firebaseUID string) (*Identity, error) {
	var r row
	err := g.WithContext(ctx).Raw(`
		SELECT u.id, u.firebase_uid, u.email, u.full_name, u.tenant_role, u.is_active,
		       u.tenant_id AS tenant_id,
		       t.name      AS tenant_name,
		       t.slug      AS tenant_slug,
		       t.status    AS tenant_status,
		       t.timezone  AS tenant_timezone
		FROM users u
		LEFT JOIN tenants t ON t.id = u.tenant_id
		WHERE u.firebase_uid = ?`, firebaseUID).Scan(&r).Error
	if err != nil {
		return nil, fmt.Errorf("identity: load user: %w", err)
	}
	if r.ID == uuid.Nil {
		return nil, ErrNoUser
	}
	if !r.IsActive {
		return nil, ErrInactive
	}

	id := &Identity{
		UserID:         r.ID,
		FirebaseUID:    r.FirebaseUID,
		Email:          r.Email,
		FullName:       r.FullName,
		TenantRole:     r.TenantRole,
		EnabledModules: map[string]bool{},
		ModuleRoles:    map[string]string{},
	}

	if id.IsSuperadmin() {
		// No tenant, no module roles, no tenant transaction. Superadmin routes
		// run on the erp_admin pool, which is revoked from every business
		// table (§7).
		return id, nil
	}

	if r.TenantID == nil {
		// Unreachable while users_superadmin_has_no_tenant holds. If the
		// constraint is ever dropped, this must not degrade into "a user with
		// no tenant context sees whatever the pool last set".
		return nil, fmt.Errorf("identity: user %s is %s with no tenant: %w",
			r.ID, r.TenantRole, ErrNoUser)
	}

	id.TenantID = *r.TenantID
	id.TenantName = derefOr(r.TenantName, "")
	id.TenantSlug = derefOr(r.TenantSlug, "")
	id.TenantStatus = derefOr(r.TenantStatus, "")
	id.TenantTimezone = derefOr(r.TenantTimezone, "")

	if id.TenantStatus == "suspended" {
		return nil, ErrTenantSuspended
	}

	enabled, roles, err := entitlements(ctx, g, id.UserID, id.TenantID)
	if err != nil {
		return nil, err
	}
	id.EnabledModules = enabled
	id.ModuleRoles = roles
	return id, nil
}

// entitlements returns the tenant's enabled modules and, within them, the
// caller's explicit level in each.
//
// It is one query rather than two because identity resolution runs on *every*
// request (I9), and the entitlement set is the driving table: every row is a
// module the tenant has, and role_level is NULL when the user holds no row in
// it. So a tenant admin — who correctly has no rows at all — still comes back
// with a full entitlement set.
//
// A role in a module the tenant does not have is not reported at all: the
// entitlement gate comes first, and the two failures must stay distinguishable
// to the client (module_not_enabled vs insufficient_module_role, §7).
func entitlements(ctx context.Context, g *gorm.DB, userID, tenantID uuid.UUID) (map[string]bool, map[string]string, error) {
	var rows []struct {
		ModuleCode string
		RoleLevel  *string
	}
	err := g.WithContext(ctx).Raw(`
		SELECT tm.module_code, umr.role_level
		FROM tenant_modules tm
		JOIN modules m ON m.code = tm.module_code AND m.is_available = true
		LEFT JOIN user_module_roles umr ON umr.module_code = tm.module_code
		                              AND umr.user_id     = ?
		WHERE tm.tenant_id = ? AND tm.enabled = true`, userID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("identity: load entitlements: %w", err)
	}

	enabled := make(map[string]bool, len(rows))
	roles := make(map[string]string, len(rows))
	for _, r := range rows {
		enabled[r.ModuleCode] = true
		if r.RoleLevel != nil {
			roles[r.ModuleCode] = *r.RoleLevel
		}
	}
	return enabled, roles, nil
}

func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
