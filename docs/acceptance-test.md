# Acceptance test

> Run by hand after Phase 7. Doubles as the interview demo script.
>
> Part of the mini-erp build docs. Index: [`README.md`](README.md)

---

## 14. Acceptance test

Run by hand before calling the project done. It doubles as the interview demo script. Uses the seed data from Section 15.

**Entitlement and isolation**

1. Log in as `super@erp.test`. Confirm you land on the tenant list with **no** business modules.
2. Open **Bahari Logistics**. Confirm Finance is toggled off.
3. Log in as `dewi@nusantara.test` (Finance `admin`, Procurement `viewer`, Inventory `none`). Confirm the sidebar shows Procurement and Finance but **not** Inventory. Call `/api/inventory/products` directly — expect `403 insufficient_module_role`.
4. Log in as `staff@bahari.test`. Call `/api/finance/journal-entries` — expect `403 module_not_enabled` (a different code than step 3; the distinction matters).
5. Log in as `agus@bahari.test` — a **tenant admin**. Confirm he reaches Procurement and Inventory fully, but sees **no Finance nav item**, and that `/api/finance/journal-entries` returns `403 module_not_enabled`. **Entitlement outranks the admin shortcut.**

**Permission levels**

6. As `sari@nusantara.test` (staff, Procurement `user`), create a requisition for 2 products and submit it. Confirm no "Approve" button is rendered.
7. As Sari, call the approve endpoint directly — expect `403 insufficient_module_role`.
8. As `budi@nusantara.test` (staff, Procurement `approver`), create and submit a second requisition, then try to approve **your own** — expect `403 self_approval_forbidden`.
9. As Budi, approve **Sari's** requisition. Confirm a PO is generated with matching lines and correct total.
10. As `rina@nusantara.test` (tenant `admin`), confirm she reaches all three modules **without** any `user_module_roles` rows in the database. Create a requisition as Rina, then try to approve it — expect `403 self_approval_forbidden`. **Admins are not exempt from segregation of duties.**
11. Still as Rina, open user settings. Confirm the per-module matrix is shown for staff users and hidden (or read-only, marked "implicit") for admins. Grant Sari `approver` in Procurement; confirm Sari's "Approve" button appears on next load.
12. As Rina, attempt to demote yourself to `staff` — expect `409 last_admin`, since Rina is Nusantara's only admin. Promote Budi to `admin`, then demote Rina — this should now succeed.

**The cross-module transaction**

13. As Budi, open the PO and receive **partial** quantities on both lines. Confirm:
    - PO status is `partially_received`
    - Stock ledger has 2 new `receipt` entries
    - Stock balances increased by exactly the received amounts
    - A journal entry exists whose debit total equals its credit total
    - The confirmation panel links to both
14. Receive the remaining quantities. Confirm PO status flips to `received`.
15. Attempt to receive **one more unit** — expect `422 over_receipt`, and confirm no new ledger or journal rows were written.
16. Open the Finance page as Dewi. Confirm both journal entries are listed and balanced.

**Concurrency**

17. Post a goods receipt, then replay the exact same request with the same `Idempotency-Key`. Confirm the response is `200` with the original receipt, and that the stock ledger and journal entry counts are **unchanged**.

**Data preservation**

18. As Rina, soft-delete a product that appears on an existing PO line. Confirm it vanishes from the product picker, the existing PO line **still renders its name**, and the stock ledger history is intact. Restore it and confirm it reappears.
19. Attempt to delete a supplier with an open PO — expect `409 in_use` naming the blocking documents.
20. Attempt to cancel the fully-received PO from step 13 — expect `409`. Cancel a different `open` PO and confirm it moves to `cancelled` rather than disappearing from the list.

**CRUD completeness** (Section 9.6.1)

21. As Rina, for **each** of products, suppliers, and warehouses: create a record, open its detail screen, edit a field and confirm the change persists after reload, soft-delete it, confirm it disappears from lists and pickers, find it under "Show deleted", and restore it. All three entities must complete the full loop.
22. Create a product, then attempt to create a second with the same SKU — expect a clear validation error, not a 500.
23. As Rina, create a new tenant user, assign them `approver` in Procurement, log in as them, and confirm the Approve button appears.

**Isolation, again**

24. As `manager@bahari.test`, confirm none of Nusantara's requisitions, POs, products, or ledger entries are reachable — by UI navigation or by pasting a Nusantara UUID into a detail URL.
25. As `agus@bahari.test` (a tenant admin), repeat step 24. Confirm that being an admin of *another* tenant grants nothing here — RLS is not role-aware.

If all twenty-five pass, the build is complete.
