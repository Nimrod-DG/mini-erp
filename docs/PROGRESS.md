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
| Password reset | **Working, confirmed by the user 2026-07-26.** The emailed link opens Firebase's hosted page, not this app's `/auth/action` — see the Phase 2 log and the Phase 9 note about `callbackUri` |
| Firebase **prod** project ID | **none, deliberately** — the deployment reuses the dev project so the seeded demo accounts work on the live URL. See *Decisions* |
| Hosting site | `erp-project-b66ce`, auto-linked at app registration; Phase 9 deploys to it |
| Database host | local Docker (Phase 0–8); **Neon** Postgres 17, AWS `ap-southeast-1`, database `erp` (Phase 9) |
| Cloud Run project | `banded-torus-476311-q1` — **a different Google account from the Firebase project.** Display name renameable, project ID immutable |
| Cloud Run service | `mini-erp-api`, region `asia-southeast1` (Singapore, to match Neon) |
| The deployment runbook | [`DEPLOY.md`](DEPLOY.md) — read that, not the reference docs, when deploying |

Wherever the docs say `erp-dev`, read `erp-project-b66ce`.
Config values live in [`reference/env-setup.md`](reference/env-setup.md).

---

## Current state

**Phase:** 9 — **the repository half is done and rehearsed; nothing is deployed
yet.** Everything that had to change in code, SQL or configuration for this
application to run on Neon + Cloud Run + Firebase Hosting now exists and has been
proven against a database shaped like the real one. What remains is entirely
account work: creating the Neon project, enabling billing, and running the
commands in [`DEPLOY.md`](DEPLOY.md) — none of which can be done from here,
because the two Google accounts involved are the developer's.

| Phase 9 "done when" | State |
|---|---|
| Acceptance test against the **deployed** URLs | **Not started** — nothing is deployed |
| Test A10 against the production database | **Tooling built and rehearsed.** `cmd/dbverify` is the runnable form; it passed against a non-superuser-owner Postgres 17, which is Neon's shape |
| Test J1 against the production database | Same — `dbverify` checks all three roles |

Phase 8's remaining box, **CI green on a pull request**, is still open and still
cannot be checked from this machine. It is worth having green *before* the
deploy: `go test -race` has never run anywhere, and the only real concurrency in
the system is the goods receipt's `FOR UPDATE` and its savepoint/rollback pair.

**Next action:** follow [`DEPLOY.md`](DEPLOY.md) from §2. The database steps
(§2–§4) are independent of the GCP account work and are the ones to do first.

### What was and was not verified at Phase 7.5

**Verified mechanically, and green:** the whole Go suite (Groups A–L, including
the new Group L), the frontend build and lint, and seed idempotency at the
*content* level — an md5 over every document, ledger row, and journal line is
byte-identical across three consecutive `make seed` runs.

**Verified against the running application:** roughly **150 assertions covering
all twenty-five acceptance steps**, driven with real Firebase ID tokens for all
eight seeded accounts against the real server on the real seeded database. Every
business rule the acceptance test names holds: the two refusal codes, segregation
of duties for staff *and* admins, `last_admin`, the cross-module transaction and
its exact ledger/journal/balance effects, over-receipt writing nothing, the
idempotent replay, `in_use`, the full soft-delete/restore loop for all three
entities, and cross-tenant isolation by pasted UUID.

The driver was a throwaway — it needs a live server and live Firebase, so it
cannot run in CI and would rot in the repo, and every assertion it makes is
already in the Go suite against a container. It was deleted after use. The
database was snapshotted first and **restored byte-identically afterwards**
(same md5), so the demo is untouched.

**Not verified, and it is the gate:** everything above the API. Whether a control
is *rendered* or hidden (I12 makes hiding cosmetic, so both readings pass at the
API), the 360px layout, both themes, whether a link goes where it claims, and the
copy as a person reads it.

**Two real findings came out of the API pass**, both fixed: the `/api/me`
effective-roles bug below, and four touch targets under 44px. That is the
calibration for what the browser walk should expect.

**Two things the API pass established about the *local database*, not the code**,
which will otherwise read as failures:

- `HND-GLOVE` and `PKG-BOX-L` sit on **open** orders, so deleting them is refused
  with `in_use` — correct (G4), and step 18 needs a product on a closed order:
  `OFF-PAPER` or `HND-PALLET`.
- Nusantara has **three** tenant admins, because two scratch accounts from Phases
  2–3 are still here. `last_admin` therefore does not fire for Rina. Confirmed
  correct by temporarily demoting the other two.

### The browser walkthrough was deferred to this phase, and is what remains

**Decided 2026-07-26. Do not treat an unwalked screen as an oversight.** The MVP
gate is the full twenty-five-step [acceptance test](acceptance-test.md), on a
360px viewport, in both light and dark mode — a walkthrough of the whole
application by construction. Running one per phase as well would have meant
walking the same flows five times, and only the last walk is performed against
the finished thing.

Phase 4 is the calibration: its walkthrough found three real problems in about an
hour, on one module, one of which
(`TestDeletedProductsStockStaysVisibleEverywhere`) was a genuine correctness bug.
Phase 7's API-only pass has already found a second
(`TestB13MeReportsTheEffectiveLevelForATenantAdmin`). Expect more.

Still never opened in a browser:

| Screens | From |
|---|---|
| `/admin/tenants`, `/admin/tenants/new`, `/admin/tenants/:id`, `/settings/users`, `/settings/users/new`, `/settings/users/:id` | Phase 3 |
| the six procurement screens — requisition list, create, detail, PO list, PO detail, suppliers | Phase 5A |
| `/procurement/orders/:id/receive` and **the §10.3 confirmation panel** | Phase 5B |
| `/finance` — including **both** links out of the confirmation panel | Phase 6 |
| **the four dashboard widgets**, the card view of the two document lists, the frozen columns on the two grids, and the sticky action bars | Phase 7 |

Walked already, and still worth re-walking at the gate: Phase 2's sign-in and
Phase 4's six inventory screens.

**The approval step needs two people, and now it has them.** C2 forbids approving
your own requisition for *everybody*, tenant admins included, so the walk needs a
second user holding `procurement: approver`. The seed provides one per tenant —
Budi at Nusantara, Intan at Bahari. That is no longer a manual step.

### The seeded demo (§15)

`make seed` is idempotent and self-verifying. Everything below exists in the
local database now. Password for every account: `password123`.

| Email | Workspace | Tenant role | Procurement | Inventory | Finance |
|---|---|---|---|---|---|
| `super@erp.test` | *platform* | superadmin | — | — | — |
| `rina@nusantara.test` | Nusantara Retail | admin | *implicit* admin | *implicit* admin | *implicit* admin |
| `budi@nusantara.test` | Nusantara Retail | staff | approver | viewer | none |
| `sari@nusantara.test` | Nusantara Retail | staff | user | user | none |
| `dewi@nusantara.test` | Nusantara Retail | staff | viewer | none | admin |
| `agus@bahari.test` | Bahari Logistics | admin | *implicit* admin | *implicit* admin | **none — not entitled** |
| `manager@bahari.test` | Bahari Logistics | staff | approver | approver | none |
| `staff@bahari.test` | Bahari Logistics | staff | user | viewer | none |

Nusantara is `Asia/Jakarta` with all three modules; Bahari is `Asia/Makassar`
with **no Finance**. Per tenant: 2 warehouses, 10 products (1 soft-deleted, 1
discontinued, **3 below reorder point**), 5 suppliers (1 inactive), ~30 ledger
rows spread across the preceding 60 days, 13 requisitions (2 draft, 3 submitted,
1 rejected, 1 cancelled, 6 approved) and the 6 orders those approvals produced
(2 open — one overdue — 1 partially received, 2 received, 1 cancelled), with 4
goods receipts that each wrote stock ledger rows **and** a balanced journal entry
in the same transaction.

**Leftover scratch rows from Phases 2–3 are still in the local database and were
deliberately not deleted:** a product `SKU-001 Widget`, a warehouse `WH-1`, and
three accounts (`dgjy2019@gmail.com`, `phase2-check@example.test`,
`superadmin@example.test`). They are harmless — none of the acceptance steps
touch them — and `dgjy2019@gmail.com` is the developer's own Firebase account and
a working Nusantara tenant admin, which is useful. To get a pristine demo instead:

```sql
DELETE FROM stock_ledger WHERE product_id IN (SELECT id FROM products WHERE sku = 'SKU-001');
DELETE FROM products   WHERE sku  = 'SKU-001';
DELETE FROM warehouses WHERE code = 'WH-1';
DELETE FROM users      WHERE email IN ('phase2-check@example.test','superadmin@example.test');
```

**The seed adopts rather than collides.** `tenants` upserts on `slug` and `users`
on `email`, returning the row's real id — which is why the seed ran against a
database that already held a hand-made `nusantara` workspace instead of aborting
on `tenants_slug_key`. Every other row uses a UUIDv5 derived from what it is
(`cmd/seed/ids.go`), so a reseed writes to the same rows and demo URLs survive a
rebuild.

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
| `006_pr_cancel_from_draft.up.sql` | relaxes `pr_submitted_has_timestamp` so a draft can be cancelled without a submission that never happened — see *Decisions* |

Backend packages as of Phase 7:

| Package | Contents |
|---|---|
| `internal/auth` | `Verifier` (UID only, never claims) and `UserManager` (create/delete/disable), both satisfied by one `Firebase` value |
| `internal/identity` | `Identity`, `Resolve` — the one database lookup behind I9 — plus `RoleLevel` and **`LevelFor`**, the single function every permission check goes through |
| `internal/middleware` | `RequestID` `FirebaseAuth` `ResolveIdentity` `TenantTx` `RequireModule` `RequireSuperadmin` `RequireTenantAdmin`, and the accessors `IdentityFrom` / `TxFrom` |
| `internal/httpx` | the §9.8 error envelope (`Fail`, `FailWith`, `Unauthenticated`), the §9.0 list contract (`ParseList`, `ListResponse`), and **`Numeric`** — the exact-decimal type every NUMERIC crosses the wire as |
| **`internal/docnum`** | **`Allocate(tx, tenant, docType)`** — §8.1 numbering, in the caller's transaction. `AllocateAt` is the same thing with an explicit instant, which is how E5 can fail. Constants `PR` `PO` `GR` `JE` |
| `internal/db` | pools, `WithTenant`, migrations, and `SQLState` / `IsUniqueViolation` / `ConstraintName` for mapping constraints to business outcomes |
| `internal/api` | `New` (route wiring, so tests drive the real chain), `Me`, the seven `/admin/*` handlers, the six `/tenant/users` handlers, the sixteen `/inventory/*` handlers, the twenty `/procurement/*` handlers, the two `/finance/*` handlers, and **`dashboard.go` — the one route that spans modules**. **`procurement_receipts.go` is §8.4** — the one handler that writes to three modules in one transaction — with its inventory and finance halves in **`inventory_receipt.go`** and **`finance_journal.go`**, both taking the same `tx`. It now exports **`api.PostGoodsReceipt`**, the same transaction with the HTTP peeled off, which is what `cmd/seed` calls. **`finance.go` is the finance read side** and is the whole of §9.6 |
| **`cmd/seed`** | §15's demo, in four files: **`data.go`** (the two tenants, seven users, master data — the specification transcribed), **`documents.go`** (one recipe of thirteen requisitions and eight adjustments, applied to both tenants by *index* into their own data), **`platform.go`** (the five RLS-free tables, on the erp_admin pool), **`tenant.go`** (everything else, in one `db.WithTenant` transaction per tenant on the erp_app pool), and **`ids.go`** (UUIDv5 from what a row *is* — the whole of the idempotency story) |
| `testsupport` | `FakeVerifier`, `FakeUsers`, the shared HTTP `Harness` (used by both test packages), the fixtures, and `WithTenantOn` / `NoSuchTenant` |

Frontend routes as of Phase 7: `/login` `/auth/action` `/` `/admin/tenants`
`/admin/tenants/new` `/admin/tenants/:id` `/settings/users` `/settings/users/new`
`/settings/users/:id` `/inventory/products` `/inventory/products/new`
`/inventory/products/:id` `/inventory/warehouses` `/inventory/stock`
`/inventory/ledger` `/procurement/requisitions` `/procurement/requisitions/new`
`/procurement/requisitions/:id` `/procurement/orders` `/procurement/orders/:id`
`/procurement/orders/:id/receive` `/procurement/suppliers` **`/finance`**. All
signed-in screens render inside `AppShell`.

**Every module in the naming contract now has a screen and an entitlement path.**
`AppShell`'s `modulePaths` holds all three; Finance is the one with no `children`,
because the module is a single page.

**The §10.3 confirmation panel is now complete.** Both of its lines link — the
inventory one to `/inventory/ledger?sourceId=<receipt id>`, the finance one to
`/finance?sourceId=<receipt id>` — and each lands on a list filtered to that one
document. The `TODO(phase-6)` Phase 5 left in `ReceiveGoods.tsx` is gone, and
there are no `TODO(phase-*)` markers left anywhere in `frontend/src` or
`backend/internal`.

**`/` is no longer the identity dump it was.** `pages/Dashboard.tsx` is gone;
`pages/dashboard/` holds `DashboardPage` plus `WidgetCard`, `ApprovalQueue`,
`LowStockCard`, and `RecentActivityCard`. The approve/reject decision happens on
the dashboard itself for an approver, which is §10.7.1's "two-button decision
between meetings".

**What Phase 8 inherits from the responsive pass.**

| Need | Use |
|---|---|
| A list that is a table on a desktop and cards on a phone | **`ResponsiveList`** (`components/ResponsiveList.tsx`). It owns the breakpoint switch, all four §10.7.6 states in *both* views, and the pagination. It does **not** own the cells: `row` and `card` are render props, for the same reason `TableHead` shares only the heading row |
| The card itself | **`DocumentCard`** from `components/CardList.tsx` — number, caption, chip, label/value fields, footer. The card is not one big link, because several rows carry a second link and a link inside a link is invalid markup |
| "Am I below `md`?" in JavaScript | **`useCompact()`**. Used only where a *genuinely different component* is needed (§10.7.4). Everything that is merely layout stays in Tailwind |
| A dense grid that scrolls sideways | **`ScrollableTable`** + `TableHead sticky` + `{ sticky: true }` on the first column + **`frozenCell`** on the matching `<td>`, and `group` on the `<tr>` so the frozen cell follows the row's hover |
| A form's primary action on a phone | **`StickyActions`**. `sticky`, not `fixed`, so nothing needs a spacer and nothing can hide behind it; `bottom-14` clears the tab bar |

**What Phase 7 inherits from finance.** `GET /api/finance/journal-entries` takes
`sourceId`, `accountId`, `sourceType`, `from`, `to`, and the §9.0 list parameters,
and returns each entry **with its lines** (`journalEntryDetail`, so acceptance
step "the entry is balanced" is answerable from one response). `GET
/api/finance/accounts` returns the two seeded rows. Neither has a write
counterpart and neither should grow one before the real Finance module.

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
| Allocate a document number | `docnum.Allocate(tx, caller.TenantID, docnum.GR)`, on the **same** `tx` as the insert. Never a sequence, never `to_char(now(), …)` |
| Refuse because the document moved on | `stateConflict(c, "This purchase order", current, "open")` → 409 `state_conflict`, with `details.status` so a screen can refresh |
| Refuse on a business rule | `unprocessable(c, "<code>", …)` → 422. Codes so far: `empty_requisition` `reason_required` `supplier_required` `over_receipt` |
| Refuse on a record-level rule | `forbidden(c, …)` → 403 `forbidden`. Segregation of duties is `self_approval_forbidden`, which is its own code |
| Lock a document before checking its status | `lockRequisition` in `procurement_requisitions.go` is the worked example: `SELECT … FOR UPDATE`, *then* read `status`. Read-then-check is a race (§8.6.2, H4) |
| A status chip or filter chips, in the browser | `StatusChip` / `StatusFilter` from `components/StatusChip.tsx`; the words come from `statusLabel` in `lib/format.ts` |
| A master-data list screen | `MasterDataList` from `components/MasterDataList.tsx` — search, "show deleted", the four §10.7.6 states, pagination — plus `useRowActions` for the per-row toast handling. Suppliers and warehouses both use it |
| A table heading row, in the browser | `TableHead` with `Column[]` from `components/ListStates.tsx`. Every table in the application uses it; do not write a `<thead>` by hand |
| Post a journal entry | `postReceiptJournal` in `internal/api/finance_journal.go` is the worked example: `accountByCode` resolves 1300/2150, `docnum.Allocate(tx, …, docnum.JE)` numbers it, and **`assertJournalBalances`** is called before the transaction can commit. Both sums are computed in SQL — never compare two `httpx.Numeric` in Go |
| Write a cross-module transaction | `createGoodsReceipt` in `procurement_receipts.go`. One `tx`, passed into the other modules' functions. **D8 is the test that proves it**; read its comment before changing step order, because it depends on the ledger being written *before* the journal |
| Take an idempotency key | `idempotencyKey(c)` plus `receiptByKey` and the `SavePoint`/`RollbackTo` pair in `createGoodsReceipt`. Match the *constraint name*, never bare SQLSTATE `23505` |
| A list row that carries child rows | `attachJournalLines` in `finance.go` — one query for the page's children, keyed by parent id. A `LEFT JOIN` on the page query multiplies the rows and breaks both the page size and the total; a query per row is 25 round trips. Note the two types: GORM's `Scan` **cannot fill a slice-of-struct field** and errors the whole query, so the scannable row and the row-with-children are separate structs (`journalEntryRow` / `journalEntryDetail`, like `goodsReceiptRow` / `goodsReceiptDetail`) |
| The banner over a list narrowed by `?sourceId=` | `SourceFilterNotice` from `components/ListStates.tsx`. It owns the URL parameter as well as the banner — `onCleared` is only for the page's own state, which in both callers means resetting to page 1 |

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
| 2026-07-26 | **The deployment authenticates against the *dev* Firebase project, and the demo accounts are seeded into it.** `phases/phase-9-deployment.md` step 7 and `reference/auth.md` §3.5.1 both say the opposite | This is a portfolio deployment, and the thing it has to do is let somebody who has no credentials from the developer sign in and walk the acceptance test. A clean `erp-prod` pool would mean handing out a password by hand to every reader. The dev project already holds the eight §15 accounts at deterministic UIDs. The cost is stated rather than hidden: the live URL is a demo, `password123` is public, and nothing real goes in it. If the application ever holds real records, this is the first decision to reverse — nothing depends on it but two env vars. |
| 2026-07-26 | **I3 is asserted for `erp_app` and `erp_admin`, and *reported* for `erp_migrate`** | The open question Phase 8 wrote down. Locally `erp_migrate` **is** the container's superuser — `docker-compose.yml` boots Postgres as it and nothing else could create the schema — so a hard assertion would fail every local run, and a check that always fails is a check nobody reads. On a managed host it is an ordinary role and 005's `FORCE ROW LEVEL SECURITY` is exactly what lets the owner be unprivileged. So `cmd/dbverify` fails on the two application roles and warns on the owner, and the warning is a finding in a deployment. |
| 2026-07-26 | **`000_roles.sql` no longer forces the role attributes it asserts**, and `GRANT CONNECT` names `current_database()` rather than the literal `erp` | The file is re-applied by `make migrate` on every run, including against a managed host where the migrate role is not a superuser. PostgreSQL requires superuser to touch `SUPERUSER`/`BYPASSRLS` **even to turn them off**, and to `ALTER ROLE` a role you have no ADMIN on — so the previous file aborted the first production migration with `must be superuser to alter superuser roles`, an error that reads like a schema fault. Every privileged statement is now attempted and skipped, and I3 is asserted from `pg_roles` afterwards, which needs no privilege. That is the stronger claim anyway: forcing an attribute and checking it are not the same thing, and only the check catches a role provisioned through a console. |
| 2026-07-26 | **`config.Load` no longer requires `MIGRATE_DATABASE_URL`**; `cmd/migrate` demands it through `RequireMigrateURL` | It is the schema owner's credential. Requiring it to boot would mean the deployed Cloud Run service carried a connection string that can drop its own tables, purely to satisfy a validation loop. The separation was already the design — "a running service must never be able to change its own schema" — and this makes it true of the process environment as well. |
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
| 2026-07-26 | **Migration `006` relaxes `pr_submitted_has_timestamp` to `status IN ('draft','cancelled') OR submitted_at IS NOT NULL`** | §6.10.3 writes it as `status = 'draft' OR submitted_at IS NOT NULL`, which reads as "anything past draft has been submitted". True of `submitted`, `approved`, `rejected`; false of `cancelled` — §6.9.2 says in as many words that "a draft requisition may be cancelled by its creator", and such a requisition was never submitted. As written the constraint makes that transition impossible, and the only way past it is to stamp `submitted_at` with a submission that did not happen, in the column the status timeline renders from. A requisition cancelled *after* submission still carries the real timestamp, because cancelling never clears it. |
| 2026-07-26 | Document numbering is its own package, `internal/docnum`, and exports **`AllocateAt`** alongside `Allocate` | Four document types across two modules allocate from it, so it is not procurement's. The package boundary also buys the tests: `internal/api`'s tests cannot import `testsupport` (which imports `api`), so Group E could only have been written through HTTP — and E3's twenty concurrent allocations, E4's rollback, and E5's timezone are not visible in a response body. `AllocateAt` takes the instant to date the allocation at; every caller in the application passes now. It exists because **E5 cannot fail without a controllable clock** — no real timezone moves an ordinary mid-month afternoon into another month, so a version using `now()` would pass whether the tenant-timezone conversion were there or not on all but two days a month. The seam is one `COALESCE` inside the same expression production evaluates. |
| 2026-07-26 | 409s that mean "the document has moved on" carry a new code, **`state_conflict`** | §9.8 names "409 state conflict" as a category but gives it no string, and the naming contract's list has no member that fits. `in_use` means something else — that something still *references* this row — and a client that cannot tell "this requisition was approved an hour ago" from "three orders reference this supplier" cannot tell the user what to do. `details.status` carries where the document actually is, so a screen can refresh rather than re-ask. |
| 2026-07-26 | Three 422 codes rather than one generic one: `empty_requisition`, `reason_required`, `supplier_required` | §9.8 says only "422 business-rule violation". Each of these is a different missing thing and the screen's right response differs — add a line, focus the reason box, show a supplier picker. One code would put every business refusal in the same banner and make the client parse prose to tell them apart, which is what §9.8's whole `error`/`message` split exists to avoid. |
| 2026-07-26 | **Editing a draft requisition replaces its lines, which is a `DELETE`** — the second and last exception to I5 | I5 says master data soft-deletes, documents cancel, ledgers append; a draft's lines are none of those three. A draft is a form the user has not committed: nothing references its lines, `005` grants `erp_app` `DELETE` on `purchase_requisition_lines` deliberately (only the ledger tables are revoked), and the `qty > 0` CHECK means a line cannot be zeroed out instead. Without it, a mistyped line can only be fixed by cancelling and re-keying the requisition — consuming a document number to fix a typo, which is exactly the gap §9.6.1 calls "creatable but not editable". Scoped to `lines` on a `draft`, by its author. Once submitted, the status check freezes them (C5). |
| 2026-07-26 | A requisition line's `est_unit_cost` defaults to the product's `standard_cost`, not to zero | The column defaults to 0 and nothing would complain — but that zero is copied to the PO line as `unit_cost`, and from there to the goods receipt's journal entry, which would post **Dr 0 / Cr 0**. `jel_balanced` passes it happily: zero equals zero. A balanced entry for nothing is worse than an error, because it looks fine, and Session B's confirmation panel is the screenshot the project exists for. Same reasoning as Phase 4's adjustment `unit_cost`. |
| 2026-07-26 | Approval sets the PO's `expected_at` to today **in the tenant's timezone** plus the supplier's `lead_time_days` | §8.3 lists five steps and does not mention `expected_at`, but the column exists, `suppliers.lead_time_days` exists and is otherwise never read, and the PO list has a column with nothing in it. One expression in the insert. The date is computed `(now() AT TIME ZONE t.timezone)::date`, which is the §2.5.3 rule, and crosses the wire as `YYYY-MM-DD` text rather than as an instant so no browser can render it a day early. |
| 2026-07-26 | Cancelling a requisition or a purchase order **requires a reason** | §6.9.2: cancellation "records who cancelled, when, and why — the same shape as approve and reject", and rejection's reason is mandatory both in the handler (C3) and in the database (G13). No CHECK constraint enforces the cancel reason, so this one is the handler's alone; noted in case a later phase wants the constraint. |
| 2026-07-26 | Approval checks the **status before** the self-approval rule | Both are refusals and either order passes C2 and C4. But telling the author of an already-approved requisition "somebody else has to approve this" sends them to argue with the wrong person, when the answer is "it was approved yesterday". Status first, then segregation of duties. |
| 2026-07-26 | `wantsDeleted` / `refuseDeletedView` moved to `validate.go` and take a module code | Second concrete use case, which is the bar §4 sets. The rule is identical in every module and the *level* is not — products answer to inventory `admin`, suppliers to procurement `admin` — so the module is the parameter and nothing else changed. |
| 2026-07-26 | `MasterDataList` and `useRowActions` extracted, and Phase 4's `WarehouseList` refactored onto them | §12A.4 in as many words: "catching a duplicated form component after two copies is a ten-minute fix; after five it is an afternoon." Suppliers is the second copy, and `npx fallow audit` reported the pair as a 220-line clone. What is shared is the scaffolding — search, the recycle-bin toggle, the four §10.7.6 states, pagination — so the fields stay with their entity behind a `row` render prop rather than moving into a config object that would grow a case per column type. Clone groups fell from 24 to 19 and the two largest disappeared. |
| 2026-07-26 | The requisition detail screen opens **read-only, with an explicit Edit** | The opposite of Phase 4's product detail, whose live-form-on-load was the biggest of the three things the walkthrough disliked: there was no reading mode and no point at which you had committed to changing something. Warehouses and suppliers already work this way, so this makes three screens out of four agree; product detail is the outlier and is worth revisiting. |
| 2026-07-26 | The frontend route is `/procurement/orders`, the API path is `/procurement/purchase-orders` | Both are as specified — §10.3 names the screen, §9.4 names the endpoint. They differ, and that is fine: the URL a person reads and the URL a client posts to are different audiences. Recorded because it looks like a typo when you meet it. |
| 2026-07-26 | `POST /procurement/requisitions` always creates a **draft**; submitting is a second request | §10.3's create form offers "save draft or submit", which could have been a `submit: true` flag. Two calls instead: a client that dies between them leaves a draft the user can find and finish, rather than a document in a state nobody chose, and the second call's failure is reported instead of being folded into the first. |
| 2026-07-26 | **The "401 rather than 404 means the route is wired" live check from Phases 3–4 does not prove that** | Measured this phase: `GET /api/nonexistent-path` also returns `401`. The auth chain is group middleware on `/api`, so it answers before the router can 404 — the 401 says the chain is mounted and nothing at all about the route table. What does prove it is `TestProcurementRoutesCarryTheLevelsFromTheSpec` (and its inventory twin), which walks every route in the spec table against the real `api.New` app and asserts the level each is registered at. |
| 2026-07-26 | `testsupport.WarehousesPath` → **`TenantTxPath`**, and the route itself `/api/warehouses` → `/api/probe/tenant-tx` | Carried from Phase 4, fixed now. The old name was fine while no such endpoint existed and misleading the moment `/api/inventory/warehouses` shipped: someone reading a failing transaction test would find a real endpoint one path segment away and reasonably assume they were the same thing. It now sits in the `/api/probe/` namespace with the other test-only routes, where nothing real can collide with it. The handler is `tenantTxProbe`; it still reads warehouses, because that is what proves the transaction is live. |
| 2026-07-26 | `/admin/tenants/new` reads its module list from **`GET /admin/modules`** instead of a constant in the file | Carried from Phase 4, which flagged `listModuleCatalogue` as an unused export and called it "the interesting one". Deleting the client function would have been the smaller change; wiring it up is the right one, because the hardcoded array was a second copy of the `modules` rows that nothing kept in step — and a fourth module added to the catalogue would have been invisible to the one screen whose job is choosing modules. The names and descriptions it now renders are identical to what was hardcoded, so nothing changed on screen. |
| 2026-07-26 | `apiFetch` is no longer exported | Every endpoint has a named wrapper with a return type, and nothing outside `lib/api.ts` used it. A screen reaching past the wrappers would be a request whose shape nothing checks — the first step towards one URL spelled two ways. |
| 2026-07-26 | The §10.7.3 **bottom tab bar** now exists, below `md` | Deferred through Phases 3-4 on the grounds that "a tab bar with one tab is not a navigation aid". Procurement makes that argument expire: Home, Requests, Orders, and Stock is four destinations. It is a *shortcut* and not the whole map — suppliers, warehouses, and settings stay in the drawer, because master data is not what anyone reaches for one-handed in a warehouse aisle, which is the case §10.7.3 gives for the bar existing at all. Tabs respect entitlements exactly like the sidebar, and `tabItems` still returns nothing when fewer than two would show. |
| 2026-07-26 | **The idempotent replay uses a SAVEPOINT, not §8.6.1's "second transaction"** | §8.6.1 is right that a bare unique violation aborts the transaction and that the replay then needs a fresh one. A `SAVEPOINT` before the `goods_receipts` insert removes the premise: `ROLLBACK TO` releases the failure and leaves the *same* transaction usable, so the lookup runs on the connection that already has tenant context, no second connection is held open while the first still holds locks, and TenantTx's `COMMIT` is a real commit rather than one issued against an aborted transaction — which returns the tag `ROLLBACK` and would silently discard a 200 response's own writes. Under READ COMMITTED the post-rollback read takes a fresh snapshot, so it sees the row the racing transaction committed. The savepoint also wraps the `docnum.Allocate` call, so a detected replay gives its GR number back instead of leaving a gap. |
| 2026-07-26 | The idempotency key is read **twice** — before the order lock and again after it — and the unique violation is only the last backstop | The ordinary replay is answered by the first `SELECT` and never takes a lock or touches a constraint. The second read exists because a test found the bug the first one cannot catch: a twin retry still in flight at that moment commits while this request waits for the lock, and everything after the lock then judges the receipt against quantities its own twin has already booked — answering `422 over_receipt` to the person whose receipt worked. The two reads are not redundant. The first must precede the *status* check, or a retry of a receipt that completed its order is told the order is `received` (409). The second must follow the *lock*, or it cannot see the twin. Both paths return the same body because both call `receiptResult`, which rebuilds the whole response from the committed rows rather than from anything remembered. |
| 2026-07-26 | A fresh receipt is **201**, a replay is **200**, and the body carries `replayed: true` | §8.6.1 pins only the replay at 200. 201 for the creation is the ordinary meaning of the verb, both are `response.ok`, and the flag lets the confirmation panel say "had already been posted" rather than "posted" — which is the truthful sentence, and the one that stops a user wondering whether they have now received the goods twice. |
| 2026-07-26 | `Idempotency-Key` must parse as a **UUID** | §8.6.1 says "missing or malformed → 400" without defining malformed. Requiring the UUID the contract says the client generates is what makes the word mean something: the failure this header exists to prevent is a key that *repeats across forms*, and a key that is a timestamp, a form id, or a PO number does exactly that while looking perfectly well-formed. |
| 2026-07-26 | Two new 422 codes: `empty_receipt` and `idempotency_key_reused` | Same reasoning as Session A's three. A receipt with no lines is refused where an empty requisition is (`empty_requisition`), for symmetry of code shape rather than of endpoints. `idempotency_key_reused` is a key already spent on a receipt against a *different* order: the friendly reading — hand back that other order's receipt — is the dangerous one, because the phone would then report goods arriving against an order nobody touched. |
| 2026-07-26 | **The receipt locks the purchase order header as well as its lines**, header first — and the lock's necessity was measured rather than assumed | §8.6.3 asks only for the affected lines, which is enough for over-receipt (H5). It is *not* enough for step 4: two receipts each completing a different line both re-read `po_line_status`, each sees the other line outstanding because the other transaction has not committed, and both write `partially_received` — leaving an order half-received with nothing outstanding on it, which no screen can explain and no later receipt can fix. Header-then-lines is also a fixed lock order, so two receipts on overlapping subsets cannot deadlock. **Neither lock mutation makes any test fail**, and the reason is worth knowing: `docnum.Allocate` upserts the tenant's GR sequence row, and *that* row lock serialises every receipt in a tenant long before either reaches step 4. Give each request its own sequence row and the bug appears 10 times out of 10; restore the header lock alone and all 10 pass. The locks stay explicit because the serialisation this handler needs must not depend on how documents happen to be numbered. Recorded in the comment on `TestConcurrentReceiptsOnDifferentLinesCloseTheOrder`. |
| 2026-07-26 | The cross-module halves are **`inventory_receipt.go`** and **`finance_journal.go`** in `internal/api`, not new `internal/inventory` and `internal/finance` packages | §8.4's note names `procurement.PostGoodsReceipt(tx, actor, poID, req)` calling "inventory and finance service functions, passing the same `tx`". Three new packages would be the first service layer in a codebase that has none — every handler in Phases 3–5A is a method on `server` running raw SQL on the request's transaction — and §4 says not to abstract before the second concrete use case. What the note is actually protecting is that the three modules' writes are visibly one transaction, and file boundaries carry that: `postReceiptStockLedger` and `postReceiptJournal` each take `tx` as their first argument and are called from labelled `[INVENTORY]` and `[FINANCE]` steps. Phase 6 is where a finance package earns itself, if it does. |
| 2026-07-26 | A missing `1300`/`2150` account is a **500**, not a business refusal — and that is what D8 injects | The chart of accounts is seeded when the workspace is created (§4.2.1) and nothing in the MVP can remove an account, so its absence means the database has been edited by hand. §9.8 has no code for "this workspace is misconfigured", and inventing one would put a code in the contract that tells a warehouse clerk to go and create a ledger account. The valuable consequence is that **D8 needs no test hook in production code**: soft-deleting `2150` fails step 6 on the real path, after the receipt, its lines, the status change, and two ledger rows are already written — so what rolls back is exactly what would roll back in production. |
| 2026-07-26 | §9.4's `GET /goods-receipts` and `GET /goods-receipts/:id` were both built, though the Session B brief says "one route to each side" | §9.4 lists three receipt routes at `viewer`, and `TestReceiptRoutesCarryTheLevelsFromTheSpec` walks that table — a route in the spec and not in the router is a gap the test would find in Phase 6 rather than now. The list is also what the order screen's receipt history reads. There is deliberately **no `getGoodsReceipt` client wrapper**: no screen reads a single receipt, and an unused client function is the symmetry §9.6.1 warns about. |
| 2026-07-26 | The stock ledger gained a **`sourceId` filter** and resolves `sourceNumber` / `sourcePoId` | The confirmation panel says "2 stock ledger entries created" and §10.3 requires that to be a link. Without the filter it is a claim the reader cannot check; without the resolved number, §10.4's "rows linked to source documents" is a UUID. Both are a `LEFT JOIN goods_receipts` guarded on `source_type`, shared by the list and the single-row read so they cannot disagree. The alternative — writing the GR number into `stock_ledger.note` — would have duplicated a value nothing keeps in step. |
| 2026-07-26 | The confirmation panel's **finance line does not link**; the inventory line does | §10.3 wants both lines to link, and one of them cannot yet: `/finance` is Phase 6's page (§10.5). Linking early would not even 404 — `App.tsx` redirects unknown paths to the dashboard, so the click would quietly land the reader on the home screen, which is worse than text in the one screenshot this panel exists for. Building the target instead would mean doing two of Phase 6's three build items inside Phase 5, which is what the phase split exists to prevent. So the finance line shows the JE number and both account names and amounts, and carries a `TODO(phase-6)` naming the exact one-line change and why it waits. Recorded in *Current state* as well, because that is what the next session reads. |
| 2026-07-26 | **`TableHead` and `Column` extracted to `components/ListStates.tsx`**, and all eleven tables in the application adopted it | The receipt form and the receipt history made this the *fourth* copy of the same nineteen lines, and Phase 5A recorded the trigger in as many words: "worth another look if Session B adds a third of either". Only the heading row moved. The cells stay with their screen, because a column's content is where the decisions live — a link target, a deleted-product marker, a signed delta — and a config object rich enough for those would grow a case per column type. `MasterDataList` already had a private copy of exactly this loop, so `Column` moved to sit beside the shared version and `MasterDataList` now renders `<TableHead columns={columns} />` like everything else. Clone groups fell from 15 to 10. **`Column` has exactly one import path**: an `export type { Column }` re-export from `MasterDataList` shipped briefly and was removed — one type reachable by two paths is the same failure as one URL spelled two ways, and it is how a later screen ends up importing the "other" `Column` and nobody noticing they have drifted. `WarehouseList` and `SupplierList` import it from `ListStates` alongside the component that consumes it. |
| 2026-07-26 | `OrderDetailPage` was split into `OrderSummary`, `OrderLines`, `OrderProgress`, `CancelOrderPanel`, and `ReceiptHistory` | The receipt history and the two Receive-goods entry points took it to 299 lines and 20 cyclomatic — CRITICAL, and worse than `RequisitionDetailPage` was before Session A split it the same way. Now 104 lines and 13. `CancelOrderPanel` owns its own three state variables, because whether a reason box is open is nothing the rest of the screen needs to know. Still labelled CRITICAL, like its requisition twin, and further splitting would fragment a page component for a metric's sake. |
| 2026-07-26 | **Every browser walkthrough is deferred to Phase 7 and done in one pass**, rather than one per phase | Phase 7's MVP gate is already the full twenty-five-step acceptance test at 360px in both themes, which walks the whole application — so a per-phase walk means walking the same flows five times, and only the last one is performed against the finished thing. The cost is real and is written down rather than glossed: problems then arrive in bulk, and Phase 4's walk found three in an hour on one module. Recorded prominently in *Current state* with the list of screens nobody has opened, because "no walkthrough yet" must not read as an oversight to the session that finds it. |
| 2026-07-26 | **The journal list returns each entry with its lines**, rather than the entry alone | An entry without its lines is a number and a description; what makes a posting legible is Dr 1300 against Cr 2150, and that is the whole reason §10.5 shows a live list rather than a count. It also makes "the entry is balanced" answerable from one response, which is what Phase 7's acceptance step needs. Cost is one extra query per page (`attachJournalLines`), not one per row. |
| 2026-07-26 | The entry's `amount` is the **debit** side only, summed in SQL | The credit side is equal by construction — `jel_balanced` is a deferred constraint trigger that refuses to commit anything else — so presenting both as "the amount" would invite somebody to compare them in Go or TypeScript to check. The one place that comparison is made is `assertJournalBalances`, in SQL, where both operands are still NUMERIC (I8). |
| 2026-07-26 | `GET /finance/journal-entries` takes an `accountId` filter, implemented as an **`EXISTS`** rather than a join | "Which entries touched 1300" is a question about entries, and a join would return an entry twice if it ever debited and credited the same account — inflating both the page and the total. Nothing in the MVP posts such an entry, so this is a shape chosen to be right rather than one a test can currently fail. The filter has a user: the chart-of-accounts chips on `/finance` are what set it. |
| 2026-07-26 | The Finance page shows the **chart of accounts**, which §10.5 does not ask for | §9.6.1's completeness table marks Accounts *List* ✅ and says in as many words that "every ✅ above needs a working UI, not just an endpoint". Without it `GET /finance/accounts` would be an endpoint no screen calls and `listAccounts` a client function nothing imports — the same unused-symmetry problem that had `getGoodsReceipt`'s wrapper removed in Phase 5B. Two chips above the journal, each filtering it. |
| 2026-07-26 | Accounts have no `?includeDeleted=true`, unlike every master-data list | The recycle bin exists because things get deleted. Nothing in the MVP deletes an account — there is no endpoint, and §9.6 says the chart is seeded rather than user-managed — so the parameter would be a view onto a state no code path produces. The column is still read (`deleted_at IS NULL`), because D8 soft-deletes `2150` by hand to prove the rollback. |
| 2026-07-26 | **`SourceFilterNotice` extracted to `components/ListStates.tsx`**, and `LedgerPage` refactored onto it | The finance page's `?sourceId=` banner is the same twenty-five lines as the ledger's with one noun changed, and §4 sets the bar for abstracting at the second concrete use case — which this is, exactly as `MasterDataList` was for suppliers. The component owns the URL parameter as well as the markup: a version where the caller deleted `sourceId` itself would render a button whose behaviour lived in another file. |
| 2026-07-26 | The local superadmin was provisioned by a throwaway, deleted after use, rather than by `cmd/seed` | Carried from Phase 3 — `/admin/*` had been unreachable by hand for three phases. Phase 7 owns the seed script and §3.5.3 already specifies the deterministic-UID shape it should take, so building it now would be doing that work twice and probably differently. The throwaway followed §3.3's order (provider account first, row second, compensating delete on failure); the account and the SQL are recorded below so the next person does not need the program. **Superseded: `cmd/seed` now creates `super@erp.test`.** |
| 2026-07-26 | **`/api/me`'s `moduleRoles` is now the EFFECTIVE map — what `LevelFor` resolves — not what `user_module_roles` stores. This was a bug, not a refinement** | A tenant admin correctly has *no* role rows (§5.4, B7), so the stored map is empty for them. `/api/me` sent that, and `AppShell`'s nav is driven straight off it — so Rina, the seed's Nusantara admin and the subject of acceptance step 10, signed in to a sidebar with no modules, no bottom tabs, and no way to reach a single screen, while every endpoint behind those links answered her perfectly. It survived four phases because the only account anybody signed in with by hand had explicit rows, which makes the two maps identical; B7 could not catch it because B7 asks whether an implicit admin gets *past the gate*, and she always did. `lib/levels.ts` already documented the contract the server was breaking: "the server intersected the user's levels with the tenant's entitlements and applied the implicit-admin rule before sending it." It had not. Fixed by routing through the same `effectiveRoles` the user-admin screens use, so there is one implementation of §5.4; **B13** is the test, and it fails on the old line. Found by the Phase 7 API pass, at exactly the place the deferral note said to expect it. |
| 2026-07-26 | **`postGoodsReceipt` was extracted from its handler, and refusals became `*refusal` values** | §15 requires the seed's receipts to go "through `PostGoodsReceipt` rather than inserting ledger and journal rows directly — that way the seed exercises the same code path as the application and cannot drift from it", and §8.4's own note writes the signature. The seed has no `*fiber.Ctx`, so the business outcomes had to leave as errors. Rather than a second vocabulary, `validate.go` gained `notFoundError` / `stateConflictError` / `unprocessableError` / `malformedError` returning a `*refusal`, and the existing `notFound` / `stateConflict` / … became one-line wrappers that `write` one — so a code, a message, or a `details` payload cannot differ between "the endpoint refused" and "the function refused", and **every existing call site is unchanged**. Note the shape against Trap 1: a `*refusal` is a *real* error, so `if err != nil` fires on it; `httpx.Fail` still returns nil and only `write` calls it. The whole of Groups D and H — idempotency, over-receipt, the D8 rollback, both concurrency tests — passed unchanged, which is the evidence the extraction preserved the semantics. |
| 2026-07-26 | The seed writes requisitions and purchase orders as **SQL**, and only the receipts through the application | Two reasons, and the first is the phase brief's: §15 singles out the receipt because it is the one event that writes to three modules at once, and a second implementation of *that* would be a second place atomicity could be wrong. A requisition is a document with no consequences beyond itself. The second reason is dates: §15 wants orders spread over sixty days with `expected_at` "a mix of past and future", and approval computes `expected_at` as **today** plus the supplier's lead time (§8.3) — so every seeded order would be expected next week and "overdue" could not be demonstrated at all. Numbers still come from `docnum.AllocateAt` dated at the document's own instant, so a requisition raised in June is `PR-202606-…` rather than filed under this month. |
| 2026-07-26 | Goods receipts are dated **now**, while the orders behind them are historical | `PostGoodsReceipt` takes no timestamp, and giving it one would add a production seam for the seed's benefit — the opposite of the reasoning that justified `docnum.AllocateAt`, which exists because E5 *cannot fail* without a controllable clock. Here nothing fails without one. And the result is more realistic rather than less: orders placed across sixty days whose deliveries arrived recently is what a lead time looks like. The ledger still spans the window, because the twenty opening balances and eight manual adjustments per tenant do. |
| 2026-07-26 | The seed upserts `tenants` on **slug** and `users` on **email**, taking the id the row actually has | Everything else uses a UUIDv5 derived from what it is, and conflicts on the id. These two cannot: a database that has been developed against already holds hand-made rows with ids nothing derived, and `ON CONFLICT (id)` collides with `tenants_slug_key` and aborts. It did, on the first run. Adopting the existing row means the seed can be run against a working database without destroying what is there — which is also why the Phase 2–3 scratch rows are still present and were not deleted. |
| 2026-07-26 | `auth.Firebase` gained `EnsureSeedUser`, and it is deliberately **not** on the `UserManager` interface | §3.5.3's deterministic UIDs need `CreateUser` with an explicit UID, and `UserManager` is the interface the two provisioning *endpoints* reach the provider through. Adding "create an account at a UID of your choosing" to it would put that within reach of a request handler for the benefit of a build-time tool. `cmd/seed` depends on the concrete `*auth.Firebase` instead, which is the honest dependency. An address already belonging to a *different* UID is an error rather than a shrug: pressing on would write a `users` row whose `firebase_uid` names nothing, which is §3.3's whole failure mode. |
| 2026-07-26 | The dashboard route carries **no `RequireModule`**, and a widget the caller cannot read is **absent** rather than empty | It is the only route below `/api` that spans modules: its four widgets answer to two of them, and a caller entitled to one must get that one rather than a 403 for the other. So the gate is inside, per widget, through the same `LevelFor` as everything else. Absent rather than empty is the load-bearing half — `{"lowStock":{"count":0}}` says "nothing is low", and telling somebody who cannot see Inventory that nothing is low is a lie the nav does not tell. A superadmin has no tenant and so gets `{}` rather than a 500. |
| 2026-07-26 | "Open purchase orders" means **outstanding** — `open` *and* `partially_received` | `open` is a specific status in the naming contract, so this reading needed a decision. An order half of which has arrived is emphatically still an order somebody is waiting on, and a widget that dropped it the moment the first box landed would count down to zero while goods were in transit. The value is the whole ordered value, not the outstanding remainder: that is the commitment the company has made, which is the question a dashboard number answers. |
| 2026-07-26 | The approval queue **keeps the caller's own submissions**, and the count includes them | Filtering them out is the tempting thing — C2 refuses self-approval for everybody — and it would make the count above the list disagree with the list itself. "3 awaiting approval" over two rows is a bug report waiting to be filed. The row carries `requestedById`, the screen compares it with `me.user.id` and disables its own buttons with the reason, and the server refuses regardless (I12). Same for a requisition with no supplier: the queue has nowhere to pick one, so the row says to open it. |
| 2026-07-26 | `lowStockFrom` / `lowStockSelect` and `ledgerSelect` / `ledgerFrom` were extracted, so the widget and the list ask one question | The widget says "4 products are low" and links to a list that must show four rows; two copies of "below reorder point" is how one gains a clause and the pair silently stops agreeing. **L4 asserts the widget's count equals the list's total**, which is a test about drift rather than about arithmetic. The ledger projection was already an exact duplicate between `listLedger` and `ledgerEntry` before the widget made it a third. |
| 2026-07-26 | The low-stock shortcut carries **quantities** in the URL: `?products=<id>:<qty>` | §10.2 asks only that the shortcut "pre-fills them", and ids alone would satisfy that — but a requisition for one unit of something short by forty is a shortcut that produces a wrong document. The quantity is the shortfall PostgreSQL computed, and putting it in the URL keeps the link a link (copyable, bookmarkable, no second request) and needs no Inventory access on a Procurement form. Nothing is decided by it: it is what the box is pre-filled with, and the server validates whatever is finally sent. |
| 2026-07-26 | Card transformation uses **`useCompact()`**, a media-query hook, rather than `md:hidden` | §10.7.4: "render a genuinely different component below the breakpoint rather than CSS-hiding cells — a `<td>` styled to look like a card still announces as a table cell." The mirror image is just as bad: rendering both and hiding one puts every row in the accessibility tree twice, and a screen reader reads the invisible copy. `useSyncExternalStore` rather than an effect, so the first render already knows and no phone paints the desktop table for a frame. Everything that is *merely* layout stays in Tailwind. |
| 2026-07-26 | Cards for the two **document** lists; frozen first column for the two **grids** | §10.7.4 says choose per screen and be able to say why. Requisitions and orders are read one row at a time, in modest numbers, and are what §10.7.1 puts on a phone. The stock grid and the ledger exist for horizontal comparison across many columns, and a stack of cards throws exactly that away — so they scroll sideways with the row's identity pinned. The frozen cell needs an *opaque* background and therefore has to follow the row's hover, which is what `group-hover` on `frozenCell` is for; and the header wins the z-index collision §10.7.4 warns about, because the header is what says what the frozen column *is*. |
| 2026-07-26 | `ScrollableTable` uses `overflow-auto` with a max height, not `overflow-x-auto` | A container with `overflow-x: auto` computes `overflow-y` to `auto` as well, so it is already a scroll container and a `sticky top-0` header inside it stops tracking the page. Making both axes explicit and capping the height puts the sticky header and the frozen column in the same scroll box, which is the only way the two work at once. The cap only bites when there are more rows than fit. |
| 2026-07-26 | `StickyActions` is `sticky`, not `fixed` | A sticky element keeps its place in the flow, so nothing needs a spacer underneath and nothing can end up hidden behind it — the two bugs a fixed bar produces. `bottom-14` clears the bottom tab bar. |
| 2026-07-26 | **The touch-target audit found four real failures**, one of which had a comment claiming otherwise | `ThemeToggle` was ~26px, the `SourceFilterNotice` clear button and the two sign-in mode buttons had no height at all, and the toast dismiss carried the comment "`-m-2 p-2` keeps the 44px target of §10.7.5" while measuring about 36px. That last one is the useful lesson: a comment asserting a measurement is not a measurement. `inputmode` needed no work — every quantity and money field already had it from Phases 4–5. |
| 2026-07-26 | `ResponsiveList` was extracted, and clone groups fell from **8 to 1** | The Phase 6 log named the target — "the pagination-and-empty-state block still wants a `DataTable` wrapper" — and the card transformation is what made it worth building: a second view doubled the scaffolding, and `fallow audit` reported the two screens as a 28-line and a 22-line clone the moment the second copy landed. What it does *not* own is the cells and the card fields: `row` and `card` are render props, for the same reason `TableHead` shares only the heading row. The one remaining clone group is the inherited pagination block on the two grids, which are not card-transformed and so do not use it. |

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

**Added in Phase 5A** — the platform superadmin, which the local database had
never had (carried open from Phase 3). Not in a tenant, and deliberately not in
the table above: it belongs to no workspace.

| Email | Firebase UID | Password | Role |
|---|---|---|---|
| `superadmin@example.test` | `wBz85wzGcHfSdXMBtUYwM0JDsK63` | `superadmin123` | `superadmin`, `tenant_id NULL` |

Verified by signing in and calling the API: `/api/me` returns
`tenantRole: superadmin` with `tenant: null` and no module roles, and
`/api/admin/tenants` returns the workspace list.

**To recreate it on a fresh database** — Phase 7's seed script replaces this:

1. Create the Firebase account (console, or `auth.Firebase.CreateUser`), and note
   the UID it returns.
2. Insert the row, which needs no tenant and gets none — the
   `users_superadmin_has_no_tenant` CHECK makes `tenant_role = 'superadmin'` and
   `tenant_id IS NULL` biconditional:

```sql
INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role)
VALUES (gen_random_uuid(), NULL, '<uid>', '<email>', 'Platform superadmin', 'superadmin');
```

Provider account first, row second (§3.3): a `users` row pointing at a UID that
does not exist is a login that fails with no clue why.

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
- **`npx fallow dead-code` has ten standing findings, none of them blocking**
  (the tool is `reference/discipline.md` §12A.4, and Phase 5's checklist gates on
  `npx fallow audit`, which only looks at files changed since the last commit —
  so these surface only if their file is touched again).
  - Phase 4's one real hit, `formatDate`, is deleted.
  - `apiFetch`, `listModuleCatalogue`, and `firebase.app` are unused exports from
    Phases 2–3. `listModuleCatalogue` is the interesting one: `/admin/modules`
    exists and is tested, and no screen calls it — the create-tenant form has the
    three module codes inline. Worth resolving one way or the other.
  - The flagged type exports — `ListResponse`, `StockCell`, `LowStockRow`,
    `ModuleCatalogueEntry`, `AsyncState`, `AuthState` — appear only inside
    exported function signatures. Removing the export would make those
    signatures unusable by a caller, so this is a false-positive class and
    should be suppressed rather than "fixed" if it ever becomes noisy.
  - `tailwindcss` is reported as a dev dependency used in production. It is a
    build-time tool and belongs where it is; Phase 0 put it there deliberately.
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

## Phase 5, Session A — 2026-07-26

**Done:** a requisition can be raised, edited while it is a draft, submitted,
approved, rejected, or cancelled — and approval generates the purchase order, in
the same transaction, with a number allocated from the tenant's own monthly
counter. Seventeen `/api/procurement/*` routes cover suppliers (full CRUD, soft
delete, restore, the in-use check), the requisition lifecycle, and the purchase
order read side plus cancellation. Every route carries its own `RequireModule` at
the level §9.4 gives it.

Every state transition takes `SELECT … FOR UPDATE` on the requisition **before**
reading its status, which is what makes two managers tapping Approve produce one
purchase order and one clean 409 rather than two orders (§8.6.2).

Received quantity is derived everywhere it appears: the order list and detail read
`po_line_status`, and there is no `qty_received` column to drift from it (I6).

Screens: `/procurement/requisitions` (status filter chips),
`/procurement/requisitions/new`, `/procurement/requisitions/:id` (lines, status
timeline, and the actions this reader may actually take, including editing a
draft), `/procurement/orders` (status and supplier filters),
`/procurement/orders/:id` (ordered against received per line), and
`/procurement/suppliers`. `AppShell` gained the Procurement nav item and its three
sub-items.

**Tests green:** C1–C6, E1–E5, G4, G6, G7, G8, G12–G14, and **H4**, plus Groups A,
B, F, I–J unchanged. **145 top-level tests** (217 including subtests), up from 100
at Phase 4. `go test ./... -p 1` clean, `go vet` clean, `gofmt` clean, `tsc
--noEmit`, `npm run build`, and `oxlint` all clean — oxlint reports only the four
pre-existing fast-refresh warnings and none in new code.

Group D and the rest of Group H are the goods receipt, which is Session B. **H4
was written now**, though it is a Group H test, because this is the phase that
builds the lock it is about — a `FOR UPDATE` nothing exercises is a comment.

Beyond the listed IDs:

- **`TestProcurementRoutesCarryTheLevelsFromTheSpec`** walks every §9.4 route and
  asserts the level it is registered at, including the two that are lower than
  they sound: `/cancel` and `/submit` are `user`, because §6.9.2 gives the
  *creator* rights that no role level expresses, and the rest is decided against
  the row.
- **C2 is asserted against a tenant admin as well as an approver.** A tenant admin
  resolves to `admin` in every module implicitly, so a self-approval rule written
  in the middleware would let exactly the wrong person through.
- **Isolation from two tenants**, through both lists, by ID on all three
  documents, and on three different writes — with tenant B's own data re-read
  afterwards, so the emptiness is a filtering result rather than a broken fixture.
- The §9.0 list contract on the procurement lists, including that the status
  filter is a server parameter: `?status=draft` returns a `totalItems` for the
  whole result set, not a count of the current page.
- A refused requisition **does not consume a document number**, asserted through
  HTTP as well as by E4 directly.

**Three mutations were run to check the tests bite rather than merely pass. All
three were caught:**

| Mutation | Result |
|---|---|
| `FOR UPDATE` removed from `lockRequisition` | **caught** — H4 got `500 internal_error` where it wanted a clean 409 |
| `total_amount` computed as `SUM(qty) * SUM(cost)` | **caught** — C6 reported `29457.75, want 16001.50` |
| `AND p.deleted_at IS NULL` added to the order line's product join | **caught** — G6 reported "the order has 0 lines, want 1" |

The first is the interesting one. Without the lock, the *database* still prevented
the second purchase order — `pr_terminal_immutable` refused the second UPDATE — so
no data was corrupted. What the lock buys is the difference between a 409 the
screen can explain and a 500 the user has to guess at. Belt and braces, with the
belt doing the user-facing work, exactly as §8.6.3 describes for the receipt.

The third is the mutation a later phase makes by reflex, and it is Trap 3 in *What
a module phase inherits*. `TestG6OrderLinesStillResolveADeletedProductsName` now
asserts it at the endpoint a user would notice, not just at the query level.

**Live check against the local stack:** migration `006` applied cleanly to the
running database and `pg_constraint` confirms the new definition. The API binary
on `:8080` was rebuilt and replaced; it boots and `/api/health` is 200 without a
token.

**One thing earlier phases recorded is wrong, and is now corrected in
*Decisions*:** "all N routes return `401` rather than `404` — they are wired" does
not follow. `GET /api/nonexistent-path` returns `401` too, because the auth chain
is group middleware on `/api` and answers before the router can 404. The route
table is proved by the route-levels test, not by curl.

**Deviations from spec:** fifteen, all recorded in *Decisions taken* above. The
four worth knowing before Session B:

- **`docnum.Allocate(tx, tenant, docnum.GR)`** takes the caller's transaction. The
  receipt's GR number and the journal entry's JE number both come from it, in the
  same transaction as the rest, or a rollback consumes a number.
- **`state_conflict` is the 409 for a document that has moved on**, and
  `unprocessable(c, "over_receipt", …)` is the shape Session B's 422 should take.
- **A requisition line's cost defaults to the product's standard cost**, so the
  journal entry Session B posts has a non-zero value to work with. A PO whose
  lines cost zero produces a perfectly balanced Dr 0 / Cr 0 posting.
- **Migration 006 exists**, so Session B's migrations start at `007` if it needs
  one. It should not.

**TODO(post-mvp) markers added:**
- `frontend/src/lib/requisitionForm.ts:48` — replace the product `<select>` with a
  search-as-you-type picker; it loads the §9.0 maximum of 100 and stops there.

`frontend/src/pages/procurement/OrderDetail.tsx:22` carries a plain comment, not a
marker: the receipt history and "Receive goods" belong on that screen in Session B,
which is this phase rather than after it.

**Known broken / left half-done:**

- **Session B is the whole point of the phase and is not started.** `POST
  /purchase-orders/:id/receipts`, Group D — especially **D8** — and the rest of
  Group H (H1–H3, H5, H6) are all outstanding.
- **No live browser walkthrough of the six new screens.** They build and lint
  clean and the API is covered end to end, but nobody has signed in and raised a
  requisition by hand. The dev account (`dgjy2019@gmail.com`) is a tenant admin,
  so it can do every step *except* approve its own requisition — a second user
  with `procurement: approver` is needed to walk the approval, and Phase 4's
  walkthrough found three real problems, so this is worth doing before Session B
  rather than after.
- **`npx fallow audit` is not fully clean**, and the phase's "done when" asks for
  it at the end of Session B. **Dead code is down to one finding** — `tailwindcss`
  reported as a dev dependency used in production, which Phase 0 put there
  deliberately and Phase 4 already ruled correct. What is left elsewhere:
  - **20 clone groups**, down from 24 — the two largest (220 and 223 lines) are
    gone, replaced by `MasterDataList`. The residue is 20-40 line fragments: two
    table-header blocks between the requisition and order lists, and the
    Edit/Save/Cancel button cluster shared by the two master-data rows. Below the
    "duplicated component" bar; worth another look if Session B adds a third of
    either.
  - **`RequisitionDetailPage` is still flagged CRITICAL** at 138 lines and 18
    cyclomatic, down from 305 and 32 after `Summary`, `LinesTable`, and
    `ActionsPanel` were split out. Further splitting would fragment a page
    component for a metric's sake.
  - `min-w-[36rem]` and friends are advisory `css-token-drift`. A table's minimum
    width is a one-off measurement per table; adding scale tokens for six of them
    would be worse.

  **Four `fallow-ignore-next-line unused-type` suppressions now sit in
  `lib/api.ts`**, on the four types fallow reports as unused exports and which are
  each the return type of an exported function — removing the export would make
  that signature unusable. Phase 4 identified the class and said to suppress
  rather than "fix"; this is that. Note the directive takes **only issue kinds**
  after it: prose on the same line is parsed as further kinds and comes back as
  two dozen "stale suppressions", which is how the first attempt went.
- **Still no frontend tests at all.** §12.5 defines them; no phase brief has asked
  for them, and the count is now six more untested screens.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it,
  and H4 is now the second test that would genuinely benefit.
- A freshly built API binary is running on `:8080`, having replaced the Phase 4
  one. `make dev` restarts everything.

**Five things carried from earlier phases were closed in this session**, all
recorded in *Decisions taken*:

- **The password reset email is confirmed working** — the last open item from
  Phase 2, carried through three phases. The flow is end to end: the email
  arrives, the link opens, and the reset completes. The `callbackUri` caveat in
  the Phase 2 log still stands for Phase 9 — the emailed link lands on Firebase's
  hosted page rather than this app's `/auth/action` — but the mechanism works.
- **A superadmin exists in the local database**, so `/admin/*` is reachable by
  hand for the first time. Account and recreation SQL are in *Project facts*.
- **The `/api/warehouses` probe route is renamed** `/api/probe/tenant-tx`.
- **`listModuleCatalogue` is wired into `/admin/tenants/new`**, which no longer
  carries its own copy of the module list; `apiFetch` is no longer exported.
- **The §10.7.3 bottom tab bar exists**, below `md`.

The five inventory-screen readability candidates from Phase 4 are **deliberately
still open**: they belong to a phase where the frontend is the focus, not to this
one. Worth knowing that one of them got worse rather than better — the requisition
detail screen built here opens read-only with an explicit Edit, and warehouses and
suppliers already did, so `/inventory/products/:id` is now the only screen of four
that has no reading mode.

**Next:** the walkthrough above, then **Session B** of
[`phases/phase-5-procurement.md`](phases/phase-5-procurement.md) — read §8.4 and
§8.6 twice — in a new session.

## Phase 5, Session B — 2026-07-26

**Done: the thing the project exists to demonstrate.** `POST
/api/procurement/purchase-orders/:id/receipts` receives goods, and one request is
one transaction across three modules: the goods receipt and its lines
(procurement), the purchase order's new status, one `stock_ledger` row per line
(inventory), and one balanced journal entry — Dr 1300 Inventory / Cr 2150 GRNI —
posted for `SUM(qty × unit_cost)` (finance). If any step fails, none of it
happened, and **D8 proves that rather than asserting it**.

The receipt is idempotent (§8.6.1): the client generates a UUID when the form
opens and the same key on any retry returns the original receipt with nothing
written twice. It locks the order and then its lines before validating, so two
receipts posted at the same second cannot jointly over-receive (H5) and cannot
leave a completed order looking half-received. Received quantity is still derived
everywhere — the status of an order is decided by re-reading `po_line_status`,
not by a counter (I6).

Screens: `/procurement/orders/:id/receive` — per-line quantities defaulting to
outstanding, and **the confirmation panel of §10.3**, which names what happened in
all three modules and links to the ledger rows it wrote. `/procurement/orders/:id`
gained the receipt history and the way in to receiving. `/inventory/ledger` now
resolves a receipt's document number and links it to its order.

**Tests green:** **D1–D9** and **H1–H6**, plus Groups A, B, C, E, F, G, I–J
unchanged. **170 top-level tests** (242 including subtests), up from 145 at
Session A. `go test ./... -p 1` clean, `go vet` clean, `gofmt` clean, `tsc
--noEmit`, `npm run build`, and `oxlint` all clean — oxlint reports only the four
pre-existing fast-refresh warnings and none in new code.

H7 is the last outstanding Group H test; it cross-checks B10 (two goroutines
demoting the last two admins) and belongs to the tenant plane rather than to this
handler.

**One test found a real bug, and it is the one worth reading.**
`TestConcurrentRetriesOfOneReceiptPostItOnce` — two retries of *one* form, in
flight at the same moment — failed with `422 over_receipt`. The handler read the
idempotency key once, at the top. The loser passed that read (its twin had not
committed yet), then blocked on the order lock, woke up holding it, and judged its
own twin's already-booked 25 against the 40 ordered. So the ordinary flaky-wifi
retry — the exact case §8.6.1 exists for — told the user that too much had arrived
and asked them to correct a receipt that had posted correctly a millisecond
earlier. **The fix is a second `receiptByKey` read, taken after the lock and before
any quantity is judged.** Both reads are needed and for different reasons: the first
has to precede the status check, or a retry of a receipt that *completed* its order
gets a 409; the second has to follow the lock, or it cannot see a twin. The savepoint
is now only the last narrow window rather than the mechanism.

Beyond the listed IDs:

- **`TestConcurrentReceiptsOnDifferentLinesCloseTheOrder`** — two receipts each
  completing a different line must leave the order `received`, not
  `partially_received`. This is the case the header lock exists for, and its
  comment records the measurement described in *Decisions*.
- **`TestTheIdempotencyConstraintIsNamedWhatTheHandlerMatchesOn`** asserts the
  literal `goods_receipts_tenant_id_idempotency_key_key`. The replay path branches
  on that string precisely so a duplicate `gr_number` — also a `23505`, and a real
  numbering bug — cannot be answered with a cheerful 200. A rename in a migration
  would otherwise turn every replay into a 500 that only shows up on warehouse
  wifi.
- **Isolation from two tenants** through the receipt list, by id, and on the write
  itself, with tenant B's own data re-read afterwards.
- Receiving against a `received` or `cancelled` order is `409 state_conflict`
  carrying where it went; a line belonging to another order is the same 404 an
  unknown line gets; a key already spent on another order is
  `422 idempotency_key_reused`.
- **`TestAReceiptsLedgerRowsAreFindableByItsSourceId`** — the link the confirmation
  panel offers actually returns the two rows, with the GR number resolved.

**Five mutations were run. Three were caught, two survived, and the survivors
were the interesting ones:**

| Mutation | Result |
|---|---|
| over-receipt compared with `>=` instead of `>` | **caught** — D1 and D3 got 422 where they wanted 201: receiving exactly what was ordered was refused |
| ledger `unit_cost` taken from the product's `standard_cost` instead of the order line's | **caught** — D5 reported `4.0000 @ 25.00, want 10.0000 @ 1500.00` |
| `FOR UPDATE` removed from the **PO line** lock | **survived** |
| `FOR UPDATE` removed from the **PO header** lock | **survived** |
| both locks removed **and** `docnum` giving each request its own sequence row | **caught, 10 attempts out of 10** — every order left `partially_received` with nothing outstanding |

The two survivors are one finding, and it is recorded in *Decisions*: every receipt
in a tenant serialises on the `document_sequences` row its GR number comes from,
which happens before either request reaches the status computation. So the locks
are currently invisible to a black-box test while remaining entirely load-bearing
— restoring the header lock alone turns 10 failures into 10 passes. That is an
accidental invariant of the numbering scheme, not a property of the handler, which
is exactly why the locks stay written down.

**A test bug worth remembering:** D5's first version ordered by `unit_cost`, which
PostgreSQL resolves against the **output** column — `unit_cost::text` — so
`'250.50'` sorted above `'1500.00'`. Production is unaffected because every
`sortable` map in the codebase uses *qualified* column names, which bind to the
table rather than to the alias. Qualify the column in a `::text` projection or the
sort is lexical.

**Live check against the local stack:** a freshly built binary was booted on a
spare port against the real Docker database — it starts, `/api/health` is 200
without a token, and both new routes answer. No migration was needed; Session B
added none, so the schema is still `000`–`006`.

**Deviations from spec:** thirteen, all recorded in *Decisions taken* above. The
four worth knowing before Phase 6:

- **`finance_journal.go` already posts to `journal_entries`**, so Phase 6 is a read
  side over data that exists. `assertJournalBalances` is the pattern for any new
  posting, and both sums are compared in SQL.
- **A missing `1300`/`2150` account is a 500**, deliberately — and it is what D8
  injects, so Phase 6 must not "helpfully" turn it into a business refusal without
  giving D8 another way to fail.
- **The savepoint, not a second transaction**, is how the idempotent replay works.
- **`TableHead`** is now how every table in the frontend renders its heading row.

**TODO(post-mvp) markers added:**
- `backend/internal/api/procurement_receipts.go:388` — `audit gr.posted` (§8.4
  step 7)

**One `TODO(phase-6)` marker added** — a *different* marker word, so a grep for
`post-mvp` will not find it and a grep for `TODO(` will:
- `frontend/src/pages/procurement/ReceiveGoods.tsx` — link the confirmation panel's
  finance line once `/finance` exists. This is not deferred past the MVP; it is
  deferred by exactly one phase, and calling it `post-mvp` would have buried it.

**Known broken / left half-done:**

- **No live browser walkthrough of the goods receipt, or of Session A's six
  screens** — and this is now **deliberate, not outstanding**: every walkthrough is
  consolidated into Phase 7's acceptance-test run. See the block in *Current
  state*, which is where a later session will look. The confirmation panel is the
  screenshot the project exists for and nobody has seen it rendered, so Phase 7
  should walk `raise → submit → approve as a second user → Receive goods →
  partial → receive the rest` early rather than last.
- **The API on `:8080` is still the Session A binary.** The spare-port instance
  used for the live check was stopped again; `make dev` picks up the receipt
  routes.
- **`npx fallow audit` is closer to clean but not clean.** Dead code is down to the
  one standing `tailwindcss` false positive (Phase 0 put it there deliberately;
  Phase 4 ruled it correct), and there are no stale suppressions. `TableHead` took
  clone groups 15 → 10; what remains is the pagination-and-empty-state block shared
  by six list screens, which is genuinely the same twenty lines and would want a
  `DataTable` wrapper rather than another header-sized extraction — a Phase 7 or
  frontend-focused job. The reported count then went **10 → 13**, which is not a
  regression: removing the `Column` re-export meant touching `WarehouseList` and
  `SupplierList`, and `fallow audit` only looks at changed files — so it pulled the
  three pre-existing clone groups *between those two screens* into scope. Phase 5A
  already recorded that pair (the Edit/Save/Cancel cluster) as known residue.
  `OrderDetailPage` is still flagged CRITICAL at 104 lines after the split, as
  `RequisitionDetailPage` is at 138; `OrderList` at 168 is worse than either and
  was not touched this session.
- **Still no frontend tests at all.** §12.5 defines them; no phase brief has asked
  for them, and the count is one more untested screen.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it, and
  H5 and the different-lines test are now the third and fourth tests that would
  benefit.
- The five inventory-screen readability candidates from Phase 4 are **still open**,
  deliberately.

**Next:** [`phases/phase-6-finance.md`](phases/phase-6-finance.md) in a new
session. The browser walkthrough is **not** next: it and every other one belong to
Phase 7's acceptance-test run — see *Current state*.

## Phase 6 — 2026-07-26

**Done:** the Finance module exists, as a stub that does not lie. Two endpoints,
both `viewer` — `GET /api/finance/journal-entries` and `GET /api/finance/accounts`
— live in `internal/api/finance.go` and are registered by `registerFinance`, one
`RequireModule` per route like inventory and procurement. `/finance` renders the
§10.5 page: the header reads *Finance — coming soon*, the line under it says
postings from other modules are already flowing in and that reporting and period
close are not built, and below that is the journal itself, live, each entry shown
with its Dr/Cr lines and linked back to the goods receipt that posted it.

Build item 1 needed no work: `seed_tenant_accounts()` has run for every tenant
since Phase 1, and K5 is the test that now says so.

**The §10.3 confirmation panel is finished.** Its finance line is a `Link` to
`/finance?sourceId=<receipt id>`, the counterpart of the inventory line's
`/inventory/ledger?sourceId=<receipt id>`. Both claims the panel makes are now one
click from the rows that back them.

**Tests green:** K1–K7 in `internal/api` (package `api_test`), plus everything
from Phases 0–5 unchanged — `go test ./... -count=1 -p 1` clean, `go vet` clean,
`gofmt` clean, `npx tsc --noEmit` clean, `npm run lint` clean (four pre-existing
fast-refresh warnings), `npm run build` clean.

- **K1** — both routes answer `200` for a finance `viewer` and
  `403 insufficient_module_role` for an `admin` in another module, asserted
  against the real app from `api.New`.
- **K2** — the Phase 6 "done when", second half: a tenant whose Finance
  entitlement is revoked gets `403 module_not_enabled`, with `details.module`,
  while the user still holds finance `admin` — so only the entitlement can be
  doing the refusing.
- **K3 — the Phase 6 gate, and the one worth reading.** Goods are received
  through the real procurement endpoint by a procurement `approver`; the entry is
  then found through the real finance endpoint by Dewi, who holds finance
  `viewer` and nothing anywhere else. Nothing is shared between the two halves but
  the database. It asserts the id and number the receipt reported, the amount
  (3 × 1,500,000), the resolved GR number and PO id, debits equal credits, **and
  which side is which** — Dr 1300 Inventory, Cr 2150 GRNI, because an entry that
  balanced with the sides swapped would pass a sum and be visibly wrong.
- **K4** — two receipts against one order post two entries; `?sourceId=` returns
  the one the reader clicked, each findable by its own id. Also that `accountId`
  counts an entry **once** rather than once per line.
- **K5** — the seeded chart is readable in a workspace nobody has posted into,
  and a fresh journal is an empty page rather than `null`.
- **K6** — another tenant sees zero entries, and cannot reach one by asking for
  it by `sourceId` either. Written because finance is a new surface over three
  tables and a single-tenant test cannot detect an isolation failure at all.
- **K7** — six malformed filters, all `400 malformed`, including an unknown
  `sort` on both endpoints.

**Deviations from spec:** six, all recorded in *Decisions taken* above — entries
returned with their lines, `amount` as the debit side only, the `EXISTS` account
filter, the chart of accounts on the page (which §9.6.1 actually requires), no
`includeDeleted` on accounts, and the `SourceFilterNotice` extraction.

**TODO(post-mvp) markers added:** none.

**TODO(phase-6) markers removed:** the one Phase 5B left in
`frontend/src/pages/procurement/ReceiveGoods.tsx`. There are now **no
`TODO(phase-*)` markers anywhere** in `frontend/src` or `backend/internal`. The
two `TODO(post-mvp)` markers that remain are the whole standing list:

- `backend/internal/api/procurement_receipts.go:412` — audit `gr.posted` (§8.4
  step 7). *Phase 5B's log records this as line 388; it is 412.*
- `frontend/src/lib/requisitionForm.ts:48` — replace the product `<select>` with
  a search-as-you-type picker.

**Known broken / left half-done:**

- **`/finance` has never been opened in a browser**, like most of this
  application. Deliberate and consolidated into Phase 7 — see *Current state*.
  Two things on it are worth looking at first, because they are the only markup
  in the project without a precedent to copy: the Dr/Cr line list inside a table
  cell at 360px, and the chart-of-accounts chips, which are `aria-pressed`
  toggles rather than links.
- **`npx fallow audit` was not run this phase.** `discipline.md` §— asks for it at
  the end of Phases 4, 5, and 7, and Phase 6 is not one of them. The duplication
  it would most likely have found was found by hand instead and extracted
  (`SourceFilterNotice`). The pagination-and-empty-state block is still the
  largest known clone family — `Pagination` now has nine call sites, eight
  screens plus `MasterDataList` — and still wants a `DataTable` wrapper rather
  than another header-sized extraction.
- **Still no frontend tests at all**, and `/finance` is one more untested screen.
  §12.5 defines them; Phase 8 is where they live.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it.
- The five inventory-screen readability candidates from Phase 4 are still open,
  and `OrderList` at 168 lines is still the worst-scoring component in the
  frontend.

**Next:** [`phases/phase-7-dashboard-seed.md`](phases/phase-7-dashboard-seed.md)
in a new session. Phase 7 is the MVP gate and is the biggest remaining phase by
some distance: it owns the seed script (§3.5.3's deterministic UIDs, seven users
— **including the second procurement `approver` the approval step cannot be
walked without**), the four dashboard widgets, and the twenty-five-step
acceptance test at 360px in both themes, which is also the first browser
walkthrough of nearly every screen in this application. Budget for finding
things, not for confirming them.

---

## Phase 7 — 2026-07-26

**Done:** all three build parts. The MVP gate itself is **not** crossed — see
below.

**Part 1, the dashboard.** `GET /api/dashboard/summary` (§9.7) and the four
§10.2 widgets. It is the only route below `/api` that spans modules, so it
carries no `RequireModule` and gates per widget inside, through `LevelFor`; a
widget the caller cannot read is *absent from the response*, not present and
empty. `/` is no longer the identity dump it was: `pages/dashboard/` holds the
page plus `WidgetCard`, `ApprovalQueue`, `LowStockCard`, and
`RecentActivityCard`, and an approver decides requisitions from the dashboard
itself — §10.7.1's "two-button decision between meetings". The low-stock widget's
"Create requisition" links to `/procurement/requisitions/new?products=<id>:<qty>`
with the shortfall PostgreSQL computed already in the boxes.

**Part 2, the seed.** `cmd/seed`, in five files, producing §15 exactly: two
tenants with different entitlements and different timezones, seven users plus the
platform superadmin, ~30 ledger rows per tenant across the preceding 60 days,
13 requisitions and the 6 orders their approvals produced, and 4 goods receipts
that each wrote stock ledger rows *and* a balanced journal entry in the same
transaction. It is idempotent by construction — every row's UUID is derived from
what the row is — and it verifies itself on every run: the §15 document counts,
"at least 3 products below reorder point", and that every receipt has both a
ledger entry and a balanced journal entry behind it.

The receipts go through **`api.PostGoodsReceipt`**, which §15 requires and which
needed the handler split at the HTTP boundary. That is the one structural change
this phase made to shipped code, and Groups D and H passing unchanged is the
evidence it preserved the semantics.

**Part 3, the responsive pass.** Card transformation on the requisition and PO
lists, through a real media-query hook rather than `md:hidden`; frozen first
column plus sticky header on the stock and ledger grids; sticky bottom action
bars on the receipt and requisition forms; and a touch-target audit that found
four real failures. `inputmode="decimal"` needed no work — every quantity field
already had it. `ResponsiveList` was extracted at the end and clone groups fell
from 8 to 1.

**Tests green:** the whole suite, `go test ./... -p 1`. Groups A–L, with **Group
L new this phase** (7 tests, `internal/api/dashboard_test.go`) and **B13 added**
to Group B. Frontend builds clean; `oxlint` reports the same 4 pre-existing
fast-refresh warnings and nothing new.

- **L1** — the four widgets are filtered by module *individually*: a
  procurement-only user gets two widgets and not an empty stock panel. Plus the
  unentitled-module case (Agus) and the superadmin case, which must be `{}` and
  not a 500.
- **L2** — the count is everybody's, the queue is the approver's, and the
  approver's *own* submissions stay in both. The test then confirms the server
  still answers `self_approval_forbidden`, which is what makes the row's presence
  cosmetic rather than a hole.
- **L3** — "open" counts `partially_received` too, and the value is the ordered
  value.
- **L4 — the one worth reading.** The widget's count and
  `GET /inventory/stock/low`'s total must be the same number. It is a test about
  two queries not drifting, which is why they share `lowStockFrom`; the scenario
  includes all three edge cases of the rule (exactly at the point, no point set,
  no ledger rows at all).
- **L5** — the last fifteen, newest first, with a real receipt posted through the
  real endpoint so the top row's `sourceNumber` is a resolved GR number rather
  than a UUID.
- **L6** — two tenants, twice as much next door, and every widget counts only its
  own. Every widget query is a bare aggregate; RLS is the only thing scoping
  them, and this is the test that says so.
- **B13** — `/api/me` reports the *effective* level. See below.

**Verified beyond the suite:**

- **Seed idempotency at content level.** An md5 over every requisition, order,
  receipt, journal entry, journal line, ledger row, and product is byte-identical
  across three consecutive `make seed` runs. Row counts are unchanged and
  `document_sequences` burns no numbers on a re-run.
- **26 acceptance-test assertions driven with real Firebase ID tokens** for all
  eight seeded accounts, against the real API: steps 1, 2, 3, 4, 5, 7, 10, 16,
  24, and 25 in full, plus "each seeded user sees a different set of widgets".
  All pass. Kept deliberately read-only so the browser walk starts from a clean
  seed.

**Deviations from spec:** fifteen, all recorded in *Decisions taken*. The three
that matter most: `/api/me` now sends the effective role map rather than the
stored one (a **bug fix**, see below); the seed writes requisitions and orders as
SQL while only the receipts go through the application, because §15 singles out
the receipt and because approval can only date `expected_at` as today-plus-lead;
and the dashboard route carries no `RequireModule`, gating per widget instead.

**The bug this phase found.** `/api/me`'s `moduleRoles` was the map *stored* in
`user_module_roles`, and a tenant admin correctly has no rows there (§5.4). The
sidebar, the bottom tabs, and `holds()` are all driven off that map — so Rina,
the seed's Nusantara admin and the subject of acceptance step 10, signed in to an
application with no navigation at all, while every endpoint behind the missing
links answered her requests perfectly. It survived four phases because the only
account used by hand had explicit rows, which makes the two maps identical, and
because B7 asks whether an implicit admin gets *past the gate* — she always did.
`lib/levels.ts` had documented the contract the server was breaking the whole
time. B13 is the test; it fails on the old line.

**TODO(post-mvp) markers added:** none. The standing list is still exactly two:

- `backend/internal/api/procurement_receipts.go` — audit `gr.posted` (§8.4
  step 7), in `postGoodsReceipt` after step 6.
- `frontend/src/lib/requisitionForm.ts` — replace the product `<select>` with a
  search-as-you-type picker.

**`npx fallow audit` run:** yes, as `discipline.md` asks at the end of Phase 7.
Clone groups **8 → 1**; the survivor is the inherited pagination block shared by
`LedgerPage` and `StockGrid`, which are not card-transformed and so do not use
`ResponsiveList`. Dead code: 2 unused type exports, both inherited from Phase 6
(`Account`, `JournalEntry`). Complexity: `OrderList` (162 lines) and
`RequisitionList` (143) are still flagged, and further splitting would fragment a
page component for a metric's sake — the render props are what keep the cells
next to the screen that owns them.

**Known broken / left half-done:**

- **THE MVP GATE IS NOT CROSSED.** All twenty-five steps hold at the API, but the
  gate is what a person sees: whether a control is rendered or hidden (I12 makes
  hiding cosmetic, so the API cannot tell the two apart), the 360px layout, both
  themes, whether links go where they claim, and the copy. That walk is
  [`phases/phase-7.5-acceptance-walk.md`](phases/phase-7.5-acceptance-walk.md).
  **Do not start Phase 8 until it is done.**
- **Nothing built this phase has been seen in a browser**, including the four
  widgets, the card view, the frozen columns, and the sticky bars. Two things are
  worth looking at first because they have no precedent to copy: `StickyActions`
  where it meets the bottom tab bar (`bottom-14` is measured from the tab bar's
  `min-h-14`, and a screen with fewer than two tabs has no tab bar, which leaves
  the action bar floating 56px up), and the frozen first column on the ledger,
  where "When" is a wide value to pin on a 360px screen.
- **The `md` sidebar tier of §10.7.3 is still not built.** That section describes
  three tiers — persistent at `lg`, "collapses to icons, expands on hover" at
  `md`, drawer below — and the app has two: drawer below `lg`, persistent above.
  Left as it is deliberately, because §10.7.5 says "no hover-only actions,
  anywhere" and an expands-on-hover sidebar is exactly that. Worth resolving in
  the docs rather than in the code.
- **Still no frontend tests at all.** §12.5 defines them; Phase 8 is where they
  live. `ResponsiveList`, `useCompact`, and `prefillLines` are the three new
  things most worth having them.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it.
- Scratch rows from Phases 2–3 are still in the local database, deliberately —
  see *Current state* for what they are and the SQL to remove them.

**Next:** [`phases/phase-7.5-acceptance-walk.md`](phases/phase-7.5-acceptance-walk.md),
in a new session. It is the browser walk and nothing else: the twenty-five steps
ordered to minimise sign-ins, with the API half already confirmed, so what is
being checked is the screen. Record what it finds under a `## Phase 7.5` block
here. Only then is the MVP done, and only then does Phase 8 exist.

---

## Phase 7.5 — 2026-07-26

**Done: the browser walk. The MVP gate is crossed.** All twenty-five acceptance
steps walked at 360px in dark mode, then Part C re-walked in light. Parts A, B
and C are ticked or have a finding written against them.

**The bug that justified the phase.** `Idempotency-Key` was not in the CORS
`AllowHeaders`, and the receipt form is the only request that sends it. A browser
answers the preflight `204`, then **silently declines to send the real request** —
so `POST /purchase-orders/:id/receipts` was unreachable from the UI, and had been
since Phase 5B. The symptom in the server log is a run of `OPTIONS` lines with no
`POST` behind them, which is what diagnosed it.

Every Go test passed throughout, and always would have: `app.Test` speaks to Fiber
in-process, so there is no origin, no preflight, and the allow-list is never
consulted. Phase 7's API pass with real Firebase ID tokens missed it for the same
reason. **This bug was only ever reachable from a browser**, which is the whole
argument for this phase existing as a phase.

**Tests green:** the whole Go suite, `go test ./... -p 1 -count=1`, plus **Group M,
new** (`internal/api/cors_test.go`): `M1` drives a real preflight through the app
and asserts every header the frontend sends is blessed, parameterised per header so
the failure names the missing one; `M2` asserts an unconfigured origin is not.
**M1 was confirmed to fail on the old config, on the `Idempotency-Key` subtest
only, before the fix was restored.** `gofmt` and `go vet` clean; frontend
typechecks, builds, and lints with the same 4 pre-existing fast-refresh warnings
and nothing new.

### The nine findings

| # | Finding | Resolution |
|---|---|---|
| 1 | **`Idempotency-Key` blocked by CORS** — receipts unpostable from a browser | **Fixed**, Group M behind it |
| 2 | **Own role change did not refresh the navigation** — demote yourself and the sidebar, tabs and badges still describe who you were; a `Users` link that 403s | **Fixed**, no test (Phase 8) |
| 3 | **Requisition line editor unusable at 360px** — four controls stacked per line with no boundary between lines | **Fixed**, no test (Phase 8) |
| 4 | **Dashboard left a row-height hole** under every short widget on a desktop | **Fixed**, no test (Phase 8) |
| 5 | **Page overflowed horizontally at 360px** on the three procurement detail pages | **Fixed** by the developer, no test (Phase 8) |
| 6 | **`below reorder point` badge breaks across two lines** at 360px | **Deferred** — one class, `whitespace-nowrap` |
| 7 | **Native blue checkboxes** against a teal design system, 4 call sites | **Deferred** — one class, `accent-*` |
| 8 | **Finance has no bottom tab, for anybody** — Dewi's only admin module is unreachable from the tab bar | **Deferred** — a design decision, not a slip |
| 9 | **Finance journal needs sideways scrolling with no frozen column** — `min-w-[52rem]`, the widest table in the app, so the Dr/Cr *amounts* are off-screen at 360px and scrolling to them loses the entry number | **Deferred** — `frozenCell` went to stock and ledger only |

**What was fixed, and where.**

- **CORS.** `middleware.HeaderIdempotencyKey` is a new constant beside
  `HeaderRequestID`, so the allow-list and the handler read one string;
  `router.go` uses it in `AllowHeaders`; `procurement_receipts.go` uses it instead
  of a bare literal. The constant's comment is the explanation of the failure mode,
  because the next person to add a header needs it.
- **`refreshMe()`** on the auth context (`hooks/useAuth.tsx`), called from
  `UserDetail`'s single `run()` funnel when `next.id === me.user.id`. Keyed off the
  **server's** returned id, not the `:id` in the URL. Guarded by the same
  generation counter as `onAuthStateChanged`, so a refresh resolving after a
  sign-out cannot reinstate a signed-in user; a *failed* refresh keeps the last
  known identity, because signing someone out over a background refetch is worse
  than being briefly stale. A self-demotion now redirects off `/settings/users`
  via `RequireTenantAdmin`, which is the intended outcome.
- **`RequisitionFields`** wraps each line in a bordered card with a `Line N`
  header carrying its own Remove below `sm`; from `sm` the card, padding and
  header all vanish and it is the original single-row grid again.
- **`DashboardPage`** is two independent flex columns rather than four cards in a
  `grid-cols-2`. A grid *aligns rows*, and the approval queue is several times
  taller than the figure card beside it, so `items-start` stopped the card
  stretching but could not reclaim the row's height. Masonry is not yet
  shippable. The cost, recorded at the call site: the phone order becomes
  column-major, so the queue falls from second to third.
- **The 360px overflow**, fixed by the developer: `min-w-0` on the grid children
  and `lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]` on `OrderDetail`,
  `RequisitionDetail` and `ReceiveGoods` — the pattern `ProductDetail` already
  used and Phase 5 had not inherited — plus wrapping header controls in
  `AppShell` and a `table-fixed` colgroup on `StockGrid`. `StockGrid` keeps
  `min-w-[28rem]`, so the table still scrolls at 360px and the frozen-column
  check stays meaningful.

**Deviations from spec:** three, all in the acceptance test's wording rather than
in the application.

- **Step 1's superadmin dashboard is unreachable.** `Home` redirects a superadmin
  to `/admin/tenants` (`RequireRole.tsx`, and §10.1 intends it), so `DashboardPage`
  never renders for them and `EmptyDashboard`'s `superadmin` branch is dead code.
  The behaviour is *better* than the step asks for. Fix the step, or delete the
  branch, in Phase 8.
- **Steps 15 and 20's refusals are not reachable by clicking.** The receive form
  disables Post receipt when a line is over-entered, and the PO cancel button is
  only rendered while `status === "open"`. Both are I12 hiding an action the server
  independently refuses, and both server refusals are asserted in the Go suite.
  What a person *sees* is the client warning and an absent button; that is what was
  walked.
- **Step 18 names two products that do not work.** On a pristine seed `OFF-PAPER`
  sits on an open order (refused with `in_use`) and `HND-PALLET` has no PO line at
  all, so neither can show "the PO line still shows its name marked deleted". The
  sets are disjoint: every Nusantara product carrying a PO line is on an
  `open`/`partially_received` order. **`PKG-WRAP` is the answer**, and only after
  step 20 cancels `PO-202607-0002` — which is also why step 20 must not cancel
  `PO-202607-0001`, the open order step 19's `in_use` refusal depends on.

**TODO(post-mvp) markers added:** none. The standing list is still exactly two:

- `backend/internal/api/procurement_receipts.go` — audit `gr.posted` (§8.4 step 7).
- `frontend/src/lib/requisitionForm.ts` — replace the product `<select>` with a
  search-as-you-type picker.

**Known broken / left half-done:**

- **Findings 6–9 are deferred**, with reasons in the table above. 6 and 7 are one
  Tailwind class each and were left only because no frontend test can stand behind
  them yet; do them first in Phase 8.
- **Three of the five fixes have no test behind them**, because there is no
  frontend suite until Phase 8. `refreshMe` is the one that most wants one — it has
  real logic (generation guard, state-updater no-op, swallowed failure), unlike the
  three CSS changes.
- **`refreshMe` only covers changing your *own* role.** Another admin demoting you
  while you are signed in still leaves your navigation stale until you navigate or
  reload. The server refuses everything regardless (I9, I12), so it is cosmetic,
  and polling `/api/me` would be the wrong trade for an MVP.
- **`erp_migrate` holds `BYPASSRLS` and `SUPERUSER`**, which I3 forbids flatly.
  A10 is deliberately scoped to `IN ('erp_app','erp_admin')` — the roles the
  application connects as — and locally `erp_migrate` is the container's
  `POSTGRES_USER`, which the postgres image always creates as a superuser. Not a
  Phase 7.5 defect, but **I3 is written wider than anything enforced, and on a
  managed provider the migration role is provisioned by hand. Resolve it in Phase
  9**, either by narrowing I3's wording or by testing the migration role too.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it.
- **The local database is the walked-on demo, not a pristine one:** extra
  requisitions and receipts, `PO-202607-0002` cancelled, Rina demoted and
  restored. Rebuild before demoing:
  `docker compose down -v && make up && make migrate && make seed`.

**Next:** [`phases/phase-8-frontend-tests.md`](phases/phase-8-frontend-tests.md).
Feed findings 6–9 into it first — they are the cheapest real work in the file — and
give `refreshMe`, `ResponsiveList`, `useCompact` and `prefillLines` tests before
anything else. FE1–FE26 run against MSW, which is a different class of bug from
this walk: a mock encodes what you *believe* the server sends, which is exactly why
neither it nor the Go suite could ever have caught the CORS bug.

---

## Phase 8 — 2026-07-26

**Done:** the frontend has a test suite, both applications have a CI workflow and
a container image, and the coverage question §12.6 asks is now answerable by a
command rather than by a guess.

**Tests green:** **FE1–FE15 and FE22–FE26** — all twenty §12.5 assigns to this
phase — as **102 Vitest tests in 8 files**, plus the whole Go suite at **366
tests** (54 of them new here). `npm run lint`, `npm run build`, `gofmt`, `go vet`
and `golangci-lint run` are all clean.

### The frontend suite

`frontend/src/test/` — infrastructure in six files, assertions in eight.

| File | What it is |
|---|---|
| `setup.ts` | jest-dom, the MSW lifecycle, and the two `vi.mock` calls that replace Firebase. `onUnhandledRequest: "error"` — an endpoint no handler answers fails by name instead of timing out five seconds later |
| `firebaseFake.ts` | §12.4's rule applied to the browser half: `getAuth`, `onAuthStateChanged`, `signOut`, `signIn`. `setFirebaseUser(uid)` drives the **real** `AuthProvider`, so every test goes through the real `/api/me` round trip. It is a separate module because a `vi.mock` factory is hoisted above the imports and cannot close over anything beside it |
| `media.ts` | `window.matchMedia`, which jsdom does not implement at all. A small real query engine, because `useCompact` and `useTheme` both ask it. **The lists are cached by query string on purpose:** both hooks call `matchMedia` more than once, and with fresh objects `removeEventListener` would never find the listener it was meant to remove |
| `server.ts` | MSW handlers. Every endpoint answered with the emptiest legal §9.0 response, so a test states only what it cares about |
| `fixtures.ts` | The seeded people of §15 — `rina` `budi` `sari` `dewi` `agus` `superadmin` — and row builders. `renderApp("/finance", agus)` says what it is checking |
| `render.tsx` | `renderApp(path, me)` renders **`<App/>`**, pushing the path onto jsdom's history rather than swapping in a `MemoryRouter`. So the real router, the real guards and the real providers are all in the picture; a harness that had to be handed a router would be testing a different tree from the one that ships |
| `layout.ts` | Measures the size a component *declares*, by reading its Tailwind utilities. **Read its comment before trusting FE9 or FE13** — see the honesty note below |

| Group | Covers |
|---|---|
| `navigation.test.tsx` | FE1, FE8 |
| `permissions.test.tsx` | FE2, FE6 |
| `receipt.test.tsx` | FE3, FE4, FE5, FE9 |
| `responsive.test.tsx` | FE7 |
| `theme.test.tsx` | FE10, FE11 |
| `presentation.test.tsx` | FE12, FE13, FE26 |
| `lists.test.tsx` | FE14, FE15 |
| `masterdata.test.tsx` | FE22–FE25 |

**What FE9 and FE13 can and cannot claim.** jsdom has no layout engine and
Tailwind is never compiled during a unit test, so neither measures a pixel a
browser produced. They measure the minimum a control *declares*. That is weaker
than "44px on a phone" and it is still the check that catches the regression:
every one of the four targets the Phase 7 audit found was a control that had
declared no minimum at all. The real pixels were checked at the 360px walk.

### Three real findings, from the tests

- **FE9 found two sub-44px targets on the goods receipt form** — the one screen
  §10.7.1 calls genuinely mobile. "Back to the order" was a bare `text-sm` link at
  about 20px, and the note textarea declared no minimum (it was tall enough by
  accident, via `rows={2}`, so nothing stopped a later `rows={1}`). Both fixed.
- **FE13 found every skeleton row 4px short.** The placeholder bar was `h-4`
  against `text-sm`'s 1.25rem line box — a 20px lurch over five rows, which is the
  exact thing §10.7.6 uses skeletons instead of a spinner to avoid. Now `h-5`.
- **FE6 had no behaviour to test.** A `403 module_not_enabled` left the reader on a
  screen of a module they no longer had, holding an inline error, with a sidebar
  still offering the link that produced it. Reachable because `moduleRoles` comes
  from one `/api/me` read at sign-in while the *server* is never stale (I9). Built:
  `lib/api.ts` gained `onEntitlementRevoked`, and `components/EntitlementWatcher.tsx`
  turns that refusal into the server's own sentence, a `refreshMe()`, and a
  redirect. `insufficient_module_role` is deliberately **not** routed through it —
  that means "right module, not senior enough for this one action", and the answer
  to it is the inline refusal where the action was.

### Phase 7.5's deferred findings

| # | Resolution |
|---|---|
| 6 | **Fixed** — `whitespace-nowrap` on the reorder badge |
| 7 | **Fixed** — `accent-accent` on all four checkboxes, so the control follows the theme token in both modes |
| 9 | **Fixed** — the journal is now `ScrollableTable` + a frozen first column. **The Entry column had to move first**: only the first column can sensibly be frozen, and §10.0.2 says a document number is the identity rather than metadata anyway, so the reorder is right regardless. Test in `responsive.test.tsx` |
| 8 | **Confirmed deferred, not forgotten.** §10.7.3 names the tab bar's destinations explicitly — "Dashboard, Requisitions, Orders, and Stock if entitled" — and Finance is not among them. Dewi reaches it from the drawer, which the section calls the whole map. Changing the spec's list is a design decision for whoever wants it, not a Phase 8 slip |

### CI

`.github/workflows/ci.yml`, three jobs, `TZ=UTC` at the top because Group J
asserts the process agrees (J2 is literally `time.Local == time.UTC`).

- **backend** — `go vet` → `golangci-lint` → `govulncheck` → `go test -race` with
  coverage.
- **frontend** — `npm ci` → lint → tests with coverage → build.
- **containers** — both images, `needs: [backend, frontend]`. Building an image of
  a failing commit tells nobody anything.

**No deploy job**, per §12.7. Phase 9 deploys by hand, once.

New files CI needs, which did not exist: **`backend/Dockerfile`** (distroless
static, non-root, shipping `migrate` beside `api` — a release that cannot migrate
itself needs a laptop), **`frontend/Dockerfile`** + **`nginx.conf`**, two
`.dockerignore`s, and **`backend/.golangci.yml`**.

**`nginx.conf`'s `try_files` line is the load-bearing one.** Every route in
`App.tsx` is client-side, so `/procurement/orders/<uuid>/receive` is not a file —
and the acceptance test pastes UUIDs into the address bar. `index.html` is also
explicitly `no-store`: it is the one filename that does not change between
releases, and a cached copy points at hashed assets that no longer exist, which is
a white screen only a hard refresh fixes.

**Verified locally:** `go vet`, `golangci-lint run` (0 issues), the Go suite, the
frontend lint/test/build, and **both images built *and run*** — building an image
is not evidence it starts:

| Check | Result |
|---|---|
| web `/` | 200 |
| web `/procurement/orders/<uuid>/receive` | 200, serves `index.html` — `try_files` works |
| web `/assets/nope.js` | 404 — the SPA fallback is not swallowing everything |
| api boot | `mini-erp api listening on :8080` |
| `/api/health` | `{"status":"ok"}` |
| `/api/me`, no token and a garbage token | 401 both, not 500 — the Admin SDK really initialised and was consulted |
| CORS preflight incl. `Idempotency-Key` | 204 with the header — Phase 7.5's shipping bug stays fixed *in the container* |
| `/migrate` in the same image | ran against the live database, idempotent: "schema already up to date" |

One trap for whoever repeats this on Windows: Git Bash rewrote
`-e GOOGLE_APPLICATION_CREDENTIALS=/secrets/key.json` into
`C:/Program Files/Git/secrets/key.json`, and the container exited with a
credentials error that looks like a code bug. Prefix the command with
`MSYS_NO_PATHCONV=1`.

**Not verified:** `govulncheck` — installing it needs `sum.golang.org`, which this
machine cannot reach — and the workflow itself, which needs a push. Both are
first-run risks on the pull request.

**And `go test -race` has still never run anywhere.** No C toolchain here, and CI
has never been pushed, so "CI runs it" describes the workflow file rather than
anything that has happened. The locking and idempotency paths — `SELECT … FOR
UPDATE`, the `SavePoint`/`RollbackTo` pair in `createGoodsReceipt` — are exactly
where a race would live, so that first CI run is worth having green **before** the
Phase 9 deploy rather than after it.

`golangci-lint` found exactly one issue, `QF1001` on a `!(a < b)` in
`identity/level_test.go`; rewritten as `>=`. `.golangci.yml` keeps the **default**
linter set rather than widening it: every default linter reports a defect, and the
first time a step reports a preference instead is the last time anybody reads it.

### Coverage — 4 of 6 §12.6 targets met, and the two that are not

`make cover` now prints this, via **`backend/cmd/covreport`**:

```
procurement (incl. receipt.go)     80.6%  ( 681/ 845)  target  90%  SHORT
internal/db                        88.5%  (  85/  96)  target  90%  SHORT
internal/middleware                91.2%  (  83/  91)  target  90%  OK
inventory                          81.5%  ( 365/ 448)  target  80%  OK
finance                            85.3%  (  58/  68)  target  80%  OK
handlers and wiring                82.9%  ( 568/ 685)  target  60%  OK
all of internal/ (not a target)    81.5%  (1980/2429)
```

Frontend `src/components`: **81.9% lines**, against §12.6's 60%. Enforced as a
Vitest threshold on that directory, so it cannot quietly fall back.

**`cmd/covreport` exists because both obvious ways to measure this are wrong.**
Per-package coverage understates the suite badly — `internal/db` scores **42.7%**
that way, while `internal/api`'s tests are in fact exercising most of it, because
every request goes through `WithTenant`. The honest number needs
`-coverpkg=./...`; but that makes every one of the seven test binaries write a
full profile of the whole module, `go test` concatenates them, and `go tool cover`
reading that file double-counts. The blocks have to be **unioned**. Re-deriving
that twice in one session is why it is a checked-in tool.

**Where the coverage went, and it was not busywork.** Seven functions were at
**0.0%** — not error paths, whole endpoints:

- `getWarehouse`, `patchWarehouse`, `restoreWarehouse`, `duplicateWarehouseCode` —
  **three warehouse endpoints the frontend calls on every warehouse edit**, with no
  test at all. §9.6.1's failure mode from the other direction: fully built and half
  *tested*, which looks identical until something regresses.
- `getTenant`, `listTenantModules` — the workspace detail screen and its
  entitlement matrix, reachable in the browser since Phase 3.
- **`api.PostGoodsReceipt`** — §8.4 with the HTTP peeled off, the function
  `cmd/seed` calls. The one seam §15 leans on ("the seed cannot drift from the
  application") was itself never asserted; a wrong argument order there would have
  surfaced as `make seed` failing at the demo.

54 new Go tests in six files: `inventory_master_test.go`, `seed_seam_test.go`,
`admin_tenant_reads_test.go`, `malformed_input_test.go`,
`procurement_edits_test.go`, `db/migrate_test.go`. The two most useful are not
about a percentage:

- **A migration that fails is not recorded.** Recorded-but-not-applied is the worst
  state a migration runner can reach: every later run skips the file and the schema
  is permanently behind what the table claims.
- **Garbage in, 4xx out.** 21 routes, table-driven: a non-UUID path parameter is a
  **404**, and a bad query parameter is a **400 naming the parameter**. A `uuid`
  parse error escaping as a 500 is the difference between "you asked for something
  that cannot exist" and "this server is broken" — it pages somebody, spends error
  budget, and tells a prober they found something.

**Deviations from spec:** four, all recorded rather than worked around.

- **FE24 is asserted as a guarantee, not as optimistic UI.** §12.5 words it
  "optimistically removes the row and restores it if the request fails". This
  application deletes pessimistically. The reason is that a **refused** delete is
  the ordinary case here, not the edge one: a warehouse holding stock is refused
  with `in_use` (G5) and so is a supplier with open orders (G4), the seeded demo
  has both, and the acceptance test walks them. Optimistic removal would make the
  common outcome a row that vanishes and comes back — which reads as a bug — to
  hide about 200ms in the uncommon one. What FE24 exists to guarantee is asserted:
  a successful delete removes the row, a refused one leaves it exactly where it was
  with the reason on screen.
- **FE10 is a three-state radiogroup, not a cycling button.** §12.5 says "cycles
  light → dark → system"; §10.8.3 says "offer light / dark / system, defaulting to
  system". What is built offers all three in one press each, and the substance of
  FE10 — three states, the applied class, the persistence — is asserted.
- **FE23's "submits only changed fields" was implemented, not reworded.** The
  product edit form sent all five every time, which quietly widens a lost update:
  two admins with the screen open, one renames the product, the other fixes the
  cost and sends the name they loaded a minute ago over the top of it. It now
  diffs, and says "Nothing to save" rather than a "Changes saved" for a request it
  never sent.
- **§12.6's package targets name packages that do not exist.** There is no
  `internal/procurement`, `internal/inventory` or `internal/finance` — all three
  live as files inside `internal/api`. `cmd/covreport` maps the spec's intent onto
  the files that exist, and that mapping is the reason it is checked in.

**TODO(post-mvp) markers added:** none. The standing list is still exactly two:

- `backend/internal/api/procurement_receipts.go` — audit `gr.posted` (§8.4 step 7).
- `frontend/src/lib/requisitionForm.ts` — replace the product `<select>` with a
  search-as-you-type picker.

**Known broken / left half-done:**

- **The two SHORT coverage targets will not close without fault injection, and that
  is the honest reason rather than an excuse.** Of procurement's 164 remaining
  uncovered statements, **144 (88%) are `if err != nil` guards on database calls**;
  of `internal/db`'s 11, **9 are**. Reaching 90% on procurement means covering
  ~80 of those, which needs a harness that makes the Nth query fail. §12.6 says
  outright that "coverage percentage is a weak signal on its own", and building a
  query-failure injector to exercise `return err` lines is testing for the metric
  rather than the behaviour. **Every reachable branch that was uncovered now is
  covered** — that was the 54 tests. If a later phase wants the number, the tool to
  build is a GORM plugin that fails a chosen query, and the honest thing is to
  decide that on purpose.
- **`internal/auth/firebase.go` is 0% and should stay there.** §12.4: "Do not test
  Firebase itself." It is the SDK wrapper the fake replaces.
- **`govulncheck` is unverified locally** (no route to `sum.golang.org`), and the
  workflow has never run. Both could fail on the first pull request.
- **`components/PasswordField.tsx` is 0%.** The login screen is not in FE1–FE26;
  Phase 2 walked it by hand and Phase 7.5 re-walked it.
- **`npm audit` reports 2 high advisories**, both `react-router` GHSA-qwww-vcr4-c8h2
  — an **RSC-mode** CSRF bypass. This is a plain SPA with `BrowserRouter` and no RSC
  anywhere, so it does not apply; the fix is a major version bump and was not worth
  churning the router for in this phase. Revisit at Phase 9.
- **`vitest` was pinned at `^3` first and had to be `^4`.** Vitest 3 bundles its
  own Vite 7, whose plugin types are incompatible with the project's Vite 8, and
  `tsc -b` fails on `vite.config.ts` with a 40-line type error that says nothing
  about the cause. Vitest 4 peers on Vite 8. Worth knowing before the next upgrade.
- `go test -race` still cannot run here — no C toolchain on `PATH`. CI runs it.
- **The local database is still the walked-on demo**, not a pristine one. Rebuild
  before demoing: `docker compose down -v && make up && make migrate && make seed`.

**Next:** push and get the workflow green — that is the one remaining box in
`phases/phase-8-frontend-tests.md`, and it cannot be checked from this machine.
`govulncheck` and the two container builds are the likely first failures. Get it
green **before** the deploy, not after: `go test -race` has never run anywhere, and
the only real concurrency in the system is the goods receipt's `FOR UPDATE` and
its savepoint/rollback idempotency pair.

### What Phase 9 inherits, and the shape the developer intends

**Decided by the developer 2026-07-26, before Phase 9 opened:** the **frontend goes
to Firebase Hosting** (`npm run build`, then `firebase deploy --only hosting`), and
the **backend plus the database go to GCP**. The hosting site already exists —
`erp-project-b66ce`, auto-linked at app registration and unused since.

Four things follow from that choice, and the first is easy to miss:

- **`frontend/nginx.conf` is then not on the serving path.** It was written for the
  container image, and Firebase Hosting needs the same rule expressed its own way:
  a `firebase.json` **rewrite of `**` to `/index.html`**, or every deep link and
  every reload 404s. The container image stays useful as a
  build-anywhere/run-anywhere artefact and as the thing CI proves still assembles,
  but the SPA fallback has to be stated twice, in two syntaxes, and both have to be
  checked. There is no test behind either.
- **The order of operations is forced.** `VITE_API_BASE_URL` is baked in at build
  time, so the backend must be deployed first, its URL read off, and only then can
  the frontend be built and shipped. A frontend built before the backend exists
  points at `localhost:8080`.
- **`CORS_ORIGINS` must name the Hosting origin**, and the API refuses anything
  else. This is the pair that Phase 7.5's shipping bug lived in; the allow-list
  already carries `Idempotency-Key`, and the container run above confirms it.
- **The password reset action URL** (Console → Authentication → Templates →
  Password reset → Customize action URL) must point at `<hosting origin>/auth/action`.
  It still points at Firebase's hosted page — known since Phase 2, and the reason
  `ResetPassword` has never been reached by a real emailed link.

And the two questions this phase wrote down and did not answer: **I3's wording
versus `erp_migrate`'s `SUPERUSER`** — on a managed instance that role is
provisioned by hand, so the deploy is the moment to either narrow I3 or test the
migration role too — and the `react-router` **RSC-mode** advisory, which does not
apply to this SPA but will keep showing up in `npm audit`.

---

## Phase 9, Session A — 2026-07-26

**Nothing is deployed.** This session did the half of Phase 9 that lives in the
repository, and rehearsed it against a database shaped like the real one. The
account work — Neon project, billing, `gcloud`, `firebase deploy` — is untouched,
because both Google accounts involved belong to the developer.

**Done:** the topology was decided, the four things in the code that would have
broken on a managed host were found and fixed, and the whole database path was
proven end to end against a Postgres 17 container configured to refuse what Neon
refuses.

**Tests green:** the full Go suite (**376**, 10 new in `internal/config`), 102
Vitest tests, `gofmt`, `go vet`, `npm run lint`, `npm run build`. `golangci-lint`
is **not installed on this machine any more** and was not run; CI runs it.

### The topology, and the one thing that is unusual about it

| Piece | Where |
|---|---|
| Frontend | Firebase Hosting, `erp-project-b66ce` |
| Backend | Cloud Run `mini-erp-api`, `banded-torus-476311-q1`, `asia-southeast1` |
| Database | Neon Postgres 17, AWS `ap-southeast-1`, database `erp` |
| Auth | Firebase Authentication, `erp-project-b66ce` — with the demo accounts |
| Secrets | Secret Manager, `banded-torus-476311-q1` |

**The two projects sit under different Google accounts.** That was the
developer's constraint, not a choice, and it costs exactly one thing: the
Firebase service-account key has to live in the *Cloud Run* project's Secret
Manager and mount as a file, because Cloud Run's own identity belongs to the
wrong project to be granted anything in the other. No cross-project IAM is
needed — the key authenticates directly. `DEPLOY.md` §5.

Regions are co-located deliberately: Neon in AWS Singapore, Cloud Run in Google
Singapore. Every request here makes several round trips, so cross-region latency
multiplies rather than adds.

### Four things would have broken on the first production migration

None of these were visible locally, because locally the migrate role is the
container's superuser and can do anything.

1. **`ALTER ROLE erp_app NOBYPASSRLS NOSUPERUSER`** — PostgreSQL requires
   superuser to touch either attribute *even to turn it off*. `make migrate`
   re-applies `000_roles.sql` on every run, so the first production migration
   would have aborted with `must be superuser to alter superuser roles`.
2. **`ALTER ROLE erp_app SET timezone = 'UTC'`** — needs ADMIN on the role,
   which the migrate role does not have on a role somebody else created.
3. **`GRANT CONNECT ON DATABASE erp`** — hard-coded, and a managed host's
   database is called whatever the provider called it. Now `current_database()`.
4. **`ALTER DATABASE erp OWNER TO erp_migrate`** in the bootstrap — PostgreSQL
   16+ grants a `CREATEROLE` role ADMIN on the roles it creates but **not SET**,
   and that statement requires the ability to `SET ROLE` to the incoming owner.
   One `GRANT … WITH SET TRUE` line fixes it; without it the error reads like a
   permissions dead end.

`000_roles.sql` now attempts each privileged statement and skips it when
refused, then **asserts I3 from `pg_roles`**, which needs no privilege at all.
That is the stronger arrangement: forcing an attribute and checking it are
different claims, and only the check catches a role created through a console.

### The rehearsal, and why it is worth more than reading the docs

A Postgres 17 container was booted with an owner role holding `CREATEROLE` and
`CREATEDB` but **not** `SUPERUSER` — Neon's exact shape — and then:

| Step | Result |
|---|---|
| `deploy/neon-bootstrap.sql` as the provider's owner | Three roles, all `rolsuper=f rolbypassrls=f`, database `erp` owned by `erp_migrate` |
| `cmd/migrate` | 001–006 applied, then `000_roles.sql` re-applied |
| `cmd/migrate` again | `schema already up to date` — idempotent under the new file |
| `cmd/seed` | Both tenants, 8 goods receipts through `api.PostGoodsReceipt`, journals balanced |
| `cmd/dbverify` | **all 11 checks passed**, no warning |

The first run of that rehearsal is what found problems 1–4, one at a time. It
also produced two warnings that were real: after `ALTER DATABASE … OWNER TO`,
the provider's role can no longer grant on that database, and PostgreSQL says so
only in a `WARNING` — so the bootstrap's `GRANT CONNECT`/`GRANT USAGE` were
silently doing nothing. They belong to `erp_migrate` and `000_roles.sql` now
issues them; the bootstrap is a single connection as a result.

### New in the repository

| File | What it is |
|---|---|
| **`docs/DEPLOY.md`** | The runbook. Thirteen sections, every command filled in for these projects, plus the troubleshooting table. **This is what to read when deploying** — not the reference docs |
| **`deploy/neon-bootstrap.sql`** | Run once against Neon as `neondb_owner`. Creates the three roles with SQL — never through the console, which grants `neon_superuser` and with it `BYPASSRLS` — pins their timezones, and creates database `erp` owned by `erp_migrate` |
| **`deploy/cloudrun.env.yaml`** | The non-secret Cloud Run environment. A file rather than `--set-env-vars` because `CORS_ORIGINS` contains a comma, which that flag parses as a separator |
| **`backend/cmd/dbverify`** | A10, J1, I4, RLS-forced-on-all-fourteen, and an end-to-end "no tenant context returns no rows" — against **any** database, from an env file. This is how Phase 9's last two boxes get ticked; both tests run against a testcontainer by construction and could not be pointed at a deployment |
| **`frontend/firebase.json`** + `.firebaserc` | The SPA rewrite, and the cache headers `nginx.conf` states for the container image. Both have to agree and nothing checks that they do |
| **`frontend/.env.production.example`** | `VITE_API_BASE_URL`, which forces the deploy order |
| `backend/internal/config/config_test.go` | 10 tests. The package had none |

### Two ergonomics that change how the commands are run

- **`migrate`, `seed` and `dbverify` take an optional env file**:
  `go run ./cmd/migrate .env.production`. Its values *override* the shell, so
  there is no way to point one at the local database by accident. The API
  deliberately does not take one — a server reads its environment, and Cloud Run
  has no file to name.
- **`make verify-db`, `make deploy-api`, `make deploy-web`** are in the Makefile,
  and `make help` now lists the deployment section.

### Verified, not assumed

- **Vite's env precedence.** `.env.production` beats `.env.local`, so the dev URL
  cannot leak into a release — checked by building with a probe value and
  grepping `dist/`: the probe was in the bundle and `localhost:8080` was not.
  Worth checking rather than trusting, because the failure mode is a deployed
  frontend that calls localhost and looks like a CORS bug.
- **The whole database path on a Neon-shaped host**, as above.

### Not verified, and it is the gate

Everything that needs an account. Nothing has been deployed, no Neon project
exists, and `gcloud`/`firebase` were never invoked. The acceptance test against
deployed URLs — Phase 9's first "done when" — has not started.

The prepared-statement interaction between pgx and Neon's transaction-mode
pooler is the one runtime unknown left; it is in `DEPLOY.md` §13 with its fix,
because it will show up on the first request or never.

**Deviations from spec:** four, all in *Decisions taken* above — the dev Firebase
project for the deployment, I3's precise wording for `erp_migrate`,
`000_roles.sql` asserting rather than forcing, and the API no longer requiring
`MIGRATE_DATABASE_URL`.

**TODO(post-mvp) markers added:** none. The standing list is still exactly two —
`procurement_receipts.go` (audit `gr.posted`) and `lib/requisitionForm.ts` (the
product picker).

**Known broken / left half-done:**

- **Phase 8's CI box is still open.** Get the workflow green before deploying,
  not after: `go test -race` has still never run anywhere.
- **`golangci-lint` is no longer installed on this machine.** Phase 8 ran it;
  this session could not. `gofmt` and `go vet` are clean.
- **The `react-router` RSC advisory** still shows in `npm audit` and still does
  not apply to this SPA. Phase 8 deferred it to here; deferring it again is a
  choice, not an oversight — the fix is a major version bump.
- **The local database is still the walked-on demo.** Rebuild before demoing:
  `docker compose down -v && make up && make migrate && make seed`.

**Next:** `DEPLOY.md` §2 — create the Neon project. §2–§4 are independent of the
GCP account work, and §4 ends with `dbverify` green against the real database,
which is two of Phase 9's three boxes.

---

## Phase 9, Session B — 2026-07-27

**Database and frontend are live. The backend is not, and the reason is a
payment wall rather than anything in the code.**

| Piece | State |
|---|---|
| Database — Neon Postgres 17, `ap-southeast-1`, database `erp` | **Live.** Bootstrapped, migrated, seeded, and `dbverify` green |
| Frontend — Firebase Hosting, `erp-project-b66ce` | **Live** at https://erp-project-b66ce.web.app |
| Backend — Cloud Run, then Render | **Blocked.** Neither host can be reached without a payment method |

### What is actually running

`deploy/neon-bootstrap.sql` ran against Neon exactly as rehearsed — three roles
with `rolsuper=f rolbypassrls=f`, database `erp` owned by `erp_migrate`, all
twelve statements in one pass. Then, against the real database:

```
go run ./cmd/migrate  .env.production   → 001..006 applied, 000_roles re-applied
go run ./cmd/seed     .env.production   → both tenants, 8 receipts, journals balanced
go run ./cmd/dbverify .env.production   → all 11 checks passed
```

**That is two of Phase 9's three "done when" boxes**: A10 and J1 against the
production database, and no `warn` line — `erp_migrate not elevated` came back
`super=false bypassrls=false`, which is the check that catches a role
provisioned through a provider's console. I4 and RLS-forced-on-all-fourteen
passed too, and the end-to-end one reads `0 without tenant context, 10 with it`.

The frontend went up with a **placeholder** `VITE_API_BASE_URL`, so it serves,
routes, and signs in against Firebase, and then fails at `/api/me`. The SPA
rewrite was confirmed working on a pasted deep link.

### Why the backend is not deployed

Both hosts were tried, in this order.

**Cloud Run.** The GCP project's billing account (`013F40-ACAA77-9793ED`, IDR)
is **closed** — `gcloud billing accounts describe` returns `open: false`. It is
almost certainly an expired Free Trial. This produced a genuinely misleading
sequence worth writing down:

- `gcloud services enable …` **succeeded**, because that only checks whether a
  billing account is *linked*.
- `gcloud secrets versions add` then failed with `BILLING_DISABLED`, because
  real API calls check whether the account can *pay*.
- The developer's other project has a Cloud Run service still answering, which
  reads as "billing is fine". It is not: Cloud Run keeps serving an
  already-deployed revision after billing lapses. Only *new* billable operations
  are refused.

**Render.** The free instance type advertises no card, and the whole service was
configured correctly — `rootDir: backend`, Singapore, Free, health check
`/api/health`, the Firebase key as a secret file, Pre-Deploy left empty because
it is paid-gated and migrations deliberately do not run from the service. Render
still raises an **Add Card** modal on deploy. The modal's copy is about paid
instances, but it appears every time, so it is account verification rather than
instance selection.

**The blocker is a payment method, not the application.** The developer is in
Indonesia with a BCA debit card; Google and Stripe both reject it. If the card
is GPN-only it cannot work internationally at all; a Visa/Mastercard debit needs
international online transactions enabled by the bank.

### The one configuration error found, and not yet corrected

The Render form carried **four** environment variables. **`CORS_ORIGINS` was
missing**, so the API would have fallen back to `http://localhost:5173` and
refused every request from the deployed frontend — which presents as a broken
backend, not as a missing variable. Whoever picks this up: it is the fifth line
of `deploy/render.env`.

### What is committed

`f8c1d44` — the Phase 9 work, pushed to `Nimrod-DG/mini-erp@master`, rebased
over a README edit made on GitHub. Includes `render.yaml`, which configures the
Render service declaratively and is still correct; only the account wall stands
between it and a deploy.

Gitignored and **on the developer's machine only**: `deploy/prod.filled.sql`
(the bootstrap with real passwords), `deploy/prod.env`, `deploy/render.env`, and
`backend/.env.production`. Losing those means resetting three Neon role
passwords, not losing the deployment.

### To finish this, in order

1. A working payment method on **either** GCP or Render. Nothing else is needed
   for Cloud Run — `deploy/deploy-api.sh` is written and its secret/IAM half was
   tested up to the billing refusal.
2. Add `CORS_ORIGINS` to the service's environment.
3. Read the service URL, put it in `frontend/.env.production` as
   `VITE_API_BASE_URL`, then `make deploy-web`. It is baked at build time, so
   the frontend must be rebuilt after the backend exists — not before.
4. Walk the acceptance test against the deployed URLs. That is the third and
   last "done when", and the only one still untouched.

**Next:** step 1 is not an engineering task. Everything downstream of it is
about twenty minutes.

---

## Priority change — 2026-07-27

**The next session is Phase 10, and it happens in the other repository.**

Deployment is parked, not abandoned: the database and the frontend are live, the
backend is blocked on a payment method rather than on anything in the code, and
`docs/DEPLOY.md` plus `deploy/deploy-api.sh` and `render.yaml` are ready for
whenever that clears. See the Phase 9 Session B log above.

The goal is now to get this project into the developer's portfolio site:

| | |
|---|---|
| Where the work happens | **`D:\Work\lw-sports-portfolio`** (`Nimrod-DG/portofolio`, branch `main`) |
| The plan | **`D:\Work\lw-sports-portfolio\PLAN-mini-erp.md`** — self-contained; start a session in that directory |
| This repository's role | Read-only source material, plus a Phase 10 entry appended here when the write-up exists |

The plan covers: rebuilding the local seeded database, a Playwright capture
script, a 27-shot list spanning **procurement, inventory, finance, tenants,
users, responsive and both themes**, the case-study sections to write, and the
one structural problem in the portfolio page — its project nav has hardcoded
section IDs that collide when a second case study is added.

The wording for deployment is fixed and should not drift: **deploy-ready, not
deployed.** Screenshots come from the app running locally. The deployed frontend
is **not** to be linked as "Live" — it stops at the login screen, because it was
built against a placeholder API URL.

---

## Phase 10 — 2026-07-27

Done: the portfolio write-up exists. `mini-erp` is the second case study on
`D:\Work\lw-sports-portfolio` (`Nimrod-DG/portofolio`), reachable at
`#project-mini-erp`, and the home grid now says "2 projects".

What was produced in that repository:

- **`capture-erp.mjs`** — a Playwright capture script. Sessions grouped by
  account × viewport × theme; document IDs resolved by navigating the lists
  rather than hard-coded; the one mutating shot (posting a goods receipt) runs
  last, after every read-only shot.
- **27 screenshots** in `shots/erp-*.jpg`, optimized into `shots/opt/`. Twenty-two
  of them are used on the page; the five dropped are plain CRUD lists
  (suppliers, product detail, warehouses, users, login) that carried no argument.
- **The case study** in `src/page.html`, with sections namespaced `erp-*` and a
  per-project topbar nav, which is what stops the two case studies' section links
  colliding.

Numbers cited on the page, all verified against this repository on the day:
376 backend tests, 102 frontend tests, 6 migrations, 14 tables under forced RLS,
2 `security_invoker` views, 11 `cmd/dbverify` checks, 3 modules, 5 ranked role
levels. Both suites were run green before publishing.

Deployment wording held as specified: **deploy-ready, not deployed**. The
deployed frontend is not linked anywhere on the page — the `.live` nav slot and
both call-to-action buttons point at the GitHub repository instead.

Also changed here: **`README.md`**, at the developer's request, so the project can
be run correctly from a clean clone. `make migrate` and `make seed` were missing
from the run sequence entirely, and three traps are now written down — the
Firebase service-account key that `make seed` needs, the demo accounts and what
each one demonstrates, and the fact that Vite silently moving off `:5173` puts it
outside `CORS_ORIGINS` and makes every screen render empty with nothing in any
log to explain it.

Tests green: full backend suite (376) and frontend suite (102), plus
`make verify-db` — all 11 checks against the local database.

Deviations from spec: two, both deliberate.
1. `optimize.mjs` crop fractions were *measured* — a throwaway script found the
   last row of real content in each capture — rather than eyeballed, because an
   ERP screen is mostly table and the dead canvas below the content varies
   enormously between a nine-row list and a two-line filtered result.
2. `build.mjs` now stamps intrinsic `width`/`height` onto every inlined image.
   This was a real bug, not a nicety: lazy images reserve no height, so the
   document grew *during* a smooth scroll and in-page nav links landed ~3,400px
   short of their section. Sizes are read from the files, so re-cropping a shot
   cannot leave a stale number behind.

TODO(post-mvp) markers added: none.

Next: nothing is blocked here. When a payment method clears, `make deploy-api`
and then `make deploy-web` with a real `VITE_API_BASE_URL`; at that point the
portfolio's status section and the `.live` nav slot are the two places that need
updating.

---

## Phase 10 (continued) — the README · 2026-07-27

Done: `README.md` rewritten in full, which closes the second half of Phase 10 —
the phase file asks for "a README with an architecture diagram, the RLS
explanation and the permission model", and the earlier entry above had only
fixed the run instructions.

What it now contains, in the order a reader needs it:

- **What it is** — three modules, the stack, and "modular monolith, not
  microservices" stated where nobody can miss it.
- **An architecture diagram** in Mermaid, so GitHub renders it inline rather than
  linking a picture that goes stale. It draws the six-step middleware chain
  explicitly, because the order *is* the design: token → identity from the
  database → `SET LOCAL` → entitlement × role → handler.
- **The RLS explanation** — the six-step chain in prose, then the actual policy
  SQL, including why `WITH CHECK` and the `NULLIF` are both load-bearing.
- **The permission model** — the three layers, the five ranked levels, the three
  tenant roles, the two distinguishable 403s, and segregation of duties as the
  layer-3 example.
- **The cross-module transaction** — a second Mermaid diagram plus the
  idempotency-key and savepoint reasoning.
- **Running it locally** — the section the developer asked to be unambiguous.
  Numbered from a clean clone, with backend and frontend as *separate* steps in
  *separate* terminals, each with both its `make` target and the raw command
  behind it. `make dev` is offered second, with the caveat that it interleaves
  both logs and needs a POSIX shell.
- Troubleshooting table, Make-target table, tests, layout, docs index, the
  invariants I1–I13, honest scope, and deployment status.

Both Mermaid blocks were parsed against mermaid 11 with `htmlLabels: false` and
`securityLevel: 'strict'` — the conservative configuration GitHub renders with —
so no `<b>`/`<i>` and no parentheses inside node labels. Every relative link in
the file was checked to resolve against a real path.

Phase 10 "Done when" — all four met:
- A reader who has never seen the code can explain the isolation model from the
  case study alone ✅ (and now from the README alone as well)
- Screenshots include the cross-module confirmation panel ✅
- The set covers all three modules plus tenants and users ✅
- The LW Sports case study still works unchanged ✅ — though it was *deliberately*
  changed afterwards, see below

Deviation from spec: the portfolio's LW Sports case study is no longer
byte-for-byte unchanged. Its Cloud Run backend went down for the same reason this
one is not deployed, so its "Live" link pointed at a login that cannot succeed.
It now reads "Beta · paused" and links to an explanation. Phase 10's criterion
meant "do not break it", and it is not broken.

Tests green: unchanged from the entry above — 376 backend, 102 frontend, 11
`dbverify` checks. No code was touched in this entry, only documentation.

Next: nothing outstanding for Phase 10. Phase 11 (the audit log) is the next
build work, and remains post-MVP.

---

## Next session — 2026-07-27

**The next piece of work is Phase 9.5, and the plan is written:
[`phases/phase-9.5-vercel.md`](phases/phase-9.5-vercel.md).** It is
self-contained; open it and follow it, and load only the three files it names.

**What it is.** Phase 9 finished the deployment work and stopped at a payment
wall — Cloud Run and Render both want a verified card. Vercel's Hobby tier does
not, so the phase ports the API onto it. It is a hosting workaround, not an
architectural improvement, and everything in it is additive: the Cloud Run and
Render paths must still work afterwards.

**Start with the spike in step 2, not with code.** Three unknowns decide whether
the rest is worth doing — whether Hobby still deploys without a card, whether a
catch-all preserves the original request path, and what the current function
duration limit is. Thirty minutes in a throwaway repository answers all three. If
the first answer is "no", record it here and stop; that closes the phase rather
than failing it.

**The four things that will actually cost time**, in order: pgx's prepared
statements against Neon's PgBouncer pooler (fails only under concurrency, and
never locally), connection limits from a `SetMaxOpenConns(10)` that serverless
multiplies sideways, the routing spike, and the Firebase key arriving as JSON in
an env var rather than as a file path.

**Two things already work in our favour.** The pools are lazy behind `sync.Once`,
so a cold start opens one pool and not two. And tenant scope is `SET LOCAL`
inside an explicit transaction (I2), which is precisely the unit transaction-mode
pooling preserves — so the isolation model should survive PgBouncer untouched.
Verify that rather than assuming it; step 7 has the two-tenant test. If it holds,
it is worth writing up.

**The trap to avoid** is putting a `go.mod` at the repository root to satisfy
Vercel's `api/` convention. That breaks I13. Set the Vercel project's root
directory to `backend` instead — step 3.

Nothing else is outstanding. Phase 10 is closed, both repositories are pushed,
and Phase 11 (the audit log) remains post-MVP.
