-- 003_constraints — the constraints and indexes that make 002 correct and fast.
--
-- The centrepiece is §6.10.1: FOREIGN KEY checks run with the privileges of the
-- table owner and therefore BYPASS row-level security. A plain
-- `REFERENCES products(id)` will happily accept another tenant's product ID.
-- Making tenant membership part of the reference is what closes that hole.

-- --------------------------------------------------------------------------
-- 6.10.1 Parents: a redundant unique key that includes tenant_id
-- --------------------------------------------------------------------------
ALTER TABLE products              ADD CONSTRAINT products_id_tenant_uq              UNIQUE (id, tenant_id);
ALTER TABLE warehouses            ADD CONSTRAINT warehouses_id_tenant_uq            UNIQUE (id, tenant_id);
ALTER TABLE suppliers             ADD CONSTRAINT suppliers_id_tenant_uq             UNIQUE (id, tenant_id);
ALTER TABLE accounts              ADD CONSTRAINT accounts_id_tenant_uq              UNIQUE (id, tenant_id);
ALTER TABLE purchase_requisitions ADD CONSTRAINT purchase_requisitions_id_tenant_uq UNIQUE (id, tenant_id);
ALTER TABLE purchase_orders       ADD CONSTRAINT purchase_orders_id_tenant_uq       UNIQUE (id, tenant_id);
ALTER TABLE goods_receipts        ADD CONSTRAINT goods_receipts_id_tenant_uq        UNIQUE (id, tenant_id);
ALTER TABLE journal_entries       ADD CONSTRAINT journal_entries_id_tenant_uq       UNIQUE (id, tenant_id);

-- purchase_order_lines needs two: one for the tenant-composite FK and one so a
-- receipt line cannot name a different product from the line it receives
-- against (§6.10.9). Declared together — §6.10.1's
-- purchase_order_lines_id_tenant_uq and §6.10.9's pol_id_tenant_uq are the same
-- constraint under two names; this is the one (AUDIT B4).
ALTER TABLE purchase_order_lines
  ADD CONSTRAINT pol_id_tenant_uq  UNIQUE (id, tenant_id),
  ADD CONSTRAINT pol_id_product_uq UNIQUE (id, product_id);

-- --------------------------------------------------------------------------
-- 6.10.1 Children: composite FKs replacing the single-column ones.
--
-- References to `users` stay single-column: users has no tenant_id for
-- superadmins, and superadmin actors legitimately appear on tenant rows.
-- ON DELETE CASCADE is carried across wherever the original FK had it.
-- --------------------------------------------------------------------------
ALTER TABLE stock_ledger
  DROP CONSTRAINT stock_ledger_product_id_fkey,
  DROP CONSTRAINT stock_ledger_warehouse_id_fkey,
  ADD CONSTRAINT stock_ledger_product_tenant_fk
      FOREIGN KEY (product_id, tenant_id)   REFERENCES products   (id, tenant_id),
  ADD CONSTRAINT stock_ledger_warehouse_tenant_fk
      FOREIGN KEY (warehouse_id, tenant_id) REFERENCES warehouses (id, tenant_id);

ALTER TABLE purchase_requisitions
  DROP CONSTRAINT purchase_requisitions_warehouse_id_fkey,
  DROP CONSTRAINT purchase_requisitions_supplier_id_fkey,
  ADD CONSTRAINT pr_warehouse_tenant_fk
      FOREIGN KEY (warehouse_id, tenant_id) REFERENCES warehouses (id, tenant_id),
  ADD CONSTRAINT pr_supplier_tenant_fk
      FOREIGN KEY (supplier_id, tenant_id)  REFERENCES suppliers  (id, tenant_id);

ALTER TABLE purchase_requisition_lines
  DROP CONSTRAINT purchase_requisition_lines_requisition_id_fkey,
  DROP CONSTRAINT purchase_requisition_lines_product_id_fkey,
  ADD CONSTRAINT prl_requisition_tenant_fk
      FOREIGN KEY (requisition_id, tenant_id)
      REFERENCES purchase_requisitions (id, tenant_id) ON DELETE CASCADE,
  ADD CONSTRAINT prl_product_tenant_fk
      FOREIGN KEY (product_id, tenant_id) REFERENCES products (id, tenant_id);

ALTER TABLE purchase_orders
  DROP CONSTRAINT purchase_orders_requisition_id_fkey,
  DROP CONSTRAINT purchase_orders_supplier_id_fkey,
  DROP CONSTRAINT purchase_orders_warehouse_id_fkey,
  ADD CONSTRAINT po_requisition_tenant_fk
      FOREIGN KEY (requisition_id, tenant_id) REFERENCES purchase_requisitions (id, tenant_id),
  ADD CONSTRAINT po_supplier_tenant_fk
      FOREIGN KEY (supplier_id, tenant_id)    REFERENCES suppliers  (id, tenant_id),
  ADD CONSTRAINT po_warehouse_tenant_fk
      FOREIGN KEY (warehouse_id, tenant_id)   REFERENCES warehouses (id, tenant_id);

ALTER TABLE purchase_order_lines
  DROP CONSTRAINT purchase_order_lines_po_id_fkey,
  DROP CONSTRAINT purchase_order_lines_product_id_fkey,
  ADD CONSTRAINT pol_po_tenant_fk
      FOREIGN KEY (po_id, tenant_id)
      REFERENCES purchase_orders (id, tenant_id) ON DELETE CASCADE,
  ADD CONSTRAINT pol_product_tenant_fk
      FOREIGN KEY (product_id, tenant_id) REFERENCES products (id, tenant_id);

ALTER TABLE goods_receipts
  DROP CONSTRAINT goods_receipts_po_id_fkey,
  ADD CONSTRAINT gr_po_tenant_fk
      FOREIGN KEY (po_id, tenant_id) REFERENCES purchase_orders (id, tenant_id);

-- One statement, three drops, four adds. §6.10.1 and §6.10.9 both want a
-- composite FK on po_line_id and they coexist; dropping the generated FK twice
-- across two migrations would fail (AUDIT B4).
ALTER TABLE goods_receipt_lines
  DROP CONSTRAINT goods_receipt_lines_gr_id_fkey,
  DROP CONSTRAINT goods_receipt_lines_po_line_id_fkey,
  DROP CONSTRAINT goods_receipt_lines_product_id_fkey,
  ADD CONSTRAINT grl_gr_tenant_fk
      FOREIGN KEY (gr_id, tenant_id)
      REFERENCES goods_receipts (id, tenant_id) ON DELETE CASCADE,
  ADD CONSTRAINT grl_po_line_tenant_fk
      FOREIGN KEY (po_line_id, tenant_id)  REFERENCES purchase_order_lines (id, tenant_id),
  ADD CONSTRAINT grl_po_line_product_fk
      FOREIGN KEY (po_line_id, product_id) REFERENCES purchase_order_lines (id, product_id),
  ADD CONSTRAINT grl_product_tenant_fk
      FOREIGN KEY (product_id, tenant_id)  REFERENCES products (id, tenant_id);

ALTER TABLE journal_entry_lines
  DROP CONSTRAINT journal_entry_lines_journal_entry_id_fkey,
  DROP CONSTRAINT journal_entry_lines_account_id_fkey,
  ADD CONSTRAINT jel_entry_tenant_fk
      FOREIGN KEY (journal_entry_id, tenant_id)
      REFERENCES journal_entries (id, tenant_id) ON DELETE CASCADE,
  ADD CONSTRAINT jel_account_tenant_fk
      FOREIGN KEY (account_id, tenant_id) REFERENCES accounts (id, tenant_id);

-- --------------------------------------------------------------------------
-- 6.10.2 Enum-like columns need CHECK constraints. A comment constrains
-- nothing: 'recieved' inserts cleanly and then fails every status filter.
-- --------------------------------------------------------------------------
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_status_valid
  CHECK (status IN ('draft','submitted','approved','rejected','cancelled'));
ALTER TABLE purchase_orders ADD CONSTRAINT po_status_valid
  CHECK (status IN ('open','partially_received','received','cancelled'));
ALTER TABLE stock_ledger ADD CONSTRAINT ledger_entry_type_valid
  CHECK (entry_type IN ('receipt','issue','adjustment'));
-- 'reversal' is permitted but unused: reversing entries are post-MVP (AUDIT C6).
ALTER TABLE stock_ledger ADD CONSTRAINT ledger_source_type_valid
  CHECK (source_type IN ('goods_receipt','manual_adjustment','reversal'));
ALTER TABLE stock_ledger ADD CONSTRAINT ledger_qty_nonzero
  CHECK (qty_delta <> 0);
ALTER TABLE accounts ADD CONSTRAINT account_type_valid
  CHECK (type IN ('asset','liability','equity','revenue','expense'));
ALTER TABLE journal_entries ADD CONSTRAINT je_source_type_valid
  CHECK (source_type IN ('goods_receipt','manual'));
ALTER TABLE document_sequences ADD CONSTRAINT doc_seq_type_valid
  CHECK (doc_type IN ('PR','PO','GR','JE'));

-- --------------------------------------------------------------------------
-- 6.10.3 Conditional field requirements
-- --------------------------------------------------------------------------
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_reject_needs_reason
  CHECK (status <> 'rejected' OR (reject_reason IS NOT NULL AND reject_reason <> ''));
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_decided_fields_together
  CHECK ((decided_by IS NULL) = (decided_at IS NULL));
ALTER TABLE purchase_requisitions ADD CONSTRAINT pr_submitted_has_timestamp
  CHECK (status = 'draft' OR submitted_at IS NOT NULL);

-- --------------------------------------------------------------------------
-- 6.10.4 Line numbering
-- --------------------------------------------------------------------------
ALTER TABLE purchase_requisition_lines
  ADD CONSTRAINT prl_line_no_uq UNIQUE (requisition_id, line_no),
  ADD CONSTRAINT prl_line_no_positive CHECK (line_no > 0);
ALTER TABLE purchase_order_lines
  ADD CONSTRAINT pol_line_no_uq UNIQUE (po_id, line_no),
  ADD CONSTRAINT pol_line_no_positive CHECK (line_no > 0);

-- The same product twice on one order makes receipt quantities ambiguous.
CREATE UNIQUE INDEX pol_one_line_per_product ON purchase_order_lines (po_id, product_id);

-- --------------------------------------------------------------------------
-- 6.9.1 Master-data uniqueness — partial, so a soft-deleted SKU can be reused.
--
-- These are the ONLY uniqueness declaration on these four tables: the CREATE
-- TABLE statements in 002 declare none (AUDIT B5). Skip them and duplicate
-- SKUs are legal.
-- --------------------------------------------------------------------------
CREATE UNIQUE INDEX products_sku_active
  ON products   (tenant_id, sku)  WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX suppliers_code_active
  ON suppliers  (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX warehouses_code_active
  ON warehouses (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX accounts_code_active
  ON accounts   (tenant_id, code) WHERE deleted_at IS NULL;

-- --------------------------------------------------------------------------
-- 6.10.5 Indexes. PostgreSQL does not index FK columns automatically, and the
-- RLS policy adds a tenant_id predicate to every query — so tenant_id leads
-- almost every index here.
-- --------------------------------------------------------------------------
CREATE INDEX idx_products_tenant   ON products   (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_suppliers_tenant  ON suppliers  (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_warehouses_tenant ON warehouses (tenant_id) WHERE deleted_at IS NULL;

CREATE INDEX idx_pr_tenant_status ON purchase_requisitions (tenant_id, status, created_at DESC);
CREATE INDEX idx_po_tenant_status ON purchase_orders (tenant_id, status, ordered_at DESC);

-- The approval queue is the hottest dashboard query.
CREATE INDEX idx_pr_pending ON purchase_requisitions (tenant_id, submitted_at DESC)
  WHERE status = 'submitted';

CREATE INDEX idx_po_open ON purchase_orders (tenant_id, ordered_at DESC)
  WHERE status IN ('open','partially_received');

CREATE INDEX idx_prl_requisition ON purchase_requisition_lines (requisition_id);
CREATE INDEX idx_pol_po          ON purchase_order_lines (po_id);
CREATE INDEX idx_grl_gr          ON goods_receipt_lines (gr_id);
CREATE INDEX idx_grl_po_line     ON goods_receipt_lines (po_line_id);
CREATE INDEX idx_gr_po           ON goods_receipts (po_id);
CREATE INDEX idx_jel_entry       ON journal_entry_lines (journal_entry_id);

-- "Show me the ledger rows this receipt created."
CREATE INDEX idx_ledger_source ON stock_ledger (tenant_id, source_type, source_id);
CREATE INDEX idx_je_source     ON journal_entries (tenant_id, source_type, source_id);
