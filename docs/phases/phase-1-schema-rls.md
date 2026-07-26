# Phase 1 — Schema, RLS, and database-enforced invariants

**MVP:** yes · **Estimate:** 5h (the §13 table's 4h is optimistic) · **Depends on:** Phase 0 green

> This is the phase everything downstream assumes. **Do not proceed to Phase 2
> until Groups A, I, and J are green.** A leak found here costs an hour; found in
> Phase 5 it costs the migration history.

## Load only these

1. [`../reference/tenancy-and-rls.md`](../reference/tenancy-and-rls.md) — in full.
2. [`../reference/schema.md`](../reference/schema.md) — in full.
3. [`../reference/constraints-and-indexes.md`](../reference/constraints-and-indexes.md) — in full.
4. [`../reference/deletion-policy.md`](../reference/deletion-policy.md) — the three tiers and the partial-index gotcha.
5. [`../reference/tests.md`](../reference/tests.md) — Groups **A**, **I**, **J** only, plus §12.2 (why a real database).
6. [`../AUDIT.md`](../AUDIT.md) — skim only. Five corrections (A3, A4, B2, B4, B5) landed in this phase's files and are already applied above; the audit records what changed and why.

Do not load auth, permissions, API, business logic, or any frontend doc.

## Build

**Everything below goes in the first migrations.** Adding a composite FK or a
partial unique index after data exists means backfilling under pressure.

1. **Platform tables** — `tenants`, `modules`, `tenant_modules`, `users`,
   `user_module_roles`. No `tenant_id` on the first three; **no RLS on any of the five**.
2. **Tenant tables** — inventory, procurement, finance, `document_sequences`.
   Not `audit_log` — that is Phase 11.
3. **RLS** on every table in the §6.8 list: `ENABLE` **and** `FORCE`, with the
   `USING` + `WITH CHECK` policy template. Both clauses; omitting `WITH CHECK`
   lets a tenant write rows tagged with another tenant's ID.
4. **Both views** `WITH (security_invoker = true)`. The leak it prevents comes
   from view *ownership*, not from any role holding `BYPASSRLS` — do not "fix" a
   failing A7 by granting anything.
5. **Composite FKs** carrying `tenant_id` (§6.10.1), including the parents'
   redundant `UNIQUE (id, tenant_id)`. The `goods_receipt_lines` FKs are declared
   in a single statement — see the ordering note in §6.10.9.
6. **Partial unique indexes** on `products`, `suppliers`, `warehouses`, `accounts`.
   These are the *only* uniqueness declaration on those tables — the `CREATE TABLE`
   statements declare none. Skip them and duplicate SKUs are legal.
7. **CHECK constraints** on every status/type column, plus the conditional-field
   constraints and line-number uniqueness.
8. **The four triggers:**
   - `grl_no_over_receipt` — `AFTER INSERT`, `DEFERRABLE INITIALLY IMMEDIATE`.
   - `jel_balanced` — **`DEFERRABLE INITIALLY DEFERRED`**. An immediate trigger
     fails on every first line insert; this is the whole point.
   - `pr_terminal_immutable` / `po_terminal_immutable` — `BEFORE UPDATE`,
     inspecting `OLD.status` so the transition *into* a terminal state still works.
   - `touch_updated_at` on each mutable table.
9. **Grants**: `REVOKE UPDATE, DELETE` on the five ledger tables from `erp_app`;
   `REVOKE ALL` on tenant business tables from `erp_admin`.
10. **The index set** from §6.10.5. `tenant_id` leads almost every index because
    the RLS policy adds that predicate to every query.
11. **`testsupport/` harness** — testcontainers, migrations, `NewTestDB`,
    `NewTenant`, `NewUser`. Connect as **`erp_app`**, never the owner: tests
    connected as the owner silently bypass policies and pass while production leaks.

## Do not build

Handlers. Models beyond what the tests need. Any Go business logic. The audit log
tables. Seed data beyond test fixtures.

## Tests to write in this phase

**Group A** (A1–A11) · **Group I** (I1–I8) · **Group J** (J1–J4)

Every test that touches tenant data creates **two** tenants and asserts against
both. A single-tenant test cannot detect an isolation failure.

The five that matter most, because each one catches a class of silent failure:

| ID | Catches |
|---|---|
| A5 | A table added later without a policy — fails automatically |
| A7 | Isolation through `stock_balances`, not just the base table |
| A9 | Cross-tenant FK reference — run **as owner**, so RLS is not what blocks it |
| A11 | `erp_admin` gets a *permission error*, not an empty result |
| I2 | The journal trigger fires at **commit**, not at insert |

## Done when

- [ ] Groups A, I, J all green
- [ ] `SHOW timezone` returns `UTC` in the container (J1)
- [ ] A11 raises `permission denied`, not zero rows — if it returns zero rows the
      `REVOKE` did not apply
- [ ] `make migrate` runs clean from an empty database, twice in a row

## Then

Record in `PROGRESS.md` that the schema is green and which migration files exist.
Phase 2 assumes the `erp_app` grants from `000_roles.sql` are in place.
