// Package auth turns a Firebase ID token into a verified UID. That is its
// entire job: it establishes *who* is calling and says nothing about what they
// may do. Authorization is resolved from the database on every request, by
// internal/identity (I9).
package auth

import (
	"context"
	"errors"
)

// ErrInvalidToken is returned for every rejection — expired, malformed, wrong
// audience, revoked. Callers map it to 401 without inspecting the reason: the
// distinction is useful in a log line and never in a response body.
var ErrInvalidToken = errors.New("auth: invalid id token")

// Verifier verifies a Firebase ID token.
//
// It is an interface so that tests never touch the network (§12.4), and it
// returns a UID and nothing else on purpose. Custom claims are refreshed
// lazily — up to an hour stale — and are readable by the client, so they can
// never be an authorization input (§3.4). A type that cannot return them
// cannot be misused to.
type Verifier interface {
	Verify(ctx context.Context, idToken string) (uid string, err error)
}
