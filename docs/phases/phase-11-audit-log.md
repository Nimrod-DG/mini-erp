# Phase 11 — Audit log · *post-MVP*

**MVP:** no · **Estimate:** 3h · **Depends on:** Phase 10

## Load only these

1. [`../post-mvp/audit-log.md`](../post-mvp/audit-log.md) — in full.
2. [`../reference/permissions.md`](../reference/permissions.md) — **§5.7** (why superadmins stay out).
3. [`../reference/tests.md`](../reference/tests.md) — **FE16–FE21**.
4. [`../reference/tenancy-and-rls.md`](../reference/tenancy-and-rls.md) — **§4.2.1**. The dual-write depends on the same `SECURITY DEFINER` pattern used for `seed_tenant_accounts()`.

## Build

1. `audit_log` (tenant-scoped, **RLS with the same policy template as every other
   tenant table**) and `platform_audit_log` (no `tenant_id`, no RLS).
2. Append-only at the **grant** level, not in application code:
   `GRANT INSERT, SELECT` / `REVOKE UPDATE, DELETE`. The application then
   literally cannot rewrite history, even with a compromised handler.
3. Replace every `// TODO(post-mvp): audit` marker with a real insert. Grep for
   them — Phases 3, 5, and 6 should have left a complete set.
4. Entitlement changes write to **both** tables. The superadmin is on the
   `erp_admin` pool, which has no grant on `audit_log` and never sets tenant
   context, so add `log_tenant_event(p_tenant, p_actor, p_action, p_metadata)` as
   a `SECURITY DEFINER` function in the shape of §4.2.1 and grant `EXECUTE` to
   `erp_admin`. Do not grant the table.
5. **Privileged-access tagging**: when `LevelFor()` returns `admin` through the
   tenant-admin shortcut rather than an explicit `user_module_roles` row, set
   `metadata.via = "tenant_admin_implicit"`. That one field is what lets an
   auditor distinguish "was specifically granted this" from "has it because they
   run the company".
6. UI: a per-document trail panel on requisition and PO detail (visible to anyone
   who can read the document — visibility is *derived*, not a separate
   permission), a global filterable audit page for tenant admins, and a platform
   audit page for superadmins.

## Done when

- [ ] Test A5 passes **with `audit_log` included** in the RLS assertion
- [ ] FE16–FE21 green
- [ ] Approving a requisition produces a visible, attributable trail entry
- [ ] `UPDATE audit_log` as `erp_app` raises a permission error (FE20)
