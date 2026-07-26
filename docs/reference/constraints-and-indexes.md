# Reference — Constraints, triggers, and indexes

> Phase 1, same migrations as the schema. Do not defer any of this.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 6.10 Integrity constraints and indexes

The table definitions above are deliberately readable. This section adds the constraints and indexes that make them correct and fast. **Include these in the same migrations, not as a later pass.**

#### 6.10.1 Composite foreign keys — tenant consistency

This is the most important constraint in the schema, and its absence is a real vulnerability rather than a style issue.

**Foreign key checks bypass row-level security.** PostgreSQL performs referential integrity checks with the privileges of the table owner, precisely so that RLS cannot break data integrity. The consequence: a plain `FOREIGN KEY (product_id) REFERENCES products(id)` will happily accept a product ID belonging to **another tenant**, because the FK check never consults the policy.

RLS stops tenant A *reading* tenant B's products, but nothing above stops a bug — or a guessed UUID — from writing a `purchase_order_lines` row tagged `tenant_id = A` that points at a product owned by B. The row then passes the `WITH CHECK` policy (its own `tenant_id` is A) and becomes a permanent cross-tenant reference. It also creates a probe: FK violation versus success reveals whether a UUID exists in another tenant.

The fix is to make tenant membership part of the reference. Add a redundant unique key on every parent, then reference both columns:

```sql
-- Parents: unique key that includes tenant_id
ALTER TABLE products              ADD CONSTRAINT products_id_tenant_uq              UNIQUE (id, tenant_id);
ALTER TABLE warehouses            ADD CONSTRAINT warehouses_id_tenant_uq            UNIQUE (id, tenant_id);
ALTER TABLE suppliers             ADD CONSTRAINT suppliers_id_tenant_uq             UNIQUE (id, tenant_id);
ALTER TABLE accounts              ADD CONSTRAINT accounts_id_tenant_uq              UNIQUE (id, tenant_id);
ALTER TABLE purchase_requisitions ADD CONSTRAINT purchase_requisitions_id_tenant_uq UNIQUE (id, tenant_id);
ALTER TABLE purchase_orders       ADD CONSTRAINT purchase_orders_id_tenant_uq       UNIQUE (id, tenant_id);
ALTER TABLE purchase_order_lines  ADD CONSTRAINT purchase_order_lines_id_tenant_uq  UNIQUE (id, tenant_id);
ALTER TABLE goods_receipts        ADD CONSTRAINT goods_receipts_id_tenant_uq        UNIQUE (id, tenant_id);
ALTER TABLE journal_entries       ADD CONSTRAINT journal_entries_id_tenant_uq       UNIQUE (id, tenant_id);

-- Children: composite FKs replacing the single-column ones
ALTER TABLE purchase_requisition_lines
  DROP CONSTRAINT purchase_requisition_lines_requisition_id_fkey,
  DROP CONSTRAINT purchase_requisition_lines_product_id_fkey,
  ADD FOREIGN KEY (requisition_id, tenant_id)
      REFERENCES purchase_requisitions (id, tenant_id) ON DELETE CASCADE,
  ADD FOREIGN KEY (product_id, tenant_id)
      REFERENCES products (id, tenant_id);
```

Apply the same treatment to every tenant-scoped child reference:

| Child table | Composite FKs required on |
|---|---|
| `purchase_requisitions` | `warehouse_id`, `supplier_id` |
| `purchase_requisition_lines` | `requisition_id`, `product_id` |
| `purchase_orders` | `requisition_id`, `supplier_id`, `warehouse_id` |
| `purchase_order_lines` | `po_id`, `product_id` |
| `goods_receipts` | `po_id` |
| `goods_receipt_lines` | `gr_id`, `po_line_id`, `product_id` |
| `stock_ledger` | `product_id`, `warehouse_id` |
| `journal_entry_lines` | `journal_entry_id`, `account_id` |

References to `users` stay single-column: `users` has no `tenant_id` for superadmins, and superadmin actors legitimately appear on tenant audit rows (Section 6.7).

This costs one redundant unique index per parent and buys a guarantee the application cannot violate even with a bug. It is also the single best thing in this schema to be able to explain in an interview, because most people assume RLS covers it.

#### 6.10.2 Enum-like columns need CHECK constraints

Every status and type column above is `TEXT` with the permitted values in a comment. Comments do not constrain anything — a typo like `'recieved'` inserts cleanly and then silently fails every status filter.

```sql
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_status_valid
  CHECK (status IN ('draft','submitted','approved','rejected','cancelled'));
ALTER TABLE purchase_orders ADD CONSTRAINT po_status_valid
  CHECK (status IN ('open','partially_received','received','cancelled'));
ALTER TABLE stock_ledger ADD CONSTRAINT ledger_entry_type_valid
  CHECK (entry_type IN ('receipt','issue','adjustment'));
ALTER TABLE stock_ledger ADD CONSTRAINT ledger_source_type_valid
  CHECK (source_type IN ('goods_receipt','manual_adjustment','reversal'));
ALTER TABLE stock_ledger ADD CONSTRAINT ledger_qty_nonzero
  CHECK (qty_delta <> 0);
ALTER TABLE accounts ADD CONSTRAINT account_type_valid
  CHECK (type IN ('asset','liability','equity','revenue','expense'));
ALTER TABLE journal_entries ADD CONSTRAINT je_source_type_valid
  CHECK (source_type IN ('goods_receipt','manual'));
ALTER TABLE tenants ADD CONSTRAINT tenant_status_valid
  CHECK (status IN ('active','suspended'));
```

Prefer `CHECK` over a Postgres `ENUM` type: adding a value to an enum requires `ALTER TYPE` and is awkward to roll back, whereas a check constraint is a normal migration.

#### 6.10.3 Conditional field requirements

Fields that are mandatory only in certain states should say so:

```sql
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_reject_needs_reason
  CHECK (status <> 'rejected' OR (reject_reason IS NOT NULL AND reject_reason <> ''));
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_decided_fields_together
  CHECK ((decided_by IS NULL) = (decided_at IS NULL));
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_submitted_has_timestamp
  CHECK (status = 'draft' OR submitted_at IS NOT NULL);
```

#### 6.10.4 Line numbering

```sql
ALTER TABLE purchase_requisition_lines
  ADD CONSTRAINT prl_line_no_uq UNIQUE (requisition_id, line_no),
  ADD CONSTRAINT prl_line_no_positive CHECK (line_no > 0);
ALTER TABLE purchase_order_lines
  ADD CONSTRAINT pol_line_no_uq UNIQUE (po_id, line_no),
  ADD CONSTRAINT pol_line_no_positive CHECK (line_no > 0);
```

Also add a partial unique index so the same product cannot appear twice on one order — a real data-entry mistake that makes receipt quantities ambiguous:

```sql
CREATE UNIQUE INDEX pol_one_line_per_product ON purchase_order_lines (po_id, product_id);
```

#### 6.10.5 Indexes

PostgreSQL does **not** automatically index foreign key columns, and **every query in this application filters by `tenant_id`** because the RLS policy adds that predicate. So `tenant_id` should lead almost every index.

```sql
-- Tenant-leading indexes for list screens
CREATE INDEX idx_products_tenant        ON products (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_suppliers_tenant       ON suppliers (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_warehouses_tenant      ON warehouses (tenant_id) WHERE deleted_at IS NULL;

-- Document lists, filtered by status and sorted by recency
CREATE INDEX idx_pr_tenant_status ON purchase_requisitions (tenant_id, status, created_at DESC);
CREATE INDEX idx_po_tenant_status ON purchase_orders (tenant_id, status, ordered_at DESC);

-- The approval queue is the hottest dashboard query
CREATE INDEX idx_pr_pending ON purchase_requisitions (tenant_id, submitted_at DESC)
  WHERE status = 'submitted';

-- Open POs for the dashboard widget
CREATE INDEX idx_po_open ON purchase_orders (tenant_id, ordered_at DESC)
  WHERE status IN ('open','partially_received');

-- FK columns used in joins
CREATE INDEX idx_prl_requisition ON purchase_requisition_lines (requisition_id);
CREATE INDEX idx_pol_po          ON purchase_order_lines (po_id);
CREATE INDEX idx_grl_gr          ON goods_receipt_lines (gr_id);
CREATE INDEX idx_grl_po_line     ON goods_receipt_lines (po_line_id);
CREATE INDEX idx_gr_po           ON goods_receipts (po_id);
CREATE INDEX idx_jel_entry       ON journal_entry_lines (journal_entry_id);

-- Source lookups: "show me the ledger rows this receipt created"
CREATE INDEX idx_ledger_source ON stock_ledger (tenant_id, source_type, source_id);
CREATE INDEX idx_je_source     ON journal_entries (tenant_id, source_type, source_id);

-- Identity resolution, run on every request
CREATE INDEX idx_users_tenant ON users (tenant_id) WHERE is_active = true;
```

The existing `idx_stock_ledger_lookup (tenant_id, product_id, warehouse_id, occurred_at DESC)` also serves the `stock_balances` aggregate.

#### 6.10.6 Cross-table constraints: the over-receipt trigger

An earlier draft of this schema stored `qty_received` on `purchase_order_lines` as a running total, purely so that `CHECK (qty_received <= qty_ordered)` could guard against over-receipt. That was a workaround for a real limitation — **a CHECK constraint cannot reference another table** — but it contradicted the "derive, never store" principle behind the stock ledger, and it created a value that could drift.

The correct tool for a cross-table invariant is a **constraint trigger**:

```sql
CREATE OR REPLACE FUNCTION check_no_over_receipt() RETURNS trigger AS $$
DECLARE
  ordered  NUMERIC(18,4);
  received NUMERIC(18,4);
BEGIN
  -- Lock the parent line. This serialises concurrent receipts against the
  -- same line: a second transaction blocks here until the first commits.
  SELECT qty_ordered INTO ordered
  FROM purchase_order_lines
  WHERE id = NEW.po_line_id
  FOR UPDATE;

  IF ordered IS NULL THEN
    RAISE EXCEPTION 'po_line % not found', NEW.po_line_id
      USING ERRCODE = 'foreign_key_violation';
  END IF;

  SELECT COALESCE(SUM(qty_received), 0) INTO received
  FROM goods_receipt_lines
  WHERE po_line_id = NEW.po_line_id;

  IF received > ordered THEN
    RAISE EXCEPTION
      'over_receipt: po_line % ordered %, would be received %',
      NEW.po_line_id, ordered, received
      USING ERRCODE = 'check_violation';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER grl_no_over_receipt
  AFTER INSERT ON goods_receipt_lines
  DEFERRABLE INITIALLY IMMEDIATE
  FOR EACH ROW EXECUTE FUNCTION check_no_over_receipt();
```

**Why this is race-safe**, which is the part that usually sinks this approach:

| Step | Transaction A | Transaction B |
|---|---|---|
| 1 | Inserts a receipt line for PO line X | Inserts a receipt line for the same X |
| 2 | Trigger takes `FOR UPDATE` on X | Trigger requests the same lock — **blocks** |
| 3 | Sums receipts (sees its own row), passes, commits | still blocked |
| 4 | lock released | Acquires the lock |
| 5 | | The `SUM` is a separate statement, so under `READ COMMITTED` it takes a fresh snapshot that **includes A's committed row** — sees A + B, correctly rejects |

The shared parent-row lock is what serialises them. Note the trigger must be `AFTER INSERT`, so `NEW` is already visible to the aggregate, and the `SUM` must be a separate statement from the `FOR UPDATE` — Postgres does not permit `FOR UPDATE` alongside `GROUP BY`.

**What this buys.** `qty_received` disappears from the schema; the value is read from the `po_line_status` view; there is no duplicated aggregate and therefore no drift and no reconciliation test to run. The guarantee is stronger than before: the old CHECK only held if every write path also updated the stored column, whereas the trigger fires on any insert from any code path, including a manual `psql` session.

**The honest trade-offs:**

- Triggers are hidden control flow. Someone reading `goods_receipt_lines` will not see the rule unless they look for it. Mitigate by naming it clearly and referencing it in the model comment.
- Per-row overhead: two extra queries per receipt line. Irrelevant at this scale; would matter for bulk imports of thousands of lines, which this system does not do.
- Some teams ban triggers outright as a matter of policy. If you are in one of those teams, the stored-column version with an application-level lock is a defensible fallback — but say so deliberately rather than by accident.

**Application-level checking is still required.** The trigger raises a database exception, which is the wrong shape for an API response. `PostGoodsReceipt` should still take `SELECT … FOR UPDATE` on the affected lines and validate against `po_line_status` first, returning a clean `422 over_receipt` with per-line detail. The trigger is the backstop that catches what the handler misses — belt and braces, with the belt doing the user-facing work.

#### 6.10.7 Journal entries must balance — a deferred constraint trigger

This is the most valuable trigger in the schema, and arguably more important than the over-receipt one. **An unbalanced journal entry is a corrupt ledger**, and right now the only thing preventing one is an assertion in Go (Section 8.4, step 6). Any future code path that posts a journal — a manual adjustment screen, a data fix, a migration — bypasses that assertion entirely.

The invariant spans rows: for every `journal_entry_id`, the sum of debits must equal the sum of credits, and there must be at least two lines. No `CHECK` constraint can express it.

It also cannot be an **immediate** trigger. Lines are inserted one at a time, so after the first insert the entry is legitimately unbalanced — an immediate check would fail every time. This is the textbook case for `DEFERRABLE INITIALLY DEFERRED`, which runs the check once at `COMMIT`:

```sql
CREATE OR REPLACE FUNCTION check_journal_balanced() RETURNS trigger AS $$
DECLARE
  entry   UUID := COALESCE(NEW.journal_entry_id, OLD.journal_entry_id);
  debits  NUMERIC(18,2);
  credits NUMERIC(18,2);
  lines   INT;
BEGIN
  SELECT COALESCE(SUM(debit), 0), COALESCE(SUM(credit), 0), COUNT(*)
  INTO debits, credits, lines
  FROM journal_entry_lines
  WHERE journal_entry_id = entry;

  -- Entry removed entirely (e.g. cascade from a deleted tenant in tests)
  IF lines = 0 THEN
    RETURN NULL;
  END IF;

  IF lines < 2 THEN
    RAISE EXCEPTION 'journal entry % has % line(s); a posting needs at least 2',
      entry, lines USING ERRCODE = 'check_violation';
  END IF;

  IF debits <> credits THEN
    RAISE EXCEPTION 'journal entry % is unbalanced: debits %, credits %',
      entry, debits, credits USING ERRCODE = 'check_violation';
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER jel_balanced
  AFTER INSERT OR UPDATE OR DELETE ON journal_entry_lines
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION check_journal_balanced();
```

Because it is deferred, the transaction can build the entry line by line and is only rejected at commit if it does not balance. The failure aborts the **entire** transaction — which is exactly right, since an unbalanced posting means the goods receipt that produced it should not stand either.

Keep the application-level assertion too. The trigger produces a database error at commit time, which is hard to attribute to a specific request; the handler check produces a clean error and fails fast. Same belt-and-braces split as the over-receipt guard.

#### 6.10.8 Terminal states must be immutable

Section 8.2 says approved, rejected, and cancelled requisitions are immutable, and the handler returns `409`. That is a handler promise, not a guarantee — a bug elsewhere can still `UPDATE` an approved requisition and silently rewrite a decided document.

```sql
CREATE OR REPLACE FUNCTION forbid_terminal_update() RETURNS trigger AS $$
BEGIN
  IF OLD.status = ANY (TG_ARGV) THEN
    RAISE EXCEPTION '% % is in terminal state "%" and cannot be modified',
      TG_TABLE_NAME, OLD.id, OLD.status USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pr_terminal_immutable
  BEFORE UPDATE ON purchase_requisitions
  FOR EACH ROW EXECUTE FUNCTION forbid_terminal_update('approved','rejected','cancelled');

CREATE TRIGGER po_terminal_immutable
  BEFORE UPDATE ON purchase_orders
  FOR EACH ROW EXECUTE FUNCTION forbid_terminal_update('received','cancelled');
```

The transition *into* a terminal state still works, because the trigger inspects `OLD.status` — which is still `submitted` at the moment of approval. Only subsequent updates are blocked.

#### 6.10.9 Cheaper alternatives — prefer these where they fit

Not every cross-table rule needs a trigger. Two in this schema are better solved otherwise:

**Receipt lines must reference the same product as their PO line.** `goods_receipt_lines.product_id` duplicates what `po_line_id` already determines. Rather than a trigger, extend the composite-FK technique from Section 6.10.1:

> **Ordering — write this as one statement, not two.** Section 6.10.1 already
> dropped `goods_receipt_lines_po_line_id_fkey` when it added the tenant-composite
> FK, so dropping it again here fails. Both composite FKs are wanted and they
> coexist; declare them together:

```sql
ALTER TABLE purchase_order_lines
  ADD CONSTRAINT pol_id_tenant_uq  UNIQUE (id, tenant_id),
  ADD CONSTRAINT pol_id_product_uq UNIQUE (id, product_id);

ALTER TABLE goods_receipt_lines
  DROP CONSTRAINT goods_receipt_lines_po_line_id_fkey,
  ADD FOREIGN KEY (po_line_id, tenant_id)  REFERENCES purchase_order_lines (id, tenant_id),
  ADD FOREIGN KEY (po_line_id, product_id) REFERENCES purchase_order_lines (id, product_id);
```

Now the receipt line cannot name a different product from the line it receives against. Declarative, free, no procedural code.

**Ledger immutability.** Already handled by `REVOKE UPDATE, DELETE` (Section 6.9.3). Grants are simpler than a trigger and cannot be accidentally disabled by an `ALTER TABLE … DISABLE TRIGGER`. Keep the grants; do not add a trigger for this.

**`updated_at` maintenance.** A convenience, not an invariant. The column is already defined on every mutable table in Section 6 (`tenants`, `users`, `tenant_modules`, `user_module_roles`, `products`, `suppliers`, `warehouses`, `purchase_requisitions`, `purchase_orders`); this trigger keeps it current:

```sql
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;
```

attached `BEFORE UPDATE` on each. This is the one trigger nobody objects to.

#### 6.10.10 Where triggers would be wrong

Triggers enforce **invariants**. They must not carry **business logic**. Three specific temptations to refuse:

**Do not write the stock ledger from a trigger on `goods_receipt_lines`.** It looks tidy — receipt line in, ledger row out, automatically. It is the wrong call here for two reasons. First, the explicit cross-module orchestration in `PostGoodsReceipt` **is the thing this project exists to demonstrate**; hiding it inside a trigger makes the ERP integration story invisible in the code and unexplainable in an interview. Second, procedural logic in triggers is far harder to unit-test than a Go function, and test D8 — inject a failure mid-posting, assert nothing was written — becomes awkward to write.

**Do not post journal entries from a trigger**, for the same reasons.

**Do not compute `total_amount` or allocate document numbers in triggers.** Both are business rules with edge cases (Section 8.1's monthly reset, Section 8.3's copy semantics) that belong in testable application code.

The dividing line: *a trigger says "this state is illegal"; a service function says "this is what happens next."* If you are tempted to write `INSERT` inside a trigger body, you have crossed it.



#### 6.10.11 Cascade behaviour versus the deletion policy

Several tables declare `ON DELETE CASCADE` from `tenants`. Section 6.9 states tenants are never deleted, so these cascades should never fire in production. They are retained deliberately for two reasons: test fixtures tear down tenants between cases, and a cascade is the correct behaviour *if* a hard delete ever happens deliberately at the operations level.
