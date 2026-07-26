# Decision 001 — Modular monolith, not microservices

> Rationale. Read if you are tempted to split a package or add a queue.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 2. Architecture

### 2.1 Shape of the system

This is a **modular monolith** behind a static SPA:

- **One Go service.** Procurement, Inventory, Finance, and Admin are internal packages inside a single deployable binary. They share one database, one connection pool, and one transaction scope.
- **One PostgreSQL database.** Shared schema, tenant-isolated by row-level security.
- **One React SPA**, served as static files.

**Do not split the backend into microservices.** The entire value of this project is the atomic cross-module write in Section 8.4. Splitting Procurement, Inventory, and Finance into separate services would require distributed transactions or saga compensation to achieve what a single `BEGIN … COMMIT` gives for free. Real ERPs — Odoo, ERPNext, SAP's core — are modular monoliths for exactly this reason.

Deploying the frontend and backend separately is **deployment topology**, not service decomposition. It does not make this a microservice architecture, and it should not be described as one.

### 2.2 Module boundaries inside the monolith

Modules are enforced by convention, and the convention is worth stating explicitly because it is what makes "modular" meaningful:

- Each module owns its tables and its `internal/<module>/` package.
- A module may **read** another module's data through that module's service layer, never by reaching directly into its tables from a foreign package.
- Cross-module **writes** happen only through an explicit orchestration function that takes a transaction handle — see `procurement.PostGoodsReceipt` in Section 8.4.
- No module imports another module's HTTP handlers.

If a future maintainer wanted to split this into services, these boundaries are where the seams already are. That is the argument for a modular monolith: you get the option without paying for it upfront.
