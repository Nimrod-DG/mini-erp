# Reference — Authentication (Firebase)

> Phase 2. Also needed by the seed script in Phase 7.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 3. Authentication (Firebase Auth)

### 3.1 Division of responsibility

Firebase Authentication is the **identity provider**. The application database is the **authorization** source of truth.

| Concern | Owner |
|---|---|
| Email/password credentials, password reset, email verification | Firebase Auth |
| Session tokens (ID tokens), refresh | Firebase Auth |
| Which tenant a user belongs to | `users` table |
| What role levels a user holds per module | `user_module_roles` table |

This split is deliberate. Never trust tenant or role information supplied by the client; always resolve it server-side from the database using the verified Firebase UID.

### 3.2 Login flow

1. Frontend calls `signInWithEmailAndPassword()` via the Firebase Web SDK.
2. Frontend obtains an ID token with `getIdToken()`.
3. Every API request sends `Authorization: Bearer <firebase-id-token>`.
4. Backend middleware verifies the token with the Firebase Admin SDK (`VerifyIDToken`), extracting the `uid`.
5. Backend looks up `users WHERE firebase_uid = <uid> AND is_active = true`.
6. That row supplies `tenant_id`; a follow-up query supplies the user's module role levels.
7. Resolved identity goes into the request context.

Password reset and email verification are handled entirely by the Firebase SDK on the frontend (`sendPasswordResetEmail`, `sendEmailVerification`). The backend implements neither. This is the main practical reason for choosing Firebase Auth here.

### 3.3 User provisioning

Users are created **backend-first**, because a Firebase account with no matching `users` row is useless:

1. A tenant admin (or superadmin, for the first user) calls `POST /api/tenant/users`.
2. Backend uses the Firebase Admin SDK to create the Firebase user, receiving a `uid`.
3. Backend inserts the `users` row with that `firebase_uid`, `tenant_id`, and initial module roles — in a transaction.
4. If the database insert fails, delete the Firebase user before returning the error. An orphaned Firebase account is a real bug, not a cosmetic one: it can authenticate successfully, produce a valid token, and then resolve to no `users` row — a state the middleware must treat as `401`, not as a crash.

The reverse orphan — a `users` row whose `firebase_uid` names a deleted Firebase account — is why users are **deactivated, never deleted** (Section 6.9.4), and why the Firebase account is *disabled* rather than removed.

Self-signup is disabled. There is no public registration endpoint.

### 3.4 Custom claims — optional, not authoritative

The backend may mirror `tenant_id` into a Firebase custom claim to let the frontend render the right shell before its first API call. Treat this strictly as a rendering hint.

**The backend must never read authorization data from custom claims.** Claims are refreshed lazily (up to an hour stale) and are trivially observable by the client. Always resolve from the database.

### 3.5 Local development — use a real Firebase project

**Use a real Firebase project from day one, not the Auth Emulator.** Firebase Auth takes minutes to set up, and running the real thing removes an emulator-versus-production behaviour gap that would otherwise be discovered at Phase 9.

This costs nothing on the test side: **tests never touch Firebase in either setup**, because authentication is behind the `Verifier` interface with a hand-written fake (Section 12.4). The emulator was only ever serving manual development.

#### 3.5.1 Two projects, not one

Create **`erp-dev`** and **`erp-prod`**. Both are free.

Firebase Auth has no environment separation *within* a project — one project means one user pool. Without the split, the seed script's `password123` accounts (Section 15) share a pool with anything you later demo or show an employer, and a reseed that deletes users could delete real ones.

| Project | Used by | Contains |
|---|---|---|
| `erp-dev` | Local development, Phases 0–8 | Seeded demo users, freely resettable |
| `erp-prod` | Phase 9 deployment | Only accounts you deliberately create |

Enable **Email/Password** as the only sign-in provider in both. Leave Google OAuth off — it is not in scope (Section 1.3) and adds a consent-screen configuration step.

#### 3.5.2 Configuration

Backend needs a service account key; frontend needs the (public) web config.

```
# backend/.env  — NEVER COMMIT
FIREBASE_PROJECT_ID=erp-dev
GOOGLE_APPLICATION_CREDENTIALS=./secrets/erp-dev-service-account.json
```

```
# frontend/.env.local — safe to commit if you wish; these are identifiers, not secrets
VITE_FIREBASE_API_KEY=...
VITE_FIREBASE_AUTH_DOMAIN=erp-dev.firebaseapp.com
VITE_FIREBASE_PROJECT_ID=erp-dev
```

> **The service account key is a real credential.** It grants full admin control over authentication, including minting tokens for any user. Add `secrets/` and `*-service-account.json` to `.gitignore` in the first commit, before the file exists. At Phase 9 it moves to Secret Manager (Section 2.4.1) and is never baked into the container image.
>
> The frontend `VITE_FIREBASE_*` values are **not** secrets — they are public identifiers, and shipping them in the bundle is expected and safe (Section 2.4.4).

#### 3.5.3 Seeding real accounts idempotently

Section 15 requires an idempotent seed. Against the emulator you could simply wipe state; against a real project, users persist between runs.

**Assign deterministic UIDs** rather than letting Firebase generate them. The Admin SDK accepts an explicit UID on creation, which makes reseeding trivial and keeps `users.firebase_uid` stable across runs:

```go
uid := "seed-" + slug          // e.g. "seed-rina-nusantara"

_, err := client.GetUser(ctx, uid)
if auth.IsUserNotFound(err) {
    _, err = client.CreateUser(ctx, (&auth.UserToCreate{}).
        UID(uid).Email(email).Password("password123").DisplayName(name))
}
```

Reseeding then reuses the same UIDs, so the database rows and Firebase accounts stay aligned without a cleanup step.

The `seed-` prefix also makes demo accounts trivially identifiable — useful if you ever need to purge them, and a clear signal that they are not real users.

#### 3.5.4 If you do want the emulator

It remains a reasonable option for working offline. Set `FIREBASE_AUTH_EMULATOR_HOST=localhost:9099` and the Admin SDK honours it automatically, with no code change. Nothing in this document depends on which you choose.
