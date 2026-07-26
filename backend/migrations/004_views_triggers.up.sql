-- 004_views_triggers — the two derived views and the four invariant triggers.
--
-- A trigger says "this state is illegal". A service function says "this is what
-- happens next". Nothing below INSERTs (§6.10.10).

-- --------------------------------------------------------------------------
-- Derived views (§6.3, §6.4). Stock on hand and received quantity are computed
-- from the ledger and the receipt lines; there is no stored counter anywhere
-- in this schema (I6).
--
-- security_invoker = true is MANDATORY. Without it the view executes as its
-- owner, and an owner is not subject to its own tables' policies -- every
-- tenant would see every tenant's stock through the view while the base table
-- stayed correctly isolated. This is ownership, not the BYPASSRLS attribute;
-- no role here has that, and none should be given it to "fix" a failing A7.
-- --------------------------------------------------------------------------
CREATE VIEW stock_balances WITH (security_invoker = true) AS
SELECT tenant_id, product_id, warehouse_id, SUM(qty_delta) AS qty_on_hand
FROM stock_ledger
GROUP BY tenant_id, product_id, warehouse_id;

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

-- --------------------------------------------------------------------------
-- 6.10.6 Over-receipt. A CHECK constraint cannot reference another table, and
-- storing a running qty_received would contradict "derive, never store" and
-- could drift. A constraint trigger is the correct tool.
--
-- Race safety comes from the FOR UPDATE on the parent line: a second
-- transaction receiving against the same line blocks there until the first
-- commits, and its SUM -- a separate statement, so a fresh READ COMMITTED
-- snapshot -- then sees the committed row.
--
-- AFTER INSERT so NEW is already visible to the aggregate. The application
-- still validates first and returns a clean 422 over_receipt; this is the
-- backstop that catches every other write path (H6).
-- --------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION check_no_over_receipt() RETURNS trigger AS $$
DECLARE
  ordered  NUMERIC(18,4);
  received NUMERIC(18,4);
BEGIN
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

-- --------------------------------------------------------------------------
-- 6.10.7 Journal entries must balance. An unbalanced entry is a corrupt
-- ledger, and a Go assertion only covers the code paths that go through it.
--
-- DEFERRABLE INITIALLY DEFERRED is the whole point: lines are inserted one at
-- a time, so after the first insert the entry is legitimately unbalanced. An
-- immediate check would fail on every posting. This one runs at COMMIT.
-- --------------------------------------------------------------------------
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

-- --------------------------------------------------------------------------
-- 6.10.8 Terminal states are immutable. The handler returning 409 is a
-- promise; this is the guarantee.
--
-- The transition INTO a terminal state still works, because the trigger reads
-- OLD.status -- still 'submitted' at the moment of approval.
-- --------------------------------------------------------------------------
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
