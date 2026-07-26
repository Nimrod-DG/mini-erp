# Reference — Middleware chain

> Phase 2 and 3. Short; read in full.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 7. Middleware chain

Order matters. Every tenant-scoped route runs these in sequence:

| # | Middleware | Responsibility | Failure |
|---|---|---|---|
| 1 | `RequestID` | Correlation ID into context and response header | — |
| 2 | `FirebaseAuth` | Verify ID token, extract `uid` | `401` |
| 3 | `ResolveIdentity` | `uid` → user row, tenant, module roles | `401` / `403` if inactive |
| 4 | `TenantTx` | Open transaction, `SET LOCAL app.current_tenant` | `500` |
| 5 | `RequireModule(m, lvl)` | Entitlement check, then role-level check | `403` |
| 6 | Handler | Business logic on the transaction handle | varies |

Steps 5 and 6 are per-route. Steps 1–4 are global to `/api/*` except `/api/health`.

`RequireModule` performs **both** checks in this order, and the distinction is visible to the client:

- Module not enabled for tenant → `403 {"error":"module_not_enabled","module":"finance"}`
- Enabled, but user's level is below the minimum → `403 {"error":"insufficient_module_role","module":"procurement","required":"approver","actual":"user"}`

Superadmin routes (`/api/admin/*`) skip steps 4–5 and instead assert `tenant_role = 'superadmin'`, using the `erp_admin` pool.
