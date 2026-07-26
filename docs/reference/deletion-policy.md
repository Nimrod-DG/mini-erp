# Reference — Deletion policy (three tiers)

> Phases 1, 4, and 5. Tier is decided per table; do not generalise.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 6.9 Deletion policy — nothing is ever hard-deleted

**There is no `DELETE` statement anywhere in this application's business logic.** Data preservation is not a nice-to-have in an ERP — a deleted supplier orphans historical purchase orders, a deleted product makes last quarter's stock ledger unreadable, and either one silently corrupts reports that people make decisions on.

But "soft delete everything" is the wrong generalisation. ERPs use **three different mechanisms** depending on what the data is, and using the wrong one is its own bug.

| Tier | Data | Mechanism | Rationale |
|---|---|---|---|
| **1. Master data** | products, suppliers, warehouses, accounts, users | Soft delete (`deleted_at`) + deactivate (`is_active`) | Referenced by history forever; must remain resolvable |
| **2. Transactional documents** | requisitions, purchase orders | Status transition to `cancelled` — **never** deleted or soft-deleted | The document happened. Cancelling is itself a business event with a date and an actor. |
| **3. Immutable ledgers** | `stock_ledger`, `journal_entries`, `journal_entry_lines`, `goods_receipts`, `audit_log` | **Append-only.** No delete, no soft delete, no update. Corrections are new reversing entries. | A ledger you can edit is not a ledger. |

#### 6.9.1 Tier 1 — master data

Two separate columns, because they answer two different questions:

| Column | Question | Effect |
|---|---|---|
| `is_active` | Can this be used in **new** documents? | Hidden from pickers; still visible in lists and reports |
| `deleted_at` | Should this appear **at all**? | Hidden everywhere by default; recoverable |

A discontinued product is `is_active = false` — it still shows in the ledger, stock still counts, historical POs still resolve. A product someone created by mistake five minutes ago is `deleted_at = now()` — gone from the UI, still resolvable by foreign key.

`products`, `suppliers`, `warehouses`, and `accounts` all carry `deleted_at` and `deleted_by`, defined inline in Section 6.

**Gotcha — unique constraints must exclude deleted rows.** `UNIQUE (tenant_id, sku)` breaks the moment someone soft-deletes `SKU-001` and creates a new product with the same SKU. Replace every such constraint with a partial unique index:

> **These indexes are the *only* uniqueness declaration on these four tables.**
> The `CREATE TABLE` statements in Sections 6.3–6.5 declare no unique constraint
> on `sku` or `code` at all — only the document tables do. So this is not
> "replacing" an existing constraint; skip it and duplicate SKUs become legal,
> and acceptance step 22 fails. Create all four explicitly, in the first migration:

```sql
CREATE UNIQUE INDEX products_sku_active
  ON products   (tenant_id, sku)  WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX suppliers_code_active
  ON suppliers  (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX warehouses_code_active
  ON warehouses (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX accounts_code_active
  ON accounts   (tenant_id, code) WHERE deleted_at IS NULL;
``` This is the single most common soft-delete bug, and it surfaces only once real users start deleting things.

**Filtering is automatic, not manual.** Use GORM's built-in soft delete rather than remembering a `WHERE` clause everywhere:

```go
type Product struct {
    ID        uuid.UUID
    TenantID  uuid.UUID
    SKU       string
    DeletedAt gorm.DeletedAt `gorm:"index"`   // auto-filters every query
}
```

GORM appends `WHERE deleted_at IS NULL` to all queries on that model automatically. Use `.Unscoped()` deliberately when resolving a historical reference — a PO line pointing at a deleted product must still render the product name.

**Deleting is blocked when it would break something in flight.** Before soft-deleting a supplier, check for open or partially-received POs; before a warehouse, check for non-zero stock; before a product, check for open PO lines. Reject with `409 {"error":"in_use","detail":"3 open purchase orders reference this supplier"}` and tell the user what is blocking. Historical references do **not** block — that is the whole point of soft delete.

**Restore is a first-class action.** Any user who can delete can restore. Deleted master data is reachable through a "Show deleted" filter on the relevant list screen, not through a hidden URL.

#### 6.9.2 Tier 2 — transactional documents

Requisitions and purchase orders are **never** removed, not even softly. They move to `cancelled`, which records who cancelled, when, and why — the same shape as approve and reject.

- A `draft` requisition may be cancelled by its creator.
- A `submitted` requisition may be cancelled by its creator or an `approver`.
- An `approved` requisition cannot be cancelled — cancel the resulting PO instead.
- An `open` PO may be cancelled by an `approver`. A `partially_received` or `received` PO **cannot** — goods have physically arrived and the ledger has already recorded them. Reject with `409`.

That last rule is worth stating out loud in an interview: cancellation is constrained by what has already happened in the real world, not by what the UI finds convenient.

#### 6.9.3 Tier 3 — immutable ledgers

Stock ledger entries, journal entries, goods receipts, and audit entries are append-only. There is no delete and no edit.

A goods receipt entered in error is corrected by posting a **reversing entry** — an equal and opposite ledger row referencing the original — not by removing the mistake. Both rows stay visible. That is precisely what makes the trail trustworthy, and it is the same reasoning behind the period-close finalization in Section 8.4.

Enforce it at the grant level so a future bug cannot violate it:

```sql
REVOKE UPDATE, DELETE ON stock_ledger, journal_entries, journal_entry_lines,
  goods_receipts, goods_receipt_lines FROM erp_app;
```

> **Note for reversing entries (post-MVP).** Reversal is not required for the MVP — the acceptance test never reverses a receipt. Add `reverses_id UUID REFERENCES stock_ledger(id)` when building it, so a correction points at what it corrects.

#### 6.9.4 Users and tenants

- **Users** are deactivated (`is_active = false`), never deleted — they are the `actor_id` on every historical document. In Firebase, **disable** the account rather than deleting it, so the UID is never reissued to someone else. The last-admin rule (Section 5.4) applies to deactivation too.
- **Tenants** are suspended (`status = 'suspended'`), never deleted. A suspended tenant's users cannot log in; the data stays intact.

**Genuine data erasure — for example a GDPR deletion request — is an operational procedure run against the database with an audit record, not a button in the application.** Anything reachable from the UI is recoverable. Say this plainly in the README; conflating "user pressed delete" with "data is gone forever" is how ERPs lose quarters of history.
