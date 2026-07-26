# Reference — API surface

> Look up the routes for the phase you are in. Do not read end to end.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 9. API surface

All routes prefixed `/api`. All except `/api/health` require `Authorization: Bearer <firebase-id-token>`.

### 9.0 List conventions

Every list endpoint follows the same contract, so the frontend can share one hook and one pagination component.

**Request:** `?page=1&pageSize=25&sort=-createdAt&q=<search>` plus endpoint-specific filters. `pageSize` defaults to 25, maximum 100. A `-` prefix on `sort` means descending.

**Response:**

```json
{
  "data": [ ... ],
  "page": 1,
  "pageSize": 25,
  "totalItems": 137,
  "totalPages": 6
}
```

`totalItems` is mandatory, not optional. Section 10.7.4 requires a visible total count — "Page 3 of ?" strands people — and that number has to come from here.

**Sorting is server-side whenever results are paginated.** Sorting only the current page silently misrepresents the data, which is worse than not offering sort at all.

Master-data lists additionally accept `?includeDeleted=true` (module `admin` only) for restore workflows.

### 9.1 Session

| Method | Path | Notes |
|---|---|---|
| GET | `/health` | Unauthenticated liveness probe for Cloud Run |
| GET | `/me` | User, tenant, tenant role, and `{module: level}` map |

There is no `/auth/login` — Firebase handles credentials client-side.

### 9.2 Superadmin (`erp_admin` pool, `tenant_role = superadmin`)

| Method | Path | Notes |
|---|---|---|
| GET | `/admin/tenants` | List with user counts and enabled-module counts |
| GET | `/admin/tenants/:id` | Tenant detail — name, slug, status, timezone, entitlements, admin count |
| POST | `/admin/tenants` | Create tenant, seed chart of accounts via `seed_tenant_accounts()` (Section 4.2.1 — `erp_admin` has no direct grant on `accounts`), create first admin user. The only superadmin write scoped to a tenant — Section 5.7 |
| PATCH | `/admin/tenants/:id` | Rename, change timezone, suspend, reactivate |
| GET | `/admin/modules` | Module catalogue |
| GET | `/admin/tenants/:id/modules` | Entitlement matrix for one tenant |
| PUT | `/admin/tenants/:id/modules/:code` | `{enabled: bool}` — single toggle |

There is deliberately **no** `DELETE /admin/tenants/:id`. Tenants are suspended, never deleted (Section 6.9.4).

Suspending a tenant blocks its users at identity resolution — they receive `403 {"error":"tenant_suspended"}` rather than a confusing empty application.

### 9.3 Tenant user management (tenant role `admin`)

| Method | Path | Notes |
|---|---|---|
| GET | `/tenant/users` | Users with their module role map |
| GET | `/tenant/users/:id` | User detail — profile plus full module role matrix |
| POST | `/tenant/users` | Create Firebase user + local row + initial roles (Section 3.3) |
| PATCH | `/tenant/users/:id` | Rename, activate/deactivate, **and promote/demote between `admin` and `staff`** |
| PUT | `/tenant/users/:id/modules/:code` | `{roleLevel: "viewer"\|"user"\|"approver"\|"admin"\|"none"}` |
| PUT | `/tenant/users/:id/modules` | Bulk set the whole matrix in one request |

Notes:

- Setting `none` deletes the `user_module_roles` row rather than storing it (Section 5.3).
- The bulk endpoint exists because the UI is a matrix (Section 10.6); saving six dropdowns should be one request and one transaction, not six that can half-fail.
- `PATCH` carries the **last-admin rule** (Section 5.4): demoting or deactivating the final active admin returns `409 last_admin`, checked under `SELECT … FOR UPDATE`.
- Module roles on a `tenant_role = 'admin'` user are accepted and stored but have no effect while they remain an admin — they take effect again on demotion.
- There is no `DELETE /tenant/users/:id`. Users are deactivated (Section 6.9.4).

### 9.4 Procurement

| Method | Path | Min level |
|---|---|---|
| GET | `/procurement/suppliers` | `viewer` |
| GET | `/procurement/suppliers/:id` | `viewer` |
| POST | `/procurement/suppliers` | `admin` |
| PATCH | `/procurement/suppliers/:id` | `admin` |
| DELETE | `/procurement/suppliers/:id` | `admin` — **soft delete** (Section 6.9.1) |
| POST | `/procurement/suppliers/:id/restore` | `admin` |
| GET | `/procurement/requisitions` | `viewer` |
| POST | `/procurement/requisitions` | `user` |
| GET | `/procurement/requisitions/:id` | `viewer` |
| PATCH | `/procurement/requisitions/:id` | `user` (creator, draft only) |
| POST | `/procurement/requisitions/:id/submit` | `user` (creator) |
| POST | `/procurement/requisitions/:id/approve` | `approver` |
| POST | `/procurement/requisitions/:id/reject` | `approver` |
| POST | `/procurement/requisitions/:id/cancel` | creator or `approver` — status only (6.9.2) |
| GET | `/procurement/purchase-orders` | `viewer` |
| GET | `/procurement/purchase-orders/:id` | `viewer` |
| POST | `/procurement/purchase-orders/:id/cancel` | `approver` — `open` POs only (6.9.2) |
| POST | `/procurement/purchase-orders/:id/receipts` | `approver` — **Section 8.4** |
| GET | `/procurement/goods-receipts` | `viewer` |
| GET | `/procurement/goods-receipts/:id` | `viewer` |

### 9.5 Inventory

| Method | Path | Min level |
|---|---|---|
| GET | `/inventory/products` | `viewer` |
| GET | `/inventory/products/:id` | `viewer` |
| POST | `/inventory/products` | `admin` |
| PATCH | `/inventory/products/:id` | `admin` |
| DELETE | `/inventory/products/:id` | `admin` — **soft delete** |
| POST | `/inventory/products/:id/restore` | `admin` |
| GET | `/inventory/warehouses` | `viewer` |
| GET | `/inventory/warehouses/:id` | `viewer` |
| POST | `/inventory/warehouses` | `admin` |
| PATCH | `/inventory/warehouses/:id` | `admin` |
| DELETE | `/inventory/warehouses/:id` | `admin` — **soft delete**, blocked if stock on hand |
| POST | `/inventory/warehouses/:id/restore` | `admin` |
| GET | `/inventory/stock` | `viewer` |
| GET | `/inventory/stock/low` | `viewer` |
| GET | `/inventory/ledger` | `viewer` |
| POST | `/inventory/adjustments` | `approver` |

### 9.6 Finance (stub)

| Method | Path | Min level |
|---|---|---|
| GET | `/finance/journal-entries` | `viewer` |
| GET | `/finance/accounts` | `viewer` |

The Finance **UI** is a "coming soon" page (Section 10.5), but these endpoints must work — the goods-receipt demo needs to prove a journal entry was written.

Accounts are **seeded, not user-managed** in the MVP. A chart of accounts that users can edit needs validation rules (you cannot delete an account with postings, you cannot change the type of an account in use) that belong with the real Finance module, not the stub.

### 9.6.1 CRUD completeness check

Master data must support the **full** lifecycle. A half-built entity — creatable but not editable — is the most common way a demo falls over in front of someone, because the first thing anyone does is try to fix a typo.

| Entity | List | Detail | Create | Edit | Delete | Restore |
|---|---|---|---|---|---|---|
| Products | ✅ | ✅ | ✅ | ✅ | ✅ soft | ✅ |
| Suppliers | ✅ | ✅ | ✅ | ✅ | ✅ soft | ✅ |
| Warehouses | ✅ | ✅ | ✅ | ✅ | ✅ soft | ✅ |
| Tenant users | ✅ | ✅ | ✅ | ✅ | ➖ deactivate | ✅ reactivate |
| Accounts *(seeded; see above)* | ✅ | ➖ | ➖ | ➖ | ➖ | ➖ |

Transactional documents deliberately do **not** follow this shape — they are created, transitioned through states, and cancelled, never edited after submission or deleted (Section 6.9.2). Do not add a `DELETE /requisitions/:id` for symmetry.

Every ✅ above needs a working UI, not just an endpoint. Acceptance steps 21–23 and tests FE22–FE25 verify this end to end.

### 9.7 Dashboard

| Method | Path | Returns |
|---|---|---|
| GET | `/dashboard/summary` | Widget data, filtered to modules the caller can actually read |

### 9.8 Error format

```json
{ "error": "machine_readable_code", "message": "Human sentence.", "details": {} }
```

Codes: `400` malformed · `401` unauthenticated · `403` forbidden / `module_not_enabled` / `insufficient_module_role` · `404` not found · `409` state conflict / `in_use` / `last_admin` · `422` business-rule violation.

List endpoints accept `?includeDeleted=true` (module `admin` only) to surface soft-deleted master data for restore.
