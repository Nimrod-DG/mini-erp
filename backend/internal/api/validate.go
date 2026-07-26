package api

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/internal/middleware"
)

// tenantScope returns the caller and the tenant-scoped transaction their
// request runs in — the only handle a tenant-scoped query may use (I1).
//
// Both are guaranteed present for any route below the /api chain that a
// superadmin cannot reach: TenantTx opens a transaction for every identity that
// has a tenant, and RequireTenantAdmin and RequireModule both refuse the
// tenantless ones first. So a miss here is a wiring bug — a route gated by
// neither — and it is a loud 500 rather than a query that quietly runs on the
// wrong handle or, worse, on a transaction with no tenant context where RLS
// silently returns nothing.
func tenantScope(c *fiber.Ctx) (*identity.Identity, *gorm.DB, error) {
	id := middleware.IdentityFrom(c)
	tx := middleware.TxFrom(c)
	if id == nil || tx == nil || id.TenantID == uuid.Nil {
		return nil, nil, fiber.NewError(fiber.StatusInternalServerError,
			"a tenant-scoped handler was reached without a tenant-scoped transaction")
	}
	return id, tx, nil
}

// slugPattern is what a URL-safe tenant slug looks like. Anchored, so a slug
// cannot smuggle a slash or a space into a path.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// minPasswordLength is above Firebase's own minimum of six. Eight is not
// security by itself; it is the floor below which an initial password handed to
// a new employee is indefensible.
const minPasswordLength = 8

// malformed is the §9.8 400: the request could not be understood as written.
// Business rules that reject a well-formed request are 409 or 422, not this.
func malformed(c *fiber.Ctx, format string, args ...any) error {
	return httpx.Fail(c, fiber.StatusBadRequest, "malformed", fmt.Sprintf(format, args...))
}

// notFound covers both "no such row" and "a row belonging to another tenant".
//
// The two are deliberately indistinguishable. A tenant admin who probes another
// workspace's user ID must not be able to tell an ID that exists elsewhere from
// one that never existed — that difference is a cross-tenant existence oracle.
func notFound(c *fiber.Ctx, what string) error {
	return httpx.Fail(c, fiber.StatusNotFound, "not_found", what+" was not found.")
}

// pathUUID reads a UUID path parameter. A malformed one is a 404 rather than a
// 400: `/tenant/users/banana` names no user, and telling the caller their UUID
// was unparseable is more detail than a probe deserves.
func pathUUID(c *fiber.Ctx, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// validateTimezone checks that Go can load the zone, because every business
// date this tenant ever renders is computed in it (I7, §2.5.3). An unloadable
// name stored here would not fail here — it would fail later, on a date, in a
// report, for one tenant only.
func validateTimezone(tz string) error {
	if tz == "" {
		return fmt.Errorf("timezone is required")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("timezone %q is not a known IANA zone", tz)
	}
	return nil
}

// validateEmail is deliberately shallow. The address is proved by an email
// arriving, not by a regular expression, and every stricter pattern rejects
// somebody's real address.
func validateEmail(email string) error {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 || strings.ContainsAny(email, " \t\r\n") {
		return fmt.Errorf("email %q is not a valid address", email)
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// derefString reads an optional string field, treating absent as empty. It is
// what lets one request struct serve both create (absent means default) and
// patch (absent means leave alone) without two shapes of the same object.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nonNegative validates an optional NUMERIC field, defaulting to zero.
//
// Pure, and returns a real error rather than writing a response — see the note
// on parseMatrix for why a validating helper must never signal failure by
// returning what httpx.Fail returned.
func nonNegative(value *httpx.Numeric, field string) (httpx.Numeric, error) {
	if value == nil {
		return httpx.Zero, nil
	}
	parsed, err := httpx.ParseNumeric(value.String())
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	if parsed.IsNegative() {
		return "", fmt.Errorf("%s cannot be negative", field)
	}
	return parsed, nil
}

// plural renders "1 product" / "3 products", for refusals that have to name how
// much is in the way. Only regular nouns, deliberately: the day this needs an
// irregular is the day it should be a lookup table rather than a rule.
func plural(n int64, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// normalizeEmail lowercases and trims. Addresses are compared for uniqueness by
// the `users_email_key` unique index, which is case-sensitive — so without this,
// Rina@ and rina@ are two users with one Firebase account between them.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
