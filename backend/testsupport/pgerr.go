package testsupport

import (
	"github.com/DGosal/mini-erp/backend/internal/db"
)

// The SQLSTATEs these tests assert on. Asserting the code rather than the
// message is what makes "A11 raises a permission error, not zero rows" a
// checkable claim.
//
// The three the API itself maps to business outcomes are aliases of the
// production constants, so a test cannot assert one value while a handler
// branches on another.
const (
	CodeUniqueViolation       = db.SQLStateUniqueViolation
	CodeForeignKeyViolation   = db.SQLStateForeignKeyViolation
	CodeCheckViolation        = db.SQLStateCheckViolation
	CodeInsufficientPrivilege = "42501"
)

// PGCode returns the SQLSTATE of a PostgreSQL error, or "" if err is nil or
// came from somewhere else.
func PGCode(err error) string { return db.SQLState(err) }

// IsPGCode reports whether err is a PostgreSQL error with the given SQLSTATE.
func IsPGCode(err error, code string) bool { return PGCode(err) == code }
