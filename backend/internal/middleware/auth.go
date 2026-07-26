package middleware

import (
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/DGosal/mini-erp/backend/internal/auth"
	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// FirebaseAuth (step 2) verifies the bearer token and puts the UID in the
// request. It establishes only who is calling — see ResolveIdentity for what
// they may do.
func FirebaseAuth(v auth.Verifier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
		if !ok {
			return httpx.Unauthenticated(c)
		}
		uid, err := v.Verify(c.UserContext(), token)
		if err != nil || uid == "" {
			log.Printf("auth: token rejected (request %s): %v", requestID(c), err)
			return httpx.Unauthenticated(c)
		}
		c.Locals(uidKey, uid)
		return c.Next()
	}
}

// ResolveIdentity (step 3) turns the verified UID into the caller's user row,
// tenant, and module role map, from the database, on every request (I9).
//
// The failure modes are the reason this is not one line. An orphaned Firebase
// account and a deactivated user both authenticate successfully and then
// resolve to nothing; both are 401, and neither is a crash (§3.3).
func ResolveIdentity(pools *db.Pools) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid := UIDFrom(c)
		if uid == "" {
			// FirebaseAuth did not run, or ran and let an empty UID through.
			// Either is a wiring bug; neither may become an authenticated
			// request.
			return httpx.Unauthenticated(c)
		}

		g, err := pools.App()
		if err != nil {
			return err
		}

		id, err := identity.Resolve(c.UserContext(), g, uid)
		switch {
		case errors.Is(err, identity.ErrNoUser):
			log.Printf("auth: uid %s has no user row (request %s)", uid, requestID(c))
			return httpx.Unauthenticated(c)
		case errors.Is(err, identity.ErrInactive):
			log.Printf("auth: uid %s is deactivated (request %s)", uid, requestID(c))
			return httpx.Unauthenticated(c)
		case errors.Is(err, identity.ErrTenantSuspended):
			return httpx.Fail(c, fiber.StatusForbidden, "tenant_suspended",
				"This workspace is suspended. Contact your administrator.")
		case err != nil:
			return err
		}

		c.Locals(identityKey, id)
		return c.Next()
	}
}

// bearerToken pulls the credential out of an Authorization header. The scheme
// is matched case-insensitively, as RFC 7235 requires.
func bearerToken(header string) (string, bool) {
	const scheme = "bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	token := strings.TrimSpace(header[len(scheme):])
	return token, token != ""
}
