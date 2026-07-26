package db

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WithTenant runs fn inside a transaction that has app.current_tenant set, which
// is what every RLS policy in this system reads. A handler that touches tenant
// data without going through this helper is a bug (I1).
func WithTenant(ctx context.Context, db *gorm.DB, tenantID uuid.UUID,
	fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// set_config(..., true) is transaction-local -- identical in effect to
		// SET LOCAL, but it is a function call, so it accepts a bind parameter.
		// `SET LOCAL app.current_tenant = ?` is a syntax error under prepared
		// statements: PostgreSQL does not allow parameters in SET.
		if err := tx.Exec(
			"SELECT set_config('app.current_tenant', ?, true)",
			tenantID.String()).Error; err != nil {
			return err
		}
		return fn(tx)
	})
}
