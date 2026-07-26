# Phase 7 — Dashboard, seed, and the responsive pass

**MVP:** yes · **Estimate:** 6h · **Depends on:** Phase 6 green

This is the last MVP phase. When it is done, run the acceptance test and stop.

## Load only these

1. [`../reference/api.md`](../reference/api.md) — **§9.7 only**.
2. [`../reference/screens.md`](../reference/screens.md) — **§10.2 only**.
3. [`../reference/seed-data.md`](../reference/seed-data.md) — in full.
4. [`../reference/auth.md`](../reference/auth.md) — **§3.5.3 only** (deterministic seed UIDs).
5. [`../reference/responsive.md`](../reference/responsive.md) — in full, for the second half.
6. [`../reference/business-logic.md`](../reference/business-logic.md) — **§8.6.1 only**, because the seed calls `PostGoodsReceipt`.

## Build — part 1, dashboard

`GET /dashboard/summary`, filtered to modules the caller can actually read, and
four widgets each omitted if the user cannot read its module:

1. **Open purchase orders** — count and total value, linking to the PO list
2. **Requisitions awaiting approval** — count; for `approver`+, an inline
   approve/reject queue
3. **Low stock** — products under reorder point, with a "Create requisition"
   shortcut that pre-fills them
4. **Recent activity** — last 15 stock ledger entries, each linking to its source

## Build — part 2, seed

The seed is what makes the project reviewable cold. Get the volumes right.

- Idempotent, with deterministic `seed-<slug>` Firebase UIDs so reseeding keeps
  `users.firebase_uid` stable.
- Two tenants with **different entitlements and different timezones** — that
  difference is what makes module gating and §2.5.3 visible without touching the
  admin console.
- Seven users exactly as specified. **No `user_module_roles` rows for Rina or
  Agus** — their access derives from `tenant_role`, and the seed asserting that
  is what test B7 mirrors.
- Spread `occurred_at` across the preceding **60 days**. A ledger with one
  timestamp looks like a fixture; a spread one looks like a system that has been
  running.
- Two fully-received POs per tenant, with their ledger entries and balanced
  journal entries present, so a reviewer sees a completed
  procurement → inventory → finance flow **without performing one first**.
- **Run receipts through `PostGoodsReceipt`**, not by inserting ledger and journal
  rows directly. The seed then exercises the same code path as the application
  and cannot drift from it — and if it produces an unbalanced journal, the
  `jel_balanced` trigger rejects it, which is exactly the safety net you want.
  This means the seed needs deterministic idempotency keys —
  `seed-gr-<tenant-slug>-<n>` — or a reseed posts every receipt twice.

## Build — part 3, responsive pass

Not "shrink the desktop grid". The split is:

- **Goods receipt** — genuinely mobile-first. Performed at a loading dock,
  one-handed, standing next to the delivery. Build it for a phone.
- **Requisition approval** — phone/tablet. A two-button decision between meetings.
- Everything else — *usable* on a phone without pretending that is the context.

Work: bottom tab bar and drawer below `md` (respecting entitlements exactly like
the sidebar — three tabs, not a disabled fourth); card transformation on the
requisition and PO lists; horizontal scroll with a frozen first column on the
stock and ledger grids; sticky bottom action bars on forms; a 44×44px touch
target audit; `inputmode="decimal"` on every quantity field.

## Done when

- [ ] A freshly seeded database opens on a dashboard with real numbers
- [ ] Each seeded user sees a different set of widgets
- [ ] The **full acceptance test completes on a 360px viewport**, in both light
      and dark mode
- [ ] Reseeding twice in a row produces the same database

## Then — the MVP gate

> **Stop. Run the full [acceptance test](../acceptance-test.md) locally.**
> All **twenty-five** steps must pass and test Groups **A–J** must be green.
> (The original document stated this gate four different ways and understated the
> step count; see AUDIT C1 and C2 for the reconciliation.)
>
> This is a finished, demonstrable project. Screenshot it, write it up, talk
> about it in an interview exactly as it stands. Do not start Phase 8 until this
> line is genuinely crossed.
