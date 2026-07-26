# Post-MVP — Audit log

> Phase 11 ONLY. Do not build this during Phases 0-7. Leave TODO markers instead.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 6.7 Audit log — **POST-MVP, do not build in Phases 0–7**

> **Deferred.** Nothing in the acceptance test (Section 14) depends on this table. Ship the MVP first, then add it. It is included here because the schema is worth fixing now — later work (support impersonation, compliance reporting) builds on it, and retrofitting an audit trail is harder than designing one in.
>
> While deferred, leave `// TODO(post-mvp): audit` comments at each state transition listed below so the insertion points are obvious later.

```sql
CREATE TABLE audit_log (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  actor_id     UUID NOT NULL REFERENCES users(id),
  action       TEXT NOT NULL,   -- pr.submitted | pr.approved | gr.posted | ...
  entity_type  TEXT NOT NULL,
  entity_id    UUID NOT NULL,
  metadata     JSONB,
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_entity ON audit_log (tenant_id, entity_type, entity_id, occurred_at DESC);
```

Write an audit row for every state transition: requisition submitted/approved/rejected, PO created, goods receipt posted, module role changed, module entitlement toggled.

**Second table — platform plane.** Superadmin actions are not tenant-scoped, so they need their own table (no `tenant_id`, no RLS):

```sql
CREATE TABLE platform_audit_log (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id     UUID NOT NULL REFERENCES users(id),   -- always a superadmin
  action       TEXT NOT NULL,   -- tenant.created | tenant.suspended | entitlement.changed | impersonation.started
  tenant_id    UUID REFERENCES tenants(id),          -- which tenant was affected, if any
  metadata     JSONB,
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Entitlement changes are written to both tables.** The superadmin needs the record, but so does the tenant — "why did Finance disappear from our sidebar last Tuesday?" is a question the tenant admin should be able to answer without filing a support ticket. Write to `platform_audit_log` (actor = superadmin) and to that tenant's `audit_log` (actor = the same superadmin user; the FK works because `users.tenant_id` is nullable).

#### 6.7.1 Who can see the audit log

| Viewer | Sees |
|---|---|
| **Staff** | Per-document history only, on documents they can already read. No global audit page. |
| **Tenant admin** | Full tenant audit log — every user, every action, filterable by actor, action type, and date. |
| **Superadmin** | `platform_audit_log` only. **Not** tenant business actions. |

The rules follow directly from the permission model rather than being invented separately:

- **Staff visibility is derived, not granted.** If a staff user can open requisition `PR-202607-0004`, they can see its history — who submitted it, who approved it, when. If they cannot read the document, they cannot read its trail. There is no separate audit permission to configure, and no way for the trail to leak a document the user was never allowed to see.
- **Tenant admins get the global view**, because they are already the ones who assign roles and can reach every module the tenant is entitled to. Withholding the audit log from them would protect nothing.
- **Superadmins stay out**, consistent with Section 5.7. They can see that they disabled a module for a tenant; they cannot see who inside that tenant approved which purchase order. This is the same boundary as everywhere else in the system, and keeping it consistent is what makes it credible.

Because tenant `audit_log` is RLS-protected, a superadmin querying it through the `erp_app` pool gets zero rows automatically. The boundary is enforced by the database, not by remembering to filter — same guarantee as the rest of the tenant data.

#### 6.7.2 Immutability

An audit trail that can be edited is not an audit trail. Enforce append-only at the grant level, not in application code:

```sql
GRANT INSERT, SELECT ON audit_log TO erp_app;
REVOKE UPDATE, DELETE ON audit_log FROM erp_app;

GRANT INSERT, SELECT ON platform_audit_log TO erp_admin;
REVOKE UPDATE, DELETE ON platform_audit_log FROM erp_admin;
```

The application literally cannot rewrite history, even with a bug or a compromised handler. Add a test asserting that an `UPDATE` against `audit_log` as `erp_app` raises a permission error.

#### 6.7.3 Privileged access logging

Section 5.4 grants tenant admins implicit `admin` in every entitled module. That convenience has an audit cost: there is no moment where anyone decided a given admin should have Finance access, so the trail cannot explain *how* they got it.

The fix is not to remove the convenience — it is to log actions taken **by virtue of** it. When `LevelFor()` returns `RoleAdmin` through the tenant-admin shortcut rather than an explicit `user_module_roles` row, set `metadata.via = "tenant_admin_implicit"` on the audit entry.

Then "who approved this?" and "were they specifically granted that, or did they have it because they run the company?" are both answerable. That distinction is exactly what an auditor asks about, and it costs one field.
