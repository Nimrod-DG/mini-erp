// Package httpx holds the one response shape every error in this API takes.
package httpx

import "github.com/gofiber/fiber/v2"

// Error is the error envelope of §9.8:
//
//	{ "error": "machine_readable_code", "message": "Human sentence.", "details": {} }
//
// `error` is what the frontend branches on; `message` is what it may show.
// Never put the two the other way round — a client that has to parse prose is
// a client that breaks when the prose is edited.
type Error struct {
	Code    string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Fail writes the envelope with the given status and returns nil, so a handler
// can `return httpx.Fail(...)`.
//
// It returns nil rather than an error deliberately: the response is already
// written, and handing an error back up would have Fiber's error handler write
// a second body over it. TenantTx still rolls the transaction back, because it
// looks at the status code rather than at the returned error.
func Fail(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(Error{Code: code, Message: message})
}

// FailWith is Fail plus the `details` object — the module/required/actual
// payload that RequireModule owes the client (§7).
func FailWith(c *fiber.Ctx, status int, code, message string, details map[string]any) error {
	return c.Status(status).JSON(Error{Code: code, Message: message, Details: details})
}

// Unauthenticated is the single 401 the whole auth chain emits.
//
// Every rejection — absent header, malformed token, expired token, no matching
// user row, deactivated user — produces the same body. The differences are
// recorded server-side; telling an unauthenticated caller which of them applies
// turns the login form into an account-enumeration oracle.
func Unauthenticated(c *fiber.Ctx) error {
	return Fail(c, fiber.StatusUnauthorized, "unauthenticated",
		"Sign in to continue.")
}
