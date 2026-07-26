# Scope — what this project is and is not

> Read once at Phase 0. Do not re-read; the MVP boundary is repeated in CLAUDE.md.
>
> Part of the mini-erp build docs. Index: [`README.md`](README.md)

---

## 1. What this project is

A small but architecturally serious ERP covering three modules:

| Module | Status in this build | What it does |
|---|---|---|
| **Procurement** | Fully built | Requisitions → approval → purchase orders → goods receipt |
| **Inventory** | Built, supporting role | Products, warehouses, append-only stock ledger |
| **Finance** | Stub / "coming soon" | Chart of accounts + journal entries, written to by goods receipt |

It is **multi-tenant**: one application, one database, many customer companies, with isolation enforced at the database level by PostgreSQL row-level security. Above the tenants sits a superadmin console that controls which modules each tenant may use. Within a tenant, users hold **per-module role levels** — a person can be an approver in Procurement but read-only in Inventory.

### 1.1 Why this shape

The point of an ERP is not the individual modules — plenty of standalone apps do purchase orders. The point is that modules **share one database and one transaction**, so a single business event updates several domains at once without a sync job.

This project demonstrates that with one specific flow: **posting a goods receipt writes a stock ledger entry (Inventory) and a journal entry (Finance) in the same database transaction as the purchase order update (Procurement).** That is the ERP moment, and it is the thing worth pointing at in an interview.

### 1.2 MVP scope — what "done" means

**The MVP is Phases 0–7.** That is the bar for calling this project complete. Everything in Section 13 beyond Phase 7 is optional polish, and everything in Section 17 is explicitly future work.

The MVP is defined by one sentence: **a tenant user can raise a requisition, have it approved by someone else, receive the goods, and see that single action land atomically in inventory and finance — while a second tenant, and an under-privileged user, provably cannot see or do any of it.**

If a feature is not needed for that sentence to be true and demonstrable, it is not MVP.

**In scope for MVP:**

| Area | Included |
|---|---|
| Tenancy | RLS isolation, two seeded tenants |
| Auth | Firebase login, password reset, identity resolution |
| Permissions | Module entitlements + per-module role levels + self-approval rule |
| Procurement | Suppliers, requisition → approval → PO → goods receipt |
| Inventory | Products, warehouses, append-only ledger, balances, low stock |
| Finance | Chart of accounts, journal entries written by goods receipt, read-only page |
| Admin | Superadmin tenant/entitlement console, tenant admin user + staff role matrix |
| Dashboard | Four role-aware widgets |
| Data safety | Soft delete for master data, cancel-not-delete for documents, append-only ledgers (Section 6.9) |
| Data integrity | Composite tenant FKs, CHECK constraints, constraint triggers, index set (Section 6.10) |
| Concurrency | Idempotent receipts, `FOR UPDATE` on approve/receive (Section 8.6) |
| Responsive | Works to 360px; goods receipt and approval flows mobile-first (Section 10.7) |
| Theming | Light/dark/system via semantic tokens, no flash (Section 10.8) |
| Tests | Groups A–F (Section 12.3) |

**Deferred — build only after the MVP is demonstrably working:**

| Deferred item | Why it can wait |
|---|---|
| **Audit log + visibility rules** (Section 6.7) | Nothing in the acceptance test depends on it. Valuable, cheap to add later, and the natural foundation for support impersonation. |
| Support impersonation | Needs the audit log first. |
| Deployment to Cloud Run / Firebase Hosting (Phase 9) | The MVP is provable locally. Deploy once it works. |
| CI pipeline (Phase 8) | Tests matter; automating them does not block completion. |
| Frontend component tests (Section 12.5) | Backend Groups A and D are the ones that prove the architectural claims. |
| Loading skeletons, empty states, toasts | Cosmetic. |

**Guidance for Claude Code:** when a phase description mentions a deferred item, skip it and leave a `// TODO(post-mvp):` comment at the call site. Do not let deferred work block a phase from being marked done. If time runs short, cut from the bottom of the deferred list upward — never cut tests from Groups A or D.

### 1.3 Explicit non-goals

Do not build these. They add scope without adding architectural interest:

- Vendor invoicing, three-way match, payments, bank reconciliation
- Multi-currency, FX revaluation
- Manufacturing, work orders, bills of materials
- Sales orders / order-to-cash
- Email notifications, file uploads, PDF generation
- Real-time updates / websockets
- Multi-level or conditional approval chains (single-level approval only)
- Mobile-responsive polish beyond "does not break"

---
