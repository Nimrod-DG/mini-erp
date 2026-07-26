# Reference — Permission model

> Phase 3. Re-read §5.4 before any handler that checks a level.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 5. Permission model

This section answers a question real ERPs all have to answer: how granular should roles be?

### 5.1 Three independent layers

Access is decided by three checks, evaluated in order. All three must pass.

| Layer | Question | Controlled by | Stored in |
|---|---|---|---|
| 1. **Entitlement** | Is this module licensed to this tenant at all? | Superadmin | `tenant_modules` |
| 2. **Module role** | What level does this user hold in this module? | Tenant admin | `user_module_roles` |
| 3. **Record rule** | Does this specific record permit this action? | Business logic | Handler code |

An example of layer 3: a user with `approver` in Procurement still may not approve their *own* requisition (Section 8.2). Segregation of duties is a record-level rule, not a role level.

### 5.2 Why per-module roles rather than one global role

A single global role per user (`admin` / `manager` / `staff`) is the obvious first design, and it is what smaller apps do. Real ERPs do not, because real organisations do not work that way: a warehouse supervisor approves stock adjustments but must not see the general ledger; an accountant posts journals but must not approve purchase orders.

Every major ERP models this the same way — Odoo assigns per-module groups (`Purchase / Manager`, `Inventory / User`), NetSuite assigns permission levels per record type, SAP composes roles from per-transaction authorization objects. This project uses a simplified version of the same idea, and being able to explain that lineage is worth more than the code itself.

### 5.3 Role levels

Five levels, ordered. Each level includes everything below it.

| Level | Rank | Grants |
|---|---|---|
| `none` | 0 | No access. Module hidden in nav; API returns `403`. |
| `viewer` | 1 | Read-only. All list and detail endpoints. |
| `user` | 2 | Create and edit own draft documents. |
| `approver` | 3 | Approve/reject documents; post goods receipts. |
| `admin` | 4 | Manage that module's master data and configuration. |

Because levels are ranked, permission checks are comparisons rather than set membership:

```go
func RequireModule(module string, min RoleLevel) fiber.Handler
// usage
RequireModule("procurement", RoleApprover)
```

Absence of a `user_module_roles` row means `none`. Do not seed explicit `none` rows.

### 5.4 Tenant-level role

Each user has exactly one tenant-level role, and it determines whether the per-module matrix applies to them at all.

| Role | Grants |
|---|---|
| `admin` | Manages users and their module roles. **Implicitly holds `admin` level in every module the tenant is entitled to.** |
| `staff` | No implicit access. Every module level comes from `user_module_roles`, defaulting to `none`. |

This is deliberately asymmetric, and it reflects how small businesses actually operate: an owner-operator who does everything, plus staff who each work in one area. Forcing an admin to grant themselves `admin` in three separate modules is bureaucracy with no security benefit — they can grant it to themselves at any time anyway.

**Effective level resolution.** One function decides everything, and every permission check goes through it:

```go
func (u *Identity) LevelFor(module string) RoleLevel {
    if !u.TenantEntitled(module) {
        return RoleNone            // entitlement always wins
    }
    if u.TenantRole == TenantAdmin {
        return RoleAdmin           // admins are implicitly module admins
    }
    return u.ModuleRoles[module]   // staff: explicit, missing == none
}
```

Note the ordering: **entitlement is checked before the admin shortcut.** A tenant admin of a company without a Finance entitlement still has `none` in Finance. Admin is the ceiling *within* what the tenant has bought, never above it.

Because admins resolve to `admin` implicitly, do not create `user_module_roles` rows for them. If rows exist (e.g. a staff member was promoted), ignore them while the user is an admin, and leave them in place so a later demotion restores the previous levels.

**At least one admin per tenant — enforced.** A tenant must always have at least one active admin. Reject with `409 {"error":"last_admin"}` any attempt to:

- demote the last active admin to `staff`
- deactivate the last active admin
- delete the last active admin

There is no cap on the number of admins. "Exactly one admin" would be a single point of failure — if that person leaves or loses access, the tenant becomes unmanageable with no in-app recovery path. "At least one" gives the same simplicity without the trap.

**Admins are not exempt from record-level rules.** Specifically, a tenant admin still may not approve their own requisition (Section 8.2). Segregation of duties is a property of the record, not the role. A system that lets the same person raise and approve a purchase has no meaningful audit trail, regardless of that person's seniority.

### 5.5 Platform superadmin

Superadmins live outside all tenants (`users.tenant_id IS NULL`). They manage tenants and module entitlements only.

**Superadmins have no access to tenant business data.** There is no god-mode data browser. This mirrors how SaaS ERP vendors actually operate and avoids building a large privileged surface for no demo value.

### 5.6 Worked example

A tenant entitled to all three modules, with four users:

| User | Tenant role | Procurement | Inventory | Finance |
|---|---|---|---|---|
| Rina | `admin` | *implicit* `admin` | *implicit* `admin` | *implicit* `admin` |
| Budi | `staff` | `approver` | `viewer` | `none` |
| Sari | `staff` | `user` | `user` | `none` |
| Dewi | `staff` | `viewer` | `none` | `admin` |

Rina has no `user_module_roles` rows at all — her access is derived from her tenant role. The three staff users have one row per module they can reach.

Consequences worth checking in the acceptance test:

- Sari can raise a requisition but not approve one.
- Budi can approve Sari's requisition, but **not his own**.
- Rina can approve Sari's requisition too, but **not her own** — being an admin does not exempt her from segregation of duties.
- Dewi sees journal entries but cannot open the product list.
- Rina can manage users and reach every module, but only because this tenant is entitled to all three.

Contrast with Bahari Logistics, which has no Finance entitlement: its admin resolves to `none` in Finance and sees no Finance nav item, exactly like its staff.

---

### 5.7 How the two control planes relate

The permission system has two planes, controlled by different people, and keeping them straight is the clearest way to explain this project.

| | **Platform plane** | **Tenant plane** |
|---|---|---|
| Controlled by | Superadmin | Tenant admin |
| Manages | Tenants, module entitlements | Users, per-module role levels |
| Answers | *Which modules does this company have?* | *Who inside this company may do what?* |
| Tables | `tenants`, `tenant_modules` | `users`, `user_module_roles` |
| Endpoints | `/api/admin/*` | `/api/tenant/users/*` |
| Connection pool | `erp_admin` — platform tables only; RLS is irrelevant because those tables carry none, and every tenant business table is revoked from it (Section 4.2) | `erp_app` (RLS enforced) |

**The superadmin sets the ceiling; the tenant admin allocates within it.**

A superadmin decides that Bahari Logistics is entitled to Procurement and Inventory but not Finance. They have no say in which Bahari employee may approve a purchase order — that is the tenant admin's decision, made with the per-module role matrix. Conversely, a tenant admin can never grant anyone Finance access, because their tenant has no Finance entitlement to allocate. Neither plane can do the other's job.

This is why Section 5.1 evaluates entitlement *before* role level: a role level in an unentitled module is meaningless, and the two failures return different error codes so the client can tell them apart.

**Where the planes touch: tenant bootstrap.** A brand-new tenant has no users, so nobody inside it can create the first one. `POST /api/admin/tenants` therefore creates the tenant *and* its first admin user in one transaction. This is the **only** operation where a superadmin writes a row scoped to a tenant. After the handoff, user management belongs entirely to the tenant.

**What superadmins deliberately cannot do:** read or modify tenant business data. There is no god-mode browser over requisitions, stock, or journals. Beyond avoiding a large privileged surface for no demo value, it means a compromised superadmin account exposes the customer list, not the customers' operational data. This mirrors how SaaS ERP vendors actually operate.

The realistic omission here is **audited support impersonation** — letting a support engineer temporarily assume a tenant user's identity with every action logged and the tenant notified. That is deferred (Section 1.2) and depends on the audit log; it is the honest answer to "what would you build next?"
