# Phase 2 — Auth and identity

**MVP:** yes · **Estimate:** 3h · **Depends on:** Phase 1 green

## Load only these

1. [`../reference/auth.md`](../reference/auth.md) — in full.
2. [`../reference/middleware.md`](../reference/middleware.md) — in full. It is short.
3. [`../reference/tests.md`](../reference/tests.md) — §12.4 (faking Firebase) only.
4. If the first request fails with `permission denied for table users`, the `erp_app` grants from Phase 0's `000_roles.sql` are missing — see §4.2. Nothing else in this phase can work until they are there.

Do not load the schema, permissions, API, or frontend docs beyond the login screen.

## Build

1. `auth.Verifier` **interface** first, then the Firebase Admin SDK implementation
   behind it. The interface is what keeps tests off the network.
2. Middleware, in this exact order — order is the security property:
   `RequestID` → `FirebaseAuth` → `ResolveIdentity` → `TenantTx`.
3. `ResolveIdentity`: verified `uid` → `users WHERE firebase_uid = ? AND is_active = true`
   → `tenant_id` → module role map → request context.
   - No matching row → **401**, not 500. This is the orphaned-Firebase-account
     case and it is reachable in normal operation.
   - `is_active = false` → 401.
   - Suspended tenant → `403 tenant_suspended`.
4. `GET /api/me` — user, tenant, tenant role, and the `{module: level}` map.
5. Frontend: Firebase Web SDK login, `getIdToken()`, `Authorization: Bearer` on
   every request, password reset via `sendPasswordResetEmail`, protected routes.

## Rules that are easy to get wrong here

- **Never read authorization from a custom claim.** Claims are up to an hour
  stale and are readable by the client. `tenant_id` may be mirrored into a claim
  as a *rendering hint* for the shell, and for nothing else.
- **Provisioning is backend-first**: create the Firebase user, then the `users`
  row, in a transaction. If the insert fails, delete the Firebase user before
  returning — an orphaned Firebase account authenticates successfully and then
  resolves to nothing.
- There is no `/auth/login` endpoint and no public registration.

## Do not build

Entitlement or role-level checks — that is `RequireModule`, Phase 3. User
management endpoints — also Phase 3.

## Tests to write

Not a lettered group; these live with the middleware package (§12.4):

- [ ] Invalid token → 401
- [ ] Valid token, no `users` row → **401, not 500**
- [ ] Valid token, `is_active = false` → 401
- [ ] `tenant_id` comes from the database row, never from a token claim or a
      request parameter — assert by sending a conflicting claim

## Done when

- [ ] A user seeded by hand in `erp-dev` logs in and `/api/me` returns their
      tenant and module roles
- [ ] A password reset email actually arrives
- [ ] Invalid and deactivated tokens are rejected
- [ ] Group A still passes — `TenantTx` has not broken isolation
