-- 005_rls_grants — row-level security, then the grants that make the deletion
-- policy and the superadmin boundary structural rather than aspirational.

-- --------------------------------------------------------------------------
-- §4.4 / §6.8 — the standard policy on every tenant-scoped table.
--
-- ENABLE alone is not enough: without FORCE, the table owner bypasses the
-- policy, which silently defeats the whole mechanism in local development
-- where you are often connected as the owner.
--
-- USING governs what SELECT/UPDATE/DELETE can see; WITH CHECK governs what
-- INSERT/UPDATE may write. Both are required -- omitting WITH CHECK lets a
-- tenant write rows tagged with another tenant's ID.
--
-- NULLIF(..., '') is a deliberate hardening of the §4.4 template. After a
-- SET LOCAL transaction commits, the GUC resets to the empty string rather
-- than to NULL, and ''::uuid raises "invalid input syntax". §4.3 wants a
-- request without tenant context to see zero rows, not an error page, so the
-- empty string is mapped to NULL: NULL = tenant_id is false, and the query
-- returns nothing. Behaviour when the context IS set is unchanged.
--
-- One canonical list, iterated: a table added to it gets the full treatment,
-- and a tenant table left off it fails test A5.
-- --------------------------------------------------------------------------
DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'warehouses', 'products', 'stock_ledger',
    'suppliers', 'purchase_requisitions', 'purchase_requisition_lines',
    'purchase_orders', 'purchase_order_lines',
    'goods_receipts', 'goods_receipt_lines',
    'accounts', 'journal_entries', 'journal_entry_lines',
    'document_sequences'
    -- audit_log joins this list in Phase 11, post-MVP.
  ] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE  ROW LEVEL SECURITY', t);
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
    EXECUTE format($f$
      CREATE POLICY tenant_isolation ON %I
        USING      (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid)
        WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid)
    $f$, t);
  END LOOP;
END
$$;

-- --------------------------------------------------------------------------
-- erp_app — the request role. RLS is what scopes it; the grants are what stop
-- it rewriting history.
-- --------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE, DELETE ON
  warehouses, products, stock_ledger,
  suppliers, purchase_requisitions, purchase_requisition_lines,
  purchase_orders, purchase_order_lines,
  goods_receipts, goods_receipt_lines,
  accounts, journal_entries, journal_entry_lines,
  document_sequences
  TO erp_app;

GRANT SELECT ON stock_balances, po_line_status TO erp_app;

-- §6.9.3 Tier 3 — immutable ledgers. Append-only is enforced here rather than
-- by a trigger: a grant cannot be turned off by ALTER TABLE ... DISABLE
-- TRIGGER, and there is nothing to forget on a new code path. Corrections are
-- new reversing entries, never edits (G9).
REVOKE UPDATE, DELETE ON
  stock_ledger, journal_entries, journal_entry_lines,
  goods_receipts, goods_receipt_lines
  FROM erp_app;

-- --------------------------------------------------------------------------
-- erp_admin — the superadmin console role, structurally incapable of reading
-- tenant business data. "Superadmins have no access to tenant business data"
-- stops being a promise and becomes a property; test A11 asserts the resulting
-- permission error, not an empty result.
--
-- Nothing was granted above, so these revokes are belt and braces -- and they
-- are the statement of intent that a later GRANT would have to contradict.
-- --------------------------------------------------------------------------
REVOKE ALL ON
  warehouses, products, stock_ledger,
  suppliers, purchase_requisitions, purchase_requisition_lines,
  purchase_orders, purchase_order_lines,
  goods_receipts, goods_receipt_lines,
  accounts, journal_entries, journal_entry_lines,
  document_sequences,
  stock_balances, po_line_status
  FROM erp_admin;

-- --------------------------------------------------------------------------
-- §4.2.1 — the one write that legitimately crosses that revoke.
--
-- POST /api/admin/tenants (Phase 3) seeds a new tenant's chart of accounts in
-- the same transaction that creates the tenant. Granting erp_admin INSERT on
-- accounts would re-open exactly the surface the revoke closes; a narrow
-- SECURITY DEFINER function keeps the privileged surface two rows wide.
--
-- SET search_path is not optional on a SECURITY DEFINER function: without it a
-- caller who can create objects could shadow `accounts` and have the definer's
-- privileges applied to their table.
--
-- The set_config call is required because accounts is FORCE RLS, and FORCE
-- applies to the owner too -- the definer needs tenant context like everyone
-- else. The previous value is restored so the caller's transaction is left as
-- it was found.
-- --------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION seed_tenant_accounts(p_tenant UUID)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  prev TEXT := COALESCE(current_setting('app.current_tenant', true), '');
BEGIN
  PERFORM set_config('app.current_tenant', p_tenant::text, true);

  INSERT INTO accounts (tenant_id, code, name, type) VALUES
    (p_tenant, '1300', 'Inventory',                   'asset'),
    (p_tenant, '2150', 'Goods received not invoiced', 'liability')
  ON CONFLICT DO NOTHING;

  PERFORM set_config('app.current_tenant', prev, true);
END;
$$;

REVOKE ALL     ON FUNCTION seed_tenant_accounts(UUID) FROM PUBLIC;
GRANT  EXECUTE ON FUNCTION seed_tenant_accounts(UUID) TO   erp_admin;
