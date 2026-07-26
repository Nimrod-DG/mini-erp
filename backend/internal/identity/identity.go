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

	// ModuleRoles maps module code to role level for the modules the tenant is
	// entitled to. An absent module means level `none` — the level is the
	// absence of a row (§5.3) — so this map is never a source of `"none"`.
	//
	// It is a rendering input and an input to RequireModule (Phase 3); it is
	// not itself a permission check.
	ModuleRoles map[string]string
}

// IsSuperadmin reports whether the caller is a platform superadmin, who belongs
// to no tenant and holds no module roles.
func (i *Identity) IsSuperadmin() bool { return i.TenantRole == "superadmin" }

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
		UserID:      r.ID,
		FirebaseUID: r.FirebaseUID,
		Email:       r.Email,
		FullName:    r.FullName,
		TenantRole:  r.TenantRole,
		ModuleRoles: map[string]string{},
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

	roles, err := moduleRoles(ctx, g, id.UserID, id.TenantID)
	if err != nil {
		return nil, err
	}
	id.ModuleRoles = roles
	return id, nil
}

// moduleRoles returns the caller's level in each module their tenant is
// entitled to. A role in a module the tenant does not have is not reported:
// the entitlement gate comes first, and the two failures stay distinguishable
// to the client (module_not_enabled vs insufficient_module_role, §7).
func moduleRoles(ctx context.Context, g *gorm.DB, userID, tenantID uuid.UUID) (map[string]string, error) {
	var rows []struct {
		ModuleCode string
		RoleLevel  string
	}
	err := g.WithContext(ctx).Raw(`
		SELECT umr.module_code, umr.role_level
		FROM user_module_roles umr
		JOIN tenant_modules tm ON tm.module_code = umr.module_code
		                      AND tm.tenant_id   = ?
		                      AND tm.enabled     = true
		JOIN modules m ON m.code = umr.module_code AND m.is_available = true
		WHERE umr.user_id = ?`, tenantID, userID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("identity: load module roles: %w", err)
	}

	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ModuleCode] = r.RoleLevel
	}
	return out, nil
}

func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
