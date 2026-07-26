package testsupport

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// The SQLSTATEs these tests assert on. Asserting the code rather than the
// message is what makes "A11 raises a permission error, not zero rows" a
// checkable claim.
const (
	CodeUniqueViolation       = "23505"
	CodeForeignKeyViolation   = "23503"
	CodeCheckViolation        = "23514"
	CodeInsufficientPrivilege = "42501"
)

// PGCode returns the SQLSTATE of a PostgreSQL error, or "" if err is nil or
// came from somewhere else.
func PGCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// IsPGCode reports whether err is a PostgreSQL error with the given SQLSTATE.
func IsPGCode(err error, code string) bool { return PGCode(err) == code }
