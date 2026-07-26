# mini-erp — agent instructions

You are building a multi-tenant ERP. This file is loaded automatically on every
session. **Keep it loaded; load nothing else until you know which phase you are in.**

---

## 1. First three actions of every session

1. Read `docs/PROGRESS.md`. It tells you the current phase and what is done.
2. Open `docs/phases/phase-<N>.md` for that phase. It lists exactly which
   reference files to load — load **only** those.
3. Run the current phase's tests before writing anything, to confirm the
   starting state matches what `PROGRESS.md` claims.

Do not read the whole `docs/` tree. It is deliberately split so that no phase
needs more than three or four reference files at once.

---

## 2. Invariants — true in every phase, never negotiable

| # | Invariant |
|---|---|
| I1 | Every tenant-scoped query runs inside a transaction that has set `app.current_tenant` via `db.WithTenant`. A handler that touches tenant data outside that helper is a bug. |
| I2 | `SET LOCAL`, never plain `SET`. A session-scoped set leaks tenant context across pooled connections. |
| I3 | No database role has `BYPASSRLS` or `SUPERUSER`. Test A10 asserts this. |
| I4 | Both views (`stock_balances`, `po_line_status`) are created `WITH (security_invoker = true)`. Without it they leak every tenant. |
| I5 | There is no `DELETE` in business logic. Master data soft-deletes, documents cancel, ledgers append. |
| I6 | Stock on hand and received quantity are **derived**. Never add a stored counter column. |
| I7 | All timestamps `TIMESTAMPTZ`, stored UTC. Business *dates* use `tenants.timezone`. |
| I8 | Money is `NUMERIC(18,2)`, quantities `NUMERIC(18,4)`. Never float. |
| I9 | Authorization is resolved from the database on every request. Never from a Firebase custom claim or a client-supplied field. |
| I10 | Triggers state that something is illegal. Services state what happens next. Never `INSERT` from a trigger body. |
| I11 | Tests are written inside the phase that introduces the behaviour, never as a cleanup pass. |
| I12 | Frontend hiding is cosmetic. Every hidden control is independently enforced server-side. |
| I13 | One repo, two applications. `go.mod` lives at `backend/go.mod` and `package.json` at `frontend/package.json` — **never at the repository root**. Go commands run from `backend/`, npm commands from `frontend/`. |

---

## 3. Naming contracts

These strings appear in migrations, Go types, JSON, the frontend, and the tests.
Changing one after Phase 1 means touching all five. **Use them exactly.**

- Conventions: DB `snake_case` · Go `PascalCase` · JSON/TS `camelCase` · all PKs UUID.
- Role levels: `none` `viewer` `user` `approver` `admin` (ranked, 0–4)
- Tenant roles: `staff` `admin` `superadmin`
- Requisition status: `draft` `submitted` `approved` `rejected` `cancelled`
- PO status: `open` `partially_received` `received` `cancelled`
- Ledger entry type: `receipt` `issue` `adjustment`
- Ledger source type: `goods_receipt` `manual_adjustment` (`reversal` is post-MVP)
- Module codes: `procurement` `inventory` `finance`
- Document numbers: `<PREFIX>-<YYYYMM>-<SEQ4>`, prefixes `PR` `PO` `GR` `JE`
- Error codes: `module_not_enabled` `insufficient_module_role`
  `self_approval_forbidden` `last_admin` `in_use` `over_receipt` `tenant_suspended`

If a name genuinely seems wrong, change it in Phase 1 or not at all.

---

## 4. Scope discipline

- **The MVP is Phases 0–7.** Phases 8–11 do not exist until Phase 7 is green and
  the acceptance test passes locally.
- When a phase mentions something deferred, skip it and leave
  `// TODO(post-mvp): <thing>` at the call site. Deferred work never blocks a
  phase from being marked done.
- The audit log is **post-MVP**. Do not create the tables during Phases 0–7.
- Do not abstract before the second concrete use case exists.
- Do not add endpoints, columns, or screens for symmetry. If it is not needed for
  the sentence in `docs/00-scope.md`, it is not MVP.

---

## 5. Ending a work session

Before you stop, append to `docs/PROGRESS.md`:

```
## Phase <N> — <date>
Done: <what now works>
Tests green: <IDs>
Deviations from spec: <none | what and why>
TODO(post-mvp) markers added: <file:line list>
Next: <the single next action>
```

That file is how the next session starts without re-reading the specification.

---

## 6. Doc map

| Need | File |
|---|---|
| What phase am I in | `docs/PROGRESS.md` |
| What do I do now | `docs/phases/phase-<N>.md` |
| Why a doc says what it says | `docs/AUDIT.md` — corrections already applied; read only if something looks wrong |
| Anything else | `docs/README.md` |
