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

// --------------------------------------------------------------------------
// Soft-delete visibility, shared by every master-data list (§9.0).
// --------------------------------------------------------------------------

// wantsDeleted reads `?includeDeleted=true` and reports separately whether the
// caller asked and whether they may.
//
// Two return values rather than one refusal, because this must not write a
// response: a helper that signalled failure by returning what httpx.Fail
// returned would be signalling with nil, and the caller's `if err != nil` would
// never fire (see parseMatrix). The caller writes the 403.
//
// `admin` is the bar because §9.0 puts the restore workflow there — the recycle
// bin is an administrative view, and a viewer seeing deleted rows in an ordinary
// list would have no way to tell them apart from live ones.
//
// The module is a parameter because the rule is the same in every module and the
// level is not: products, warehouses, and suppliers each answer to their own
// module's `admin`.
func wantsDeleted(c *fiber.Ctx, caller *identity.Identity, module string) (want, allowed bool) {
	want = c.QueryBool("includeDeleted", false)
	return want, caller.LevelFor(module) >= identity.RoleAdmin
}

// refuseDeletedView is the 403 for a viewer asking to see the recycle bin. It
// reuses insufficient_module_role with the level that would have worked, so the
// console can name the dropdown to change (§7) — and so a client cannot tell
// this refusal from the middleware's and does not have to.
func refuseDeletedView(c *fiber.Ctx, caller *identity.Identity, module string) error {
	return httpx.FailWith(c, fiber.StatusForbidden, "insufficient_module_role",
		fmt.Sprintf("Only a %s administrator can see deleted records.", module),
		map[string]any{
			"module":   module,
			"required": identity.RoleAdmin.String(),
			"actual":   caller.LevelFor(module).String(),
		})
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

// forbidden is the §9.8 403 for a record-level refusal: the caller holds the
// level the route requires, and this particular row is still not theirs to act
// on. Segregation of duties and "only the author may edit their draft" are both
// this shape — rules that a role level cannot express, so they cannot live in
// the middleware.
func forbidden(c *fiber.Ctx, format string, args ...any) error {
	return httpx.Fail(c, fiber.StatusForbidden, "forbidden", fmt.Sprintf(format, args...))
}

// stateConflict is §9.8's "409 state conflict": the request was well formed and
// legal, and the document has since moved somewhere the action does not apply.
//
// A distinct code from `in_use`, which means something else — that something
// still references this row — and a client that cannot tell "this requisition
// has already been approved" from "three orders reference this supplier" cannot
// tell the user what to do next. `details.status` is the state the document is
// actually in, so a screen can refresh itself rather than re-asking.
func stateConflict(c *fiber.Ctx, what, current string, allowed ...string) error {
	return httpx.FailWith(c, fiber.StatusConflict, "state_conflict",
		fmt.Sprintf("%s is %s. %s", what, current,
			"Reload the document to see where it is now."),
		map[string]any{"status": current, "allowed": allowed})
}

// unprocessable is §9.8's "422 business-rule violation": the request is
// understood and refused by a rule of the business rather than of the schema.
//
// The code names the rule — `empty_requisition`, `reason_required`,
// `supplier_required` — so the screen can put the message next to the field that
// is missing instead of in a general-purpose banner.
func unprocessable(c *fiber.Ctx, code, format string, args ...any) error {
	return httpx.Fail(c, fiber.StatusUnprocessableEntity, code,
		fmt.Sprintf(format, args...))
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
