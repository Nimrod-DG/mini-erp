# Phase 4 — Inventory core

**MVP:** yes · **Estimate:** 4h · **Depends on:** Phase 3 green

## Load only these

1. [`../reference/schema.md`](../reference/schema.md) — **§6.3 only** (inventory tables and the balances view).
2. [`../reference/deletion-policy.md`](../reference/deletion-policy.md) — in full. This phase is where Tier 1 gets built for real.
3. [`../reference/api.md`](../reference/api.md) — **§9.5 and §9.0 (list conventions) only**.
4. [`../reference/screens.md`](../reference/screens.md) — **§10.4 only**.
5. [`../reference/tests.md`](../reference/tests.md) — **Group F and Group G (G1–G6, G9–G11) only**.

Do not load procurement or finance docs. This module is deliberately buildable
without them.

## Build

1. Products and warehouses: full CRUD with soft delete and restore.
2. **Stock ledger — append-only.** No `UPDATE`, no `DELETE`, no soft delete. The
   `REVOKE` from Phase 1 is the enforcement; the service layer should simply
   never try.
3. `stock_balances` reads. Stock on hand is **always** `SUM(qty_delta)` from the
   view — never a column, never a cache.
4. Low-stock query: products below `reorder_point`.
5. `POST /inventory/adjustments` (level `approver`) writing
   `entry_type = 'adjustment'`, `source_type = 'manual_adjustment'`.
6. In-use checks before soft delete, returning `409 in_use` **naming what blocks it**:
   - warehouse with non-zero stock
   - product with open PO lines *(this check has no data to act on until Phase 5 —
     write it now, test it in Phase 5)*
7. "Show deleted" list filter (`?includeDeleted=true`, module `admin` only) with a
   restore action. Any user who can delete can restore.
8. Screens: product list with current stock and reorder flag, product detail with
   that product's ledger history, stock grid, full filterable ledger.

## Rules that are easy to get wrong here

- `is_active` and `deleted_at` are **two different questions**. Discontinued
  (`is_active = false`) still appears in lists, reports, and the ledger; deleted
  (`deleted_at`) is hidden everywhere but still resolvable by foreign key.
- A PO line pointing at a soft-deleted product **must still render the product
  name** — use `.Unscoped()` deliberately at those call sites (G6).
- Do not add `current_stock` to `products`. If a query feels slow, add an index.

## Tests to write

**Group F** (F1–F3) · **Group G**: G1–G6, G9–G11.
G2 (reuse a soft-deleted SKU) is the one that proves the partial unique index
actually exists — it is easy to build this whole phase without it and notice
nothing until acceptance step 22.

## Done when

- [ ] Groups F and the listed G tests green
- [ ] You can create a product, post an adjustment, and watch the balance move
      through the view
- [ ] Deleting a product hides it from pickers while historical ledger rows still
      resolve its name
