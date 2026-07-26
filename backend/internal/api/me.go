package api

import (
	"github.com/gofiber/fiber/v2"

	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/internal/middleware"
)

// meUser is the caller. `firebaseUid` is deliberately absent: it is an identity
// provider's internal handle, and the frontend has no use for it.
type meUser struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	FullName   string `json:"fullName"`
	TenantRole string `json:"tenantRole"`
}

// meTenant carries the timezone because every date the frontend renders is a
// business date in the *tenant's* zone, not the browser's (§2.5.3, FE15).
type meTenant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Status   string `json:"status"`
	Timezone string `json:"timezone"`
}

type meResponse struct {
	User meUser `json:"user"`
	// Tenant is null for a superadmin, who belongs to none.
	Tenant *meTenant `json:"tenant"`
	// ModuleRoles is the caller's **effective** level in each module they can
	// reach: what LevelFor resolves, not what `user_module_roles` stores. The
	// shell hides everything absent from it (§10.1) — which is cosmetic. Every
	// hidden control is independently enforced by RequireModule (I12).
	//
	// EFFECTIVE, NOT STORED, AND THE DIFFERENCE IS NOT COSMETIC. A tenant admin
	// holds `admin` in every entitled module *implicitly* and correctly has no
	// role rows at all (§5.4), so the stored map is empty for them. Sending that
	// gave Rina — the seed's Nusantara admin, and the person acceptance step 10
	// is about — a sidebar with no modules in it, no bottom tabs, and a dashboard
	// the shell would not navigate away from, while every endpoint behind those
	// links answered her perfectly well.
	//
	// The frontend already documented this contract: `holds()` in lib/levels.ts
	// says in as many words that the server "applied the implicit-admin rule
	// before sending it". It had not. This is that promise being kept, and B13
	// is the test that keeps it.
	ModuleRoles map[string]string `json:"moduleRoles"`
}

// Me renders the resolved identity. It runs no query of its own: everything it
// returns was already read by ResolveIdentity, which runs on every request
// anyway.
func Me(c *fiber.Ctx) error {
	id := middleware.IdentityFrom(c)
	if id == nil {
		return httpx.Unauthenticated(c)
	}
	return c.JSON(newMeResponse(id))
}

func newMeResponse(id *identity.Identity) meResponse {
	resp := meResponse{
		User: meUser{
			ID:         id.UserID.String(),
			Email:      id.Email,
			FullName:   id.FullName,
			TenantRole: id.TenantRole,
		},
		// Through the same effectiveRoles the user-admin screens use, so there is
		// exactly one implementation of §5.4 and the nav cannot come to disagree
		// with the matrix an administrator is looking at. It returns `{}` rather
		// than nil, which matters — the frontend indexes into this.
		ModuleRoles: effectiveRoles(id.EnabledModules, id.TenantRole, id.ModuleRoles),
	}
	if !id.IsSuperadmin() {
		resp.Tenant = &meTenant{
			ID:       id.TenantID.String(),
			Name:     id.TenantName,
			Slug:     id.TenantSlug,
			Status:   id.TenantStatus,
			Timezone: id.TenantTimezone,
		}
	}
	return resp
}
