package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// Firebase is the production identity provider, backed by the Admin SDK. It is
// the only place in the backend that talks to Firebase.
//
// One type satisfies both Verifier and UserManager, and callers take whichever
// interface they need. The guarantee that authorization never comes from a
// custom claim lives in the *interface* Verify is reached through, not in the
// concrete type: Verifier returns a UID and nothing else, so a claim cannot
// physically reach an authorization decision (§3.4).
type Firebase struct {
	client *fbauth.Client
}

var (
	_ Verifier    = (*Firebase)(nil)
	_ UserManager = (*Firebase)(nil)
)

// NewFirebase builds the Admin SDK client for projectID.
//
// Credentials are not passed here: the SDK reads them from
// GOOGLE_APPLICATION_CREDENTIALS (a path to the service-account key), which is
// also how Secret Manager will supply them at Phase 9 without a code change.
// FIREBASE_AUTH_EMULATOR_HOST, if set, is honoured automatically (§3.5.4).
func NewFirebase(ctx context.Context, projectID string) (*Firebase, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("auth: init firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: init firebase auth client: %w", err)
	}
	return &Firebase{client: client}, nil
}

// Verify checks the token's signature, audience, issuer, and expiry, and
// returns the subject's UID.
func (f *Firebase) Verify(ctx context.Context, idToken string) (string, error) {
	token, err := f.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		// The underlying reason stays in the wrapped error for the log; the
		// caller only ever sees "this token is not acceptable".
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if token.UID == "" {
		return "", ErrInvalidToken
	}
	return token.UID, nil
}

// CreateUser provisions an account with an initial password (§3.3 step 2).
func (f *Firebase) CreateUser(ctx context.Context, email, password, displayName string) (string, error) {
	params := (&fbauth.UserToCreate{}).
		Email(email).
		Password(password).
		DisplayName(displayName).
		// The address is unverified until the person proves it, and nothing in
		// this application branches on the flag. Claiming otherwise would make
		// an admin's typo look like a confirmed address.
		EmailVerified(false)

	u, err := f.client.CreateUser(ctx, params)
	if err != nil {
		if fbauth.IsEmailAlreadyExists(err) {
			return "", fmt.Errorf("%w: %s", ErrEmailExists, email)
		}
		return "", fmt.Errorf("auth: create user: %w", err)
	}
	return u.UID, nil
}

// DeleteUser removes an account — the compensating half of §3.3 step 4.
func (f *Firebase) DeleteUser(ctx context.Context, uid string) error {
	if err := f.client.DeleteUser(ctx, uid); err != nil {
		return fmt.Errorf("auth: delete user %s: %w", uid, err)
	}
	return nil
}

// SetDisabled mirrors `users.is_active` onto the provider account.
func (f *Firebase) SetDisabled(ctx context.Context, uid string, disabled bool) error {
	if _, err := f.client.UpdateUser(ctx, uid, (&fbauth.UserToUpdate{}).Disabled(disabled)); err != nil {
		return fmt.Errorf("auth: set disabled=%t on %s: %w", disabled, uid, err)
	}
	return nil
}
