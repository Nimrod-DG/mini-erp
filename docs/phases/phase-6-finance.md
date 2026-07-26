# Phase 6 — Finance stub

**MVP:** yes · **Estimate:** 2h · **Depends on:** Phase 5 green

## Load only these

1. [`../reference/schema.md`](../reference/schema.md) — **§6.5 only**.
2. [`../reference/api.md`](../reference/api.md) — **§9.6 only**.
3. [`../reference/screens.md`](../reference/screens.md) — **§10.5 only**.

That is the whole phase. It is small on purpose.

## Build

1. Chart of accounts seeding per tenant — `1300 Inventory` (asset) and
   `2150 Goods received not invoiced` (liability). Accounts are **seeded, not
   user-managed** in the MVP: an editable chart needs validation rules (cannot
   delete an account with postings, cannot change the type of an account in use)
   that belong with the real Finance module.
2. `GET /finance/journal-entries` and `GET /finance/accounts`, both `viewer`.
3. A single `/finance` page: header reads "Finance — coming soon", and below it a
   **live** read-only journal entry list, introduced with a line saying postings
   from other modules are already flowing in and that reporting and period close
   are not built yet.

## Why the stub is shaped this way

An empty placeholder says "unfinished". A live posting list under a "coming soon"
header proves the cross-module write works while being honest that the module is
incomplete. The endpoints must genuinely work even though the UI is a stub —
without them the goods-receipt demo cannot show that a journal entry was written.

## Do not build

Trial balance, general ledger reports, period close, manual journal entry,
account CRUD, cost methods.

## Done when

- [ ] A receipt posted in Phase 5 appears as a balanced journal entry on the
      Finance page
- [ ] Dewi (Finance `admin`, Inventory `none`) can open the page; a Bahari user
      gets `403 module_not_enabled` on the endpoint
