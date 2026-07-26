# Build progress

**This file is the handoff between sessions.** Read it first, append to it last.
It exists so a new session can start without re-reading the specification.

---

## Project facts

Filled in as they are established, so a later session does not go hunting.

| | |
|---|---|
| Firebase **dev** project ID | `erp-project-b66ce` (display name "erp project", Spark plan) |
| Firebase dev web app | `erp-mini` — appId `1:889259985673:web:1bd9b28a7769be3142e1d8` |
| Firebase dev service account | `backend/secrets/erp-project-b66ce-firebase-adminsdk-fbsvc-47d25660f5.json` (gitignored, never committed) |
| Sign-in provider | Email/Password, enabled and the only one — confirmed 2026-07-26 |
| Firebase **prod** project ID | *not created — Phase 9* |
| Hosting site | `erp-project-b66ce`, auto-linked at app registration, unused until Phase 9 |
| Database host | local Docker (Phase 0–8); host chosen at Phase 9 |

Wherever the docs say `erp-dev`, read `erp-project-b66ce`.
Config values live in [`reference/env-setup.md`](reference/env-setup.md).

---

## Current state

**Phase:** 1 — done
**Next action:** open [`phases/phase-2-auth.md`](phases/phase-2-auth.md) in a **new session**

Migration files that now exist (`backend/migrations/`), applied in this order by
`cmd/migrate`:

| File | Contents |
|---|---|
| `000_roles.sql` | roles, timezone pinning, platform-table grants — **not** versioned; re-applied on every `make migrate` |
| `001_platform.up.sql` | `tenants` `modules` `tenant_modules` `users` `user_module_roles`, `touch_updated_at()`, the three `modules` rows |
| `002_tenant_tables.up.sql` | the fourteen tenant tables |
| `003_constraints.up.sql` | composite FKs, CHECKs, line numbering, partial unique indexes, the index set |
| `004_views_triggers.up.sql` | both views, `grl_no_over_receipt`, `jel_balanced`, the two terminal-state triggers |
| `005_rls_grants.up.sql` | RLS enable/force/policy, grants, ledger and superadmin revokes, `seed_tenant_accounts()` |

---

## Decisions taken

Record anything where you deviate from the spec, so a later session knows why the
code and the docs disagree.

The sixteen problems found in the original plan are already fixed in the reference
docs — see [`AUDIT.md`](AUDIT.md) for what changed. Nothing there is outstanding.

| Date | Decision | Why |
|---|---|---|
| 2026-07-26 | Raw colour tokens are named `--ch-*`, not `--color-*` as in `reference/design-system.md` §10.8.1 | The doc's `:root` block and its `@theme` block both define `--color-accent` (also `-success`, `-warning`, `-danger`). Tailwind emits `--color-accent: rgb(var(--color-accent))` into the same `:root`, which is a self-referential cycle: the declaration is invalid and every accent/success/warning/danger utility silently stops resolving. Only the raw side was renamed; the utility names (`text-accent`, `bg-surface`) are unchanged, so no later phase is affected. |
| 2026-07-26 | `000_roles.sql` guards its platform-table grants behind `to_regclass('public.users') IS NOT NULL`, and `make migrate` re-runs the file after migrations | The file runs from `docker-entrypoint-initdb.d` on first boot, when `users`/`tenants`/etc. do not exist yet. Ungrarded, the AUDIT A1 grants abort container init. The whole file is idempotent so re-running is safe, and the grants land as soon as Phase 1 has created the tables. |
| 2026-07-26 | Test A10's query uses `rolsuper`, not `rolsuperuser` | `pg_roles` has no `rolsuperuser` column — the query as written in `phases/phase-0-foundations.md` and `reference/deployment.md` errors out. Worth correcting in the acceptance test before Phase 7 relies on it. |
| 2026-07-26 | The RLS policy reads `NULLIF(current_setting('app.current_tenant', true), '')::uuid`, not the bare `current_setting` of `tenancy-and-rls.md` §4.4 | After a `SET LOCAL` transaction commits, the GUC resets to the **empty string**, not to NULL — and `''::uuid` raises *invalid input syntax*. The template as written turns "a request with no tenant context" into an error on any pooled connection that has served a request before, where §4.3 explicitly wants zero rows. `NULLIF` restores the intended safe failure. Behaviour when the context is set is identical. Test A2 asserts both shapes. |
| 2026-07-26 | `000_roles.sql` now also grants `erp_admin` `SELECT, INSERT, UPDATE` on the five platform tables | The file carried only the AUDIT A1 grants for `erp_app`; the matching `erp_admin` line from `tenancy-and-rls.md` §4.2 was never transcribed. Test A11 caught it — `erp_admin` could not read `tenants`, which is the one thing the superadmin console exists to do. Phase 3 would have failed on its first request. |
| 2026-07-26 | `make migrate` is `go run ./cmd/migrate` alone; the binary re-applies `000_roles.sql` itself | The old second line, `docker compose exec -T postgres psql … -f /docker-entrypoint-initdb.d/000_roles.sql`, cannot work on Windows: Git Bash rewrites the container-absolute path to `C:/Program Files/docker-entrypoint-initdb.d/…`. The roles file is already embedded for the test harness, so applying it in Go removes the psql dependency, the mount dependency, and the platform difference at once. |
| 2026-07-26 | `purchase_order_lines` has one `UNIQUE (id, tenant_id)`, named `pol_id_tenant_uq` | `constraints-and-indexes.md` §6.10.1 calls it `purchase_order_lines_id_tenant_uq` and §6.10.9 calls it `pol_id_tenant_uq`; they are the same constraint. Declaring both fails. Kept §6.10.9's name, since that is the block AUDIT B4 rewrote. |
| 2026-07-26 | `config.Load` pins `time.Local = time.UTC` (exported as `config.PinUTC`) | Decision 003 §2.5.2 requires the Go process to run in UTC and relies on `TZ=UTC` in the container. A developer's laptop has no such variable, so the pin has to exist in code for J2 to mean anything anywhere but production. |
| 2026-07-26 | `seed_tenant_accounts()` (§4.2.1) was built in Phase 1, not deferred to Phase 3, and it sets `app.current_tenant` internally | It is DDL, it belongs beside the `accounts` table, and the test fixture uses it — so every tenant fixture exercises it. The `set_config` call is not in the spec's version and is required: `accounts` is `FORCE` RLS, and **`FORCE` applies to the owner**, so even a `SECURITY DEFINER` function owned by `erp_migrate` needs tenant context. The previous value is restored before returning. |
| 2026-07-26 | `index.html` carries a two-line inline `<style>` duplicating the canvas colour | The pre-paint script sets the `.dark` class, but in dev Vite injects the stylesheet via JS *after* first paint, so the class alone still yields a white first frame. Canvas only; every other colour comes from the tokens. |

---

## Log

<!-- newest at the bottom; one block per session -->

<!--
## Phase N — YYYY-MM-DD

**Done:** what now works, in one or two sentences.

**Tests green:** A1-A11, I1-I8, J1-J4

**Deviations from spec:** none — or: what, where, and why.

**TODO(post-mvp) markers added:**
- internal/procurement/receipt.go:142 — audit gr.posted

**Known broken / left half-done:** nothing — or be specific.

**Next:** the single next action.
-->

## Phase 0 — 2026-07-26

**Done:** `make dev` brings up Postgres 17 in Docker, the Fiber API on `:8080`,
and the Vite frontend on `:5173` together. Three database roles exist with the
timezone pinned to each; `db.WithTenant` is in place and tested; the light/dark/
system theme is wired through semantic tokens before the first real component
exists.

**Tests green:**
- `internal/db` — `TestWithTenantSetsCurrentTenant`, `TestWithTenantDoesNotLeakAfterCommit`
- Phase 0 "done when" checklist, all seven items:
  - `make dev` starts all three; `GET /api/health` → `200 {"status":"ok"}`; `GET :5173/` → 200
  - `rolbypassrls`/`rolsuper` false for both `erp_app` and `erp_admin` (I3)
  - `git check-ignore` matches `backend/secrets/`, `backend/.env`, `frontend/.env.local`; no service-account key in `git log`
  - both `.env.example` files committed
  - `backend/go.mod` and `frontend/package.json` exist; neither at the repository root (I13)

**Deviations from spec:** four, all recorded in *Decisions taken* above —
`--ch-*` token naming, the guarded grants in `000_roles.sql`, `rolsuper` vs
`rolsuperuser`, and the inline canvas colour in `index.html`.

**TODO(post-mvp) markers added:** none.

**Known broken / left half-done:**
- `make migrate` and `make seed` reference `backend/cmd/migrate` and
  `backend/cmd/seed`, which arrive in Phase 1. The targets fail until then, by
  design.
- Theme has no white flash on reload: verified structurally (blocking script +
  inline canvas rule in `<head>`, both served), not by eye. Worth one look in a
  browser.
- `npm audit` reports two high-severity advisories in `react-router` 7.18.1,
  both scoped to **RSC mode**, which this SPA does not use. The only offered
  fix is a downgrade to 7.11.0. Left as-is; recheck when a forward fix ships.
- GNU Make was installed on this machine during the phase
  (`winget install ezwinports.make`) — it is not shipped with Git for Windows,
  and a new shell is needed for it to be on `PATH`.

**Next:** open [`phases/phase-1-schema-rls.md`](phases/phase-1-schema-rls.md) in
a new session.

## Phase 1 — 2026-07-26

**Done:** the whole schema exists and enforces itself. Five migrations create the
five platform tables and the fourteen tenant tables, every tenant table has RLS
`ENABLE`d **and** `FORCE`d with a `USING` + `WITH CHECK` policy, both views are
`security_invoker`, every tenant-scoped child reference carries `tenant_id` in a
composite FK, and the four triggers are in place. `cmd/migrate` applies them from
an embedded FS and re-applies `000_roles.sql` afterwards. `testsupport/` starts a
real PostgreSQL 17 per test process with testcontainers, migrates it exactly the
way `make migrate` does, and hands back one connection per role — tests run as
`erp_app`, never as the owner.

**Tests green:** A1–A11, I1–I8, J1–J4 — 26 tests in `internal/db`, all passing
(`go test ./... -count=1`). Plus `TestWithTenantSetsCurrentTenant`,
`TestWithTenantClearsContextOnRollback`, and `TestSeedTenantAccountsCrossesTheRevoke`.

Three of them were verified to actually bite rather than merely pass:

- **A7** — a copy of `stock_balances` without `security_invoker` returns **2**
  rows where the real view returns **1**, as `erp_app` with tenant A's context.
  The leak the option prevents is real, and the test detects it.
- **A11** — caught the missing `erp_admin` platform grants (see *Decisions*).
- **I4** — `erp_app` gets `42501` on `DELETE FROM journal_entry_lines`, the owner
  gets `23514` at commit. Grant and trigger are independently doing their jobs.

**Deviations from spec:** six, all recorded in *Decisions taken* above — the
`NULLIF` in the policy, the `erp_admin` platform grants, the `make migrate`
target, the `pol_id_tenant_uq` name collision, the `time.Local` pin, and building
`seed_tenant_accounts()` now with an added `set_config` call.

Two smaller additions, both authorised by the phase brief's "CHECK constraints on
every status/type column" and neither a departure:

- `document_sequences.doc_type` has a CHECK for `PR|PO|GR|JE`.
- The three `modules` rows are inserted by `001_platform.up.sql`
  (`ON CONFLICT DO NOTHING`). They are the enumeration the naming contract
  fixes, not seed data — every `tenant_modules` row needs a parent.

**TODO(post-mvp) markers added:** none.

**Known broken / left half-done:**
- `go test -race` cannot run on this machine: it requires cgo and there is no C
  toolchain on `PATH`. CI (`ubuntu-latest`) runs it; worth a check there.
- The over-receipt trigger `grl_no_over_receipt` is created and its shape is
  exercised by I7's fixtures, but its *rejection* path is Phase 5's H6. It is
  untested until then, deliberately.
- Composite FK names are explicit (`grl_po_line_product_fk`, …) where the
  reference doc uses anonymous `ADD FOREIGN KEY`. Same constraints, nameable in
  error handling later.

**Next:** open [`phases/phase-2-auth.md`](phases/phase-2-auth.md) in a new session.
