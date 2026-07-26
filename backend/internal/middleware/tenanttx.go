package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
)

// errRejected unwinds the transaction after the handler has already written a
// 4xx. See TenantTx.
var errRejected = errors.New("middleware: request rejected, rolling back")

// TenantTx (step 4) opens the transaction every tenant-scoped query runs in,
// with app.current_tenant set from the *resolved* identity — never from a
// parameter, a header, or a token claim (I1, I9).
//
// The whole handler runs inside it, so a request is one transaction: a
// requisition and its lines either both exist or neither does, and a rejected
// request leaves nothing behind.
func TenantTx(pools *db.Pools) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := IdentityFrom(c)
		if id == nil {
			return httpx.Unauthenticated(c)
		}

		// A superadmin belongs to no tenant, so there is no context to set.
		// Their routes use the erp_admin pool, which is revoked from every
		// business table, and skip this step by design (§7). Passing through
		// leaves TxFrom nil rather than opening a transaction with no tenant —
		// which would be a handle on which RLS silently returns zero rows.
		if id.TenantID == uuid.Nil {
			return c.Next()
		}

		g, err := pools.App()
		if err != nil {
			return err
		}

		err = db.WithTenant(c.UserContext(), g, id.TenantID, func(tx *gorm.DB) error {
			c.Locals(txKey, tx)
			if err := c.Next(); err != nil {
				return err
			}
			// httpx.Fail writes the body and returns nil, because returning an
			// error would have Fiber write a second one over it. That leaves
			// the status code as the only signal that the request failed — so
			// a rejection is read from there, not from the returned error.
			// Without this, a handler that validates halfway through a write
			// and then 409s would commit the half it had already done.
			if c.Response().StatusCode() >= fiber.StatusBadRequest {
				return errRejected
			}
			return nil
		})

		if errors.Is(err, errRejected) {
			return nil // response already written; the rollback was the point
		}
		return err
	}
}
