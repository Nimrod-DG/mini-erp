package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// The SQLSTATEs the API maps to business outcomes. A constraint firing is not
// always a bug: two admins provisioning the same address race, and the loser
// gets a 409 rather than a 500.
const (
	SQLStateUniqueViolation     = "23505"
	SQLStateForeignKeyViolation = "23503"
	SQLStateCheckViolation      = "23514"
)

// SQLState returns the SQLSTATE of a PostgreSQL error, or "" if err is nil or
// came from somewhere else.
func SQLState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// IsUniqueViolation reports whether err is a duplicate-key error.
func IsUniqueViolation(err error) bool { return SQLState(err) == SQLStateUniqueViolation }

// ConstraintName returns the constraint a PostgreSQL error names, or "".
//
// It is what lets one 23505 become two different messages — a duplicate slug
// and a duplicate email are different mistakes with different fixes, and the
// person who made one should not be told about the other. This is why Phase 1
// named its constraints explicitly rather than letting PostgreSQL generate them.
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}
