package middleware

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// RequireModule (step 5 of §7) is the per-route authorization gate. It performs
// the two checks of §5.1 in order, and the distinction between them is visible
// to the client:
//
//	not entitled          → 403 module_not_enabled
//	entitled, under-level → 403 insufficient_module_role, with required + actual
//
// Two codes rather than one because they are two different problems with two
// different fixes: the first is the superadmin's to solve (an entitlement the
// tenant does not have), the second is the tenant admin's (a level this user
// was not given). A single `forbidden` would send every user to the wrong
// person (§5.7).
//
// Every decision here comes from Identity.LevelFor, which was resolved from the
// database on this request (I9). Nothing is cached, so a superadmin toggling an
// entitlement off takes effect on the caller's *next* request with no restart
// and no invalidation step (B5).
//
// The payload rides in `details` rather than at the top level of the envelope.
// §7 writes it inline as shorthand; §9.8 fixes the envelope at exactly three
// keys, and a client that has to know which codes add siblings to `error` and
// `message` cannot parse errors generically.
func RequireModule(module string, min identity.RoleLevel) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := IdentityFrom(c)
		if id == nil {
			// The chain did not run. Not a permission failure — a wiring bug —
			// and it must not become an authorized request.
			return httpx.Unauthenticated(c)
		}

		// A superadmin belongs to no tenant, so no module is enabled for them
		// and the entitlement check below would already refuse. Said explicitly
		// because it is a deliberate guarantee, not a side effect: there is no
		// god-mode data browser, and a compromised platform account reaches the
		// customer list and not the customers' operational data (§5.5, B6).
		if id.IsSuperadmin() {
			return httpx.FailWith(c, fiber.StatusForbidden, "module_not_enabled",
				"Platform administrators have no access to workspace data.",
				map[string]any{"module": module})
		}

		if !id.TenantEntitled(module) {
			return httpx.FailWith(c, fiber.StatusForbidden, "module_not_enabled",
				fmt.Sprintf("This workspace does not have the %s module enabled.", module),
				map[string]any{"module": module})
		}

		if actual := id.LevelFor(module); actual < min {
			return httpx.FailWith(c, fiber.StatusForbidden, "insufficient_module_role",
				"You do not have permission to do this.",
				map[string]any{
					"module":   module,
					"required": min.String(),
					"actual":   actual.String(),
				})
		}

		return c.Next()
	}
}

// RequireSuperadmin gates `/api/admin/*` — the platform plane of §5.7.
//
// It asserts the tenant role rather than trusting the route prefix, because the
// prefix is a routing fact and this is an authorization decision. The pool is
// the second, independent half: those handlers run on erp_admin, which is
// revoked from every tenant business table, so even a mis-wired admin route
// cannot read a tenant's purchase orders (§4.2, A11).
func RequireSuperadmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := IdentityFrom(c)
		if id == nil {
			return httpx.Unauthenticated(c)
		}
		if !id.IsSuperadmin() {
			return httpx.Fail(c, fiber.StatusForbidden, "forbidden",
				"This area is for platform administrators.")
		}
		return c.Next()
	}
}

// RequireTenantAdmin gates `/api/tenant/*` — the tenant plane of §5.7: users
// and their per-module levels.
//
// The tenant role, deliberately, not a module role. "Who inside this company
// may do what" is not the business of any one module, and a user with `admin`
// in Inventory has no claim on the Finance half of someone's role matrix.
func RequireTenantAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := IdentityFrom(c)
		if id == nil {
			return httpx.Unauthenticated(c)
		}
		if !id.IsTenantAdmin() {
			return httpx.Fail(c, fiber.StatusForbidden, "forbidden",
				"Only a workspace administrator can manage users.")
		}
		return c.Next()
	}
}
