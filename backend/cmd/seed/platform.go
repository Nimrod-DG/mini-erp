package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The platform plane: `tenants`, `tenant_modules`, `users`, `user_module_roles`,
// and the chart of accounts.
//
// TWO THINGS ABOUT THIS FILE.
//
//  1. It runs on the **erp_admin** pool, with no tenant context, because these
//     are the five platform tables and they carry no RLS (Trap 2). That is also
//     precisely why every statement below names `tenant_id` explicitly: RLS is
//     not going to supply one, and a missing predicate here is a cross-tenant
//     write rather than an empty result.
//
//  2. The Firebase account comes first and the `users` row second, which is
//     §3.3's order and is not interchangeable. This way a failure leaves an
//     unused provider account; the other way round leaves a `users` row whose
//     firebase_uid names nothing, which is a person who can never sign in.

// provisioner is the slice of the identity provider the seed needs: create an
// account at a UID of my choosing, or leave the existing one alone.
//
// An interface with one method rather than the concrete *auth.Firebase, so
// `--skip-firebase` can hand in a no-op and the database half of the seed stays
// runnable on a machine with no service-account key.
type provisioner interface {
	EnsureSeedUser(ctx context.Context, uid, email, password, displayName string) error
}

// noFirebase satisfies provisioner by doing nothing. It exists for
// `--skip-firebase`, and the seed says loudly when it is in use: the database
// will describe seven people who cannot sign in.
type noFirebase struct{}

func (noFirebase) EnsureSeedUser(context.Context, string, string, string, string) error {
	return nil
}

// seedUID is the §3.5.3 deterministic UID. The `seed-` prefix makes demo
// accounts identifiable at a glance in the Firebase console, and purgeable
// without touching anything real.
func seedUID(slug string) string { return "seed-" + slug }

// seedSuperadmin provisions the platform operator: `tenant_role = 'superadmin'`
// and `tenant_id NULL`, which `users_superadmin_has_no_tenant` makes a
// biconditional pair.
//
// Before Phase 7 this account had to be created by a throwaway program, because
// nothing in the application can create the first superadmin — there is no
// endpoint, deliberately, since the endpoint that would do it is the one only a
// superadmin may call. This is where that stops being a manual step.
func seedSuperadmin(ctx context.Context, g *gorm.DB, fb provisioner) error {
	uid := seedUID("superadmin")
	if err := fb.EnsureSeedUser(ctx, uid, superadminEmail, seedPassword, superadminName); err != nil {
		return err
	}

	_, err := upsertUser(ctx, g, seedID("user", "superadmin"), nil, uid,
		superadminEmail, superadminName, "superadmin")
	return err
}

// upsertUser writes one person and returns the id the row actually has.
//
// THE CONFLICT TARGET IS THE EMAIL, NOT THE ID, and the returned id is the row's
// rather than the one that was offered. Both halves matter, and the reason is
// the state a database that has been developed against for a few weeks is
// actually in: it already holds hand-made accounts from earlier phases, with
// ids nothing derived. Conflicting on the id would collide with `users_email_key`
// and abort the seed; adopting the existing row instead means the seed can be
// run against a working database without destroying what is already there.
//
// The address is the right natural key because it is the one Firebase enforces
// too — one user pool per project, so the same address cannot belong to two
// people (§3.5.1).
//
// `firebase_uid` IS updated on adoption, deliberately: an adopted row that kept
// a stale UID would name a provider account whose password nobody knows, and the
// seeded credentials are the whole point of seeding people.
func upsertUser(ctx context.Context, g *gorm.DB, id uuid.UUID, tenantID *uuid.UUID,
	uid, email, fullName, tenantRole string) (uuid.UUID, error) {

	// A slice, not a bare uuid.UUID: GORM scans a single destination by driver
	// type and reads a UUID's [16]byte as a number, which fails with a syntax
	// error rather than a type error. Every RETURNING in this codebase takes the
	// slice form for that reason.
	var actual []uuid.UUID
	if err := g.WithContext(ctx).Raw(`
		INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role, is_active)
		VALUES (?, ?, ?, ?, ?, ?, true)
		ON CONFLICT (email) DO UPDATE
		SET firebase_uid = EXCLUDED.firebase_uid,
		    full_name    = EXCLUDED.full_name,
		    tenant_id    = EXCLUDED.tenant_id,
		    tenant_role  = EXCLUDED.tenant_role,
		    is_active    = true
		RETURNING id`,
		id, tenantID, uid, email, fullName, tenantRole).Scan(&actual).Error; err != nil {
		return uuid.Nil, err
	}
	if len(actual) == 0 {
		return uuid.Nil, fmt.Errorf("upserting %s returned no id", email)
	}
	return actual[0], nil
}

// seededTenant is what the tenant-data phase needs from the platform phase: the
// ids of rows it must reference across a pool boundary.
type seededTenant struct {
	spec tenantSpec
	id   uuid.UUID
	// users maps a user slug to the row's id.
	users map[string]uuid.UUID
	// raiser and approver are the two actors of documents.go, resolved to this
	// tenant's own people by the level they hold rather than by position.
	raiser   uuid.UUID
	approver uuid.UUID
	// approverUID is the Firebase UID the receipts are posted as. PostGoodsReceipt
	// takes a resolved *identity.Identity, and resolving one from a real UID is
	// how the seed's caller comes out identical to a request's (I9).
	approverUID string
}

// seedTenantPlatform writes the tenant, its entitlements, its chart of accounts,
// and its people.
func seedTenantPlatform(ctx context.Context, g *gorm.DB, fb provisioner, spec tenantSpec) (*seededTenant, error) {
	out := &seededTenant{
		spec:  spec,
		id:    seedID("tenant", spec.Slug),
		users: map[string]uuid.UUID{},
	}

	// Conflicting on the SLUG rather than on the id, and taking the id the row
	// actually has — see upsertUser for the reasoning. A database that has been
	// developed against already holds hand-made workspaces from earlier phases,
	// and a seed that could not be run against one would be a seed nobody could
	// run twice.
	var actual []uuid.UUID
	if err := g.WithContext(ctx).Raw(`
		INSERT INTO tenants (id, name, slug, timezone, status)
		VALUES (?, ?, ?, ?, 'active')
		ON CONFLICT (slug) DO UPDATE
		SET name = EXCLUDED.name, timezone = EXCLUDED.timezone, status = 'active'
		RETURNING id`,
		out.id, spec.Name, spec.Slug, spec.Timezone).Scan(&actual).Error; err != nil {
		return nil, fmt.Errorf("tenant %s: %w", spec.Slug, err)
	}
	if len(actual) == 0 {
		return nil, fmt.Errorf("tenant %s: upsert returned no id", spec.Slug)
	}
	out.id = actual[0]

	// A row per module in the catalogue, enabled or not — the same shape POST
	// /admin/tenants writes, so the entitlement matrix is complete from the start
	// and the console's toggle is always an update rather than a first insert.
	var catalogue []string
	if err := g.WithContext(ctx).Raw(`
		SELECT code FROM modules WHERE is_available = true ORDER BY sort_order`).
		Scan(&catalogue).Error; err != nil {
		return nil, err
	}
	for _, code := range catalogue {
		if err := g.WithContext(ctx).Exec(`
			INSERT INTO tenant_modules (tenant_id, module_code, enabled)
			VALUES (?, ?, ?)
			ON CONFLICT (tenant_id, module_code) DO UPDATE SET enabled = EXCLUDED.enabled`,
			out.id, code, spec.Modules[code]).Error; err != nil {
			return nil, fmt.Errorf("tenant %s module %s: %w", spec.Slug, code, err)
		}
	}

	// Through the SECURITY DEFINER function of §4.2.1, exactly as createTenant
	// does: erp_admin has no grant on `accounts` and must not be given one —
	// that revoke is the surface A11 exists to keep closed. The function is
	// idempotent, so a reseed does not double the chart.
	if err := g.WithContext(ctx).Exec(`SELECT seed_tenant_accounts(?)`, out.id).Error; err != nil {
		return nil, fmt.Errorf("tenant %s accounts: %w", spec.Slug, err)
	}

	for _, user := range spec.Users {
		id, err := seedUser(ctx, g, fb, out.id, user)
		if err != nil {
			return nil, fmt.Errorf("tenant %s user %s: %w", spec.Slug, user.Slug, err)
		}
		out.users[user.Slug] = id

		// Resolved by level, not by position: Nusantara has four users and
		// Bahari three, and the recipe in documents.go refers to the *role*
		// somebody plays rather than to a row number.
		switch user.Roles["procurement"] {
		case "user":
			out.raiser = id
		case "approver":
			out.approver = id
			out.approverUID = seedUID(user.Slug)
		}
	}
	if out.raiser == uuid.Nil || out.approver == uuid.Nil {
		// Every tenant needs both, or the history cannot be written without
		// somebody approving their own requisition (C2).
		return nil, fmt.Errorf("tenant %s has no procurement `user` or no `approver`; "+
			"the seeded history needs two different people to raise and decide", spec.Slug)
	}
	return out, nil
}

// seedUser provisions one person: the provider account first, then the row, then
// their levels.
func seedUser(ctx context.Context, g *gorm.DB, fb provisioner, tenantID uuid.UUID, spec userSpec) (uuid.UUID, error) {
	uid := seedUID(spec.Slug)
	if err := fb.EnsureSeedUser(ctx, uid, spec.Email, seedPassword, spec.FullName); err != nil {
		return uuid.Nil, err
	}

	id, err := upsertUser(ctx, g, seedID("user", spec.Slug), &tenantID, uid,
		spec.Email, spec.FullName, spec.TenantRole)
	if err != nil {
		return uuid.Nil, err
	}

	// A tenant admin gets NO rows here, and that absence is the correct shape
	// rather than a shortcut: they hold `admin` implicitly in every entitled
	// module (§5.4), and seeding rows for them would make a later demotion
	// restore levels they never chose. §15 says so in as many words about Rina
	// and Agus, and B7 is the test that the empty shape resolves to full access.
	for module, level := range spec.Roles {
		if err := g.WithContext(ctx).Exec(`
			INSERT INTO user_module_roles (user_id, module_code, role_level)
			VALUES (?, ?, ?)
			ON CONFLICT (user_id, module_code) DO UPDATE SET role_level = EXCLUDED.role_level`,
			id, module, level).Error; err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}
