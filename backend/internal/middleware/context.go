// Package middleware is the request chain of §7:
//
//	RequestID → FirebaseAuth → ResolveIdentity → TenantTx → RequireModule → handler
//
// The order is the security property, not a style choice. Nothing downstream of
// ResolveIdentity may read a tenant from anywhere but the Identity it put in
// the request, and nothing downstream of TenantTx may touch tenant data on any
// handle but the transaction it opened (I1).
package middleware

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// Locals keys. Unexported and typed so nothing outside this package can plant
// an identity or a transaction handle of its own.
type ctxKey string

const (
	uidKey      ctxKey = "auth.uid"
	identityKey ctxKey = "auth.identity"
	txKey       ctxKey = "db.tx"
)

// IdentityFrom returns the resolved caller, or nil if the chain has not run.
// A handler registered under the /api group can treat nil as impossible; one
// that wants to be defensive should 401 rather than guess.
func IdentityFrom(c *fiber.Ctx) *identity.Identity {
	id, _ := c.Locals(identityKey).(*identity.Identity)
	return id
}

// TxFrom returns the tenant-scoped transaction opened by TenantTx — the only
// handle a tenant-scoped query may use (I1).
//
// It is nil for a superadmin, who has no tenant to scope to. A handler that
// needs it and finds nil is on the wrong route, not in a recoverable state.
func TxFrom(c *fiber.Ctx) *gorm.DB {
	tx, _ := c.Locals(txKey).(*gorm.DB)
	return tx
}

// UIDFrom returns the verified Firebase UID. Present from FirebaseAuth onward.
func UIDFrom(c *fiber.Ctx) string {
	uid, _ := c.Locals(uidKey).(string)
	return uid
}
