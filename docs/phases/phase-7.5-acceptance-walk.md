# Phase 7.5 — The acceptance walk

**MVP:** yes — **this is the gate** · **Estimate:** 1–1.5h · **Depends on:** Phase 7 green

Phase 7 built the last of the MVP. This phase is the only thing between it and a
finished project: **opening the application in a browser and looking at it.**

It exists as its own phase because it is not a build. Nothing here is written; it
is walked, and what it finds is written down.

## Load only these

1. [`../acceptance-test.md`](../acceptance-test.md) — the twenty-five steps, in
   full. This document is how to walk them, not a replacement for them.
2. [`../PROGRESS.md`](../PROGRESS.md) — *Current state* only, and only if you
   want the detail of what is in the demo. **This document is otherwise
   self-contained**: the accounts, the setup, and the expected numbers are all
   below.

Do not load the reference docs. If a screen looks wrong, the question is whether
it looks wrong, not what the specification said — and the answer to that goes in
`PROGRESS.md` for a later phase to fix.

---

## What is already verified, and what is not

**All twenty-five acceptance steps have been verified at the API level** —
roughly 150 assertions, driven against the real server with real Firebase ID
tokens for all eight seeded accounts, on the real seeded database. Every business
rule the acceptance test names has been confirmed to hold: the entitlement codes,
segregation of duties, `last_admin`, the cross-module transaction, over-receipt,
the idempotent replay, `in_use`, the soft-delete/restore loop for all three
entities, and cross-tenant isolation by pasted UUID.

**So the backend is not what you are testing.** What has never been looked at is
everything above it:

| Not verified | Why the API pass cannot cover it |
|---|---|
| Whether a control is **rendered** or hidden | I12 makes hiding cosmetic; the server refuses either way, so both pass at the API |
| The **360px layout** | Cards, frozen columns, sticky bars, the tab bar — none of it exists to an HTTP client |
| **Both themes** | Same |
| Whether a link **goes where it says** | The endpoint answers correctly whether or not anything links to it |
| **Copy** — empty states, refusal messages as a person reads them | A test asserts a code; a person reads a sentence |

Two findings came out of the API pass and are already fixed, which is the
calibration for what to expect: `/api/me` was sending the stored role map instead
of the effective one (a tenant admin got a sidebar with nothing in it), and four
touch targets were under 44px. Both were invisible to the Go suite.

---

## Before you start

### 1. Stop anything already running — this one bites

**An API server started before Phase 7 does not have `/api/dashboard/summary`,
and it still serves the old `/api/me`.** Walking against it means the dashboard
404s and every tenant admin appears to have no navigation — the exact bug Phase 7
fixed, back again, because the process predates the fix.

`Ctrl-C` the terminal running `make dev`. If nothing is obviously running but the
port is held — `make dev` reporting *"Only one usage of each socket address"* is
the symptom — find and kill it:

```bash
netstat -ano | grep ":8080" | grep LISTENING     # last column is the PID
taskkill //F //PID <pid>
```

Same for `:5173` if the Vite server is stuck.

### 2. Rebuild the database

```bash
docker compose down -v && make up && make migrate && make seed
```

`down -v` guarantees a pristine demo. It also drops three scratch accounts left
over from Phases 2–3, one of which is the developer's own Google account
(`dgjy2019@gmail.com`). To put that back afterwards — the Firebase account itself
survives, only the row goes:

```sql
INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role)
SELECT gen_random_uuid(), t.id, '<the Firebase UID>', 'dgjy2019@gmail.com',
       'Dev Account', 'admin'
FROM tenants t WHERE t.slug = 'nusantara';
```

To keep it instead, skip `down -v` and run `make seed` alone — but then Nusantara
has three tenant admins and **step 12's `last_admin` will not fire**, because
Rina is not the last admin. That is correct behaviour, not a bug; it was
confirmed by temporarily demoting the other two.

### 3. Start

```bash
make dev          # api :8080, web :5173
```

**Do not try to probe whether the running server is current.** There is no cheap
way: the auth chain is group middleware on `/api`, so it answers `401` before the
router is reached and `GET /api/anything-at-all` returns `401` whether the route
exists or not. Phase 5 measured this and recorded it in *Decisions taken* — the
"401 rather than 404 means the route is wired" check does not prove that.

So the only reliable move is step 1: kill it and start it again. If you skip that
and the old binary is still up, the symptom is the dashboard failing to load and
every tenant admin appearing to have no navigation.

### 4. The browser

Set it to **360px** — devtools → responsive → 360×800 — and leave it there for
the whole walk.

**Do not walk it twice for the two themes.** Do the functional pass once in dark,
then Part C is a look-only pass in light.

### The accounts

Password for all of them is `password123`. `backend/cmd/seed/data.go` is the
source of truth if this ever disagrees.

| Email | Workspace | Tenant role | Procurement | Inventory | Finance |
|---|---|---|---|---|---|
| `super@erp.test` | *platform* | superadmin | — | — | — |
| `rina@nusantara.test` | Nusantara Retail | admin | *implicit* admin | *implicit* admin | *implicit* admin |
| `budi@nusantara.test` | Nusantara Retail | staff | approver | viewer | none |
| `sari@nusantara.test` | Nusantara Retail | staff | user | user | none |
| `dewi@nusantara.test` | Nusantara Retail | staff | viewer | none | admin |
| `agus@bahari.test` | Bahari Logistics | admin | *implicit* admin | *implicit* admin | **none — not entitled** |
| `manager@bahari.test` | Bahari Logistics | staff | approver | approver | none |
| `staff@bahari.test` | Bahari Logistics | staff | user | viewer | none |

Nusantara is `Asia/Jakarta` with all three modules; Bahari is `Asia/Makassar`
with **no Finance**.

---

## Part A — the walk

Ordered to minimise sign-ins, not by step number. **Order is load-bearing in two
places**, both flagged.

For each step, the API behaviour is already confirmed. You are checking the
screen: that the right things are visible, that they are readable at 360px, and
that clicking them does what they say.

### A1 · `super@erp.test`

- [ ] **1** Lands on the tenant list. Sidebar shows **Workspaces only**. A
      `platform` badge in the top bar. The dashboard says you administer
      workspaces rather than working inside one — **not** four empty widgets.
- [ ] **2** Open **Bahari Logistics** → Finance toggled **off**, the other two on.
- [ ] `/admin/tenants/new` renders its module list. Do not submit.

### A2 · `dewi@nusantara.test` — Finance admin, Procurement viewer, no Inventory

- [ ] **3** Sidebar shows **Procurement and Finance, not Inventory**. No Inventory
      tab in the bottom bar.
- [ ] Dashboard shows **exactly two** widgets — no stock panel reporting zero.
      "Awaiting approval" shows a count with **no queue** and the line "An
      approver has to make these decisions."

### A3 · `staff@bahari.test`

- [ ] **4** No Finance in the nav. Force `/finance` in the URL — it must not
      render finance data.

### A4 · `agus@bahari.test` — tenant admin, no Finance

- [ ] **5** **The most important one.** This is the case the `/api/me` bug broke,
      so look hard: Procurement and Inventory reachable **in full** — sidebar
      populated, bottom tabs present, screens open — and **no Finance nav item**.
- [ ] **25** Paste a Nusantara order UUID into `/procurement/orders/<id>` →
      not found. Get one with:
      `SELECT po.id FROM purchase_orders po JOIN tenants t ON t.id=po.tenant_id WHERE t.slug='nusantara' LIMIT 1;`

### A5 · `manager@bahari.test`

- [ ] **24** Same as 25, as staff rather than an admin.

### A6 · `sari@nusantara.test` — Procurement `user`

- [ ] **6** Create a requisition for **two products** and submit it. **No Approve
      button** anywhere on it. *(Call this **PR-A**.)*
- [ ] **7** She has no way to reach approval in the UI at all.
- [ ] The requisition **list at 360px is cards**, not a table — number as a link,
      status chip, four fields. Widen past 768px and watch it become a `<table>`.

### A7 · `budi@nusantara.test` — Procurement `approver`

- [ ] **8** Create and submit one of his own, then try to approve it → refused.
      Do it **from the dashboard queue** too: his own row should show "You raised
      this, so somebody else has to approve it" instead of buttons.
- [ ] **9** Approve **PR-A from the dashboard queue**. Open the generated PO and
      confirm the lines and total match. *(Call this **PO-A**.)*
- [ ] The queue row for the requisition with no supplier says "No supplier chosen
      yet — open it to pick one" instead of offering Approve.

**⚠ 13 → 14 → 15 must run in this order, on PO-A.**

- [ ] **13** PO-A → Receive goods. Receive **partial** quantities on both lines.
      On the confirmation panel:
      - status now `partially_received`
      - "2 stock ledger entries created" — **click it**, lands on the ledger
        filtered to this receipt
      - "journal entry JE-… posted (Dr Inventory / Cr GRNI)" — **click it**, lands
        on `/finance` filtered to this entry
      - both amounts equal
      **This is the screenshot the project exists for.** Look at it properly.
- [ ] **14** Receive the remainder → status flips to `received`.
- [ ] **15** Try one more unit → refused, and the message names the line
      readably.
- [ ] **17** *(optional — fully verified at the API, and it cannot be done by
      clicking: the form mints a new key each time it opens.)* Skip, or replay the
      POST from devtools' network panel.
- [ ] **20** Cancel PO-A → refused. Cancel an open one → it moves to `cancelled`
      and **stays in the list**.

### A8 · `dewi@nusantara.test` again

- [ ] **16** `/finance` lists the entries from 13 and 14 **with their Dr/Cr
      lines**. The two account chips filter the list. Check the Dr/Cr line list
      renders legibly inside a table cell at 360px — it has no precedent to copy.

### A9 · `rina@nusantara.test` — tenant admin

**⚠ 12 last. It removes her own access.**

- [ ] **10** All three modules reachable. Create a requisition, submit, try to
      approve it → refused. Admins are not exempt.
- [ ] **18** Soft-delete a product that is on a **closed** order line —
      `OFF-PAPER` or `HND-PALLET`. (A product on an *open* order is refused with
      `in_use`, which is correct and is step 19's rule.) Confirm: gone from the
      picker, the PO line still shows its name marked deleted, the ledger history
      intact, the **stock grid still shows its balance, marked**. Restore it.
- [ ] **19** Delete **PT Kemas Sejahtera** (has an open PO) → refused, and the
      message names the blocking documents in a sentence a person can act on.
- [ ] **21** The full CRUD loop for **each** of products, suppliers, warehouses:
      create → detail → edit → reload → soft-delete → gone from lists *and
      pickers* → "Show deleted" → restore.
- [ ] **22** Create a product with SKU `PKG-TAPE` → a clear message, not a 500.
- [ ] **23** Create a user, give them `approver` in Procurement, sign in as them,
      confirm the Approve button appears.
- [ ] **11** User settings: the per-module matrix is shown for staff and hidden
      or marked "implicit" for admins. Grant **Sari** `approver` → sign in as
      Sari → her Approve button appears.
- [ ] **12** ← **last.** Demote yourself → refused (`last_admin`). Promote Budi,
      then demote Rina → succeeds.

---

## Part B — Phase 7's own "done when"

New this phase and never rendered. Check while walking Part A.

- [ ] **The dashboard opens on real numbers.** Nusantara: **3** open orders,
      **3** awaiting approval, **3** low stock (`PKG-BOX-S` 140/200, `PKG-TAPE`
      90/150, `HND-TROLLEY` 2/4), **15** recent movements.
- [ ] **Different users, different widgets** — Dewi two, Rina four, superadmin
      none.
- [ ] **Low stock → "Create requisition"** pre-fills all three products *with
      their shortfalls* as quantities.
- [ ] **Recent activity** rows link to their source; adjustments name the person
      instead.
- [ ] **Frozen first column** on `/inventory/stock` and `/inventory/ledger`.
      Scroll sideways: the first column and the header stay put and nothing shows
      through them. **Riskiest new markup** — the ledger pins "When", which is
      wide at 360px.
- [ ] **Sticky action bar** on the receive form and `/procurement/requisitions/new`.
      It must sit **above** the bottom tab bar. **Second riskiest** — `bottom-14`
      assumes a tab bar exists, and a user with fewer than two tabs has none,
      which will leave it floating 56px up.
- [ ] **Bottom tab bar respects entitlements** — Dewi gets three tabs, no
      disabled fourth.
- [ ] **Touch targets** — theme toggle, toast dismiss ✕, "Show all movements",
      "Forgot your password?" were all under 44px and were fixed. Tap them.

---

## Part C — the light-mode pass

No data changes. Flip the theme, still at 360px:

- [ ] Dashboard · [ ] requisition list + detail · [ ] PO list + detail
- [ ] The receive form and **the confirmation panel**
- [ ] `/inventory/stock` and `/inventory/ledger` — the frozen columns, where an
      opaque background is load-bearing and a wrong token shows as a smear
- [ ] `/finance` · [ ] sign out and `/login`

---

## Done when

- [ ] Every box in Parts A, B, and C is ticked, or has a finding written against it
- [ ] Findings are recorded in `PROGRESS.md` under a `## Phase 7.5` block: what
      passed, what it found, and one line per fix
- [ ] Anything found is either **fixed with a test standing behind it**, or
      explicitly deferred with a reason — the Phase 4 precedent is
      `TestDeletedProductsStockStaysVisibleEverywhere`, the Phase 7 one is `B13`
- [ ] The demo database is put back: `docker compose down -v && make up && make migrate && make seed`

## Then — the MVP is done

> When the boxes are ticked, **Phase 7 is complete and the MVP gate is crossed.**
> Mark it so in `PROGRESS.md`, and
> [`phase-8-frontend-tests.md`](phase-8-frontend-tests.md) opens.
>
> Take the screenshots first. This is a finished, demonstrable project and the
> confirmation panel is the picture worth having.

**Feed the findings into Phase 8.** FE1–FE26 run against MSW mocking `/api/*`,
so they catch a different class of bug from this walk — a mock encodes what you
*believe* the server sends, which is exactly why it could never have caught the
`/api/me` bug. What this walk finds is the best available list of what those
tests should assert first.
