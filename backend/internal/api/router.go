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
		AllowOrigins: strings.Join(deps.CORSOrigins, ","),
		// Every header the browser is allowed to send. A header missing here is
		// not a 4xx — the preflight succeeds and the browser then declines to
		// send the real request, so the endpoint looks silently dead. Add a
		// header to this list whenever a request starts sending one.
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, " +
			middleware.HeaderRequestID + ", " + middleware.HeaderIdempotencyKey,
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

	// §9.7. Deliberately NOT behind a RequireModule: its four widgets answer to
	// two different modules, so a caller entitled to one of them must get that
	// one rather than a 403 for the other. The per-widget gate is inside the
	// handler, through the same LevelFor every other check goes through, and a
	// widget the caller cannot read is absent from the response entirely.
	api.Get("/dashboard/summary", s.getDashboardSummary)

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
	registerProcurement(api, s)
	registerFinance(api, s)

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

// registerProcurement mounts §9.4, the same way registerInventory mounts §9.5:
// one RequireModule per route, at the level the spec gives it, so this table is
// the readable copy of that one.
//
// Two of the levels are lower than the action sounds, and both are deliberate:
//
//   - `/cancel` on a requisition is `user`, not `approver`, because §6.9.2 gives
//     the *creator* the right to cancel their own draft, and a creator holds
//     `user`. Which of the two rules applies is decided inside the handler,
//     against the row — a record rule the middleware cannot express.
//   - `PATCH` and `/submit` are `user` and additionally creator-only, for the
//     same reason.
//
// POST /purchase-orders/:id/receipts is §8.4, the one endpoint that writes to
// three modules in one transaction. It sits here with the rest of procurement
// because that is whose event it is; the inventory and finance halves are in
// inventory_receipt.go and finance_journal.go, and both take the same `tx`.
func registerProcurement(api fiber.Router, s *server) {
	proc := api.Group("/procurement")

	at := func(min identity.RoleLevel) fiber.Handler {
		return middleware.RequireModule(ModuleProcurement, min)
	}
	viewer, user := identity.RoleViewer, identity.RoleUser
	approver, admin := identity.RoleApprover, identity.RoleAdmin

	proc.Get("/suppliers", at(viewer), s.listSuppliers)
	proc.Post("/suppliers", at(admin), s.createSupplier)
	proc.Get("/suppliers/:id", at(viewer), s.getSupplier)
	proc.Patch("/suppliers/:id", at(admin), s.patchSupplier)
	proc.Delete("/suppliers/:id", at(admin), s.deleteSupplier)
	proc.Post("/suppliers/:id/restore", at(admin), s.restoreSupplier)

	proc.Get("/requisitions", at(viewer), s.listRequisitions)
	proc.Post("/requisitions", at(user), s.createRequisition)
	proc.Get("/requisitions/:id", at(viewer), s.getRequisition)
	proc.Patch("/requisitions/:id", at(user), s.patchRequisition)
	proc.Post("/requisitions/:id/submit", at(user), s.submitRequisition)
	proc.Post("/requisitions/:id/approve", at(approver), s.approveRequisition)
	proc.Post("/requisitions/:id/reject", at(approver), s.rejectRequisition)
	proc.Post("/requisitions/:id/cancel", at(user), s.cancelRequisition)

	proc.Get("/purchase-orders", at(viewer), s.listPurchaseOrders)
	proc.Get("/purchase-orders/:id", at(viewer), s.getPurchaseOrder)
	proc.Post("/purchase-orders/:id/cancel", at(approver), s.cancelPurchaseOrder)

	// §8.4 — the cross-module transaction. `approver`, because receiving goods
	// commits the company to owing for them: it writes the stock ledger and posts
	// a journal entry, neither of which can be edited afterwards.
	proc.Post("/purchase-orders/:id/receipts", at(approver), s.createGoodsReceipt)
	proc.Get("/goods-receipts", at(viewer), s.listGoodsReceipts)
	proc.Get("/goods-receipts/:id", at(viewer), s.getGoodsReceipt)

	// No DELETE on any of the three documents, deliberately. Requisitions and
	// orders are cancelled, never removed (§6.9.2, I5), and §9.6.1 says in as
	// many words not to add one for symmetry. The route not existing is the
	// enforcement.
}

// registerFinance mounts §9.6 — two routes, both `viewer`, both reads.
//
// The Finance UI is a "coming soon" page (§10.5) and these endpoints still have
// to work: they are how the goods-receipt demo proves a journal entry was
// written, and a stub whose endpoints do not answer proves nothing.
//
// There is no POST here on purpose. Manual journal entry, account CRUD, period
// close, and every report are out of scope (§9.6) — and accounts specifically
// are seeded rather than user-managed, because an editable chart needs rules
// about accounts that already carry postings.
func registerFinance(api fiber.Router, s *server) {
	fin := api.Group("/finance")

	at := func(min identity.RoleLevel) fiber.Handler {
		return middleware.RequireModule(ModuleFinance, min)
	}
	viewer := identity.RoleViewer

	fin.Get("/journal-entries", at(viewer), s.listJournalEntries)
	fin.Get("/accounts", at(viewer), s.listAccounts)
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
