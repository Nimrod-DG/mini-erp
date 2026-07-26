# Reference — Tenancy, roles, and RLS

> Phase 1. The single most important file in this set.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 4. Tenancy model

### 4.1 The approach

**Shared database, shared schema, row-level security.**

Every business table carries a `tenant_id` column. PostgreSQL RLS policies filter every query by the tenant ID held in a per-transaction session variable. The application sets that variable at the start of each request; from then on the database itself refuses to return another tenant's rows, even if application code contains a bug and forgets a `WHERE` clause.

This is deliberately stricter than filtering in application code. It is the part of this project hardest to fake and most worth demonstrating.

### 4.2 The three database roles

**No role in this system requires `BYPASSRLS`.** That is deliberate: it keeps the design portable to any managed Postgres (Neon, Supabase, RDS, Cloud SQL), where you cannot connect as a real superuser and `BYPASSRLS` is awkward or impossible to grant. It also produces a stronger guarantee than bypassing would.

| Role | Privileges | Used for |
|---|---|---|
| `erp_app` | RLS **enforced**. `SELECT/INSERT/UPDATE/DELETE` on tenant tables (minus the ledger revokes in 6.9.3). Read on `tenants`, `modules`, `tenant_modules`; read/write on `users`, `user_module_roles` — see below. | Every tenant-scoped request |
| `erp_admin` | Full access to platform tables only. **Explicitly revoked from every tenant business table.** | Superadmin console only |
| `erp_migrate` | Schema owner. DDL only. | Running migrations |

```sql
CREATE ROLE erp_app    LOGIN PASSWORD :'app_pw'    NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE erp_admin  LOGIN PASSWORD :'admin_pw'  NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;

-- erp_app needs the five unRLS'd platform tables: identity resolution reads
-- users and user_module_roles on every request, RequireModule reads
-- tenant_modules, document numbering reads tenants.timezone (Section 8.1.1),
-- and /api/tenant/users writes the first two. These tables carry no RLS
-- (Section 6.8), so they are scoped in application code by the tenant_id
-- derived from the verified Firebase UID -- never from a client parameter.
GRANT SELECT                 ON tenants, modules, tenant_modules TO erp_app;
GRANT SELECT, INSERT, UPDATE ON users, user_module_roles         TO erp_app;

-- Superadmins touch platform tables only
GRANT SELECT, INSERT, UPDATE ON tenants, modules, tenant_modules, users, user_module_roles TO erp_admin;

-- ...and are structurally incapable of reading tenant business data
REVOKE ALL ON warehouses, products, stock_ledger, suppliers,
              purchase_requisitions, purchase_requisition_lines,
              purchase_orders, purchase_order_lines,
              goods_receipts, goods_receipt_lines,
              accounts, journal_entries, journal_entry_lines,
              document_sequences
  FROM erp_admin;
```

**Why revocation beats bypassing.** The original design gave `erp_admin` the power to see everything and relied on the handler never asking. Revoking instead means a superadmin endpoint *cannot* read a tenant's purchase orders even with a bug, a bad join, or a compromised handler — the database refuses. Section 5.5's "superadmins have no access to tenant business data" stops being a promise and becomes a property.

Note that `erp_admin` needs no RLS exemption because platform tables carry no RLS (Section 6.8), and `erp_app` needs none because it always operates within one tenant context.

The application holds **two connection pools**, one per role. Handlers under `/api/admin/*` use the admin pool; everything else uses the app pool.

#### 4.2.1 The two writes that cross the revoke — `SECURITY DEFINER`

Two operations legitimately need a superadmin to write one row into tenant-scoped
territory, and the revoke above correctly forbids both:

| Operation | Needs | Spec |
|---|---|---|
| `POST /api/admin/tenants` seeds the chart of accounts for a brand-new tenant | `INSERT` on `accounts` | Section 9.2 |
| An entitlement change is mirrored into that tenant's audit trail | `INSERT` on `audit_log` | Section 6.7 (post-MVP) |

Do **not** solve these by granting `erp_admin` access to the tables — that
re-opens exactly the surface this section closed, and weakens what test A11
proves. Expose a narrow `SECURITY DEFINER` function instead, owned by
`erp_migrate`, which runs with the owner's privileges regardless of the caller's:

```sql
CREATE FUNCTION seed_tenant_accounts(p_tenant UUID)
RETURNS void
LANGUAGE sql
SECURITY DEFINER
SET search_path = public          -- mandatory on any SECURITY DEFINER function
AS $$
  INSERT INTO accounts (tenant_id, code, name, type) VALUES
    (p_tenant, '1300', 'Inventory',                     'asset'),
    (p_tenant, '2150', 'Goods received not invoiced',   'liability')
  ON CONFLICT DO NOTHING;
$$;

REVOKE ALL     ON FUNCTION seed_tenant_accounts(UUID) FROM PUBLIC;
GRANT  EXECUTE ON FUNCTION seed_tenant_accounts(UUID) TO   erp_admin;
```

The privileged surface is now two rows wide and named, rather than a table-level
grant, and `POST /api/admin/tenants` keeps its single transaction. Phase 11 adds
`log_tenant_event(...)` in the same shape for the audit mirror.

`SET search_path` is not optional: without it, a caller who can create objects
could shadow `accounts` and have the definer's privileges applied to their table.

**Nothing needs to write across tenants**, including the seed script and test fixtures — both loop tenant by tenant, setting `app.current_tenant` for each. If you find yourself wanting `BYPASSRLS` to make a fixture simpler, set the tenant context in a loop instead.

### 4.3 How tenant context is set

Every tenant-scoped request runs inside a transaction that begins with:

```sql
SET LOCAL app.current_tenant = '<tenant-uuid>';
```

`SET LOCAL` scopes the value to the transaction, so it is discarded on commit or rollback. This matters enormously: with a connection pool, a plain `SET` would leak one tenant's context onto the next request that reuses the connection. **Always `SET LOCAL`, always inside a transaction.**

Policies use the two-argument form of `current_setting`:

```sql
current_setting('app.current_tenant', true)
```

The `true` means "return NULL instead of raising if unset". If a request reaches the database without tenant context, the comparison against NULL yields false and the query returns zero rows — a safe failure rather than an error page or, worse, an unfiltered scan.

### 4.4 Standard policy template

Applied to every tenant-scoped table (full list in Section 6.8):

```sql
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON <table>
  USING       (tenant_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK  (tenant_id = current_setting('app.current_tenant', true)::uuid);
```

`FORCE ROW LEVEL SECURITY` is required — without it the table owner bypasses the policy, which silently defeats the mechanism in local development where you are often connected as the owner.

`USING` governs which rows are visible to `SELECT`/`UPDATE`/`DELETE`. `WITH CHECK` governs what `INSERT`/`UPDATE` may write. Both are needed; omitting `WITH CHECK` would let a tenant insert rows tagged with another tenant's ID.

---
