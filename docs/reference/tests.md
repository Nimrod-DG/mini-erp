# Reference — Testing strategy and full test inventory

> Each phase names its own test IDs. Look them up here; do not read the whole inventory.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 12. Testing strategy

Tests are written **as part of each phase**, not afterwards. A phase is not done until its tests pass.

### 12.1 Tooling

| Layer | Tool |
|---|---|
| Go unit + integration | standard `testing`, `stretchr/testify` |
| Real Postgres in tests | `testcontainers-go` |
| Firebase verification | hand-written fake implementing the `auth.Verifier` interface |
| Frontend unit | Vitest + React Testing Library |
| Frontend API mocking | MSW (Mock Service Worker) |
| Coverage | `go test -coverprofile`, `vitest --coverage` |

### 12.2 Why a real database in tests

**RLS cannot be tested against a mock, an in-memory fake, or SQLite.** The isolation guarantee is a property of PostgreSQL policy evaluation. A test suite that stubs the database proves nothing about the mechanism this project exists to demonstrate.

Use `testcontainers-go` to start a real Postgres per test package, run migrations against it, and connect as `erp_app` — the RLS-enforced role — not as the owner. Tests connected as the owner silently bypass policies and will pass while production leaks.

Shape of the harness in `testsupport/`:

```go
// Starts Postgres, runs migrations, creates roles, returns pools.
func NewTestDB(t *testing.T) *TestDB

// Creates a tenant with seeded accounts, warehouses, products, suppliers.
func (db *TestDB) NewTenant(t *testing.T, name string) *TenantFixture

// Creates a user in that tenant with the given per-module levels.
func (f *TenantFixture) NewUser(t *testing.T, roles map[string]string) *UserFixture
```

Every test that touches tenant data creates **two** tenants and asserts against both. Single-tenant tests cannot detect isolation failures.

### 12.3 Backend test inventory

Every test has a stable ID of the form `<Group><n>`. Reference these IDs in commit messages and in the phase checklists — they are cited throughout this document.

**Group A — Tenant isolation** (`internal/db`) · *highest priority, Phase 1*

| ID | Test |
|---|---|
| A1 | Rows created under tenant A are invisible when the session is set to tenant B |
| A2 | A query with no tenant context set returns zero rows, not all rows |
| A3 | An `INSERT` carrying tenant B's `tenant_id` while the session is tenant A is rejected by `WITH CHECK` |
| A4 | `UPDATE` and `DELETE` cannot reach another tenant's rows even by primary key |
| A5 | Every table in Section 6.8 has RLS enabled **and forced** — assert via `pg_tables.rowsecurity` and `pg_class.relforcerowsecurity`. Fails automatically when a table is added without a policy |
| A6 | `WithTenant` uses `SET LOCAL`: after commit, a fresh query on the same pooled connection sees no tenant context |
| A7 | **Isolation holds through `stock_balances`, not just `stock_ledger`** — a base-table test would pass even with `security_invoker` missing (Section 6.3) |
| A8 | Every view has `security_invoker = true` — assert via `pg_class.reloptions` |
| A9 | **Composite FK:** a `purchase_order_lines` row with `tenant_id` A but a product belonging to B is rejected by the FK. Run as table owner so RLS is not what blocks it (Section 6.10.1) |
| A10 | `erp_app` and `erp_admin` both have `rolbypassrls = false` and `rolsuperuser = false` — catches a role provisioned through a managed-provider console (Section 2.3.3) |
| A11 | A `SELECT` on `purchase_orders` as `erp_admin` raises a **permission error**, not an empty result (Section 4.2) |

**Group B — Permissions** · *Phase 3*

| ID | Test |
|---|---|
| B1 | `RequireModule` returns `module_not_enabled` when the tenant lacks entitlement |
| B2 | Returns `insufficient_module_role` when entitled but under-levelled |
| B3 | Level ordering: `admin` satisfies a `viewer` requirement; `viewer` does not satisfy `approver` |
| B4 | A missing `user_module_roles` row is treated as `none` |
| B5 | Toggling an entitlement off takes effect on the next request without a restart |
| B6 | A superadmin cannot reach any tenant business endpoint |
| B7 | A tenant `admin` resolves to `admin` in every entitled module with **no** `user_module_roles` rows |
| B8 | A tenant `admin` of a tenant lacking an entitlement still resolves to `none` — entitlement beats the admin shortcut |
| B9 | Demoting, deactivating, or deleting the last active admin returns `409 last_admin` |
| B10 | Two concurrent attempts to demote the last two admins cannot both succeed |

**Group C — Procurement rules** · *Phase 5*

| ID | Test |
|---|---|
| C1 | Submitting a requisition with zero lines → `422` |
| C2 | Approving one's own requisition → `403 self_approval_forbidden` |
| C3 | Rejecting without a reason → `422` |
| C4 | Approving an already-approved requisition → `409` |
| C5 | Editing a submitted requisition → `409` |
| C6 | Approval generates a PO whose lines and `total_amount` match the requisition |

**Group D — Goods receipt (the critical path)** · *Phase 5*

| ID | Test |
|---|---|
| D1 | Full receipt sets PO status to `received` |
| D2 | Partial receipt sets `partially_received`, and `po_line_status` reflects the new quantity |
| D3 | Two sequential partial receipts completing the order set `received` |
| D4 | Over-receipt by any amount → `422`, and **nothing is written** — assert ledger and journal counts unchanged |
| D5 | A receipt writes exactly one ledger row per line, with correct sign, cost, and warehouse |
| D6 | After a partial receipt, `po_line_status` reports correct `qty_received` and `qty_outstanding` with no stored column |
| D7 | A receipt writes exactly one journal entry whose debit total equals its credit total |
| **D8** | **Rollback test** — inject a failure at the journal-posting step; assert the goods receipt, receipt lines, PO status change, and stock ledger entries are **all** absent. This proves atomicity and is the single most valuable test in the suite |
| D9 | Stock balances after a receipt equal the sum of ledger deltas |

**Group E — Document numbering** · *Phase 5*

| ID | Test |
|---|---|
| E1 | Sequence increments per tenant independently |
| E2 | Sequence resets at a period boundary |
| E3 | 20 goroutines allocating simultaneously produce 20 distinct numbers with no gaps |
| E4 | A rolled-back transaction does not consume a number |
| E5 | Period is computed in the **tenant's** timezone, not the server's (Section 8.1.1) |

**Group F — Inventory** · *Phase 4*

| ID | Test |
|---|---|
| F1 | Balance view equals the ledger sum for a product/warehouse pair |
| F2 | Low-stock query returns exactly the products under their reorder point |
| F3 | Manual adjustment with a negative delta reduces the balance |

**Group G — Deletion policy** (Section 6.9) · *Phases 4–5*

| ID | Test |
|---|---|
| G1 | A soft-deleted product is absent from default queries and present under `.Unscoped()` |
| G2 | Soft-deleting a product then creating a new one with the same SKU succeeds — the partial unique index permits it |
| G3 | Restoring the first product while the replacement SKU exists is rejected with `409` |
| G4 | Deleting a supplier with an open PO returns `409 in_use`; one with only closed POs succeeds |
| G5 | Deleting a warehouse with non-zero stock returns `409 in_use` |
| G6 | A PO line referencing a soft-deleted product still resolves the product name |
| G7 | Cancelling a `received` PO returns `409`; cancelling an `open` PO succeeds and records actor and timestamp |
| G8 | Cancelling an `approved` requisition returns `409` — the PO must be cancelled instead |
| G9 | `UPDATE`/`DELETE` against `stock_ledger` and `journal_entries` as `erp_app` raise permission errors |
| G10 | Deactivating a user preserves their `actor_id` on historical documents |
| G11 | `po_line_status.qty_received` equals the sum of that line's receipt lines; `qty_outstanding` equals the remainder |
| G12 | An invalid status string (e.g. `'recieved'`) is rejected by the CHECK constraint |
| G13 | Rejecting a requisition without a reason is rejected by `pr_reject_needs_reason` at the database level, not only the handler |
| G14 | Adding the same product twice to one PO violates `pol_one_line_per_product` |

**Group H — Concurrency and idempotency** (Section 8.6) · *Phase 5*

| ID | Test |
|---|---|
| H1 | Replaying a receipt with the same `Idempotency-Key` returns `200` with the original receipt and creates **no** second set of ledger or journal rows |
| H2 | A receipt without an `Idempotency-Key` header returns `400` |
| H3 | Two different keys against the same PO both post, as two legitimate partial receipts |
| H4 | Two goroutines approving the same requisition produce exactly one PO; the loser gets `409` |
| H5 | Two goroutines posting receipts that would jointly over-receive: one succeeds, one fails; derived `qty_received` never exceeds `qty_ordered` |
| H6 | **Trigger backstop** — insert into `goods_receipt_lines` via raw SQL, bypassing the service layer, in an over-receiving amount. `grl_no_over_receipt` must reject it, proving the guard is not merely application-level |
| H7 | Two goroutines demoting the last two admins: exactly one succeeds (cross-checks B10) |

**Group I — Database-enforced invariants** (Sections 6.10.6–6.10.9) · *Phase 1*

| ID | Test |
|---|---|
| I1 | Committing a journal entry with one line raises `check_violation` — a posting needs at least two |
| I2 | Two lines whose debits and credits differ raise at **commit**, not at insert — proves the trigger is deferred |
| I3 | A balanced two-line entry commits cleanly; the intermediate single-line state does not fail |
| I4 | Deleting one line of a balanced pair raises at commit |
| I5 | Updating an `approved` requisition via raw SQL raises; `submitted` → `approved` succeeds |
| I6 | Updating a `received` PO via raw SQL raises; `open` → `partially_received` succeeds |
| I7 | A `goods_receipt_lines` row whose product differs from its PO line's product is rejected by the composite FK |
| I8 | Every mutable table's `updated_at` advances on update |

**Group J — Time and timezone** (Section 2.5) · *Phase 1*

| ID | Test |
|---|---|
| J1 | The database session timezone is `UTC` — assert `SHOW timezone`, run against both the local container and the deployed instance |
| J2 | Go reports `time.Local == time.UTC` in the API process |
| J3 | A timestamp written and read back is byte-identical regardless of the client's `TZ` environment variable |
| J4 | A document created at 23:30 Asia/Jakarta on the last day of a month lands in the **tenant's** period, not the UTC one (cross-checks E5) |

### 12.4 Faking Firebase in tests

Define the verifier as an interface so tests never touch the network:

```go
type Verifier interface {
    Verify(ctx context.Context, idToken string) (uid string, err error)
}
```

Production wires the Firebase Admin SDK. Tests wire a fake whose `Verify` maps a token string straight to a UID. Handler tests then simply send `Authorization: Bearer user-sari` and the fake resolves it.

Do not test Firebase itself. Test that your middleware rejects invalid tokens, resolves valid ones to the right user, and refuses deactivated users. Specifically:

- A valid token whose UID matches no `users` row returns `401`, not a `500` — the orphaned-account case from Section 3.3.
- A token for a user whose `is_active = false` returns `401`.
- Identity resolution reads `tenant_id` from the database row, never from a token claim or request parameter.

### 12.5 Frontend tests

Component tests with MSW mocking `/api/*`. IDs are `FE<n>`.

**All of FE1–FE26 are built in Phase 8, post-MVP.** MVP verification of CRUD
completeness is acceptance steps 21–23, run by hand; FE22–FE26 are the automation
of those steps, not the gate for them.

| ID | Test |
|---|---|
| FE1 | Nav renders only modules present in `/api/me` |
| FE2 | The "Approve" button is absent for a `user`-level account, present for `approver` |
| FE3 | The receipt form rejects a quantity exceeding outstanding before submitting |
| FE4 | The receipt form pre-fills outstanding quantities |
| FE5 | The cross-module confirmation panel renders both the ledger and journal links |
| FE6 | A `403 module_not_enabled` response redirects to the dashboard with a message |
| FE7 | Below `md`, the requisition list renders as cards; at `lg` it renders a semantic `<table>` |
| FE8 | The bottom tab bar renders only entitled modules — no disabled fourth tab |
| FE9 | No interactive control in the receipt form has a hit area below 44×44px |
| FE10 | Theme toggle cycles light → dark → system and persists to `localStorage` |
| FE11 | With `theme=system`, changing `prefers-color-scheme` flips the applied class without a reload |
| FE12 | Every status badge renders text alongside colour — assert on accessible name |
| FE13 | Skeleton rows match loaded row height — assert no layout shift |
| FE14 | First-run empty state and no-results empty state render different copy and actions |
| FE15 | Timestamps render in the **tenant's** timezone, not the browser's (Section 2.5.3) |
| FE22 | Each master-data list renders create, edit, and delete affordances for an `admin` and none of them for a `viewer` |
| FE23 | The edit form pre-populates from the detail response and submits only changed fields |
| FE24 | Deleting from a list optimistically removes the row and restores it if the request fails |
| FE25 | "Show deleted" reveals soft-deleted rows with a restore action |
| FE26 | Numeric columns render with `tabular-nums` and right alignment (Section 10.0.2) |

Post-MVP, with Phase 11 (audit log):

| ID | Test |
|---|---|
| FE16 | A staff user reads the audit trail of a document they can access; `403` for one they cannot |
| FE17 | A tenant admin sees entries from all users; staff have no global audit route |
| FE18 | A superadmin querying tenant `audit_log` through the app pool gets zero rows |
| FE19 | An entitlement change writes to both `platform_audit_log` and the tenant's `audit_log` |
| FE20 | `UPDATE`/`DELETE` against `audit_log` as `erp_app` raise permission errors |
| FE21 | An action via the tenant-admin implicit shortcut records `metadata.via = "tenant_admin_implicit"` |

### 12.6 Coverage targets

| Package | Target |
|---|---|
| `internal/procurement` (incl. `receipt.go`) | **90%+** |
| `internal/db`, `internal/middleware` | **90%+** |
| `internal/inventory`, `internal/finance` | 80%+ |
| Handlers and wiring | 60%+ |
| Frontend components | 60%+ |

Coverage percentage is a weak signal on its own.

**The MVP gate, stated once and used everywhere:** Groups **A–J** green, plus all
**25** acceptance steps. Every one of those groups is assigned to an MVP phase
(A/I/J in Phase 1, B in 3, F and part of G in 4, C/D/E/H and the rest of G in 5),
so a narrower gate would leave MVP work unverified.

Under time pressure, Groups **A**, **D**, and **I** may never be cut — they are
the tests that prove the two claims the project exists to make.

### 12.7 CI

GitHub Actions on push and pull request:

1. `go vet ./...`
2. `golangci-lint run`
2b. `govulncheck ./...`
3. `go test ./... -race -coverprofile=coverage.out` (testcontainers needs Docker — available on `ubuntu-latest`)
4. `npm run lint && npm run test -- --coverage` in `frontend/`
5. Build both containers

Do not deploy from CI for this project; deploy manually. Automated deploy adds setup cost with no portfolio value.
