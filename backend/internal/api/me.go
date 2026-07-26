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
	// ModuleRoles holds only modules the tenant is entitled to, and only those
	// the caller holds a level in. The shell hides everything absent from it
	// (§10.1) — which is cosmetic. Every hidden control is independently
	// enforced by RequireModule (I12).
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
		ModuleRoles: id.ModuleRoles,
	}
	if resp.ModuleRoles == nil {
		// `{}` rather than `null`: the frontend indexes into this.
		resp.ModuleRoles = map[string]string{}
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
