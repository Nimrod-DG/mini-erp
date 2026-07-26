# What to say about this project / future work

> Phase 10. Not needed while building.
>
> Part of the mini-erp build docs. Index: [`README.md`](README.md)

---

## 16. What to say about this project

For the portfolio write-up and interview, lead with these four:

1. **Tenant isolation is enforced by the database, not by application code.** Postgres RLS with per-transaction session context means a forgotten `WHERE tenant_id = ?` cannot leak data across customers. Explain `SET LOCAL` and why connection pooling makes the `LOCAL` part essential — that detail signals you have actually thought about it rather than copied a tutorial.
2. **Permissions are per-module, not one global role.** A user can approve purchases while being read-only in inventory and locked out of finance entirely. This mirrors how Odoo groups, NetSuite permission levels, and SAP authorization objects work, and it is why real ERPs need a permission matrix rather than three role names.
3. **Stock is derived from an append-only ledger, not a mutable counter.** Every movement is attributable and timestamped, so "why is stock 47?" is always answerable.
4. **One business event writes to three modules in one transaction.** A goods receipt updates the purchase order, appends to the stock ledger, and posts a balanced journal entry — atomically. Test D8 injects a failure mid-way and asserts nothing at all was written. That integration is what distinguishes an ERP from three separate CRUD apps.

5. **Invariants live in the database, business logic lives in Go.** Cross-table rules a `CHECK` cannot express — no over-receipt, journals must balance, terminal documents are immutable — are constraint triggers, so no code path can bypass them. The balanced-journal trigger is `DEFERRABLE INITIALLY DEFERRED` because an entry is legitimately unbalanced between its first and second line. What is deliberately *not* in a trigger: the cross-module posting itself, because that orchestration is the thing worth showing.

6. **Two derived quantities, no stored aggregates.** Stock on hand comes from the ledger; received quantity comes from the receipt lines. Neither is cached in a column that could drift. Over-receipt is prevented by a constraint trigger, because a `CHECK` constraint cannot reference another table — and the trigger takes a row lock on the parent line, which is what makes it safe under concurrent receipts.

7. **Nothing is ever hard-deleted, but not everything is soft-deleted either.** Master data gets `deleted_at` with partial unique indexes so a SKU can be reused; transactional documents move to `cancelled` because the document genuinely happened; ledgers are append-only and corrected with reversing entries. Being able to explain why those are three different mechanisms — rather than putting `deleted_at` on every table — is the part that signals ERP thinking.

On the stack, avoid a common overclaim: **Go binaries are compiled, not encrypted** (Section 2.4.1). The defensible reasons are memory safety, a small container attack surface, and static typing. Encryption in this system is TLS in transit and provider-managed encryption at rest — the language has nothing to do with it. Getting this right matters more than it sounds: an interviewer who hears "Go is encrypted" learns something about your security model that no amount of good code offsets.

On architecture, be precise rather than fashionable: this is a **modular monolith** deployed as a container on Cloud Run with a static SPA on Firebase Hosting. Splitting the frontend from the backend is deployment topology, not service decomposition. The monolith is a deliberate choice — the atomic cross-module write in point 4 is exactly what microservices would have cost you.

Be straightforward about the limits too: Finance is a stub, there is no invoicing or payment cycle, approvals are single-level, and there is no period close. Scoping honestly reads better than overclaiming, and it leaves you room to talk about what you would build next.

---

## 17. Future work

Explicitly out of scope for this build. Listed because "what would you build next?" is a normal interview question, and having a considered answer is worth more than having built any one of these.

**Immediately next — the natural extensions**

| Item | Why it comes first |
|---|---|
| **Audit log** (Phase 11) | Already schema-designed. Every state transition has a marked insertion point. Foundation for everything below. |
| **Support impersonation** | Lets a superadmin temporarily assume a tenant user's identity to debug a support ticket — with every action written to the audit log and the tenant notified. Closes the one deliberate gap in the superadmin model (Section 5.7). |
| **Multi-level approval** | Approval routing by amount threshold or org hierarchy. Currently single-level. This is where a real org chart would enter the data model. |

**Completing the procure-to-pay loop**

The full ERP cycle is requisition → PO → **receipt → vendor invoice → payment**. This build stops at receipt. Continuing it would mean:

- **Vendor invoices** matched against POs and receipts (the classic "three-way match")
- On match, reversing the GRNI liability posted at receipt and booking accounts payable
- **Payments** clearing that payable

That chain is why `2150 Goods received not invoiced` exists as a liability account rather than posting straight to payables — the receipt creates an obligation before the invoice arrives. Being able to explain that account is a good signal of understanding the domain rather than the code.

**Making Finance real**

- Trial balance and general ledger reports
- Period close with locking, so posted periods become immutable
- Cost methods beyond standard cost — moving average or FIFO valuation from the ledger

**Operational maturity**

- Structured logging with request correlation, exported to Cloud Logging
- Read replicas for reporting queries
- Rate limiting per tenant
- Bulk import (CSV) for products and suppliers
