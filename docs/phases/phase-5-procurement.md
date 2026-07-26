# Phase 5 — Procurement · **the main event**

**MVP:** yes · **Estimate:** 8h · **Depends on:** Phase 4 green

> This phase contains the one thing the project exists to demonstrate: a single
> business event writing to three modules in one transaction. If you are short of
> time anywhere, be short of time somewhere else.

This is the largest phase. **Split it across at least two sessions** at the
marked checkpoint — the context needed for the requisition lifecycle is not the
context needed for the receipt handler.

## Load only these

### Session A — requisitions and POs

1. [`../reference/schema.md`](../reference/schema.md) — **§6.4 and §6.6 only**.
2. [`../reference/business-logic.md`](../reference/business-logic.md) — **§8.1, §8.2, §8.3, §8.6.2**.
3. [`../reference/api.md`](../reference/api.md) — **§9.4 only**.
4. [`../reference/decisions/003-time-and-timezone.md`](../decisions/003-time-and-timezone.md) — before writing the numbering code.
5. [`../reference/tests.md`](../reference/tests.md) — **Groups C and E** only.

### Session B — the goods receipt

1. [`../reference/business-logic.md`](../reference/business-logic.md) — **§8.4 and §8.6** in full. Read twice.
2. [`../reference/schema.md`](../reference/schema.md) — **§6.5** (finance tables) and the `po_line_status` view.
3. [`../reference/constraints-and-indexes.md`](../reference/constraints-and-indexes.md) — **§6.10.6 and §6.10.7** (the two triggers you are working against).
4. [`../reference/screens.md`](../reference/screens.md) — **§10.3**, especially the confirmation panel.
5. [`../reference/tests.md`](../reference/tests.md) — **Groups D and H**.
6. [`../reference/api.md`](../reference/api.md) — **§9.0** (list conventions) if you have not already.

---

## Session A — build

1. **Suppliers**: CRUD with soft delete, restore, and the in-use check
   (`409 in_use` when open or partially-received POs reference it).
2. **Document numbering** (§8.1). Three things make this correct:
   - the period is `to_char(now() AT TIME ZONE t.timezone, 'YYYYMM')` — the
     **tenant's** timezone, read from `tenants`, not `now()` in the session zone;
   - the locking upsert on `document_sequences`, whose `DO UPDATE` takes the row
     lock that serialises concurrent allocation;
   - it runs in the **same transaction** as the document insert, so a rollback
     does not consume a number.
3. **Requisition lifecycle** (§8.2): create, edit-while-draft, submit, approve,
   reject, cancel. Approve/reject/cancel each take `SELECT … FOR UPDATE` on the
   requisition **before** checking status — read-then-check without the lock is
   a race that produces two POs from one requisition.
4. **Segregation of duties**: `decided_by == requested_by` → `403
   self_approval_forbidden`. Applies to tenant admins too. This is a *record*
   rule, not a role rule — do not implement it in the middleware.
5. **PO generation on approval** (§8.3), in the same transaction: allocate the PO
   number, copy lines with `line_no` preserved, compute `total_amount`. Nothing
   to initialise for received quantity — it is derived.
6. Screens: requisition list with status chips, create form, detail with status
   timeline and role-appropriate actions, PO list and detail.

**Checkpoint — stop here.** Groups C and E should be green. Record progress.

---

## Session B — build the goods receipt

`procurement.PostGoodsReceipt(tx *gorm.DB, actor Identity, poID uuid.UUID, req ReceiptRequest) (*ReceiptResult, error)`

One function. One transaction. It calls into `inventory` and `finance` service
functions, **passing the same `tx`**. Do not split it across HTTP calls,
goroutines, or background jobs — atomicity is the entire point, and hiding it in
a trigger makes the story invisible in the code.

| Step | Do |
|---|---|
| 1 | `SELECT … FOR UPDATE` on the affected PO lines, **then** validate: PO status is `open` or `partially_received`; each `poLineId` belongs to this PO; each qty > 0; `qty_received + new <= qty_ordered` read from `po_line_status`. Over-receipt → `422 over_receipt` naming the offending lines. |
| 2 | Allocate the GR number, insert `goods_receipts` with the idempotency key |
| 3 | One `goods_receipt_lines` row per submitted line |
| 4 | Re-read `po_line_status`; set PO to `received` if every line is complete, else `partially_received`. No per-line column to update. |
| 5 | **[INVENTORY]** one `stock_ledger` row per receipt line — `entry_type='receipt'`, `qty_delta=+qty`, `unit_cost` from the PO line, `warehouse_id` from the PO, `source_type='goods_receipt'`, `source_id`=GR id |
| 6 | **[FINANCE]** one `journal_entries` row, two lines: **Dr 1300** Inventory, **Cr 2150** GRNI, both `SUM(qty × unit_cost)`. **Assert debits == credits before insert.** |
| 7 | `// TODO(post-mvp): audit gr.posted` — do not build the audit row |
| 8 | Commit |

Response carries the receipt, the new PO status, and the IDs of the ledger rows
and journal entry, so the confirmation panel can link straight to them.

### Idempotency (§8.6.1)

The client generates a UUID **when the form opens**, not when it submits, and
sends it as `Idempotency-Key`. Missing or malformed → `400`; do not generate one
server-side, that defeats the purpose.

On unique violation the replay lookup needs a **new** transaction — the aborted
one can issue no further reads — and it must match on the constraint name
`goods_receipts_tenant_id_idempotency_key_key`, not bare SQLSTATE `23505`. A
duplicate `gr_number` is also a `23505` and is a real bug; returning `200` for it
would hide a numbering failure.

### Two guards, two jobs

The `grl_no_over_receipt` and `jel_balanced` triggers are backstops. The handler
does the user-facing work and produces the clean error. A trigger violation
reaching the client is a bug to investigate, not a normal path.

### The confirmation panel

> Goods receipt `GR-202607-0004` posted.
> → Inventory: 2 stock ledger entries created
> → Finance: journal entry `JE-202607-0004` posted (Dr Inventory 4,500,000 / Cr GRNI 4,500,000)

Both lines link. This is the screenshot that goes in the portfolio, and it is the
one place in the whole UI where boldness is budgeted.

## Do not build

Reversing entries. Vendor invoices. Multi-level approval. `DELETE
/requisitions/:id` for symmetry. The audit log.

## Tests to write

**Group C** (C1–C6) · **Group D** (D1–D9) · **Group E** (E1–E5) ·
**Group G**: G7, G8, G12–G14 · **Group H** (H1–H7)

**D8 is the single most valuable test in the suite.** Inject a failure at the
journal-posting step and assert the goods receipt, its lines, the PO status
change, **and** the stock ledger entries are all absent. That test is the proof
behind the project's main claim.

H6 is its structural twin: insert into `goods_receipt_lines` via raw SQL, past
the service layer, in an over-receiving amount, and assert the trigger rejects it.

## Done when

- [ ] Groups C, D, E, H green — **especially D8 and H1**
- [ ] G7, G8, G12–G14 green
- [ ] The acceptance test steps 6–20 pass end to end by hand
- [ ] `npx fallow audit` in `frontend/` is clean (§12A.4)
