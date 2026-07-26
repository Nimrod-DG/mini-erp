# Build progress

**This file is the handoff between sessions.** Read it first, append to it last.
It exists so a new session can start without re-reading the specification.

---

## Project facts

Filled in as they are established, so a later session does not go hunting.

| | |
|---|---|
| Firebase **dev** project ID | `erp-project-b66ce` (display name "erp project", Spark plan) |
| Firebase dev web app | `erp-mini` — appId `1:889259985673:web:1bd9b28a7769be3142e1d8` |
| Firebase dev service account | `backend/secrets/erp-project-b66ce-firebase-adminsdk-fbsvc-47d25660f5.json` (gitignored, never committed) |
| Sign-in provider | Email/Password, enabled and the only one — confirmed 2026-07-26 |
| Firebase **prod** project ID | *not created — Phase 9* |
| Hosting site | `erp-project-b66ce`, auto-linked at app registration, unused until Phase 9 |
| Database host | local Docker (Phase 0–8); host chosen at Phase 9 |

Wherever the docs say `erp-dev`, read `erp-project-b66ce`.
Config values live in [`reference/env-setup.md`](reference/env-setup.md).

---

## Current state

**Phase:** 4 — done
**Next action:** open [`phases/phase-5-procurement.md`](phases/phase-5-procurement.md) in a **new session**

Migration files that now exist (`backend/migrations/`), applied in this order by
`cmd/migrate`:

| File | Contents |
|---|---|
| `000_roles.sql` | roles, timezone pinning, platform-table grants — **not** versioned; re-applied on every `make migrate` |
| `001_platform.up.sql` | `tenants` `modules` `tenant_modules` `users` `user_module_roles`, `touch_updated_at()`, the three `modules` rows |
| `002_tenant_tables.up.sql` | the fourteen tenant tables |
| `003_constraints.up.sql` | composite FKs, CHECKs, line numbering, partial unique indexes, the index set |
| `004_views_triggers.up.sql` | both views, `grl_no_over_receipt`, `jel_balanced`, the two terminal-state triggers |
| `005_rls_grants.up.sql` | RLS enable/force/policy, grants, ledger and superadmin revokes, `seed_tenant_accounts()` |

Backend packages as of Phase 4:

| Package | Contents |
|---|---|
| `internal/auth` | `Verifier` (UID only, never claims) and `UserManager` (create/delete/disable), both satisfied by one `Firebase` value |
| `internal/identity` | `Identity`, `Resolve` — the one database lookup behind I9 — plus `RoleLevel` and **`LevelFor`**, the single function every permission check goes through |
| `internal/middleware` | `RequestID` `FirebaseAuth` `ResolveIdentity` `TenantTx` `RequireModule` `RequireSuperadmin` `RequireTenantAdmin`, and the accessors `IdentityFrom` / `TxFrom` |
| `internal/httpx` | the §9.8 error envelope (`Fail`, `FailWith`, `Unauthenticated`), the §9.0 list contract (`ParseList`, `ListResponse`), and **`Numeric`** — the exact-decimal type every NUMERIC crosses the wire as |
| `internal/db` | pools, `WithTenant`, migrations, and `SQLState` / `IsUniqueViolation` for mapping constraints to business outcomes |
| `internal/api` | `New` (route wiring, so tests drive the real chain), `Me`, the seven `/admin/*` handlers, the six `/tenant/users` handlers, and the sixteen `/inventory/*` handlers |
| `testsupport` | `FakeVerifier`, `FakeUsers`, the shared HTTP `Harness` (used by both test packages), and the tenant/user/inventory fixtures |

Frontend routes as of Phase 4: `/login` `/auth/action` `/` `/admin/tenants`
`/admin/tenants/new` `/admin/tenants/:id` `/settings/users` `/settings/users/new`
`/settings/users/:id` `/inventory/products` `/inventory/products/new`
`/inventory/products/:id` `/inventory/warehouses` `/inventory/stock`
`/inventory/ledger`. All signed-in screens render inside `AppShell`.

### What a module phase inherits

Phases 5–6 each build a module. These already exist — use them rather than
rebuilding them, and note the traps at the bottom. **Phase 4's
`internal/api/inventory_*.go` is the worked example of every row in this table**;
read it before building procurement rather than deriving the shape again.

| Need | Use |
|---|---|
| Gate a route on a module level | `middleware.RequireModule("inventory", identity.RoleApprover)` as a per-route handler. Levels: `identity.RoleNone` `RoleViewer` `RoleUser` `RoleApprover` `RoleAdmin`, ranked |
| A record-level rule inside a handler | `middleware.IdentityFrom(c).LevelFor("inventory")`, compared with `>=`. Never re-derive a level from `ModuleRoles` — that map has no entitlement ceiling and no implicit-admin rule |
| The tenant-scoped transaction | `middleware.TxFrom(c)`. Every tenant query goes on it (I1). Nil only for a superadmin, who cannot reach a gated route |
| A paginated list endpoint (§9.0) | `httpx.ParseList(c, sortable, "defaultSort")` + `httpx.NewListResponse(rows, params, total)`. `sortable` maps API field → SQL column and is the injection guard; `params.OrderBy(tieBreak)`, `.Offset()`, `.Like()` |
| Map a constraint to a business outcome | `db.IsUniqueViolation(err)`, `db.ConstraintName(err)`, `db.SQLState(err)` |
| Reject a request | `httpx.Fail(c, status, code, msg)` or `FailWith(..., details)`. 400 `malformed`, 404 `not_found`, 409 `in_use` / `last_admin`, 422 business rule |
| HTTP tests | `testsupport.NewHarness(t)` → `h.Get/Post/Patch/Put`, `testsupport.Decode[T]`, `AssertStatus`, `AssertErrorCode`. Put them in `internal/api` (package `api_test`) |
| Fixtures | `h.DB.NewTenant(t, name)` gives a tenant with all three modules, master data, and a staff user with `admin` everywhere. Also `NewUser(roles)` `NewAdmin()` `NewUserAs(role, roles)` `NewSuperadmin()` `Deactivate` `Suspend` `SetModule`, and from Phase 4 `NewProduct(name, reorderPoint)` `NewWarehouse(name)` `PostLedger(product, warehouse, qty, entryType)` `SetPOStatus` `NewGoodsReceiptLine`. Quantities are decimal **strings** — no float in the fixtures either (I8) |
| Frontend list screen | `useAsync(key, fetcher)` plus `SkeletonRows` `EmptyState` `ErrorNotice` `Pagination` from `components/ListStates.tsx`. Screens render inside `<AppShell title=… actions=…>` |
| A money or quantity column | `httpx.Numeric` on the Go struct, and `::text` on the column in the SELECT. Never `float64` (I8) |
| Comparing or summing two NUMERICs | Write it in SQL. `Numeric` has no arithmetic on purpose — `belowReorderPoint` and `shortfall` are computed by PostgreSQL and sent as answers |
| Hide a control by level, in the browser | `holds(me.moduleRoles, "inventory", "admin")` from `lib/levels.ts`, and `<RequireModule module=…>` for a whole route. Cosmetic (I12) |
| Render a timestamp | `formatDateTime(iso, me.tenant.timezone)` from `lib/format.ts` — the tenant's zone, never the browser's (I7) |

**Trap 1 — a helper must never signal failure by returning what `httpx.Fail`
returned.** `Fail` returns `nil` deliberately: the body is already written, and
handing an error up would have Fiber write a second one over it. So
`if err := helper(c); err != nil` is *always false*. The refusal's status code
sticks and the success path's own `c.JSON` overwrites the body — a 403 carrying a
success payload. Validating helpers are pure functions returning real errors; see
`parseMatrix` in `internal/api/tenant_users.go`.

**Trap 2 — `users` and `user_module_roles` carry no RLS**, and neither do the
other four platform tables. Any query against them needs an explicit
`tenant_id = <the caller's>`; RLS protects only the fourteen tenant tables. This
does not apply to the inventory, procurement, or finance tables, which are all
RLS-forced.

**Trap 3 — resolving a row by ID must not filter `deleted_at`.** The lists
filter and the writes refuse, but `getProduct` and every join that renders a
historical reference read deleted rows deliberately (§6.9.1). These are raw SQL,
not GORM models, so there is no implicit `WHERE deleted_at IS NULL` to opt out
of with `.Unscoped()` — every filter is written by hand, which means every
*omission* has to be deliberate too. Adding `AND p.deleted_at IS NULL` to the
ledger's product join by reflex deletes last quarter's history from the screen;
a mutation confirmed G1 catches exactly that.

**Both things Phase 4 was told to replace are done:**

- `testsupport/harness.go` still registers the twelve
  `/api/probe/<module>/<level>` routes — Group B asserts against them — but
  Phase 4's own gate test (`TestInventoryRoutesCarryTheLevelsFromTheSpec`) runs
  against the **real** routes and asserts the levels in the §9.5 table. A probe
  route cannot catch a real route registered at the wrong level.
- `AppShell.tsx`'s `modulePaths` now holds `inventory`, and gained a second
  level: a module's own screens appear as sub-items only while you are inside
  it. Phase 5 adds `procurement` the same way.

---

## Decisions taken

Record anything where you deviate from the spec, so a later session knows why the
code and the docs disagree.

The sixteen problems found in the original plan are already fixed in the reference
docs — see [`AUDIT.md`](AUDIT.md) for what changed. Nothing there is outstanding.

| Date | Decision | Why |
|---|---|---|
| 2026-07-26 | Raw colour tokens are named `--ch-*`, not `--color-*` as in `reference/design-system.md` §10.8.1 | The doc's `:root` block and its `@theme` block both define `--color-accent` (also `-success`, `-warning`, `-danger`). Tailwind emits `--color-accent: rgb(var(--color-accent))` into the same `:root`, which is a self-referential cycle: the declaration is invalid and every accent/success/warning/danger utility silently stops resolving. Only the raw side was renamed; the utility names (`text-accent`, `bg-surface`) are unchanged, so no later phase is affected. |
| 2026-07-26 | `000_roles.sql` guards its platform-table grants behind `to_regclass('public.users') IS NOT NULL`, and `make migrate` re-runs the file after migrations | The file runs from `docker-entrypoint-initdb.d` on first boot, when `users`/`tenants`/etc. do not exist yet. Ungrarded, the AUDIT A1 grants abort container init. The whole file is idempotent so re-running is safe, and the grants land as soon as Phase 1 has created the tables. |
| 2026-07-26 | Test A10's query uses `rolsuper`, not `rolsuperuser` | `pg_roles` has no `rolsuperuser` column — the query as written in `phases/phase-0-foundations.md` and `reference/deployment.md` errors out. Worth correcting in the acceptance test before Phase 7 relies on it. |
| 2026-07-26 | The RLS policy reads `NULLIF(current_setting('app.current_tenant', true), '')::uuid`, not the bare `current_setting` of `tenancy-and-rls.md` §4.4 | After a `SET LOCAL` transaction commits, the GUC resets to the **empty string**, not to NULL — and `''::uuid` raises *invalid input syntax*. The template as written turns "a request with no tenant context" into an error on any pooled connection that has served a request before, where §4.3 explicitly wants zero rows. `NULLIF` restores the intended safe failure. Behaviour when the context is set is identical. Test A2 asserts both shapes. |
| 2026-07-26 | `000_roles.sql` now also grants `erp_admin` `SELECT, INSERT, UPDATE` on the five platform tables | The file carried only the AUDIT A1 grants for `erp_app`; the matching `erp_admin` line from `tenancy-and-rls.md` §4.2 was never transcribed. Test A11 caught it — `erp_admin` could not read `tenants`, which is the one thing the superadmin console exists to do. Phase 3 would have failed on its first request. |
| 2026-07-26 | `make migrate` is `go run ./cmd/migrate` alone; the binary re-applies `000_roles.sql` itself | The old second line, `docker compose exec -T postgres psql … -f /docker-entrypoint-initdb.d/000_roles.sql`, cannot work on Windows: Git Bash rewrites the container-absolute path to `C:/Program Files/docker-entrypoint-initdb.d/…`. The roles file is already embedded for the test harness, so applying it in Go removes the psql dependency, the mount dependency, and the platform difference at once. |
| 2026-07-26 | `purchase_order_lines` has one `UNIQUE (id, tenant_id)`, named `pol_id_tenant_uq` | `constraints-and-indexes.md` §6.10.1 calls it `purchase_order_lines_id_tenant_uq` and §6.10.9 calls it `pol_id_tenant_uq`; they are the same constraint. Declaring both fails. Kept §6.10.9's name, since that is the block AUDIT B4 rewrote. |
| 2026-07-26 | `config.Load` pins `time.Local = time.UTC` (exported as `config.PinUTC`) | Decision 003 §2.5.2 requires the Go process to run in UTC and relies on `TZ=UTC` in the container. A developer's laptop has no such variable, so the pin has to exist in code for J2 to mean anything anywhere but production. |
| 2026-07-26 | `seed_tenant_accounts()` (§4.2.1) was built in Phase 1, not deferred to Phase 3, and it sets `app.current_tenant` internally | It is DDL, it belongs beside the `accounts` table, and the test fixture uses it — so every tenant fixture exercises it. The `set_config` call is not in the spec's version and is required: `accounts` is `FORCE` RLS, and **`FORCE` applies to the owner**, so even a `SECURITY DEFINER` function owned by `erp_migrate` needs tenant context. The previous value is restored before returning. |
| 2026-07-26 | `TenantTx` rolls the transaction back whenever the response status is ≥ 400, not only when the handler returns an error | `httpx.Fail` writes the body and returns `nil` — it must, or Fiber's error handler writes a second body over it. That leaves the returned error unable to distinguish "committed successfully" from "rejected with a 409", so a handler that writes half a document and *then* rejects would commit the half. Reading the status code instead closes that off before Phase 4 writes anything. A sentinel error unwinds the transaction and is swallowed above. |
| 2026-07-26 | `config.Load` now requires `FIREBASE_PROJECT_ID` | Without it the Admin SDK cannot check a token's audience, so every authenticated request 500s. Failing at boot is the only useful time to find out. |
| 2026-07-26 | `/api/me`'s `moduleRoles` is the **intersection** of the user's levels and the tenant's enabled modules | `business-logic.md` §—: "`/api/me` returns the enabled module list with the user's level in each". A role level in a module the tenant is not entitled to is not access, and the nav is driven straight off this map (§10.1, FE1). This is a response shape, not an entitlement check — `RequireModule` is still Phase 3. |
| 2026-07-26 | `TenantTx` passes through without opening a transaction when the identity has no tenant | Superadmins have `tenant_id NULL`, which `users_superadmin_has_no_tenant` makes biconditional with `tenant_role = 'superadmin'`. §7 has their routes skip step 4 anyway. Opening a transaction with *no* tenant context would be worse than none: it is a live handle on which RLS silently returns zero rows. `TxFrom` returns nil instead, which fails loudly. |
| 2026-07-26 | Identity resolution fetches the user row **without** an `is_active` filter and checks the flag in Go, where the phase brief writes `WHERE firebase_uid = ? AND is_active = true` | Identical outcome — both are 401 — but an orphaned Firebase account and a deactivated employee are different operational events, and only this shape can tell them apart in a log line. |
| 2026-07-26 | Route wiring lives in `internal/api.New`, not in `cmd/api/main.go` | So the middleware tests can drive the *real* chain with only the Verifier faked. A test that assembles its own middleware stack asserts things about a stack that does not ship. |
| 2026-07-26 | `index.html` carries a two-line inline `<style>` duplicating the canvas colour | The pre-paint script sets the `.dark` class, but in dev Vite injects the stylesheet via JS *after* first paint, so the class alone still yields a white first frame. Canvas only; every other colour comes from the tokens. |
| 2026-07-26 | `000_roles.sql` now also grants `erp_app` **`DELETE` on `user_module_roles`**, and on nothing else anywhere | §5.3 requires setting a level to `none` to *delete* the row — the CHECK on `role_level` refuses to store `'none'` — but the file granted only `SELECT, INSERT, UPDATE`, so every revocation would have been a `42501`. This is the one exception to I5, and it is narrow on purpose: a grant row is a present-tense permission with no history worth preserving, unlike the documents and ledgers I5 protects. Deliberately not extended to `users` (deactivated, never deleted) or `tenant_modules` (toggled, never deleted). |
| 2026-07-26 | `RequireModule`'s `module`/`required`/`actual` payload rides in the envelope's `details` object, not as siblings of `error` | `middleware.md` §7 writes it inline as `{"error":"module_not_enabled","module":"finance"}`, but §9.8 fixes the envelope at exactly three keys. A client that has to know which error codes add extra top-level keys cannot parse errors generically. Phase 2's `httpx.FailWith` had already committed to `details` for precisely this payload. |
| 2026-07-26 | `Identity` gained `EnabledModules` alongside `ModuleRoles` | `ModuleRoles` is the *intersection* of the user's levels and the tenant's entitlements (Phase 2 decision), and an intersection cannot distinguish "this tenant never bought Finance" from "this user was given nothing in Finance" — which is exactly the `module_not_enabled` / `insufficient_module_role` split. A tenant admin makes it acute: they correctly have no role rows at all, so their `ModuleRoles` is empty everywhere. Resolution is still two queries, not three: the entitlement set is the driving table and role level a `LEFT JOIN` onto it. |
| 2026-07-26 | `auth.FirebaseVerifier` became `auth.Firebase`, satisfying both `Verifier` and `UserManager` from one Admin SDK client | Two constructors meant two `firebase.NewApp` calls reading the same key off disk at boot. The guarantee that authorization never comes from a custom claim lives in the *interface* `Verify` is reached through — `Verifier` returns a UID and nothing else — not in the concrete type, so nothing is weakened. `main` hands the same value over as two narrow interfaces. |
| 2026-07-26 | Both user-creating endpoints take an **initial password** rather than emailing an invite link | The nicer flow is a provisioning email whose link lands on `/auth/action`, but Phase 2 established that `notification.sendEmail.callbackUri` **cannot currently be set on this Firebase project** — every change returns `400 EMAIL_TEMPLATE_UPDATE_NOT_ALLOWED`. An invite flow would therefore be unverifiable locally and would land users on Firebase's hosted page. The admin sets a password, the user changes it via *Forgot your password?*, and there is no method on `UserManager` to read one back. Revisit at Phase 9 once `callbackUri` is settable. |
| 2026-07-26 | `PUT /tenant/users/:id/modules` leaves modules the payload does not name **unchanged**, rather than treating absence as `none` | §9.3 calls it "bulk set the whole matrix", which reads as replace-all. But a client bug that drops a field would then silently revoke access, and the screen this endpoint exists for always sends every module anyway — so the destructive reading buys nothing and costs a whole class of incident. `none` still deletes, explicitly. |
| 2026-07-26 | `POST /admin/tenants` with `modules` absent enables **all** available modules; a row is written to `tenant_modules` for every catalogue module either way | A workspace created with nothing enabled looks broken to the admin who signs into it first. The create screen always sends the list explicitly, so the default only affects API callers. Writing a row per module (enabled or not) makes the entitlement matrix complete from the start, so `PUT .../modules/:code` is always an update. |
| 2026-07-26 | Handler helpers never signal failure by returning what `httpx.Fail` returned; the validating ones (`parseMatrix`, `resolveModules`) are pure and return real errors | `Fail` returns **nil** by design — Phase 2's reason: returning an error would have Fiber write a second body over the one just written. So `if err := helper(c); err != nil` is always false, the refusal's status code sticks, and the success path's own `c.JSON` overwrites the body. This shipped briefly and B8 caught it: a `403` whose body was a user-detail object. |
| 2026-07-26 | A superadmin hitting a `RequireModule`-gated route gets `403 module_not_enabled` | No code in the §3 contract describes "you are a platform administrator", and inventing one for a case that cannot arise from the UI is worse than reusing the honest answer: a superadmin has no tenant, so no module is enabled for them. `RequireSuperadmin` / `RequireTenantAdmin` use `forbidden`, which §9.8 does name. |
| 2026-07-26 | The user responses carry **`effectiveRoles`** (what `LevelFor` resolves) beside `moduleRoles` (what is stored) | The two differ for every tenant admin — stored empty, effective `admin` everywhere entitled — so a screen rendering the stored map shows "no access" next to the person who has all of it. Computing it server-side, through the same `LevelFor`, is what stops §5.4 being reimplemented in TypeScript where it would drift. |
| 2026-07-26 | Group B lives in a new `internal/api` test package, the HTTP harness moved into `testsupport`, and `make test` now passes `-p 1` | Three test packages now start a PostgreSQL container each; Phase 2 already saw one fail to come up with two in parallel. `-p 1` serialises packages and costs nothing measurable. The harness moved rather than being copied because two copies of a permission-test harness will drift, and the shipped chain is the thing under test. |
| 2026-07-26 | Added `/settings/users/new`, which §10.6 does not list | `POST /api/tenant/users` exists in §9.3 and the user list needs an "Add user" target. §10.6 lists `/admin/tenants/new` and omits its tenant-plane counterpart; this reads as an oversight rather than a decision. |
| 2026-07-26 | Money and quantities cross the wire as **`httpx.Numeric`**, an exact-decimal string type, and every column is selected `::text` | I8 forbids float for these values, and the prohibition does not stop at the column: a `NUMERIC(18,4)` scanned into a `float64` has lost whatever it is going to lose before any arithmetic happens. `Numeric` holds the digits PostgreSQL produced, hands the same digits back on the way in, and marshals them as a JSON *number* so the frontend needs no decimal library. It has **no arithmetic methods**, deliberately — `belowReorderPoint`, `shortfall`, and every sum are computed in SQL, where both operands are still NUMERIC. A `Compare` method here would be the first step towards deciding a business rule in float64. `numeric_test.go` round-trips 2^53+1 with a fraction, which a single implicit conversion would visibly change. |
| 2026-07-26 | `?includeDeleted=true` is refused *inside* the handler with `insufficient_module_role`, not by a separate route | §9.0 makes the recycle bin module-`admin` only, but the lists it applies to are `viewer` routes — the level varies by parameter, not by path, so `RequireModule` cannot express it. The refusal carries `required`/`actual` exactly as the middleware's does, so a client cannot tell the two apart and does not have to. |
| 2026-07-26 | `GET /inventory/products/:id` resolves a **soft-deleted** product for any `viewer`, rather than being admin-only like the deleted *list* | This is §6.9.1's "still resolvable by foreign key" case, not the recycle-bin case. Every ledger row links here, and a 404 would make last quarter's movements unreadable — the exact failure soft delete exists to prevent. `deletedAt` rides in the payload so the screen says so and offers Restore instead of the editing controls. The lists still hide it, which is where the policy actually bites. |
| 2026-07-26 | Restoring a row that is **not** deleted is a 200 no-op, not an error | The alternative needs an error code for "it was already fine", and §9.8's list has none that fits — `in_use` means something else and inventing one would put a code in the contract that no client can act on. Two admins clicking Restore on the same row should not produce a failure for the slower one. |
| 2026-07-26 | A warehouse's emptiness is asked **per product** (`EXISTS … WHERE qty_on_hand <> 0`), never as one total | A warehouse holding +5 of one product and −5 of another sums to zero and is emphatically not empty. The one-total version passes a single-product test and strands both balances somewhere nobody can see; G5 uses two products precisely so the mutation fails, and it does. |
| 2026-07-26 | A manual adjustment is refused against a **deleted** product but allowed against a **discontinued** one (`is_active = false`) | The two columns are two questions (§6.9.1). Writing off the last of a discontinued product is the ordinary reason to reach for this endpoint, and refusing it would leave that stock on the books permanently. A deleted product is refused because this is a picker, not a historical reference. |
| 2026-07-26 | Low stock requires `reorder_point > 0`, and the comparison is strict `<` | Every product defaults to a reorder point of zero, so without the guard every product with no stock is "low" and the §10.2 widget is noise on day one. Strict `<` means a product sitting exactly *at* its point is not yet below it. F2 names both cases, plus the one that matters most — a product with no ledger rows at all, whose `COALESCE`d balance of zero is genuinely low. |
| 2026-07-26 | The adjustment's `unit_cost` defaults to the product's `standard_cost`, not to zero | The column defaults to 0 and nothing would have complained. But an adjustment valued at nothing understates inventory value the moment Finance reads it, and the person counting a shelf has no cost to hand. An explicit `unitCost` still overrides. |
| 2026-07-26 | `tenantScope` moved from `tenant_users.go` to `validate.go` and its message generalised | Second concrete use case, which is the bar §4 sets for abstracting. Sixteen inventory handlers need the same identity-plus-transaction pair, and a copy would drift from the one that carries the "nil means a route gated by neither, so fail loudly" reasoning. |
| 2026-07-26 | The G4-shaped in-use test was written **now** rather than deferred to Phase 5, against products | The phase brief says the product/open-PO-line check "has no data to act on until Phase 5 — write it now, test it in Phase 5". That is over-cautious: `purchase_orders` and `purchase_order_lines` exist since Phase 1 and the fixtures already build them, so the check is fully testable today. G4 *proper* — suppliers — still waits for its endpoint in Phase 5. |
| 2026-07-26 | Added `/inventory/warehouses`, which §10.4 does not list | §9.6.1 is explicit that every entity with CRUD endpoints needs a working UI, and that "a half-built entity — creatable but not editable — is the most common way a demo falls over". Warehouses have the full endpoint set from §9.5, the stock grid and the adjustment form both pick from them, and without this screen the only way to create one is curl. Kept to a flat list with inline editing: a warehouse is a code and a name, and a detail route would be two fields on an empty page. |
| 2026-07-26 | **A deleted product's balance is shown in the stock grid**, marked, rather than hidden. Reversed during the browser walkthrough | The first version hid it, reading §6.9.1's "hidden everywhere by default" literally. That was wrong, and the walkthrough found it in about a minute: the warehouse list said "1 product, 30 on hand" and refused deletion (G5) over stock the grid showed nowhere. A balance is not the product *record* — it is a quantity of goods in a place, and the goods do not leave the shelf when somebody tidies the catalogue. Three things now agree by construction: the ledger shows a deleted product's movements, the grid shows their sum, and the warehouse row counts it and refuses over it. `TestDeletedProductsStockStaysVisibleEverywhere` asserts all three in one test so they cannot drift apart again. Deleted *warehouses* are still hidden, because a warehouse can only be deleted once it is empty — with an `OR qty_on_hand <> 0` escape hatch so stranded stock could never be invisible. |
| 2026-07-26 | Action outcomes are **toasts**; load failures stay inline as `ErrorNotice` | User preference, and the split is principled rather than cosmetic. A failed *load* belongs where the data would have been — a toast there leaves an empty screen with no explanation once it fades. A refused or successful *action* belongs in a toast: the user pressed a button and is looking for the answer, and putting a row-level refusal inline shifts the table and lands the message where their eye is not. Refusals last 12s and are dismissible; confirmations 4s. The bigger win was incidental: Save changes previously gave **no feedback at all** — it refetched and looked identical to having done nothing. |
| 2026-07-26 | A deleted product's Status reads "Deleted", not "Active" | `is_active` really is still true, and the two columns really are different questions — but "Status: Active" next to a banner saying the product is deleted reads as a contradiction, and a reader cannot be expected to know there are two columns behind one word. Found in the walkthrough. |
| 2026-07-26 | Cross-tenant and malformed-ID misses are `404`, never `403` | An admin probing another workspace's user ID must not be able to tell an ID that exists elsewhere from one that never existed — that difference is a cross-tenant existence oracle. `/tenant/users/banana` is a 404 for the same reason. |

---

## Log

<!-- newest at the bottom; one block per session -->

<!--
## Phase N — YYYY-MM-DD

**Done:** what now works, in one or two sentences.

**Tests green:** A1-A11, I1-I8, J1-J4

**Deviations from spec:** none — or: what, where, and why.

**TODO(post-mvp) markers added:**
- internal/procurement/receipt.go:142 — audit gr.posted

**Known broken / left half-done:** nothing — or be specific.

**Next:** the single next action.
-->

## Phase 0 — 2026-07-26

**Done:** `make dev` brings up Postgres 17 in Docker, the Fiber API on `:8080`,
and the Vite frontend on `:5173` together. Three database roles exist with the
timezone pinned to each; `db.WithTenant` is in place and tested; the light/dark/
system theme is wired through semantic tokens before the first real component
exists.

**Tests green:**
- `internal/db` — `TestWithTenantSetsCurrentTenant`, `TestWithTenantDoesNotLeakAfterCommit`
- Phase 0 "done when" checklist, all seven items:
  - `make dev` starts all three; `GET /api/health` → `200 {"status":"ok"}`; `GET :5173/` → 200
  - `rolbypassrls`/`rolsuper` false for both `erp_app` and `erp_admin` (I3)
  - `git check-ignore` matches `backend/secrets/`, `backend/.env`, `frontend/.env.local`; no service-account key in `git log`
  - both `.env.example` files committed
  - `backend/go.mod` and `frontend/package.json` exist; neither at the repository root (I13)

**Deviations from spec:** four, all recorded in *Decisions taken* above —
`--ch-*` token naming, the guarded grants in `000_roles.sql`, `rolsuper` vs
`rolsuperuser`, and the inline canvas colour in `index.html`.

**TODO(post-mvp) markers added:** none.

**Known broken / left half-done:**
- `make migrate` and `make seed` reference `backend/cmd/migrate` and
  `backend/cmd/seed`, which arrive in Phase 1. The targets fail until then, by
  design.
- Theme has no white flash on reload: verified structurally (blocking script +
  inline canvas rule in `<head>`, both served), not by eye. Worth one look in a
  browser.
- `npm audit` reports two high-severity advisories in `react-router` 7.18.1,
  both scoped to **RSC mode**, which this SPA does not use. The only offered
  fix is a downgrade to 7.11.0. Left as-is; recheck when a forward fix ships.
- GNU Make was installed on this machine during the phase
  (`winget install ezwinports.make`) — it is not shipped with Git for Windows,
  and a new shell is needed for it to be on `PATH`.

**Next:** open [`phases/phase-1-schema-rls.md`](phases/phase-1-schema-rls.md) in
a new session.

## Phase 1 — 2026-07-26

**Done:** the whole schema exists and enforces itself. Five migrations create the
five platform tables and the fourteen tenant tables, every tenant table has RLS
`ENABLE`d **and** `FORCE`d with a `USING` + `WITH CHECK` policy, both views are
`security_invoker`, every tenant-scoped child reference carries `tenant_id` in a
composite FK, and the four triggers are in place. `cmd/migrate` applies them from
an embedded FS and re-applies `000_roles.sql` afterwards. `testsupport/` starts a
real PostgreSQL 17 per test process with testcontainers, migrates it exactly the
way `make migrate` does, and hands back one connection per role — tests run as
`erp_app`, never as the owner.

**Tests green:** A1–A11, I1–I8, J1–J4 — 26 tests in `internal/db`, all passing
(`go test ./... -count=1`). Plus `TestWithTenantSetsCurrentTenant`,
`TestWithTenantClearsContextOnRollback`, and `TestSeedTenantAccountsCrossesTheRevoke`.

Three of them were verified to actually bite rather than merely pass:

- **A7** — a copy of `stock_balances` without `security_invoker` returns **2**
  rows where the real view returns **1**, as `erp_app` with tenant A's context.
  The leak the option prevents is real, and the test detects it.
- **A11** — caught the missing `erp_admin` platform grants (see *Decisions*).
- **I4** — `erp_app` gets `42501` on `DELETE FROM journal_entry_lines`, the owner
  gets `23514` at commit. Grant and trigger are independently doing their jobs.

**Deviations from spec:** six, all recorded in *Decisions taken* above — the
`NULLIF` in the policy, the `erp_admin` platform grants, the `make migrate`
target, the `pol_id_tenant_uq` name collision, the `time.Local` pin, and building
`seed_tenant_accounts()` now with an added `set_config` call.

Two smaller additions, both authorised by the phase brief's "CHECK constraints on
every status/type column" and neither a departure:

- `document_sequences.doc_type` has a CHECK for `PR|PO|GR|JE`.
- The three `modules` rows are inserted by `001_platform.up.sql`
  (`ON CONFLICT DO NOTHING`). They are the enumeration the naming contract
  fixes, not seed data — every `tenant_modules` row needs a parent.

**TODO(post-mvp) markers added:** none.

**Known broken / left half-done:**
- `go test -race` cannot run on this machine: it requires cgo and there is no C
  toolchain on `PATH`. CI (`ubuntu-latest`) runs it; worth a check there.
- The over-receipt trigger `grl_no_over_receipt` is created and its shape is
  exercised by I7's fixtures, but its *rejection* path is Phase 5's H6. It is
  untested until then, deliberately.
- Composite FK names are explicit (`grl_po_line_product_fk`, …) where the
  reference doc uses anonymous `ADD FOREIGN KEY`. Same constraints, nameable in
  error handling later.

**Next:** open [`phases/phase-2-auth.md`](phases/phase-2-auth.md) in a new session.

## Phase 2 — 2026-07-26

**Done:** a request now carries an identity. `auth.Verifier` is an interface
returning a UID *and nothing else* — a type that cannot return a custom claim
cannot be misused to read authorization from one — with the Firebase Admin SDK
behind it and a fake in `testsupport` for tests. The §7 chain is wired in
`internal/api.New` in order: `RequestID` → `FirebaseAuth` → `ResolveIdentity` →
`TenantTx`, global to `/api/*` except `/api/health`, which is registered before
the group so liveness never needs a token. `identity.Resolve` turns the UID into
a user row, tenant, and module role map on every request, from the database.
`GET /api/me` renders it. The frontend signs in with the Firebase Web SDK,
attaches `Authorization: Bearer` via one `apiFetch`, guards routes, and offers
`sendPasswordResetEmail`. `PasswordField` carries the reveal toggle (44px
target, `type="button"` so revealing cannot submit the form).

Sign-in failures collapse to one sentence for the user — a form that separates
"no such account" from "wrong password" is an enumeration oracle — but the raw
Firebase code goes to `console.warn` always, and renders under the banner behind
`import.meta.env.DEV`, which Vite replaces statically so the branch is dropped
from the production bundle entirely.

**Tests green:** 13 in `internal/middleware`, plus Group A–J unchanged in
`internal/db` — 39 tests total, `go test ./... -count=1` clean, `go vet` clean.
The four the phase brief asks for, and more:

- invalid token → 401; absent, malformed, and non-`Bearer` headers → 401
- valid token, **no `users` row** → 401, explicitly asserted not to be a 5xx
- valid token, `is_active = false` → 401, asserted with the *same token* that
  worked a line earlier: authorization is a database fact, not a token fact
- suspended tenant → `403 tenant_suspended`
- `tenantId` in the query, `X-Tenant-Id` and `X-Tenant` headers, and a
  `tenant=<uuid>` claim inside the token, all naming someone else's tenant —
  ignored on both `/api/me` **and** a tenant-scoped route
- `TenantTx` scopes queries to the caller's tenant, asserted from two tenants,
  because a single-tenant test cannot detect an isolation failure
- superadmin → `tenant: null`, empty `moduleRoles`
- `moduleRoles` omits a module the tenant's entitlement was revoked for

Two mutations confirmed the isolation tests bite rather than merely pass:

- **`TenantTx` reading `tenantId` from the query when present** → the
  conflicting-claim test failed with `warehouses = [WH-3], want [WH-1]`. This
  mutation *survived* the first version of that test, which asserted only on
  `/api/me` — a route that never enters the transaction. Catching it required
  extending the test to a tenant-scoped route.
- **a plain `Transaction` with no `app.current_tenant`** → both isolation tests
  failed with empty result sets.

**Live check against `erp-project-b66ce`** — a real ID token, minted by
exchanging an Admin-SDK custom token, not a fake:

| Case | Result |
|---|---|
| `/api/health`, no token | `200 {"status":"ok"}` |
| `/api/me`, real token | `200`, correct tenant, `{"inventory":"viewer","procurement":"approver"}`, `X-Request-Id` echoed |
| no token / garbage token | `401 unauthenticated` |
| same token, `is_active = false` | `401 unauthenticated` |
| same token, tenant suspended | `403 tenant_suspended` |
| same token, `users` row deleted | `401 unauthenticated` |

`client.PasswordResetLink` returns a valid
`erp-project-b66ce.firebaseapp.com/__/auth/action?mode=resetPassword&oobCode=…`,
so the provider and the reset flow are configured.

**Browser check, by hand:** signed in at `/login`, landed on the dashboard,
which rendered the tenant, tenant role, timezone, and both module badges from
`/api/me`. Protected redirect and sign-out work.

**Dev accounts** in the local database, both in tenant *Nusantara Trading*
(`11111111-…`). Local Docker + `erp-project-b66ce` only; neither exists anywhere
else, and Phase 7's `seed-` prefixed accounts will not collide with either UID:

| Email | Firebase UID | Password | Roles |
|---|---|---|---|
| `dgjy2019@gmail.com` | `dev-dgjy2019` | `password123` | tenant `admin`; all three modules `admin` |
| `phase2-check@example.test` | `phase2-check` | `password123` | tenant `admin`; procurement `approver`, inventory `viewer` |

The first has a real, deliverable address specifically so the password-reset
email can be observed arriving.

**Deviations from spec:** six, all recorded in *Decisions taken* above — the
status-code rollback in `TenantTx`, `FIREBASE_PROJECT_ID` becoming required,
`moduleRoles` being an intersection, `TenantTx` passing through for tenantless
identities, checking `is_active` in Go rather than in the `WHERE`, and route
wiring living in `internal/api` rather than in `main`.

**Password reset is handled in-app, not on Firebase's hosted page.**
`/auth/action` (public, outside `ProtectedRoute` — someone resetting a password
is by definition signed out) reads `?mode=resetPassword&oobCode=…`, calls
`verifyPasswordResetCode` *before* rendering the form so an expired link is
reported before the user types a new password twice, then `confirmPasswordReset`,
then signs out locally because Firebase has just revoked the account's refresh
tokens. Modelled on `D:\Work\deus-null`'s `ResetPasswordPage`.

**The page is not yet reachable from a real email, and that is a Google-side
block, not a gap in this code.** The `url` passed to `sendPasswordResetEmail` is
only the post-reset *continue* target. The page the emailed link opens is
`notification.sendEmail.callbackUri`, still at its default:

```
https://erp-project-b66ce.firebaseapp.com/__/auth/action
```

**`callbackUri` cannot currently be changed on this project — by console or by
API.** The console reports "An error occurred updating action URL";
`PATCH admin/v2/projects/<id>/config` reports
`400 EMAIL_TEMPLATE_UPDATE_NOT_ALLOWED`. Measured, not assumed:

| Attempted value | Result |
|---|---|
| the existing value, written back unchanged | **200** |
| `http://localhost:5173/auth/action` | 400 |
| `https://localhost:5173/auth/action` | 400 |
| `https://erp-project-b66ce.web.app/auth/action` | 400 |
| `https://erp-project-b66ce.firebaseapp.com/auth/action` | 400 |
| `https://example.com/auth/action` | 400 |

So it is neither the scheme nor the domain: a no-op write passes and *every*
change fails. `localhost` is already an authorized domain, and other template
fields (`senderDisplayName`) do accept edits, so the restriction is specific to
this field.

**At Phase 9**, set `callbackUri` to `https://<deployed-origin>/auth/action` and
the emailed link opens this app's own page directly. No code change: the page
reads `mode` and `oobCode` from the query string and is origin-independent, and
the continue URL already derives from `window.location.origin`. Three caveats:

- `/auth/action` is a client-side route, so the host **must rewrite unknown
  paths to `index.html`** (a Firebase Hosting `rewrites` rule, or the nginx /
  Cloud Run equivalent). Without it the emailed link 404s before React runs, and
  it looks like a broken reset rather than a hosting config.
- The value is global to the project **and applies to every email template** —
  it cannot be localhost and production at once. A second reason `erp-dev` and
  `erp-prod` are separate projects (§3.5.1), beyond the shared user pool.
- Whether the field is settable at all on a fresh project is **unverified**.
  Possibly an Identity Platform upgrade or a deployed Hosting site; both are
  guesses, and one guess about this field has already proved wrong.

**Testing it meanwhile:** open the link from the email, then in the address bar
replace the origin and path `https://erp-project-b66ce.firebaseapp.com/__/auth/action`
with `http://localhost:5173/auth/action`, keeping everything from `?` onward.
The `oobCode` is what matters and it is origin-independent.

**TODO(post-mvp) markers added:** none.

**Known broken / left half-done:**
- **A password reset email has not been observed arriving.** The link generates
  and `dgjy2019@gmail.com` is provisioned for exactly this, but nobody has yet
  clicked *Forgot your password?* and watched an inbox. Last open item.
- `verify_phase2/` and `provision_dev_user/`, the two throwaways that minted the
  live token and created the dev accounts, were deleted after use. Phase 7's
  seed script is where that logic belongs permanently (§3.5.3 already specifies
  the deterministic-UID shape it should take), and §3.3's provisioning order —
  Firebase user first, `users` row second, delete the Firebase user if the
  insert fails — is Phase 3's `POST /api/tenant/users`.
- Firebase's `accounts:signUp` REST endpoint is still open on this project, so
  "self-signup is disabled" (§3.3) is a statement about *this application*, not
  about Firebase. That is fine and is what the orphaned-account test covers: an
  account created that way has no `users` row and gets a 401 from every endpoint.
  Worth a look at Identity Platform's signup toggle at Phase 9, not before.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it.
- **`go test ./...` is mildly flaky on this machine.** One run failed every test
  in `internal/db` at `0.00s` — a container that never came up, not a logic
  failure; the same tests passed alone immediately after, and four consecutive
  full runs since have been green. Phase 2 added a second test package, so the
  suite now starts *two* Postgres containers in parallel where Phase 1 started
  one, on top of the compose container and the API. If it recurs, `go test -p 1`
  serialises packages; a shared container across packages would be the real fix
  and is not worth it yet.

**Next:** open [`phases/phase-3-permissions.md`](phases/phase-3-permissions.md)
in a new session.

## Phase 3 — 2026-07-26

**Done:** the permission model is enforced, and both control planes have screens.
`Identity.LevelFor` is the one function every check goes through, with §5.4's
ordering intact: entitlement first, then the implicit-admin shortcut, then the
stored level. `RequireModule` turns it into the two distinct refusals a client
can act on — `module_not_enabled` is the superadmin's problem,
`insufficient_module_role` is the tenant admin's — and carries `required`/`actual`
so a console can name the dropdown to change.

The platform plane (`/api/admin/*`, `erp_admin` pool, `RequireSuperadmin`) lists
and creates workspaces, toggles entitlements, and suspends. `POST /admin/tenants`
does tenant + first admin + chart of accounts in one transaction, seeding
`accounts` through the `seed_tenant_accounts()` `SECURITY DEFINER` function
because `erp_admin` has no grant on the table and must not be given one.

The tenant plane (`/api/tenant/users`, `erp_app` pool, `RequireTenantAdmin`)
manages people and their per-module levels, including the bulk matrix endpoint
and the last-admin rule under `SELECT … FOR UPDATE`. Both user-creating endpoints
follow §3.3: provider account first, database row second, provider account
deleted again if the database refuses.

Frontend: an `AppShell` with entitlement-driven nav, plus `/admin/tenants`,
`/admin/tenants/new`, `/admin/tenants/:id` (entitlement toggles),
`/settings/users`, `/settings/users/new`, and `/settings/users/:id` (the
per-module role matrix). Superadmins land on `/admin/tenants` and see no business
modules at all.

**Tests green:** B1–B10, plus Groups A and I–J unchanged. **79 top-level tests**
(85 including subtests in `internal/api` alone), up from 39 at Phase 2.
`go test ./... -p 1` clean, `go vet` clean, `gofmt` clean, `npm run build` and
`oxlint` clean.

Beyond Group B, the tests that exist because something could go quietly wrong:

- **Cross-tenant user management.** `users` and `user_module_roles` carry no RLS —
  they cannot, since identity resolution reads `users` before tenant context
  exists — so the tenant filter is application-side on *every* query in
  `tenant_users.go`. Read, rename, deactivate, single-module grant, and bulk
  grant are each asserted to 404 against another tenant's real user ID, and the
  victim row is re-read afterwards to confirm nothing was written.
- **The compensating delete of §3.3 step 4**, which is invisible in the response
  body. `FakeUsers.ForceUID` makes the provider hand back a UID already parked on
  a row in another tenant, so the insert violates `users_firebase_uid_key`; the
  test then asserts the provider account was deleted.
- **`erp_admin` still cannot read `accounts` directly**, asserted right after it
  seeded them through the function. This is the test that fails if someone
  "solves" §4.2.1 with a table grant.
- **No `DELETE` route** on either plane, asserted with an authenticated request.
- **Privilege escalation across planes:** a tenant admin cannot create or promote
  a `superadmin` (400), and no provider account is created for the refused
  request.
- **The §9.0 list contract:** pagination, server-side sort across the whole result
  set, `-` prefix, `pageSize` clamped to 100, unknown sort field → 400.

**Four mutations were run to check the tests bite rather than merely pass. Two
survived, and both taught something:**

| Mutation | Result |
|---|---|
| `FOR UPDATE` removed from the last-admin count | **caught** — B10 reported "2 succeeded and 0 were refused", the exact race, leaving zero admins |
| `LevelFor` reordered: admin shortcut before entitlement | **survived** all of Group B |
| tenant filter dropped from `patchUser`'s `UPDATE` | **survived** |
| tenant filter dropped from `patchUser`'s target `SELECT` | **survived** |

The `LevelFor` survivor is the interesting one. `RequireModule` checks
entitlement itself before calling `LevelFor` — it has to, because the two
failures carry different error codes — so the ordering *inside* `LevelFor` is
invisible to every HTTP-level test. It stops being invisible in Phase 4, where
handlers call `LevelFor` directly for record-level rules with no middleware in
front. Fixed by adding `internal/identity/level_test.go`: six pure unit tests, no
container, and the mutation now fails two of them.

The two tenant-filter survivors are mutually redundant defence in depth: the
target `SELECT` 404s first, and if it did not, the `UPDATE`'s `RowsAffected == 0`
would. Removing **both** does make the cross-tenant test fail — verified — with
another tenant's user renamed and deactivated. So the property is covered; no
single-line mutation can express it.

**Live checks against `erp-project-b66ce`.** The fakes cannot catch a wrong Admin
SDK call shape, and that failure would first appear in production, so
`auth.Firebase`'s write half was exercised against the real project with a
throwaway account, since deleted:

| Call | Result |
|---|---|
| `CreateUser` | ok, returned a real UID |
| `CreateUser` again, same address | mapped to `auth.ErrEmailExists` → 409, not an opaque 500 |
| `SetDisabled(true)` / `SetDisabled(false)` | ok both ways |
| `DeleteUser` | ok, nothing left behind |

Also verified against the local Docker database and a freshly built binary:
`go run ./cmd/migrate` applies the new grant idempotently, and
`information_schema.role_table_grants` confirms `erp_app` holds `DELETE` on
`user_module_roles` **and** that `erp_admin` does not. The API boots and
`/api/health` is 200 without a token, while `/api/admin/tenants`,
`/api/admin/modules`, and `/api/tenant/users` all return `401 unauthenticated`
from the auth chain rather than 404 — the routes are wired.

**Deviations from spec:** fourteen, all recorded in *Decisions taken* above. The
two worth knowing about before Phase 4:

- **`erp_app` now holds `DELETE` on `user_module_roles`**, and on nothing else.
  §5.3 requires `none` to delete the row and the grant was missing, so every
  revocation would have been a `42501`. It is the one exception to I5.
- **A helper must never signal failure by returning `httpx.Fail`'s value**, which
  is `nil` by design. This shipped briefly and B8 caught it as a 403 whose body
  was a user-detail object. The validating helpers are now pure functions
  returning real errors, with a comment on `parseMatrix` explaining why.

**TODO(post-mvp) markers added:** none.

**Known broken / left half-done:**

- **The module nav items and the bottom tab bar are absent from `AppShell`.** The
  entitlement filter that drives them is already the real one, reading
  `/api/me`; each of Phases 4–6 adds a path to `modulePaths` in
  `frontend/src/components/AppShell.tsx` and nothing else. §10.7.3's bottom tab
  bar is deliberately deferred with them — its destinations are the module
  screens, and a tab bar with one tab is not a navigation aid.
- **A narrow orphan window remains in `POST /tenant/users`:** a `COMMIT` that
  fails *after* a successful `INSERT` leaves a provider account with no `users`
  row. Everything checkable is validated before the provider call and every
  insert error is compensated, so reaching it needs the connection to drop
  between the insert and the commit. There are no deferred constraints on those
  two tables. It logs `api: ORPHANED firebase account …`, and the state is
  already handled as a 401 by the middleware. A rollback hook on `TenantTx` would
  close it properly; not worth the machinery for one call site yet.
- **No live browser walkthrough of the six new screens.** They build and lint
  clean and the API they call is covered end to end by the suite, but nobody has
  signed in as a superadmin and clicked through workspace creation. Needs a
  superadmin `users` row plus a real ID token; Phase 2's throwaway token-minter
  was deleted, and Phase 7's seed script is where that belongs permanently.
  **There is currently no superadmin row in the local database at all** — every
  `/admin/*` screen is unreachable by hand until one is inserted.
- **No frontend tests at all.** §12.5 defines them and Phase 3's brief does not
  ask for them.
- The `/api/warehouses` probe route in `testsupport` returns 500 for a
  superadmin, because `TenantTx` opens no transaction for a tenantless identity
  and the handler finds nil. That is the honest answer for a misconfigured route,
  and the log line is noise in the test output; every *shipped* tenant route is
  gated by `RequireModule`, which refuses them first.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it,
  and B10 is the first test that would genuinely benefit.
- **A password reset email has still not been observed arriving** (carried from
  Phase 2). Both new endpoints hand out an initial password instead, so nothing
  in Phase 3 depends on it.
- A local API instance from an earlier session was killed to free `:8080`; the
  newly built binary is running there now. `make dev` restarts everything.

**Next:** open [`phases/phase-4-inventory.md`](phases/phase-4-inventory.md) in
a new session.

## Phase 4 — 2026-07-26

**Done:** the inventory module works end to end, and stock on hand is derived
everywhere it appears. Sixteen `/api/inventory/*` routes cover products and
warehouses (full CRUD, soft delete, restore), the stock grid, the low-stock
query, the filterable ledger, and the one endpoint that appends to it. Every
route carries its own `RequireModule` at the level §9.5 specifies — reads at
`viewer`, adjustments at `approver`, master data at `admin`.

`stock_balances` is read on every request and there is no counter column
anywhere (I6). The ledger is append-only in the strongest sense available: the
service layer never tries, and `erp_app` has `UPDATE`/`DELETE` revoked, so a
future code path that does try gets a `42501` rather than quietly rewriting
history.

Screens: `/inventory/products` (stock and reorder flag), `/inventory/products/new`,
`/inventory/products/:id` (edit, discontinue, delete/restore, per-warehouse
balances, an adjustment form, and that product's ledger history),
`/inventory/warehouses`, `/inventory/stock` (with a low-stock banner), and
`/inventory/ledger`. `AppShell` gained the Inventory nav item and a second level:
a module's screens appear as sub-items only while you are inside it.

**Tests green:** F1–F3 and G1–G3, G5, G6, G9–G11, plus Groups A, B and I–J
unchanged. **100 top-level tests** (167 including subtests), up from 79 at
Phase 3. `go test ./... -p 1` clean, `go vet` clean, `gofmt` clean, `tsc
--noEmit`, `npm run build`, and `oxlint` all clean — oxlint reports only the
three pre-existing fast-refresh warnings, and none in new code.

G4 and G7–G8 are procurement and stay in Phase 5, but the **product** analogue of
G4 is tested now: deleting a product with an open PO line is `409 in_use` naming
the line, and the same delete succeeds once the PO is `received`. The brief
expected that to wait for Phase 5; the fixtures already build purchase orders, so
it did not have to.

Beyond the listed IDs:

- **`TestInventoryRoutesCarryTheLevelsFromTheSpec`** walks every §9.5 route and
  asserts the level it is registered at. Group B already proves `RequireModule`
  works, against probe routes — this proves the *route table* matches the spec,
  which a probe route structurally cannot.
- **Isolation from two tenants**, through the list, by ID, on a write, and
  through the ledger — with tenant B's own data re-read afterwards, so the
  emptiness is a filtering result rather than a broken fixture.
- **`httpx.Numeric`'s six unit tests**, no container: the round trip preserves
  2^53+1 with a fraction, `1e400` is refused in both quoted and unquoted form,
  and `Scan(float64)` is an error rather than a silent acceptance.
- The §9.0 list contract on the inventory lists, including that page 2 of a
  descending sort holds the whole set's third and fourth items rather than a
  sorted slice of one page.

**Two mutations were run to check the tests bite rather than merely pass. Both
were caught:**

| Mutation | Result |
|---|---|
| warehouse emptiness asked as one `SUM` instead of per product | **caught** — G5 got a 200 where it wanted 409, deleting a warehouse holding +5 and −5 |
| `AND p.deleted_at IS NULL` added to the ledger's product join | **caught** — G1 reported "ledger rows = 0, want 1 — deleting a product deleted its history" |

The second is the mutation that matters, because it is the one a later phase
would make by reflex. It is now Trap 3 in *What a module phase inherits*.

**Live check against the local stack:** the Phase 3 binary on `:8080` was
replaced with a freshly built one. `/api/health` is 200 without a token, and all
sixteen inventory routes return `401 unauthenticated` from the auth chain rather
than 404 — they are wired. `GET`, `POST`, and `DELETE` were each checked, since a
missing verb registration looks identical to a working one until someone uses it.

**Deviations from spec:** twelve, all recorded in *Decisions taken* above. The
three worth knowing before Phase 5:

- **`httpx.Numeric` is how every NUMERIC crosses the wire**, and it has no
  arithmetic on purpose. Procurement's line totals and PO amounts go the same
  way, and `total_amount` must be summed in SQL rather than in Go.
- **Resolving a row by ID does not filter `deleted_at`** (Trap 3). Procurement's
  PO-line joins inherit this directly — G6 already asserts it at the query level
  precisely so Phase 5 does not undo it.
- **`/inventory/warehouses` exists** though §10.4 does not list it, because
  §9.6.1 requires a UI for every CRUD entity. Suppliers need the same treatment
  in Phase 5; §10.3 does list `/procurement/suppliers`, so no judgement call
  there.

**TODO(post-mvp) markers added:** none.

**Feedback applied after the walkthrough** — the flow worked first time, but
three things were wrong on screen, and all three are fixed and recorded in
*Decisions taken*: no success feedback on any action (now toasts), a deleted
product reading "Status: Active", and a deleted product's stock being counted by
the warehouse while the grid hid it. The last one is a genuine correctness fix
with a test, not a cosmetic one — see
`TestDeletedProductsStockStaysVisibleEverywhere`.

**Known broken / left half-done:**

- ~~No live browser walkthrough~~ — **done.** Signed in as `dgjy2019@gmail.com`
  and walked the whole flow: warehouse, product, `+50` then `-20` adjustments
  with the balance moving to 30 in the panel and both entries appearing in the
  history, delete with the ledger still resolving the product, the duplicate-SKU
  refusal, and the warehouse in-use refusal. No token minter was needed — the
  dev account is a tenant admin, so the browser's own Firebase sign-in is enough.
  Only the **superadmin** plane still needs a `users` row that does not exist.

  It found three things, all now fixed and recorded above: a deleted product's
  Status reading "Active", a deleted product's stock being counted by the
  warehouse but hidden by the grid, and no success feedback on any action.
- **Still no superadmin row in the local database**, so `/admin/*` remains
  unreachable by hand (carried from Phase 3).
- **Still no frontend tests at all.** §12.5 defines them; Phase 4's brief does
  not ask for them, and the count is now six more untested screens than it was.
- **The inventory screens work but read as unintuitive** — reported from the
  walkthrough, deliberately not acted on, because the three *wrong* things it
  found were fixed and the rest is polish that no MVP phase owns. Not
  diagnosed to a single cause; these are the candidates, worth putting in front
  of a user before rewriting anything:
  - **Product detail opens as an editable form**, not a summary you click *Edit*
    on. Every field is live the moment the page loads, so there is no "reading"
    mode and no obvious point at which you have committed to changing something.
    This is the biggest one, and the pattern is already inconsistent with
    `/inventory/warehouses`, which does have an explicit Edit.
  - **Nothing says a warehouse must exist first.** Stock cannot be held without
    one, but the empty product list talks about products only, and the
    consequence surfaces late, as a disabled *Post adjustment* button on a
    product you have already created.
  - **"Adjust stock" is the only way to set an opening balance**, and the label
    does not fit that use. Correct by design — an opening balance is a ledger
    entry with an author (I6) — but a first-time reader is looking for a
    quantity field on the create form and does not find one.
  - **Discontinue / Delete / deleted are three ideas on one screen.** They are
    genuinely three different things (§6.9.1) and the helper text explains them,
    but explaining a model in prose is what you do when the controls have not
    made it obvious.
  - Toast placement is a live trade-off, not a settled answer: top-right at
    `top-14` clears the header but can overlap the page action button. See the
    comment in `components/Toasts.tsx`.
- **The `/api/warehouses` probe route in `testsupport` is now shadowed in
  meaning by the real `/api/inventory/warehouses`.** It is a different path and
  still does its job for the transaction tests, but the name is misleading now
  that a real warehouses endpoint exists. Not worth churning Group B's fixtures
  over; worth renaming if that file is touched for another reason.
- **The §10.7.3 bottom tab bar is still absent.** With one module the sidebar
  and drawer cover it; revisit when procurement makes three destinations.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it.
- **A password reset email has still not been observed arriving** (carried from
  Phase 2). Nothing in Phase 4 depends on it.
- A freshly built API binary is running on `:8080`, having replaced the Phase 3
  one. `make dev` restarts everything.

**Next:** open [`phases/phase-5-procurement.md`](phases/phase-5-procurement.md)
in a new session.
