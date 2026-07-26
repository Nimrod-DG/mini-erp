# Reference — Business logic

> Phase 5. §8.4 is the most important handler in the codebase.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 8. Business logic specification

### 8.1 Document numbering

Format: `<PREFIX>-<YYYYMM>-<SEQ4>` — e.g. `PR-202607-0001`, `PO-202607-0003`, `GR-202607-0012`, `JE-202607-0007`.

Sequence resets monthly **in the tenant's timezone** (Section 2.5.3), is per-tenant, and must be allocated inside the same transaction as the document insert.

#### 8.1.1 Computing the period

```sql
SELECT to_char(now() AT TIME ZONE t.timezone, 'YYYYMM')
FROM tenants t WHERE t.id = $1;
```

Do **not** use `to_char(now(), 'YYYYMM')`. That evaluates in the session timezone, which is UTC, and would place a document created just after midnight in Jakarta into the previous month. Test E5 and J4 cover this. A Postgres sequence will not work — sequences are not tenant-aware and do not roll back. Allocate with a locking upsert:

```sql
INSERT INTO document_sequences (tenant_id, doc_type, period, last_number)
VALUES ($1, $2, $3, 1)
ON CONFLICT (tenant_id, doc_type, period)
DO UPDATE SET last_number = document_sequences.last_number + 1
RETURNING last_number;
```

The `DO UPDATE` takes a row lock, so concurrent allocations serialise rather than colliding. Section 12.3 includes a concurrency test for this.

### 8.2 Purchase requisition lifecycle

```
draft ──submit──> submitted ──approve──> approved  (generates PO)
                      │
                      └────reject───> rejected
draft ──cancel──> cancelled
```

Rules:

- Creating requires Procurement `user` or above. Creator is `requested_by`.
- A requisition must have at least one line to be submitted.
- Approving or rejecting requires Procurement `approver` or above.
- **A user may not approve their own requisition.** If `decided_by == requested_by`, respond `403 {"error":"self_approval_forbidden"}`. This is segregation of duties — a record-level rule that role level alone does not express.
- Rejection requires a non-empty `reject_reason`.
- Approval creates the purchase order (Section 8.3) in the same transaction.
- Approved, rejected, and cancelled requisitions are immutable. Editing one returns `409`.

### 8.3 Purchase order generation

On approval, in one transaction:

1. Set `status = 'approved'`, `decided_by`, `decided_at`.
2. Allocate a PO number.
3. Insert `purchase_orders` with `status = 'open'`, copying `supplier_id` and `warehouse_id` from the requisition.
4. Copy each requisition line: `qty_ordered = line.qty`, `unit_cost = line.est_unit_cost`, `line_no` preserved. Received quantity is derived, so there is nothing to initialise.
5. Set `total_amount = SUM(qty_ordered * unit_cost)`.
6. *(Post-MVP — Section 6.7.)* Write an `audit_log` row (`pr.approved`) and another (`po.created`).

If the requisition has no `supplier_id`, require one in the approval request body; reject with `422` if absent.

### 8.4 Goods receipt — the cross-module transaction

**This is the most important handler in the codebase.** Everything below happens in **one** database transaction. If any step fails, all of it rolls back.

Endpoint: `POST /api/procurement/purchase-orders/:id/receipts`
Required level: Procurement `approver`.

Request body:

```json
{
  "note": "Delivered by supplier truck, 2 boxes",
  "lines": [
    { "poLineId": "uuid", "qtyReceived": 25 },
    { "poLineId": "uuid", "qtyReceived": 10 }
  ]
}
```

Steps:

1. **Lock and validate.** Take `SELECT … FOR UPDATE` on the affected `purchase_order_lines` (Section 8.6.3). Then check: PO exists, belongs to this tenant, `status` is `open` or `partially_received`; every `poLineId` belongs to this PO; every `qtyReceived > 0`; and for each line, `qty_received + qtyReceived <= qty_ordered` read from the `po_line_status` view. Over-receipt is rejected with `422 {"error":"over_receipt"}` naming the offending lines. The `grl_no_over_receipt` trigger (Section 6.10.6) is the database-level backstop, but the handler should produce the clean error.
2. **Create the receipt header.** Allocate a GR number, insert `goods_receipts`.
3. **Create receipt lines.** One `goods_receipt_lines` row per submitted line.
4. **Update PO status.** Re-read `po_line_status` for this PO. If every line now satisfies `qty_received = qty_ordered`, set the header to `received`; otherwise `partially_received`. There is no per-line quantity to update — received quantity is derived (Section 6.4).
5. **[INVENTORY] Write stock ledger entries.** One row per receipt line:
   - `entry_type = 'receipt'`, `qty_delta = +qtyReceived`
   - `unit_cost` = the PO line's `unit_cost`
   - `warehouse_id` = the PO's `warehouse_id`
   - `source_type = 'goods_receipt'`, `source_id` = the GR id
6. **[FINANCE] Post a journal entry.** One `journal_entries` row with two lines:
   - **Debit** account `1300` (Inventory) for `SUM(qtyReceived * unit_cost)`
   - **Credit** account `2150` (Goods received not invoiced) for the same total
   - `source_type = 'goods_receipt'`, `source_id` = the GR id
   - **Assert debits equal credits before insert.** If not, return an error and roll back. The `jel_balanced` deferred trigger (Section 6.10.7) independently rejects an unbalanced entry at commit — but the handler check gives the clean, attributable error.
7. **Audit.** *(Post-MVP — Section 6.7.)* Write an `audit_log` row (`gr.posted`). During the MVP, leave `// TODO(post-mvp): audit gr.posted` here.
8. **Commit.**

Response returns the created receipt, the updated PO status, and the IDs of the stock ledger entries and journal entry created, so the UI can link straight to them.

> **Note for Claude Code:** implement this as a single service function `procurement.PostGoodsReceipt(tx *gorm.DB, actor Identity, poID uuid.UUID, req ReceiptRequest) (*ReceiptResult, error)`. It calls into `inventory` and `finance` service functions, passing the same `tx`. Do not split it across multiple HTTP calls, goroutines, or background jobs — atomicity is the entire point.

### 8.5 Module entitlement enforcement

Enforcement happens in **two** places, and both are required:

- **Backend (authoritative):** `RequireModule` middleware, Section 7.
- **Frontend (cosmetic):** `/api/me` returns the enabled module list with the user's level in each; the shell hides nav items accordingly.

Never rely on the frontend alone. A hidden nav item is not a permission check.

---

### 8.6 Concurrency and idempotency

Three places in this system are genuinely racy. All three are cheap to get right and awkward to retrofit.

#### 8.6.1 Duplicate goods receipts — idempotency

The goods receipt is submitted from a phone, at a loading dock, on warehouse wifi (Section 10.7.1). A request that times out client-side but succeeds server-side is not an edge case there — it is a Tuesday. Without protection, the user taps "Post receipt" again and stock is credited twice, with two journal entries to match. Nothing in the schema would flag it, because a second partial receipt is a legitimate operation.

**Require an idempotency key on the receipt endpoint.** The client generates a UUID when the form is opened — not when it is submitted — and sends it as an `Idempotency-Key` header. It stays constant across retries of that same form.

```
POST /api/procurement/purchase-orders/:id/receipts
Idempotency-Key: 7c9e6679-7425-40de-944b-e07fc1f90ae7
```

Handler behaviour, inside the transaction:

1. Insert `goods_receipts` with the key. The `UNIQUE (tenant_id, idempotency_key)` constraint is the guard.
2. On unique violation, return `200` with the **existing** receipt — the same body the first call returned. A retry must look like a success, not an error, or the user taps again.

   Two details make this work rather than merely sound right:

   - **The lookup needs a second transaction.** Once the unique violation fires,
     the transaction is aborted and can issue no further reads. Roll back, open a
     new transaction, `SELECT` the receipt by `(tenant_id, idempotency_key)`, and
     rebuild the response from it.
   - **Match on the constraint name, not just SQLSTATE `23505`.** A duplicate
     `gr_number` is also a unique violation, and it is a real bug — returning
     `200` for it would hide the numbering failure. Check for
     `goods_receipts_tenant_id_idempotency_key_key` specifically; anything else
     propagates as a `500`.
3. A missing or malformed header is `400`. Do not silently generate one server-side; that defeats the purpose.

This is the difference between a demo and something that survives contact with a warehouse, and it is a strong thing to be able to point at.

#### 8.6.2 Concurrent approval of the same requisition

Two managers open the same pending requisition and both tap Approve. A naive handler reads `status = 'submitted'`, both pass the check, and both proceed — producing two purchase orders from one requisition.

Lock the row before checking its status:

```sql
SELECT * FROM purchase_requisitions
WHERE id = $1 FOR UPDATE;
```

The second transaction blocks until the first commits, then re-reads `status = 'approved'` and correctly returns `409`. Read-then-check without the lock is a race; the `FOR UPDATE` makes the check-and-act atomic.

Apply the same pattern to requisition rejection, PO cancellation, and the last-admin rule in Section 5.4.

#### 8.6.3 Concurrent receipts against one purchase order

Two receipts posted simultaneously against the same PO can both compute `qty_received = 0` from `po_line_status` and each write 25 against a line ordered at 40. Each passes validation individually; together they over-receive.

Lock the affected PO lines with `SELECT … FOR UPDATE` before validating quantities. The `grl_no_over_receipt` constraint trigger (Section 6.10.6) independently serialises on the same row, so a missed lock produces a database error rather than corrupt stock — but rely on the handler lock for the clean error message, and treat a trigger violation reaching the client as a bug to investigate.

#### 8.6.4 Why not optimistic locking

A `version` column with a compare-and-swap would also work, and is the better choice under high contention because it avoids holding locks across a transaction. It is not used here because these documents have very low contention — a handful of users per tenant — and pessimistic `FOR UPDATE` is simpler to reason about and impossible to forget to check. Worth knowing the alternative exists and why it was not chosen.
