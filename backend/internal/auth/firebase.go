package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// FirebaseVerifier is the production Verifier, backed by the Firebase Admin
// SDK. It is the only place in the backend that talks to Firebase.
type FirebaseVerifier struct {
	client *fbauth.Client
}

// NewFirebaseVerifier builds the Admin SDK client for projectID.
//
// Credentials are not passed here: the SDK reads them from
// GOOGLE_APPLICATION_CREDENTIALS (a path to the service-account key), which is
// also how Secret Manager will supply them at Phase 9 without a code change.
// FIREBASE_AUTH_EMULATOR_HOST, if set, is honoured automatically (§3.5.4).
func NewFirebaseVerifier(ctx context.Context, projectID string) (*FirebaseVerifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("auth: init firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: init firebase auth client: %w", err)
	}
	return &FirebaseVerifier{client: client}, nil
}

// Verify checks the token's signature, audience, issuer, and expiry, and
// returns the subject's UID.
func (v *FirebaseVerifier) Verify(ctx context.Context, idToken string) (string, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
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
