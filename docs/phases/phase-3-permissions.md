# Phase 3 — Permissions

**MVP:** yes · **Estimate:** 3h · **Depends on:** Phase 2 green

## Load only these

1. [`../reference/permissions.md`](../reference/permissions.md) — in full. §5.4 and §5.7 are the load-bearing parts.
2. [`../reference/middleware.md`](../reference/middleware.md) — the `RequireModule` row.
3. [`../reference/api.md`](../reference/api.md) — **§9.2 and §9.3 only**.
4. [`../reference/screens.md`](../reference/screens.md) — **§10.6 only**.
5. [`../reference/tests.md`](../reference/tests.md) — **Group B only**.
6. [`../reference/tenancy-and-rls.md`](../reference/tenancy-and-rls.md) — **§4.2.1 only**. `POST /admin/tenants` depends on it.

## Build

1. **`Identity.LevelFor(module)`** — one function, every check goes through it.
   The ordering is the whole design:

   ```
   entitlement missing        → none    (checked FIRST — entitlement is the ceiling)
   tenant_role == admin       → admin   (implicit, no rows needed)
   otherwise                  → user_module_roles[module], missing == none
   ```

2. **`RequireModule(module, minLevel)`** middleware. Two checks, two distinct
   error codes, and the distinction is visible to the client:
   - not entitled → `403 module_not_enabled`
   - entitled, under-levelled → `403 insufficient_module_role` with `required` and `actual`
3. **Superadmin endpoints** (`/api/admin/*`, `erp_admin` pool, skip `TenantTx`
   and `RequireModule`, assert `tenant_role = 'superadmin'`).
   `POST /admin/tenants` creates tenant + first admin + chart of accounts in one
   transaction. `erp_admin` has no grant on `accounts`, deliberately — seed them by
   calling the `seed_tenant_accounts()` `SECURITY DEFINER` function from §4.2.1.
   Do not "solve" this by granting `erp_admin` access to the table; that is the
   surface test A11 exists to keep closed.
4. **Tenant user management** (`/api/tenant/users`, `erp_app` pool). Including
   the bulk matrix endpoint — six dropdowns should be one request and one
   transaction, not six that can half-fail.
5. **The last-admin rule**, under `SELECT … FOR UPDATE`, inside the same
   transaction as the demote/deactivate. Returns `409 last_admin`.
6. Screens: `/admin/tenants`, `/admin/tenants/:id` entitlement matrix,
   `/settings/users`, `/settings/users/:id` per-module role matrix.

## Things the tests exist to protect

- Setting a level to `none` **deletes** the row; never store an explicit `none`.
- A tenant admin gets no `user_module_roles` rows. If rows exist from a previous
  staff period, ignore them while they are an admin and **leave them in place** so
  demotion restores the prior levels.
- Entitlement beats the admin shortcut. An admin of a tenant without Finance
  resolves to `none` in Finance. This is acceptance step 5 and seed user Agus.

## Tests to write

**Group B** (B1–B10). B7, B8, and B10 are the ones that catch real design errors:
the implicit admin with no rows, the entitlement ceiling, and two concurrent
demotions of the last two admins.

## Done when

- [ ] Group B green
- [ ] Toggling a module off in the admin UI makes that tenant's next request
      return `403` — no restart, no cache invalidation step
- [ ] A superadmin cannot reach any tenant business endpoint (B6)
