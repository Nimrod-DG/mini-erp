// Package api wires the HTTP surface. It exists as a package rather than as
// code in cmd/api so the middleware tests can exercise the *real* chain: a test
// that assembles its own chain proves nothing about the one that ships.
package api

import (
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/DGosal/mini-erp/backend/internal/auth"
	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/internal/middleware"
)

// Deps is everything the HTTP layer needs from the outside. Verifier and Users
// are interfaces so tests wire fakes and never touch the network (§12.4).
type Deps struct {
	Pools    *db.Pools
	Verifier auth.Verifier
	// Users provisions accounts in the identity provider. Only the two
	// user-creating endpoints touch it.
	Users       auth.UserManager
	CORSOrigins []string
	// Quiet suppresses the request log. Tests set it; nothing else should.
	Quiet bool
}

// server is the handler receiver. Handlers are methods rather than package
// functions because they need the pools and the provider, and threading those
// through a package-level variable would make the test harness and the binary
// share mutable state.
type server struct {
	pools *db.Pools
	users auth.UserManager
}

// New builds the application. Route registration order is load-bearing — see
// the comment on /api/health.
func New(deps Deps) *fiber.App {
	s := &server{pools: deps.Pools, users: deps.Users}

	app := fiber.New(fiber.Config{
		AppName:               "mini-erp",
		DisableStartupMessage: true,
		ErrorHandler:          errorHandler,
	})

	app.Use(recover.New())
	if !deps.Quiet {
		app.Use(logger.New())
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:  strings.Join(deps.CORSOrigins, ","),
		AllowHeaders:  "Origin, Content-Type, Accept, Authorization, " + middleware.HeaderRequestID,
		AllowMethods:  "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		ExposeHeaders: middleware.HeaderRequestID,
	}))
	app.Use(middleware.RequestID())

	// Liveness only: it must not touch the database, or a database blip would
	// make the platform restart a container that is perfectly healthy.
	//
	// Registered BEFORE the /api group. Fiber walks its stack in registration
	// order, so this route matches and returns before the auth chain mounted at
	// the same prefix is ever reached. Move it below the group and health
	// checks start requiring a bearer token.
	app.Get("/api/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Steps 2-4 of §7, global to every other /api route. Step 5 — the
	// per-route authorization gate — is the group middleware below.
	api := app.Group("/api",
		middleware.FirebaseAuth(deps.Verifier),
		middleware.ResolveIdentity(deps.Pools),
		middleware.TenantTx(deps.Pools),
	)

	api.Get("/me", Me)

	// The platform plane (§5.7). TenantTx above passes straight through for a
	// superadmin, who has no tenant to scope to, so these handlers reach the
	// erp_admin pool directly — which is revoked from every tenant business
	// table (§4.2). A tenant user reaching here opens a transaction it does not
	// need and is then refused by RequireSuperadmin, which rolls it back.
	admin := api.Group("/admin", middleware.RequireSuperadmin())
	admin.Get("/modules", s.listModules)
	admin.Get("/tenants", s.listTenants)
	admin.Post("/tenants", s.createTenant)
	admin.Get("/tenants/:id", s.getTenant)
	admin.Patch("/tenants/:id", s.patchTenant)
	admin.Get("/tenants/:id/modules", s.listTenantModules)
	admin.Put("/tenants/:id/modules/:code", s.setTenantModule)

	// The tenant plane (§5.7). Gated on the *tenant* role, not a module role:
	// "who inside this company may do what" is not any one module's business.
	tenant := api.Group("/tenant", middleware.RequireTenantAdmin())
	tenant.Get("/users", s.listUsers)
	tenant.Post("/users", s.createUser)
	tenant.Get("/users/:id", s.getUser)
	tenant.Patch("/users/:id", s.patchUser)
	tenant.Put("/users/:id/modules", s.setUserModules)
	tenant.Put("/users/:id/modules/:code", s.setUserModule)

	// No DELETE on either group, deliberately. Tenants are suspended and users
	// are deactivated (§6.9.4, I5) — the route not existing is the enforcement.

	registerInventory(api, s)

	return app
}

// registerInventory mounts §9.5. The minimum level differs per route, so each
// carries its own RequireModule rather than the group carrying one: read at
// `viewer`, adjust stock at `approver`, change master data at `admin`.
//
// The DELETE routes here are not an exception to I5 — deleteProduct and
// deleteWarehouse issue an UPDATE that sets `deleted_at`. The HTTP verb says
// what the user meant; the SQL says what happens (§6.9.1).
func registerInventory(api fiber.Router, s *server) {
	inv := api.Group("/inventory")

	// One gate instance per route, bound at registration. Sharing a single
	// handler across routes would work — RequireModule holds no state — but the
	// levels below are then no longer visible next to the paths they guard, and
	// this table is the readable copy of §9.5.
	at := func(min identity.RoleLevel) fiber.Handler {
		return middleware.RequireModule(ModuleInventory, min)
	}
	viewer, approver, admin := identity.RoleViewer, identity.RoleApprover, identity.RoleAdmin

	inv.Get("/products", at(viewer), s.listProducts)
	inv.Post("/products", at(admin), s.createProduct)
	inv.Get("/products/:id", at(viewer), s.getProduct)
	inv.Patch("/products/:id", at(admin), s.patchProduct)
	inv.Delete("/products/:id", at(admin), s.deleteProduct)
	inv.Post("/products/:id/restore", at(admin), s.restoreProduct)

	inv.Get("/warehouses", at(viewer), s.listWarehouses)
	inv.Post("/warehouses", at(admin), s.createWarehouse)
	inv.Get("/warehouses/:id", at(viewer), s.getWarehouse)
	inv.Patch("/warehouses/:id", at(admin), s.patchWarehouse)
	inv.Delete("/warehouses/:id", at(admin), s.deleteWarehouse)
	inv.Post("/warehouses/:id/restore", at(admin), s.restoreWarehouse)

	inv.Get("/stock", at(viewer), s.listStock)
	inv.Get("/stock/low", at(viewer), s.listLowStock)
	inv.Get("/ledger", at(viewer), s.listLedger)

	// The only endpoint that appends to the ledger by hand. `approver`, not
	// `user`: correcting stock without a document behind it is a decision, and
	// the person who counted the shelf is not always the person who may say the
	// count was wrong.
	inv.Post("/adjustments", at(approver), s.createAdjustment)
}

// errorHandler renders anything that escapes a handler in the §9.8 envelope.
//
// An error reaching here is a bug or an outage, never a business outcome:
// business outcomes are written by the handler with httpx.Fail. So the body
// carries no detail — the detail goes to the log, keyed by request ID.
func errorHandler(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	var fe *fiber.Error
	if errors.As(err, &fe) {
		status = fe.Code
	}
	if status >= fiber.StatusInternalServerError {
		log.Printf("api: unhandled error on %s %s (request %s): %v",
			c.Method(), c.Path(), c.GetRespHeader(middleware.HeaderRequestID), err)
		return httpx.Fail(c, status, "internal_error", "Something went wrong.")
	}
	return httpx.Fail(c, status, "request_failed", fe.Message)
}
