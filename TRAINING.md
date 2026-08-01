# Six days — online interview with the user, Tuesday 2026-08-04

**Format:** online video call, screen share, VS Code. No fixed structure — the
recruiter said the user "just wants to know what your knowledge as a full stack
programmer is like." Expect conversation, a walkthrough of your project, and
probably some live coding on a scenario *they* choose.

**Prep:** Wed 29 Jul – Mon 3 Aug. You are a vibe coder; this plan is built for
that, not against it.

---

## 1. The three things this interview actually tests

1. **Do you understand the thing you built?** Not "can you retype it." Can you
   answer any question about any decision without hesitation.
2. **Can you apply its patterns to a problem you've never seen?** The coding
   task, if there is one, will almost certainly not be this ERP. It'll be a
   business scenario the user cares about. Memorising your repo doesn't help;
   owning the *patterns* does.
3. **Do you know the rest of the stack?** "Full stack programmer knowledge" will
   roam outside this project — indexes, N+1, caching, JWT vs sessions, CORS,
   XSS/CSRF, React re-renders, Docker, CI. Your ERP covers none of those. This
   is your highest-variance gap.

Everything below serves those three, in that order.

---

## 2. Do these today (Wednesday), before any studying

### 2.1 The screen-share audit

If you open this repo in VS Code and share your screen, the file explorer shows
**`START-HERE.md`** and **`CLAUDE.md`** in the first second. `START-HERE.md`
says, in plain text, *"Open Claude Code in that directory and say: Read
CLAUDE.md and start Phase 0."*

**Pre-empt it. Do not hide it.** Hiding is a coin flip you lose catastrophically
if it's spotted. Owning it is strong, and the honest version is good:

> "I built this with Claude Code as the implementer. What's mine is the
> architecture and the specification — `docs/` is twenty reference documents,
> three decision records, and an `AUDIT.md` listing sixteen problems I found in
> my own original plan and corrected before any code was written. I can defend
> any decision in here."

`docs/` is an **asset** with a business-side interviewer: phase plans, an audit
trail, a decisions log giving the reasoning for every deviation from spec. Show
it on purpose. Most candidates have nothing comparable.

Separately, and not as concealment: when you're *coding*, open `backend/` and
`frontend/` as the workspace so the explorer shows the code under discussion.
Answer straight if asked what else is in the repo.

### 2.2 VS Code profile

Gear → Profiles → New Profile, name it `interview`. Configure only that one:

```jsonc
{
  "editor.inlineSuggest.enabled": false,
  "github.copilot.enable": { "*": false },
  "editor.fontSize": 17,
  "terminal.integrated.fontSize": 15,
  "editor.minimap.enabled": false,
  "editor.wordWrap": "on",
  "editor.stickyScroll.enabled": true,
  "breadcrumbs.enabled": true,
  "workbench.colorTheme": "Default Dark Modern"
}
```

Disable Copilot / Cline / Continue / Cody in this profile. **Keep gopls and the
TypeScript language server** — a language server completing a struct field is
not AI and nobody will blink. What you are killing is whole-line and
whole-function ghost text, which on a call assessing your knowledge is
unrecoverable.

Avoid low-contrast themes (Solarized, Nord) — they wash out over video
compression. **Do not write custom snippets** for the RLS block or the
middleware chain: it robs you of the demonstration and looks terrible if noticed.

### 2.3 Turn off AI autocomplete everywhere, for all practice

Not Monday. Today. Every hour you practise with ghost text on is an hour of not
training the thing being tested.

**But: official docs are almost certainly allowed.** You're not training for
total recall — you're training for *"I know this exists, I know its name, I can
find the syntax in forty seconds."* Much lower bar. Notes beside you are fine
too; use them openly (§7).

---

## 3. Own your own repo — the six decisions

Wednesday's real work. For each of these, write the *why* in your own words, on
paper, without copying the phrasing from the code comments. If you can't, read
the file and try again tomorrow.

| # | Decision | Where |
|---|---|---|
| 1 | Tenant isolation in Postgres RLS, not `WHERE` clauses — and why `FORCE`, `WITH CHECK`, and `security_invoker` are each load-bearing | `migrations/005_rls_grants.up.sql`, `004_views_triggers.up.sql` |
| 2 | Composite foreign keys, because **FK checks run as the table owner and bypass RLS** | `migrations/003_constraints.up.sql` |
| 3 | The goods receipt: one transaction, locks before validation, header lock as well as lines | `internal/api/procurement_receipts.go` |
| 4 | `LevelFor`'s ordering — entitlement is the ceiling, checked before the admin shortcut | `internal/identity/level.go` |
| 5 | Derived, never stored — no `qty_on_hand` column, and money as `NUMERIC` text on the wire | `004_views_triggers.up.sql`, `internal/httpx/numeric.go` |
| 6 | Rollback on a 4xx — `httpx.Fail` returns `nil`, so the status code is the only failure signal | `internal/middleware/tenanttx.go` |

The seven snippets in §8 are the raw material. Handwrite them once per day.

---

## 4. The Floor — and why you build it three times

**Build small and add up. Never build big and cut down.** For someone unsure
they'll finish, the second shape means discovering you're behind with nothing
runnable.

**Ten tables, seven endpoints, three screens.** No requisitions, no warehouses,
no suppliers table, no document numbering, no permission matrix. It still
carries both claims worth having: *isolation lives in the database*, and *one
event writes three modules atomically.*

```
tenants(id, name, slug, timezone)
users(id, tenant_id, email, password_hash, full_name, tenant_role, is_active)
--- RLS from here down ---
products(id, tenant_id, sku, name, uom)
stock_ledger(id, tenant_id, product_id, entry_type, qty_delta,
             source_type, source_id, occurred_at)
purchase_orders(id, tenant_id, po_number, status, supplier_name)
purchase_order_lines(id, tenant_id, po_id, line_no, product_id, qty_ordered, unit_cost)
goods_receipts(id, tenant_id, gr_number, po_id, received_by, received_at)
goods_receipt_lines(id, tenant_id, gr_id, po_line_id, product_id, qty_received)
journal_entries(id, tenant_id, je_number, source_type, source_id, memo)
journal_entry_lines(id, tenant_id, journal_entry_id, account_code, debit, credit)
+ stock_balances, po_line_status        -- both WITH (security_invoker = true)
```

Suppliers become a text field. Accounts become a text code on the journal line.
The PO is seeded in SQL, so the requisition flow never gets built.

`POST /login` · `GET /me` · `GET /products` · `GET /stock` ·
`GET /purchase-orders/:id` · **`POST /purchase-orders/:id/receipts`** ·
`GET /journal-entries`

Screens: Login · Stock list · PO detail with a receive form and a confirmation
panel.

Rehearsed, that's **~2 hours**.

### Build it on a different domain every time

This is the point. The coding task won't be your ERP, so drilling your ERP
teaches you nothing transferable. Same ten-table skeleton, same RLS, same one
atomic cross-module write — different nouns:

1. **Thursday** — equipment rental: assets, rental agreements, checkout/return
   movements, a revenue posting.
2. **Sunday** — inter-branch warehouse transfers: a transfer document that
   writes an outbound and an inbound ledger row plus an in-transit journal
   entry, atomically.

If you have a third session spare: clinic appointments → visit → billing.

### The upgrade ladder — only once the floor runs

| # | Add | Cost |
|---|---|---|
| 1 | `FOR UPDATE` on header + lines, over-receipt check in SQL | 15 min |
| 2 | Per-module levels: `tenant_modules`, `user_module_roles`, `LevelFor`, `RequireModule` | 30 min |
| 3 | `Idempotency-Key` + the `SAVEPOINT` replay | 20 min |
| 4 | `document_sequences` + tenant-timezone period | 20 min |
| 5 | Requisition → submit → approve → PO, with the self-approval rule | 40 min |
| 6 | The two constraint triggers as backstops | 15 min |

For every rung you don't reach, **say it instead**: *"I'd normally take an
Idempotency-Key here, generated when the form opens, because this gets posted
from a phone on warehouse wifi."* Most of the credit, none of the minutes.

### The learning loop — three passes

**Pass 1: transcribe and narrate.** Reference open. Type it by hand, never
copy-paste, saying out loud what each line does. If you can't say it, stop and
find out. Slow, feels unproductive, does the work.

**Pass 2: gapped.** Delete it. Retype with only your recall card visible. Note
every stall.

**Pass 3: blind.** Delete it. Retype with nothing. Diff. The diff is tomorrow's
study list.

Build your one-page recall card *as you go*, by hand, from what you stalled on.

---

## 5. The six days

| Day | Focus |
|---|---|
| **Wed** | §2 setup (1h). §3 — the six decisions in your own words (2h). Handwrite the §8 snippets once. |
| **Thu** | **Floor build #1, equipment rental domain.** Passes 1–2, reference open, narrating. ~3h. |
| **Fri** | **Breadth grid** (§6) — one paragraph per question, in your own words, out loud for the weak ones. ~3h. |
| **Sat** | **Demo readiness** (§7): clean run, 8-minute tour rehearsed three times, **one full dry run over a real screen share, recorded and watched back**. ~3h. |
| **Sun** | **Floor build #2, blind, transfers domain**, 2.5h timed. Then §9 defence questions + §10 domain story, out loud. ~4h. |
| **Mon** | Mock interview, recorded, 60 min — mix conversation with one small live build. Fix your top three gaps. **Stop early. Sleep.** |

Wednesday also gets the **break-it-deliberately block** (30 min), which is the
best half hour in this plan. Each of these, watch it fail, understand why:

1. Drop `FORCE` from one table, connect as the owner, see every tenant's rows.
2. Drop `security_invoker` from `stock_balances`, query as `erp_app` — the view
   leaks every tenant while the base table stays correctly isolated.
3. Replace a composite FK with `REFERENCES products(id)`, then insert a
   `stock_ledger` row naming **another tenant's** product ID. It succeeds.
4. Use plain `SET` instead of `set_config(..., true)`, then reuse the pooled
   connection for a second tenant.

Having actually seen these fail is what lets you talk about them.

---

## 6. Breadth grid — Friday

Your ERP covers RLS, transactions, permissions and testing beautifully, and
almost nothing below. One paragraph each, your own words. Out loud for anything
you hesitate on.

**HTTP / API** — status code choice (and why 422 vs 400 vs 409); idempotency and
which verbs need it; pagination (offset vs cursor, and why offset breaks on a
moving list); API versioning; CORS — what a preflight actually is and why a
missing `AllowHeaders` entry looks like a dead endpoint rather than a 4xx.

**Auth** — JWT vs server sessions, and the honest trade; refresh tokens and why
short access-token lifetimes matter; where authorization belongs (and why *not*
in a token claim); password storage (bcrypt/argon2, salt, cost factor); what you
do when someone's access is revoked mid-session.

**SQL** — what an index actually is (B-tree), when one won't be used, composite
index column order, covering indexes; `EXPLAIN ANALYZE` and how to read it; N+1
and the two fixes (join vs batched second query — and why a `LEFT JOIN` on a
paginated query breaks both the page size and the total); transactions and the
four isolation levels; what a deadlock is and how consistent lock ordering
prevents it; `FOR UPDATE` vs `FOR SHARE`; connection pooling and why pooling
makes session-scoped state dangerous.

**Data modelling** — normalisation and when to denormalise deliberately; soft
delete and its costs; audit trails; money (never float — and why); timestamps,
`TIMESTAMPTZ`, storing UTC, and business *dates* in a user's timezone.

**Frontend** — what triggers a React re-render; keys in lists and what breaks
without stable ones; `useEffect` dependency traps and cleanup; where state
should live (and why you avoid a global store until you need one); controlled vs
uncontrolled forms; loading / error / empty / populated as four states you handle
every time; optimistic updates; XSS and why React escapes by default;
`dangerouslySetInnerHTML`; CSRF and why a bearer token in a header isn't
vulnerable the way a cookie is.

**Ops** — what Docker actually gives you; env config and the twelve-factor idea;
where secrets live and where they must not; running migrations safely in a
deploy (and why every migration must be backwards-compatible for one release);
health checks and why they must not touch the database; structured logging and
request IDs.

**Testing** — unit vs integration vs end-to-end and what each is *for*; what to
test at which layer; why you test against a real Postgres rather than a mock
when the logic is in SQL; test data strategy.

---

## 7. Demo readiness — Saturday

### The mechanics

- **Pre-warm everything before the call.** Docker up, migrated, seeded, backend
  running, Vite running, browser tabs open, a `psql` session connected. Four
  minutes of `docker pull` on air is avoidable.
- **Do Not Disturb on.** Quit WhatsApp, Discord, Slack, mail. A recruiter
  message previewing mid-share is a classic.
- **Close other repos.** VS Code's recent-files list is visible on the welcome
  screen. Clean desktop — you'll be sharing the whole screen, not one window,
  because you'll switch between editor, terminal and browser.
- **Wired ethernet if you have it.** Vite HMR + video + Docker on one machine is
  a lot.
- **Fallback if the app dies:** your README has 31 screenshots. Keep it open in
  a tab.
- **Notes beside you, used openly.** *"Let me check the exact `set_config`
  signature."* A calm visible lookup reads as professional; eyes darting
  off-camera reads as shifty, and on video it is very visible.
- **Typing on camera is slower for everyone.** Plan `schema.sql` as a strict
  top-to-bottom type-through, no jumping around. **Say what you're about to
  write before you write it** — it covers thinking time and keeps them with you.

### The 8-minute tour, rehearsed three times

1. **The sentence** (30s) — "A tenant user raises a requisition, someone else
   approves it, the goods arrive, and that single action lands atomically in
   inventory and finance — while a second tenant provably cannot see any of it."
2. **Two browsers, two tenants** (90s) — same screen, different data. Then paste
   tenant A's product UUID into tenant B's URL and get a 404.
3. **The receipt** (3 min) — the receive form, then the confirmation panel, then
   *follow both links*: the stock ledger rows and the balanced journal entry,
   both filtered to that one receipt.
4. **The code behind it** (2 min) — open `procurement_receipts.go`, point at the
   single `tx` threading through steps 4–8.
5. **The proof** (1 min) — `go test ./...` scrolling past, 255 tests. Then say
   which group asserts what.

Then **one full dry run over a real screen share** — solo Meet or OBS — and
watch it back. Font size, dead air and mouse thrash cannot be self-assessed
live.

---

## 8. Recall card — handwrite these daily

**The RLS policy.** `USING` *and* `WITH CHECK` — without `WITH CHECK` a tenant
can write rows tagged with another tenant's ID:

```sql
ALTER TABLE %I ENABLE ROW LEVEL SECURITY;
ALTER TABLE %I FORCE  ROW LEVEL SECURITY;   -- else the owner bypasses it
CREATE POLICY tenant_isolation ON %I
  USING      (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid);
```

The `true` is *missing_ok*, so an unset GUC returns NULL instead of raising.
`NULLIF(..., '')` because after the transaction commits the GUC resets to the
empty string, and `''::uuid` raises. A request with no tenant context must see
**zero rows**, not an error page.

**Setting the context:**

```go
tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantID.String())
```

`set_config(..., true)` not `SET LOCAL` — same effect, but it's a function call
so it takes a bind parameter; `SET LOCAL x = ?` is a syntax error under prepared
statements. Transaction-local, not session-local: a session `SET` leaks tenant
context to the next request that gets that pooled connection.

**Append-only as a grant, not a trigger:**

```sql
REVOKE UPDATE, DELETE ON stock_ledger, journal_entries, journal_entry_lines,
  goods_receipts, goods_receipt_lines FROM erp_app;
```

A grant can't be turned off by `ALTER TABLE ... DISABLE TRIGGER`, and there's
nothing to forget on a new code path.

**The view:**

```sql
CREATE VIEW stock_balances WITH (security_invoker = true) AS
SELECT tenant_id, product_id, warehouse_id, SUM(qty_delta) AS qty_on_hand
FROM stock_ledger GROUP BY tenant_id, product_id, warehouse_id;
```

Without `security_invoker` the view runs as its **owner**, who is not subject to
its own tables' policies — every tenant sees every tenant's stock through the
view while the base table stays correctly isolated. Ownership, not `BYPASSRLS`.

**The composite FK — the subtle one:**

```sql
ALTER TABLE products ADD CONSTRAINT products_id_tenant_uq UNIQUE (id, tenant_id);
ALTER TABLE stock_ledger
  DROP CONSTRAINT stock_ledger_product_id_fkey,
  ADD  CONSTRAINT stock_ledger_product_tenant_fk
    FOREIGN KEY (product_id, tenant_id) REFERENCES products (id, tenant_id);
```

**FK checks run with the table owner's privileges and bypass RLS.** A plain
`REFERENCES products(id)` accepts another tenant's product ID. Making tenant
membership part of the reference closes it.

**The permission funnel** — the order *is* the design:

```go
func (i *Identity) LevelFor(module string) RoleLevel {
    if !i.EnabledModules[module] { return RoleNone }    // entitlement is the CEILING
    if i.TenantRole == TenantAdmin { return RoleAdmin } // implicit, no rows needed
    level, _ := ParseRoleLevel(i.ModuleRoles[module])
    return level
}
```

Entitlement before the admin shortcut: a tenant admin of a company that hasn't
bought Finance resolves to `none` there. A superadmin has no tenant, so no
entitlements, so `none` everywhere — "platform admins can't read tenant data"
falls out of the ordering rather than being a special case. Levels ranked
`none<viewer<user<approver<admin`, so every check is `level >= min`. `none` is
the **absence of a row**.

**Rollback on a 4xx:**

```go
c.Locals(txKey, tx)
if err := c.Next(); err != nil { return err }
if c.Response().StatusCode() >= 400 { return errRejected }   // roll it back
return nil
```

`httpx.Fail` writes the body and returns `nil` deliberately — returning an error
makes Fiber write a second one over it. So the **status code** is the only signal
the request failed. Without this, a handler that writes half a document then
409s **commits the half**.

**Document numbers** — `PR-202607-0001`:

```sql
SELECT to_char(now() AT TIME ZONE t.timezone, 'YYYYMM') FROM tenants t WHERE t.id = ?;

INSERT INTO document_sequences (tenant_id, doc_type, period, last_number)
VALUES (?, ?, ?, 1)
ON CONFLICT (tenant_id, doc_type, period)
DO UPDATE SET last_number = document_sequences.last_number + 1
RETURNING last_number;
```

The period is the **tenant's** month (00:30 on 1 Aug in Jakarta is 17:30 on 31
Jul UTC, and would be filed in the wrong month); a locking upsert, not a sequence
(a sequence isn't tenant-aware and doesn't roll back, so it gaps and shares one
counter); and it runs in the **caller's** transaction, which is what makes a
rolled-back document not consume a number.

**The eight steps of the receipt:**

```
0. Idempotency-Key -> replay? return the stored receipt, 200.
1. SELECT ... FOR UPDATE the purchase_orders header.
   Then ASK ABOUT THE KEY AGAIN.            <- the one everyone misses
2. status IN ('open','partially_received'), else 409.
3. Validate lines. SELECT ... FOR UPDATE the po lines, ORDER BY id.
   Over-receipt check against po_line_status, IN SQL, 422 if any.
4. SAVEPOINT. Allocate GR number. INSERT the header.
   Unique violation on the KEY constraint NAME -> ROLLBACK TO, re-read, replay.
   Any other unique violation -> real error.
5. INSERT lines. product_id comes FROM THE ORDER LINE, never the client.
6. Re-read po_line_status: bool_and(qty_received >= qty_ordered)
   -> 'received' | 'partially_received'. UPDATE the PO. No counter is stored.
7. [INVENTORY] INSERT stock_ledger rows, source_type='goods_receipt'.
8. [FINANCE] Dr 1300 Inventory / Cr 2150 GRNI, amount SUM(qty*cost) in SQL.
   The DEFERRED jel_balanced trigger checks it at COMMIT.
```

Triggers say what is **illegal**. Services say what happens **next**. Never
`INSERT` from a trigger body.

---

## 9. Defence questions — Sunday and Monday, out loud

Three or four sentences each. Rambling means you don't know it yet.

**Tenancy** — 1. Why RLS instead of `WHERE tenant_id = ?` everywhere? 2. What
breaks without `FORCE`? Without `WITH CHECK`? Without `security_invoker`? 3.
Where does `tenant_id` come from, and why never from the request? 4. Your FKs
bypass RLS — how do you know, and what did you do? 5. Why do `users` and
`tenants` have no RLS; isn't that the hole?

**Permissions** — 6. Walk one request end to end. 7. Why two 403 codes rather
than one? 8. A superadmin toggles an entitlement off — when does it take effect?
9. You hide buttons in React; isn't that security theatre?

**The transaction** — 10. Two receipts post simultaneously against the same
line: trace it. 11. Why lock the header as well as the lines? 12. Why is the
idempotency key the client's? 13. Why a `SAVEPOINT` rather than a second
transaction for the replay? 14. Why match the constraint *name* and not SQLSTATE
`23505`?

**Data model** — 15. Why no `qty_on_hand` column? 16. Why `NUMERIC` as a string
on the wire, and where does arithmetic happen? 17. There's no `DELETE` anywhere
— why? And why does resolving a row *by ID* deliberately not filter
`deleted_at`? 18. Timestamps?

**Meta** — 19. What's missing, what would you do differently? *(No audit log
yet, and it's the natural foundation for support impersonation. Single-level
approval only. The modular monolith is right for now, but the cross-module call
is a direct function call on a shared `tx` — exactly what you'd have to break to
split into services, and that's the trade taken deliberately, because the atomic
write is the product requirement.)*

20. **How much of this did AI write?** Answer per §2.1. Rehearse it until it's
comfortable rather than defensive.

---

## 10. The domain story — your unfair advantage

The user is a business stakeholder. They will react far more strongly to this
than to `security_invoker`, and almost no candidate can say any of it.

- **A requisition is a request, a purchase order is a commitment.** They're
  separate documents because the moment money is promised to a supplier is a
  different moment from someone asking for supplies, and the approval sits
  between them.
- **Segregation of duties:** you cannot approve your own requisition — *nobody*
  can, tenant admins included. One person who can both request and approve is
  the entire control gone.
- **Goods received not invoiced (2150):** the stock is on the shelf and the
  invoice hasn't arrived. The liability is real now, so it's recognised now
  rather than whenever Finance opens the post. Dr Inventory, Cr GRNI.
- **Over-receipt is refused, not absorbed.** More arrived than was ordered means
  someone corrects the quantity or raises a second order — silently accepting it
  breaks the match against the supplier's invoice later.
- **Why derived, not stored:** a `qty_on_hand` counter that can disagree with its
  own ledger creates a permanent reconciliation job. Sum the ledger and it can
  never disagree with itself.
- **Why cancel instead of delete:** a document someone acted on is a record of
  what happened. Deleting it makes last quarter's numbers change retroactively.
- **Why per-module roles:** a warehouse supervisor approves stock adjustments and
  has no business in the chart of accounts. One global "admin" flag forces you to
  over-grant.

---

## 11. If you blank on the call

Write the plan first — the ten table names and the seven endpoints, in a file, on
screen. That's your recovery anchor and you can restart from it any time. Having
a visible plan is itself a signal.

Then **narrate constantly.** "I'm doing the database first, because the isolation
guarantee lives there." A candidate thinking out loud with a visible plan reads
as competent even while stuck. A silent candidate reads as lost even while
typing.

If you can't remember syntax: *"I know this is `set_config` with a third argument
for transaction-local — let me check the signature."* Open the docs. That is a
completely normal thing for an engineer to do, and a better signal than silence.

---

## 12. Honest expectation

Six days from here gets you: the Floor built blind on an unfamiliar domain, the
six decisions defended cold, a rehearsed demo, and the breadth grid answered.
It does not get you to reproducing this repository from memory, and it doesn't
need to — nobody is going to ask you to.

The interview is *"what is this person's knowledge like."* Knowledge is
demonstrated by explaining decisions and applying patterns, which is exactly what
this plan drills.
