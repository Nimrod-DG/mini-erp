# Spec audit — what was wrong, and what changed

A full read of the original `ERP_BUILD_PLAN` v2.0 turned up sixteen problems.
**All sixteen have been fixed in the reference docs.** This file is the record of
what changed and why, so you can review the reasoning and reverse anything you
disagree with — it is not a to-do list, and Claude Code does not need to act on it.

Severity: **A** = would have stopped the build · **B** = wrong code that passes
review · **C** = inconsistency that wastes time · **D** = cosmetic.

| # | Problem | Fixed in |
|---|---|---|
| A1 | `erp_app` had no grants on the five platform tables it must read | `reference/tenancy-and-rls.md` §4.2 |
| A2 | `POST /admin/tenants` could not seed the chart of accounts | `reference/tenancy-and-rls.md` §4.2.1 (new) |
| A3 | `WithTenant` used a bind parameter in `SET LOCAL` | `reference/project-structure.md` |
| A4 | `%%` in the over-receipt trigger's `RAISE` format string | `reference/constraints-and-indexes.md` §6.10.6 |
| B1 | §5.7 labelled `erp_admin` as `BYPASSRLS` | `reference/permissions.md` §5.7 |
| B2 | View leak attributed to `BYPASSRLS` rather than ownership | `reference/schema.md` §6.3 |
| B3 | Idempotent replay read from an aborted transaction | `reference/business-logic.md` §8.6.1 |
| B4 | Same FK dropped twice across §6.10.1 and §6.10.9 | `reference/constraints-and-indexes.md` §6.10.9 |
| B5 | Uniqueness on SKUs and codes never actually declared | `reference/deletion-policy.md` §6.9.1 |
| B6 | Audit-log dual-write not implementable (post-MVP) | `phases/phase-11-audit-log.md` |
| C1 | The MVP test gate stated four different ways | `reference/tests.md` §12.6 |
| C2 | "fourteen steps" vs twenty-five | `phases/phase-7-dashboard-seed.md` |
| C3 | FE22–FE26 defined but assigned to no phase | `reference/tests.md` §12.5, `phases/phase-8-*` |
| C4 | Phase 1 estimated at both 4h and 5h | `phases/phase-1-schema-rls.md` |
| C5 | `accounts` is Tier-1 master data with no timestamps | `reference/schema.md` §6.5 |
| D1 | Broken table row, seed idempotency keys, numbering slips | various |

The detail on each follows. **A2/B6 share one fix** — a narrow `SECURITY DEFINER`
function — because they are the same problem appearing twice.

---

## A1 — `erp_app` has no grants on the tables it must read *(blocker, Phase 2)*

§4.2 says `erp_app` has *"no access to platform tables except read on `modules`."*
But four things `erp_app` does require exactly that access:

| Operation | Needs | Spec |
|---|---|---|
| Identity resolution, every request | `SELECT` on `users` | §3.2 step 5 |
| Module role resolution | `SELECT` on `user_module_roles` | §3.2 step 6 |
| Entitlement check in `RequireModule` | `SELECT` on `tenant_modules` | §7 |
| Document period allocation | `SELECT tenants.timezone` | §8.1.1 |
| Tenant user management `/api/tenant/users` | `INSERT/UPDATE` on `users`, `user_module_roles` | §9.3 |

As written, Phase 2 fails at the first request with `permission denied for table users`.

**Fixed** — added to `000_roles.sql` and to the §4.2 role table:

```sql
GRANT SELECT                 ON tenants, modules, tenant_modules TO erp_app;
GRANT SELECT, INSERT, UPDATE ON users, user_module_roles         TO erp_app;
```

Note these five tables carry no RLS (§6.8), so scoping is application-side: every
query filters by the `tenant_id` derived from the verified Firebase UID. §6.2
already says this — §4.2's grant table just contradicts it.

---

## A2 — `POST /api/admin/tenants` cannot do what it is specified to do *(blocker, Phase 3)*

§9.2 says this endpoint creates the tenant, **seeds the chart of accounts**, and
creates the first admin user, in one transaction, on the `erp_admin` pool.

But §4.2 explicitly revokes `accounts` from `erp_admin`. The account seeding step
is impossible from that pool. Splitting it across two pools loses the atomicity
the endpoint is specified to have.

**Fixed** — option 1 below, now written up as §4.2.1. The other two were considered and rejected:

1. **A `SECURITY DEFINER` function** owned by `erp_migrate` that inserts the two
   accounts for a given tenant. `erp_admin` gets `EXECUTE`. Keeps one transaction,
   keeps the revoke meaningful, and the privileged surface is two rows wide.
   **This is what was applied.**
2. Grant `erp_admin` `INSERT` on `accounts` only. Cheapest, but it weakens the
   "structurally incapable" claim in §4.2 that test A11 exists to prove.
3. Seed accounts lazily on the tenant's first Finance read, from the app pool.
   No privilege change, but it moves a platform concern into the tenant plane.

---

## A3 — `WithTenant` will fail at runtime as written *(blocker, Phase 1)*

§11 gives the one helper every handler depends on:

```go
tx.Exec("SET LOCAL app.current_tenant = ?", tenantID.String())
```

PostgreSQL does not accept bind parameters in `SET`. With pgx's prepared
statements this is a syntax error; if the driver instead inlines the string, you
have a SQL injection path through tenant ID.

**Fixed** — `set_config`, which is a normal function call and so takes parameters:

```go
func WithTenant(ctx context.Context, db *gorm.DB, tenantID uuid.UUID,
                fn func(tx *gorm.DB) error) error {
    return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // third arg true == transaction-local, identical to SET LOCAL
        if err := tx.Exec(
            "SELECT set_config('app.current_tenant', ?, true)",
            tenantID.String()).Error; err != nil {
            return err
        }
        return fn(tx)
    })
}
```

Test A6 (context does not survive the commit) still applies unchanged.

---

## A4 — `check_no_over_receipt` raises a plpgsql error *(blocker, Phase 1)*

§6.10.6, in the exception branch:

```sql
RAISE EXCEPTION
  'over_receipt: po_line %% ordered %%, would be received %%',
  NEW.po_line_id, ordered, received
```

In `RAISE`, `%` is the placeholder and `%%` is a literal percent sign. This format
string has zero placeholders and three arguments, which Postgres rejects with
*"too many parameters specified for RAISE"* — at trigger creation time is fine,
but at fire time every over-receipt produces the wrong error. Test H6 would fail
for the wrong reason.

**Fixed** — single `%` in all three positions. The `RAISE` two lines above it is
already correct, which is what makes this a typo rather than a misunderstanding.

---

## B1 — §5.7 contradicts §4.2 on `BYPASSRLS`

The two-control-planes table in §5.7 labels the admin pool
`erp_admin` **(BYPASSRLS)**. §4.2, §12A.5, and test A10 all require the opposite,
and the whole "revocation beats bypassing" argument depends on it.

**Fixed** — the cell now reads `erp_admin` (platform tables only; RLS irrelevant —
platform tables carry none). Leftover from the v1 design.

---

## B2 — "the owner has `BYPASSRLS`" is not why the view leaks

§6.3's `security_invoker` warning explains the leak as *"since the owner has
BYPASSRLS."* That is not the mechanism. A table **owner** bypasses its own RLS
policies by ownership, regardless of the `BYPASSRLS` role attribute — which is
exactly why §4.4 mandates `FORCE ROW LEVEL SECURITY`.

The conclusion (always set `security_invoker = true`) is right and tests A7/A8
still catch it. But an agent reading B1 and B2 together may "helpfully" reconcile
them by granting the owner `BYPASSRLS`, which would be a real regression.

**Fixed** — reworded to "because the view executes as its owner, which is not subject
to the policies."

---

## B3 — Idempotent replay cannot return the receipt from inside the aborted transaction

§8.6.1 step 2: *"On unique violation, roll back and return `200` with the existing
receipt."* Once the unique violation fires, the transaction is aborted and can
issue no further reads. The lookup has to happen in a **new** transaction.

**Fixed** — the sequence is now explicit in §8.6.1 and in the Phase 5 brief:

1. Tx1: insert `goods_receipts` with the key.
2. On `23505` against `goods_receipts_tenant_id_idempotency_key_key`: roll back,
   open Tx2, `SELECT` the receipt by `(tenant_id, idempotency_key)`, rebuild the
   same response body, return `200`.
3. Any *other* unique violation is a real error — match on the constraint name,
   not just the SQLSTATE.

---

## B4 — Migration ordering hazard on `goods_receipt_lines`

§6.10.1 replaces `goods_receipt_lines_po_line_id_fkey` with a composite FK on
`(po_line_id, tenant_id)`. Then §6.10.9 says to drop
`goods_receipt_lines_po_line_id_fkey` *again* and add `(po_line_id, product_id)`.

By that point the named constraint no longer exists and the migration fails.

**Fixed** — both composite FKs are wanted and they coexist, so they are now declared in one step:

```sql
ALTER TABLE purchase_order_lines
  ADD CONSTRAINT pol_id_tenant_uq  UNIQUE (id, tenant_id),
  ADD CONSTRAINT pol_id_product_uq UNIQUE (id, product_id);

ALTER TABLE goods_receipt_lines
  DROP CONSTRAINT goods_receipt_lines_po_line_id_fkey,
  ADD FOREIGN KEY (po_line_id, tenant_id)  REFERENCES purchase_order_lines (id, tenant_id),
  ADD FOREIGN KEY (po_line_id, product_id) REFERENCES purchase_order_lines (id, product_id);
```

---

## B5 — Uniqueness on master data codes is never actually declared

§6.9.1 says to *replace* `UNIQUE (tenant_id, sku)` and friends with partial unique
indexes. But the `CREATE TABLE` statements for `products`, `suppliers`,
`warehouses`, and `accounts` in §6.3–6.5 declare **no** unique constraint at all —
only the document tables do. `DROP INDEX IF EXISTS` hides this, so an agent that
skims §6.9.1 ends up with a schema where duplicate SKUs are legal.

Acceptance step 22 (duplicate SKU must give a clean validation error) fails.

**Fixed** — the partial unique indexes are now presented as the *only* declaration,
created explicitly in the first migration:

```sql
CREATE UNIQUE INDEX products_sku_active    ON products   (tenant_id, sku)  WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX suppliers_code_active  ON suppliers  (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX warehouses_code_active ON warehouses (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX accounts_code_active   ON accounts   (tenant_id, code) WHERE deleted_at IS NULL;
```

---

## B6 — Post-MVP: the audit log dual-write is not implementable as specified

For Phase 11 only, but worth deciding early because it shapes the grants.

§6.7 requires an entitlement change to write to `platform_audit_log` **and** to the
affected tenant's `audit_log`. But that write comes from a superadmin on the
`erp_admin` pool, and:

- `audit_log` is RLS-protected (§6.8), and `erp_admin` never sets tenant context;
- §6.7.2 grants `INSERT` on `audit_log` to `erp_app` only;
- §4.2 revokes `erp_admin` from every tenant business table.

Test FE19 as written cannot pass. **Fixed** — the Phase 11 brief now specifies
`log_tenant_event(...)` as a `SECURITY DEFINER` function with `EXECUTE` granted to
`erp_admin`, in the same shape as A2's `seed_tenant_accounts()`.

---

## C1 — Four different statements of the MVP test gate

| Location | Claims the gate is |
|---|---|
| §1.2 | never cut Groups **A or D** |
| §12.6 | every test in Groups **A, D, G, H, I** passes |
| §13 intro | never cut Groups **A, D, or I** |
| MVP callout box | Groups **A–F** must be green |

The MVP box is the weakest and the most prominently placed — A–F excludes G
(deletion), H (concurrency), I (invariants), and J (timezone), all of which are
MVP work assigned to Phases 1–5.

**Fixed** — one statement, now used everywhere: *the MVP gate is Groups A–J green
plus all 25 acceptance steps.* Groups A, D, and I are the ones that may never be
cut under time pressure.

---

## C2 — "fourteen steps" vs twenty-five

The MVP callout says *"All fourteen steps must pass."* §14 ends with *"If all
twenty-five pass, the build is complete."* Steps 21–25 (CRUD completeness,
isolation re-check) are the ones a shortened run would skip — and §9.6.1 exists
precisely because those are the demo failures that hurt.

**Fixed** — twenty-five, in the Phase 7 gate.

---

## C3 — FE22–FE26 are specified but never assigned to a phase

§9.6.1 cites FE22–FE25 as verifying MVP CRUD completeness. §12.5 defines
FE22–FE26. Phase 8 says only *"The FE1–FE15 tests."* So five tests exist in the
inventory, are cited as load-bearing, and are in no phase. §1.2 also defers all
frontend tests to post-MVP, which conflicts with §9.6.1 treating them as the
verification for an MVP requirement.

**Fixed** — Phase 8 now covers FE1–FE26. MVP verification of CRUD completeness is
acceptance steps 21–23, done by hand; the FE tests are the automation of that,
not the gate for it.

---

## C4 — Phase 1 estimate *(fixed: 5h)*

Table in §13 says 5h; the Phase 1 heading says ~4h. Given Phase 1 now carries all
of §6.9, §6.10, four triggers, and Groups A, I, J — 5h is the honest number, and
arguably light.

---

## C5 — `accounts` is Tier-1 master data but has no timestamps

`accounts` gets `deleted_at`/`deleted_by` but no `created_at`/`updated_at`, unlike
every other Tier-1 table. Test I8 ("every mutable table's `updated_at` advances")
has nothing to assert against, and `touch_updated_at` cannot attach.

**Fixed** — `created_at` and `updated_at` added to `accounts`, so `touch_updated_at`
and test I8 apply uniformly.

---

## C6 — `reversal` appears in a CHECK constraint before it exists

§6.10.2 permits `stock_ledger.source_type = 'reversal'`, but §6.3's column comment
lists only two values and §6.9.3 defers reversing entries to post-MVP. Harmless —
a permitted-but-unused value — but an agent may take it as licence to build
reversal in Phase 4. Left as is deliberately — a permitted-but-unused value costs nothing, and the
Phase 4 brief says not to build reversal.

---

## D1 — Small stuff

- §9.6.1's `Accounts` row has eight cells in a seven-column table; renders broken.
- §12.7 numbers its steps `1, 2, 2b, 3, 4, 5`.
- §16 is introduced as *"lead with these four"* and then lists seven.
- §5.3 says "Five levels" and the `user_module_roles` CHECK permits four — correct,
  since `none` is the absence of a row, but worth a one-line note so nobody
  "fixes" the constraint.
- The seed script runs receipts through `PostGoodsReceipt` (§15), which requires an
  `Idempotency-Key`. The seed must generate deterministic ones (e.g.
  `seed-gr-<tenant-slug>-<n>`) or reseeding stops being idempotent.
- No `GET /procurement/purchase-orders/:id/receipts`; §10.3 specifies a receipt
  history on the PO detail screen. Either nest it in the PO detail response or add
  the route.

---

## What is *not* wrong

Worth saying, because the list above is long and the document is unusually good:

- Every `Section N.N` cross-reference resolves to a real section. All ~80 of them.
- The phase order has no dependency inversions — nothing in Phase *n* needs
  something introduced in Phase *n+1*.
- Group G test IDs partition cleanly across Phases 4 and 5 with no gaps or overlap
  (G1–G6, G9–G11 in 4; G7–G8, G12–G14 in 5).
- Every table in the §6.8 RLS list has a definition in §6, and every tenant-scoped
  table defined in §6 appears in the §6.8 list.
- The deferred-vs-MVP boundary is consistent between §1.2, §6.7, and §13.
- The concurrency analysis in §6.10.6 and §8.6 is correct, including the
  non-obvious point about `READ COMMITTED` taking a fresh snapshot per statement.
