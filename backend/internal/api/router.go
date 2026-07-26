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
	"github.com/DGosal/mini-erp/backend/internal/middleware"
)

// Deps is everything the HTTP layer needs from the outside. The Verifier is an
// interface so tests wire a fake and never touch the network (§12.4).
type Deps struct {
	Pools       *db.Pools
	Verifier    auth.Verifier
	CORSOrigins []string
	// Quiet suppresses the request log. Tests set it; nothing else should.
	Quiet bool
}

// New builds the application. Route registration order is load-bearing — see
// the comment on /api/health.
func New(deps Deps) *fiber.App {
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
		AllowMethods:  "GET, POST, PATCH, DELETE, OPTIONS",
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

	// Steps 2-4 of §7, global to every other /api route. RequireModule (step 5)
	// is per-route and arrives in Phase 3.
	api := app.Group("/api",
		middleware.FirebaseAuth(deps.Verifier),
		middleware.ResolveIdentity(deps.Pools),
		middleware.TenantTx(deps.Pools),
	)

	api.Get("/me", Me)

	return app
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
