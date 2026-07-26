-- 002_tenant_tables — every table that carries a tenant_id and is therefore
-- RLS-protected (RLS itself is applied in 005).
--
-- Table definitions only, deliberately readable. The composite foreign keys,
-- CHECK constraints, and indexes that make them correct arrive in 003 — but in
-- the SAME migration set, never as a later pass (§6.10).
--
-- audit_log is NOT here. It is Phase 11, post-MVP.

-- --------------------------------------------------------------------------
-- Inventory (§6.3)
-- --------------------------------------------------------------------------
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

-- Append-only. Never UPDATE or DELETE a row in this table — enforced by the
-- revokes in 005, not by convention (§6.9.3).
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

-- --------------------------------------------------------------------------
-- Procurement (§6.4)
-- --------------------------------------------------------------------------
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
  -- No qty_received column: received quantity is derived, via po_line_status
  -- (§6.4, I6). Over-receipt is prevented by grl_no_over_receipt in 004.
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

-- --------------------------------------------------------------------------
-- Finance (§6.5)
-- --------------------------------------------------------------------------
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

-- --------------------------------------------------------------------------
-- Document sequences (§6.6)
-- --------------------------------------------------------------------------
CREATE TABLE document_sequences (
  tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  doc_type    TEXT NOT NULL,   -- PR | PO | GR | JE
  period      TEXT NOT NULL,   -- YYYYMM, in the TENANT's timezone (§2.5.3)
  last_number INT NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, doc_type, period)
);

-- --------------------------------------------------------------------------
-- updated_at maintenance for the mutable tenant tables (§6.10.9)
-- --------------------------------------------------------------------------
CREATE TRIGGER warehouses_touch_updated_at
  BEFORE UPDATE ON warehouses FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER products_touch_updated_at
  BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER suppliers_touch_updated_at
  BEFORE UPDATE ON suppliers FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER accounts_touch_updated_at
  BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER purchase_requisitions_touch_updated_at
  BEFORE UPDATE ON purchase_requisitions FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER purchase_orders_touch_updated_at
  BEFORE UPDATE ON purchase_orders FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
