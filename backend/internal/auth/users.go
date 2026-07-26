package auth

import (
	"context"
	"errors"
)

// ErrEmailExists means the identity provider already has an account with this
// address. It is a 409, not a 500: two admins provisioning the same new hire is
// ordinary, and so is re-trying a request whose first attempt half-succeeded.
var ErrEmailExists = errors.New("auth: email already registered")

// UserManager is the *write* half of the identity provider, kept separate from
// Verifier on purpose.
//
// Verifier answers "who is calling" and is consulted on every request. This is
// consulted only by the two provisioning endpoints, and it is an interface for
// the same reason: tests must never create real Firebase accounts (§12.4).
//
// There is no method to change a password. Password reset is the user's own
// flow, run entirely client-side against Firebase, and an admin console that
// can set someone's password to a value it knows is a worse system than one
// that cannot.
type UserManager interface {
	// CreateUser provisions an account and returns its UID (§3.3 step 2).
	CreateUser(ctx context.Context, email, password, displayName string) (uid string, err error)

	// DeleteUser removes an account. It exists for exactly one caller: the
	// compensating delete when the `users` insert fails after the provider
	// account was created (§3.3 step 4). It is *not* how users leave this
	// system — they are deactivated (§6.9.4).
	DeleteUser(ctx context.Context, uid string) error

	// SetDisabled disables or re-enables an account, mirroring `users.is_active`.
	// Defence in depth only: the database is the authority on access, and
	// identity resolution already turns a deactivated user away (I9).
	SetDisabled(ctx context.Context, uid string, disabled bool) error
}
