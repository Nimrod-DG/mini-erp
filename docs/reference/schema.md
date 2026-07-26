# Reference — Database schema

> Phase 1. Table definitions. Constraints and the deletion policy are separate files.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 6. Database schema

### 6.1 Platform tables (no `tenant_id`, **no RLS**)

```sql
CREATE TABLE tenants (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL UNIQUE,
  status      TEXT NOT NULL DEFAULT 'active',        -- active | suspended
  timezone    TEXT NOT NULL DEFAULT 'Asia/Jakarta',  -- business dates (Section 2.5.3)
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE modules (
  code         TEXT PRIMARY KEY,                -- procurement | inventory | finance
  name         TEXT NOT NULL,
  description  TEXT NOT NULL,
  is_available BOOLEAN NOT NULL DEFAULT true,
  sort_order   INT NOT NULL DEFAULT 0
);

CREATE TABLE tenant_modules (
  tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  module_code  TEXT NOT NULL REFERENCES modules(code),
  enabled      BOOLEAN NOT NULL DEFAULT false,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, module_code)
);
```

### 6.2 Users and module roles

```sql
CREATE TABLE users (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID REFERENCES tenants(id) ON DELETE CASCADE,  -- NULL = superadmin
  firebase_uid TEXT NOT NULL UNIQUE,
  email        TEXT NOT NULL UNIQUE,
  full_name    TEXT NOT NULL,
  tenant_role  TEXT NOT NULL DEFAULT 'staff',  -- staff | admin | superadmin
  is_active    BOOLEAN NOT NULL DEFAULT true,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users ADD CONSTRAINT users_superadmin_has_no_tenant
  CHECK ((tenant_role =  'superadmin' AND tenant_id IS NULL)
      OR (tenant_role <> 'superadmin' AND tenant_id IS NOT NULL));

CREATE TABLE user_module_roles (
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  module_code  TEXT NOT NULL REFERENCES modules(code),
  role_level   TEXT NOT NULL,   -- viewer | user | approver | admin
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, module_code),
  CHECK (role_level IN ('viewer','user','approver','admin'))
);
```

Rows here apply to `staff` users only. A `tenant_role = 'admin'` user resolves to `admin` in every entitled module without any rows (Section 5.4), so the seed script should not create them.

Add a check constraint for the tenant role values:

```sql
ALTER TABLE users ADD CONSTRAINT users_tenant_role_valid
  CHECK (tenant_role IN ('staff','admin','superadmin'));
```

The "at least one active admin per tenant" rule (Section 5.4) is **not** expressible as a table constraint — it is a cross-row invariant. Enforce it in the service layer, inside the same transaction as the demote/deactivate/delete, with a `SELECT … FOR UPDATE` count of active admins so two concurrent demotions cannot both pass the check:

```sql
SELECT count(*) FROM users
WHERE tenant_id = $1 AND tenant_role = 'admin' AND is_active = true
FOR UPDATE;
```

No password column — Firebase Auth holds credentials.

**`firebase_uid` is the join key between the two systems, and the choice of column matters.**

| Column | Role | Mutable? |
|---|---|---|
| `firebase_uid` | The identity link. Every request resolves through it. | **Never.** Firebase UIDs are permanent. |
| `email` | Display, search, and the lookup key for inviting users. | Yes — a user can change it in Firebase. |

Join on `firebase_uid`, never on `email`. Firebase permits email changes, so an email-based join would orphan a user's entire history — their requisitions, approvals, and audit entries — the moment they updated their address. The UID is stable for the life of the account.

That makes `users.email` a **denormalised copy** that can drift from Firebase. Two acceptable ways to handle it, and this project takes the second:

1. Sync on every login — refresh the local row from the verified token's claims.
2. **Treat the local value as display-only** and accept drift, refreshing it on the next explicit profile update.

Option 2 is chosen because email changes are rare here (accounts are provisioned by an admin, not self-registered) and because a write on every request to keep a display field current is poor value. Worth knowing the trade-off exists rather than discovering the drift later.

Neither table is RLS-protected: both are read during identity resolution, before tenant context exists. Scope them in application code — every query filters by the `tenant_id` derived from the verified Firebase UID, never from a client-supplied parameter.

### 6.3 Inventory module

```sql
CREATE TABLE warehouses (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  code        TEXT NOT NULL,
  name        TEXT NOT NULL,
  is_active   BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at  TIMESTAMPTZ,
  deleted_by  UUID REFERENCES users(id)
);

CREATE TABLE products (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  sku            TEXT NOT NULL,
  name           TEXT NOT NULL,
  uom            TEXT NOT NULL DEFAULT 'pcs',
  reorder_point  NUMERIC(18,4) NOT NULL DEFAULT 0,
  standard_cost  NUMERIC(18,2) NOT NULL DEFAULT 0,
  is_active      BOOLEAN NOT NULL DEFAULT true,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at     TIMESTAMPTZ,
  deleted_by     UUID REFERENCES users(id)
);

-- Append-only. Never UPDATE or DELETE a row in this table.
CREATE TABLE stock_ledger (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  product_id    UUID NOT NULL REFERENCES products(id),
  warehouse_id  UUID NOT NULL REFERENCES warehouses(id),
  entry_type    TEXT NOT NULL,            -- receipt | issue | adjustment
  qty_delta     NUMERIC(18,4) NOT NULL,   -- signed: +in, -out
  unit_cost     NUMERIC(18,2) NOT NULL DEFAULT 0,
  source_type   TEXT NOT NULL,            -- goods_receipt | manual_adjustment
  source_id     UUID,
  note          TEXT,
  occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by    UUID NOT NULL REFERENCES users(id)
);

CREATE INDEX idx_stock_ledger_lookup
  ON stock_ledger (tenant_id, product_id, warehouse_id, occurred_at DESC);
```

**Stock on hand is derived, never stored as a mutable counter:**

```sql
CREATE VIEW stock_balances WITH (security_invoker = true) AS
SELECT tenant_id, product_id, warehouse_id, SUM(qty_delta) AS qty_on_hand
FROM stock_ledger
GROUP BY tenant_id, product_id, warehouse_id;
```

This is the most important design decision in the inventory module and the thing that makes it read as ERP rather than CRUD. Every stock movement is an immutable, attributable, timestamped fact. You can always answer "why is stock 47?" by reading the ledger. A mutable `products.current_stock` column can never answer that.

> **`security_invoker = true` is mandatory here — do not omit it.**
>
> PostgreSQL views default to `security_invoker = false`, meaning the view executes with the **view owner's** privileges. RLS policies on `stock_ledger` would then be evaluated as the owner, not as `erp_app` — and a table's **owner is not subject to that table's policies** (which is precisely why Section 4.4 mandates `FORCE ROW LEVEL SECURITY`), so **every tenant would see every tenant's stock through this view.** Note this is ownership, not the `BYPASSRLS` role attribute; no role in this system has that, and none should be given it to "fix" anything. The tables would be correctly isolated while the view leaked everything.
>
> `security_invoker` requires PostgreSQL 15+, which is why Section 2 specifies it. Test A7 (Section 12.3) asserts isolation *through the view*, not just against the base table — a test against `stock_ledger` alone would pass while the view leaked.

### 6.4 Procurement module

```sql
CREATE TABLE suppliers (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  code            TEXT NOT NULL,
  name            TEXT NOT NULL,
  contact_email   TEXT,
  contact_phone   TEXT,
  lead_time_days  INT NOT NULL DEFAULT 7,
  payment_terms   TEXT NOT NULL DEFAULT 'NET30',
  is_active       BOOLEAN NOT NULL DEFAULT true,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at      TIMESTAMPTZ,
  deleted_by      UUID REFERENCES users(id)
);

CREATE TABLE purchase_requisitions (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  pr_number      TEXT NOT NULL,
  warehouse_id   UUID NOT NULL REFERENCES warehouses(id),
  supplier_id    UUID REFERENCES suppliers(id),
  status         TEXT NOT NULL DEFAULT 'draft',  -- draft|submitted|approved|rejected|cancelled
  notes          TEXT,
  requested_by   UUID NOT NULL REFERENCES users(id),
  submitted_at   TIMESTAMPTZ,
  decided_by     UUID REFERENCES users(id),
  decided_at     TIMESTAMPTZ,
  reject_reason  TEXT,
  cancelled_by   UUID REFERENCES users(id),
  cancelled_at   TIMESTAMPTZ,
  cancel_reason  TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, pr_number)
);

CREATE TABLE purchase_requisition_lines (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  requisition_id  UUID NOT NULL REFERENCES purchase_requisitions(id) ON DELETE CASCADE,
  product_id      UUID NOT NULL REFERENCES products(id),
  qty             NUMERIC(18,4) NOT NULL CHECK (qty > 0),
  est_unit_cost   NUMERIC(18,2) NOT NULL DEFAULT 0,
  line_no         INT NOT NULL
);

CREATE TABLE purchase_orders (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  po_number       TEXT NOT NULL,
  requisition_id  UUID REFERENCES purchase_requisitions(id),
  supplier_id     UUID NOT NULL REFERENCES suppliers(id),
  warehouse_id    UUID NOT NULL REFERENCES warehouses(id),
  status          TEXT NOT NULL DEFAULT 'open',  -- open|partially_received|received|cancelled
  total_amount    NUMERIC(18,2) NOT NULL DEFAULT 0,
  ordered_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  expected_at     DATE,
  created_by      UUID NOT NULL REFERENCES users(id),
  cancelled_by    UUID REFERENCES users(id),
  cancelled_at    TIMESTAMPTZ,
  cancel_reason   TEXT,
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, po_number)
);

CREATE TABLE purchase_order_lines (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  po_id         UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
  product_id    UUID NOT NULL REFERENCES products(id),
  qty_ordered   NUMERIC(18,4) NOT NULL CHECK (qty_ordered > 0),
  unit_cost     NUMERIC(18,2) NOT NULL,
  line_no       INT NOT NULL
);

CREATE TABLE goods_receipts (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  gr_number    TEXT NOT NULL,
  po_id        UUID NOT NULL REFERENCES purchase_orders(id),
  received_by  UUID NOT NULL REFERENCES users(id),
  received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  note         TEXT,
  idempotency_key TEXT NOT NULL,
  UNIQUE (tenant_id, gr_number),
  UNIQUE (tenant_id, idempotency_key)
);

CREATE TABLE goods_receipt_lines (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  gr_id         UUID NOT NULL REFERENCES goods_receipts(id) ON DELETE CASCADE,
  po_line_id    UUID NOT NULL REFERENCES purchase_order_lines(id),
  product_id    UUID NOT NULL REFERENCES products(id),
  qty_received  NUMERIC(18,4) NOT NULL CHECK (qty_received > 0)
);
```

**Received quantity is derived, exactly like stock on hand** (Section 6.3). There is no `qty_received` column on `purchase_order_lines`; it is computed from the receipt lines:

```sql
CREATE VIEW po_line_status WITH (security_invoker = true) AS
SELECT
  pol.id            AS po_line_id,
  pol.tenant_id,
  pol.po_id,
  pol.product_id,
  pol.qty_ordered,
  COALESCE(SUM(grl.qty_received), 0)                  AS qty_received,
  pol.qty_ordered - COALESCE(SUM(grl.qty_received),0) AS qty_outstanding
FROM purchase_order_lines pol
LEFT JOIN goods_receipt_lines grl ON grl.po_line_id = pol.id
GROUP BY pol.id, pol.tenant_id, pol.po_id, pol.product_id, pol.qty_ordered;
```

Over-receipt is prevented by a constraint trigger rather than a CHECK constraint — see Section 6.10.6 for why, and for the trigger definition.

### 6.5 Finance module (stub, but functional enough to receive postings)

```sql
CREATE TABLE accounts (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  code       TEXT NOT NULL,
  name       TEXT NOT NULL,
  type       TEXT NOT NULL,   -- asset|liability|equity|revenue|expense
  is_active  BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  deleted_by UUID REFERENCES users(id)
);

CREATE TABLE journal_entries (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  entry_number  TEXT NOT NULL,
  source_type   TEXT NOT NULL,   -- goods_receipt | manual
  source_id     UUID,
  description   TEXT NOT NULL,
  posted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by    UUID NOT NULL REFERENCES users(id),
  UNIQUE (tenant_id, entry_number)
);

CREATE TABLE journal_entry_lines (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  journal_entry_id  UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
  account_id        UUID NOT NULL REFERENCES accounts(id),
  debit             NUMERIC(18,2) NOT NULL DEFAULT 0,
  credit            NUMERIC(18,2) NOT NULL DEFAULT 0,
  memo              TEXT,
  CHECK (debit >= 0 AND credit >= 0),
  CHECK (NOT (debit > 0 AND credit > 0))
);
```

Minimum chart of accounts to seed per tenant:

| Code | Name | Type |
|---|---|---|
| `1300` | Inventory | asset |
| `2150` | Goods received not invoiced | liability |

### 6.6 Document sequences

```sql
CREATE TABLE document_sequences (
  tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  doc_type    TEXT NOT NULL,   -- PR | PO | GR | JE
  period      TEXT NOT NULL,   -- YYYYMM
  last_number INT NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, doc_type, period)
);
```


### 6.8 Tables requiring RLS

Apply the Section 4.4 policy template to **all** of these:

```
warehouses, products, stock_ledger,
suppliers, purchase_requisitions, purchase_requisition_lines,
purchase_orders, purchase_order_lines,
goods_receipts, goods_receipt_lines,
accounts, journal_entries, journal_entry_lines,
document_sequences, audit_log
```

`audit_log` is post-MVP (Section 6.7); when it is added, it must get the same policy, and RLS test A5 will fail until it does — which is the intended behaviour.

Do **not** apply RLS to: `tenants`, `modules`, `tenant_modules`, `users`, `user_module_roles`.
