package testsupport

import (
	"context"
	"strings"

	"github.com/DGosal/mini-erp/backend/internal/auth"
)

// FakeVerifier maps a token string straight to a UID (§12.4). Tests then send
// `Authorization: Bearer uid-7` and the chain resolves it.
//
// It lives here, in a package only tests import, rather than beside the real
// Verifier: a type that mints authenticated requests from arbitrary strings has
// no business being linked into the API binary.
//
// The token format is deliberately richer than it needs to be:
//
//	"uid-7"                       → uid "uid-7"
//	"uid-7;tenant=<some-uuid>"    → uid "uid-7", claim ignored entirely
//
// The second form exists so a test can *send* a tenant claim and assert the
// backend ignores it. The Verifier interface cannot return claims, so the
// guarantee is structural — this makes it observable as well.
type FakeVerifier struct {
	// Reject, if set, is the exact token that fails verification. Anything else
	// verifies. Tests that need finer control use Verify's other field.
	Reject string
	// Valid, if non-nil, is the allowlist: a token absent from it is rejected.
	// Leave it nil to accept every token as its own UID.
	Valid map[string]bool
}

var _ auth.Verifier = (*FakeVerifier)(nil)

// Verify returns the UID half of the token and discards everything else.
func (f *FakeVerifier) Verify(_ context.Context, idToken string) (string, error) {
	if idToken == "" || idToken == f.Reject {
		return "", auth.ErrInvalidToken
	}
	if f.Valid != nil && !f.Valid[idToken] {
		return "", auth.ErrInvalidToken
	}
	uid, _, _ := strings.Cut(idToken, ";")
	if uid == "" {
		return "", auth.ErrInvalidToken
	}
	return uid, nil
}
