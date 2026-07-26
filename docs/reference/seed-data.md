# Reference — Seed data

> Phase 7. The acceptance test depends on these exact users and emails.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 15. Seed data

`cmd/seed` must produce a demo-ready database. Seeding must be idempotent.

**Platform**

- **One superadmin: `super@erp.test`** (`tenant_role = 'superadmin'`, `tenant_id = NULL`). This is the platform operator — the only account that can register a new tenant, create its first admin, and toggle module entitlements. It belongs to no tenant and can read no tenant business data (Sections 4.2, 5.7).
- **Two tenants**, each with its own staff and its own entitlement set:

| Tenant | Timezone | Procurement | Inventory | Finance |
|---|---|---|---|---|
| **Nusantara Retail** | `Asia/Jakarta` | ✅ | ✅ | ✅ |
| **Bahari Logistics** | `Asia/Makassar` | ✅ | ✅ | ❌ |

Two tenants is the minimum that makes isolation testable at all — a single-tenant seed cannot demonstrate that anything is isolated. The differing entitlements are what make the module-gating visible without touching the admin console, and the differing timezones exercise Section 2.5.3.

**Nusantara Retail users** (entitled to all three modules) — the Section 5.6 matrix:

| Email | Tenant role | Procurement | Inventory | Finance |
|---|---|---|---|---|
| `rina@nusantara.test` | `admin` | *implicit* `admin` | *implicit* `admin` | *implicit* `admin` |
| `budi@nusantara.test` | `staff` | `approver` | `viewer` | `none` |
| `sari@nusantara.test` | `staff` | `user` | `user` | `none` |
| `dewi@nusantara.test` | `staff` | `viewer` | `none` | `admin` |

**Bahari Logistics users** (entitled to Procurement + Inventory only — **no Finance**):

| Email | Tenant role | Procurement | Inventory | Finance |
|---|---|---|---|---|
| `agus@bahari.test` | `admin` | *implicit* `admin` | *implicit* `admin` | **`none`** — not entitled |
| `manager@bahari.test` | `staff` | `approver` | `approver` | `none` |
| `staff@bahari.test` | `staff` | `user` | `viewer` | `none` |

Agus is the important seed row: a **tenant admin who still has no Finance access**, because his tenant has no Finance entitlement. That single user demonstrates that the admin shortcut sits below the entitlement ceiling, and it is what acceptance test step 5 checks.

Do not create `user_module_roles` rows for Rina or Agus — their access is derived from `tenant_role` (Section 5.4).

All seeded users: password `password123`, created in the **`erp-dev`** Firebase project with deterministic `seed-<slug>` UIDs (Section 3.5.3) so reseeding is idempotent. Do not seed demo accounts into `erp-prod`.

**Per tenant — master data**

- The tenant's own timezone set as above
- **2 warehouses** (e.g. "Gudang Pusat", "Gudang Cabang")
- **10 products** across at least 3 loose categories, varied `reorder_point` and `standard_cost`, with **at least 3 deliberately below reorder point** after all seeding completes, so the low-stock widget is populated on first load
- **1 soft-deleted product**, so the "Show deleted" filter and restore flow have something to act on
- **5 suppliers**, varied `lead_time_days` and `payment_terms`; **1 inactive**, so the active/inactive distinction is visible
- Accounts `1300` and `2150`

**Per tenant — inventory history**

Do not seed all stock in one timestamp. Spread `occurred_at` across the **preceding 60 days** so the ledger looks like a system that has been running, and so date filters and "recent activity" have something to sort:

- An opening `adjustment` per product, dated ~60 days ago
- **15–25 further ledger entries** per tenant spread across the window: receipts from the completed POs below, plus a few manual `adjustment` entries (including at least one negative, e.g. a stock count correction)
- Entries spread across **both** warehouses, so the stock grid is not a single column

**Per tenant — procurement history**

Enough documents that every list screen has content and every status filter returns something:

| Status | Count | Purpose |
|---|---|---|
| Requisition `draft` | 2 | Edit and submit flows have a starting point |
| Requisition `submitted` | 3 | The approval queue is populated on first load |
| Requisition `rejected` | 1 | Rejection display and reason rendering |
| Requisition `cancelled` | 1 | Cancelled-state rendering |
| PO `open` | 2 | Receiving flow has a target |
| PO `partially_received` | 1 | Partial state, with its receipt and ledger rows |
| PO `received` | 2 | **Completed cross-module flows, with ledger entries and balanced journal entries present** |
| PO `cancelled` | 1 | Cancelled-state rendering |

The two fully-received POs matter most: **a reviewer opening the app cold should immediately see a completed procurement → inventory → finance flow without performing one first.** Their journal entries are what make the Finance "coming soon" page (Section 10.5) look alive rather than empty.

Vary `created_at` across the same 60-day window and set `expected_at` on open POs to a mix of past and future dates, so "overdue" is visually distinguishable.

**Seeding order.** Master data → opening stock → requisitions → approvals (generating POs) → receipts.

Because receipts go through `PostGoodsReceipt`, which requires an idempotency key
(Section 8.6.1), the seed must supply **deterministic** ones —
`seed-gr-<tenant-slug>-<n>` — or a reseed posts every receipt a second time and
the seed stops being idempotent. Run receipts through `PostGoodsReceipt` rather than inserting ledger and journal rows directly — that way the seed exercises the same code path as the application and cannot drift from it. If the seed produces an unbalanced journal, the `jel_balanced` trigger (Section 6.10.7) will reject it, which is exactly the safety net you want.
