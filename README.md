# mini-erp

A multi-tenant ERP — procurement, inventory and finance — where **tenant
isolation is enforced by the database rather than by application code**. Every
tenant-scoped query runs inside a transaction that has told PostgreSQL which
tenant is asking, and fourteen tables carry a row-level security policy that
returns nothing when it has not been told. A handler that forgets its tenant
filter returns zero rows instead of somebody else's data.

The feature that pays for that architecture is a single transaction spanning all
three modules: posting a goods receipt writes the receipt, the stock ledger rows
and a balanced journal entry — or none of them.

> **Status: deploy-ready, not deployed.** The database is live on Neon and the
> container builds and runs, but the API is not hosted: both a Cloud Run and a
> Render deployment are configured and committed, and each stops at a card
> verification that an Indonesian debit card does not pass. It is a billing wall,
> not an engineering one. **Run it locally** with the instructions below.
>
> 📖 **[Read the case study](https://nimrod-dg.github.io/portofolio/#project-mini-erp)** — screenshots of every screen, the architecture, and what shipped versus what did not.

---

## Contents

- [What it is](#what-it-is)
- [Architecture](#architecture)
- [Tenant isolation — how it actually works](#tenant-isolation--how-it-actually-works)
- [The permission model](#the-permission-model)
- [The cross-module transaction](#the-cross-module-transaction)
- [**Running it locally**](#running-it-locally) ← start here to get it working
- [Troubleshooting](#troubleshooting)
- [Make targets](#make-targets)
- [Tests](#tests)
- [Project layout](#project-layout)
- [Documentation](#documentation)
- [Invariants](#invariants--not-negotiable)
- [Scope — what is built and what is not](#scope--what-is-built-and-what-is-not)

---

## What it is

Three modules in one deployable, serving more than one company from the same
tables.

| Module | What it covers |
|---|---|
| **Procurement** | Requisitions with approval, purchase orders, goods receipts with over-receipt refusal, suppliers |
| **Inventory** | Products, warehouses, an append-only stock ledger, derived stock on hand, manual adjustments with a stated reason |
| **Finance** *(stub)* | Chart of accounts and the journal entries other modules post into it. No invoicing, no payment cycle, no period close |

**Modular monolith, not microservices.** Splitting the frontend from the backend
is deployment topology, not service decomposition. The monolith is the point —
the atomic cross-module write below is exactly what microservices would have
cost.

| | |
|---|---|
| **Backend** | Go 1.25 · Fiber · GORM · PostgreSQL 17 |
| **Frontend** | React 19 · TypeScript · Vite · Tailwind |
| **Identity** | Firebase Authentication (token verification only) |
| **Local infra** | Docker Compose (PostgreSQL) |
| **CI** | GitHub Actions, on every push and pull request |
| **Tests** | 376 backend · 148 frontend |

---

## Architecture

```mermaid
flowchart TB
    subgraph CLIENT["Client — Firebase Hosting"]
        A["React 19 · TypeScript · Vite · Tailwind<br/>route guards are cosmetic by design"]
    end

    subgraph API["API — Go + Fiber · one modular monolith"]
        direction TB
        M1["1 · RequestID"]
        M2["2 · FirebaseAuth<br/>verifies the token for its UID only"]
        M3["3 · ResolveIdentity<br/>uid to user, tenant, roles — from the DB, every request"]
        M4["4 · TenantTx<br/>BEGIN; SET LOCAL app.current_tenant"]
        M5["5 · RequireModule<br/>entitlement, then role level"]
        M6["6 · Handler<br/>procurement · inventory · finance"]
        M1 --> M2 --> M3 --> M4 --> M5 --> M6
    end

    subgraph DB["PostgreSQL 17"]
        D1["14 tables — RLS ENABLED and FORCED<br/>policy compares tenant_id to app.current_tenant"]
        D2["2 views — security_invoker"]
        D3["3 roles — erp_migrate / erp_app / erp_admin<br/>none holds BYPASSRLS or SUPERUSER"]
    end

    CLIENT -->|"HTTPS · Firebase ID token"| API
    M6 --> DB
```

Steps 1–4 are global to `/api/*` (except `/api/health`); steps 5–6 are per route.
Superadmin routes under `/api/admin/*` skip the tenant transaction and instead
assert `tenant_role = 'superadmin'`, using a separate pool.

---

## Tenant isolation — how it actually works

The usual way to build this is `WHERE tenant_id = ?` on every query. It works
until somebody writes the one query that forgets it — and then nothing breaks, no
error is raised, and a screen quietly renders another company's rows. This
project pushes the check underneath the application, so forgetting produces an
empty screen instead of a leak.

The chain, in order:

1. **A request arrives with a Firebase ID token.** It is verified for its **UID
   only** — never for claims.
2. **Identity, tenant and roles are read from the database on every request.**
   Never from a token claim: a claim is a snapshot that keeps asserting what was
   true when it was minted, long after an admin revoked the access.
3. **The handler runs inside a transaction that has issued
   `SET LOCAL app.current_tenant = <uuid>`.** `SET LOCAL`, never plain `SET` — a
   session-scoped setting outlives the transaction and would hand the tenant
   context to whichever request next borrowed that pooled connection.
4. **Fourteen tables have RLS `ENABLED` *and* `FORCED`,** with a policy comparing
   `tenant_id` to that setting. `FORCE` is the part people miss: without it the
   table's owner bypasses the policy entirely, which silently defeats the whole
   mechanism in local development, where you are usually connected as the owner.
5. **Both views are `security_invoker`.** By default a view executes with its
   *owner's* privileges — so a view over protected tables happily returns every
   tenant's rows to everybody.
6. **No application role holds `BYPASSRLS` or `SUPERUSER`,** and
   [`cmd/dbverify`](backend/cmd/dbverify) asserts it against a live database —
   because a role created through a hosting provider's web console silently gets
   that privilege and nothing looks wrong afterwards.

```sql
-- backend/migrations/005_rls_grants.up.sql, applied to all 14 tenant tables
ALTER TABLE %I ENABLE ROW LEVEL SECURITY;
ALTER TABLE %I FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON %I
  USING      (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid);
```

`WITH CHECK` is not optional — without it a tenant can *write* rows tagged with
another tenant's id. The `NULLIF` maps the empty string back to `NULL` so a
request with no tenant context returns zero rows rather than raising
`invalid input syntax for type uuid`.

Full detail: [`docs/reference/tenancy-and-rls.md`](docs/reference/tenancy-and-rls.md).

---

## The permission model

Three independent layers, evaluated in order. **All three must pass.**

| Layer | Question | Set by | Stored in |
|---|---|---|---|
| 1 · **Entitlement** | Is this module licensed to this tenant at all? | Platform superadmin | `tenant_modules` |
| 2 · **Module role** | What level does this user hold in this module? | Tenant admin | `user_module_roles` |
| 3 · **Record rule** | Does this specific record permit this action? | Business logic | Handler code |

**Five ranked levels**, each including everything below it — so checks are
comparisons, not set membership:

| Level | Rank | Grants |
|---|---|---|
| `none` | 0 | No access. Hidden in nav; API returns `403`. |
| `viewer` | 1 | Read-only — all list and detail endpoints. |
| `user` | 2 | Create and edit own draft documents. |
| `approver` | 3 | Approve/reject documents; post goods receipts. |
| `admin` | 4 | Manage that module's master data and configuration. |

**One tenant-level role per user**, which decides whether the matrix applies:

- `admin` — manages users and their module roles, and **implicitly holds `admin`
  in every module the tenant is entitled to**.
- `staff` — no implicit access; every level comes from `user_module_roles`,
  defaulting to `none`.
- `superadmin` — platform operator, belongs to no tenant.

The two failures are deliberately distinguishable to the client:

```jsonc
403 {"error":"module_not_enabled","module":"finance"}
403 {"error":"insufficient_module_role","module":"procurement",
     "required":"approver","actual":"user"}
```

**Layer 3 is where segregation of duties lives.** A user with `approver` still
may not approve a requisition they raised themselves — *including if they are the
tenant admin*. That is a per-record rule, which is exactly why it cannot live in
the role middleware: the middleware knows the caller's level, not who raised the
document in front of them.

**Frontend hiding is cosmetic.** Every control the UI hides is independently
enforced server-side. Full detail:
[`docs/reference/permissions.md`](docs/reference/permissions.md).

---

## The cross-module transaction

Posting one goods receipt writes into all three modules, in a fixed order, in a
single transaction.

```mermaid
flowchart LR
    S["POST /goods-receipts<br/>+ Idempotency-Key"] --> T{{"BEGIN<br/>SET LOCAL app.current_tenant"}}
    T --> R1["1 · goods_receipts<br/>header, under a SAVEPOINT"]
    R1 --> R2["2 · stock_ledger<br/>one row per line"]
    R2 --> R3["3 · journal_entries<br/>balanced Dr and Cr"]
    R3 --> C{{"COMMIT<br/>all three, or none"}}
```

The order is load-bearing — the journal entry is valued from what the ledger rows
recorded — so it has a test of its own, because a refactor that reordered it
would still pass every other test in the suite.

**Retries are safe.** The form generates an idempotency key when the *screen
mounts*, not when the button is pressed, so a resubmit carries the same key and
replays the first result instead of receiving the goods twice. The key is checked
three times — before the work, again after the order lines are locked (a retry
already in flight can commit while this request waits for that lock), and finally
by a unique constraint at the insert. That last one sits under a `SAVEPOINT`,
because in PostgreSQL a failed statement otherwise takes the whole transaction
with it — and the transaction is still needed to read back the receipt the
duplicate created. The savepoint also covers the document-number allocation, so a
rejected retry hands its `GR` number back instead of leaving a permanent gap in
the sequence.

Full detail: [`docs/reference/business-logic.md`](docs/reference/business-logic.md).

---

## Running it locally

Everything below assumes a clean clone. **The whole sequence takes about five
minutes**, most of it waiting for Docker and `npm install`.

> **One repository, two applications.** `go.mod` lives at `backend/go.mod` and
> `package.json` at `frontend/package.json` — **never at the repository root**.
> Go commands run from `backend/`, npm commands from `frontend/`. Every `make`
> target already `cd`s into the right one.

### Prerequisites

| | Why |
|---|---|
| **Docker**, running | PostgreSQL 17 for development, *and* for the tests — the suite starts a real database per package, because row-level security cannot be tested against a mock |
| **Go 1.25+** | The backend. Module root is `backend/` |
| **Node 20+ and npm** | The frontend |
| **`make`** *(optional)* | Convenience only — every target's raw command is listed under [Make targets](#make-targets), so you can run them by hand |
| **A Firebase project** | Email/password sign-in enabled, plus a service account key. See [step 1](#1-configure-environment) |

### 1. Configure environment

```bash
cp backend/.env.example  backend/.env          # then fill in
cp frontend/.env.example frontend/.env.local   # then fill in
```

`backend/.env` needs three database URLs (the defaults below match
`docker-compose.yml`, so they work as-is), your Firebase project id, and a path
to a service account key:

```bash
DATABASE_URL=postgres://erp_app:localdev@localhost:5432/erp?sslmode=disable
ADMIN_DATABASE_URL=postgres://erp_admin:localdev@localhost:5432/erp?sslmode=disable
MIGRATE_DATABASE_URL=postgres://erp_migrate:localdev@localhost:5432/erp?sslmode=disable

FIREBASE_PROJECT_ID=<your-project-id>
GOOGLE_APPLICATION_CREDENTIALS=./secrets/<your-key>.json

PORT=8080
CORS_ORIGINS=http://localhost:5173
```

**Three URLs, three roles, on purpose.** The application connects as `erp_app`
and never as the schema owner — an owner bypasses RLS policies, so developing as
one means building the whole system without ever exercising isolation and finding
out at deploy time that it never worked.

**About the Firebase key.** `GOOGLE_APPLICATION_CREDENTIALS` is a *path*, not the
key itself; the Admin SDK reads the variable automatically. Download a service
account key from your Firebase project's settings into `backend/secrets/` — that
directory is gitignored, and the key belongs nowhere near a commit. Step 3's
seeding creates the demo users **in that Firebase project**, so without a key it
stops at the first account.

`frontend/.env.local` holds public identifiers that ship inside the JS bundle —
that is expected and safe. Only variables prefixed `VITE_` reach the browser.

Every variable is documented in
[`docs/reference/env-setup.md`](docs/reference/env-setup.md).

### 2. Start the database

```bash
make up            # or: docker compose up -d --wait
```

Brings up PostgreSQL 17 on `localhost:5432` and waits for its healthcheck. The
container creates the three roles on first boot from
`backend/migrations/000_roles.sql`.

### 3. Create the schema and load demo data

```bash
make migrate       # or: cd backend && go run ./cmd/migrate
make seed          # or: cd backend && go run ./cmd/seed
```

**These are separate steps deliberately — nothing migrates on boot.** Starting
the API against a database that has never been migrated gives you a server that
starts and then fails every query, which looks like a code fault and is not one.

`migrate` applies the six versioned migrations, then re-applies `000_roles.sql`
(its grants on the platform tables cannot land on the container's first boot,
because those tables do not exist yet). `seed` creates two demo workspaces with
products, suppliers, requisitions, orders and already-posted receipts. Both are
idempotent and safe to re-run.

### 4. Run the backend

```bash
cd backend
go run ./cmd/api          # or, from the repo root: make dev-api
```

Listens on **`http://localhost:8080`**. Check it:

```bash
curl http://localhost:8080/api/health     # → {"status":"ok"}
```

### 5. Run the frontend

In a **second terminal**:

```bash
cd frontend
npm install               # first run only
npm run dev               # or, from the repo root: make dev-web
```

Serves on **`http://localhost:5173`**. Open that in a browser.

> ⚠️ **It must be port 5173.** `CORS_ORIGINS` in `backend/.env` names that exact
> origin, and Vite moves to `:5174` *without failing* when 5173 is already taken
> — after which every API call is refused by CORS and the app looks broken for a
> reason that appears in no log of its own. If sign-in succeeds and every screen
> is then empty, check the port first. To make the failure loud instead:
> `npm run dev -- --strictPort`.

### Both servers at once

```bash
make dev           # database + API + frontend, Ctrl-C stops all three
```

Convenient, but it interleaves both logs into one stream and needs a POSIX shell
(on Windows, Git Bash — not PowerShell or `cmd`). **Two terminals as in steps 4
and 5 is the more reliable option**, and it keeps the two logs readable.

### 6. Sign in

Every demo account uses the password **`password123`** — acceptable only because
this is a local database and none of these people exist.

| Account | Who they are | Good for seeing |
|---|---|---|
| `rina@nusantara.test` | Nusantara **workspace admin** — implicitly admin in all three modules | Everything |
| `budi@nusantara.test` | staff · procurement **approver**, inventory viewer | Approving a requisition; an account menu holding two module levels and no Finance |
| `sari@nusantara.test` | staff · procurement **user** | Raising a requisition — and that a `user` cannot approve |
| `dewi@nusantara.test` | staff · finance **admin**, procurement viewer | The Finance module |
| `agus@bahari.test` | Bahari **workspace admin** — Bahari has **no Finance entitlement** | Entitlement: no Finance link *even for an admin* |
| `super@erp.test` | Platform **superadmin**, belongs to no tenant | `/admin/tenants` and the module entitlement matrix |

Nusantara Retail runs all three modules in `Asia/Jakarta`; Bahari Logistics runs
two, in `Asia/Makassar`. That contrast is the point — signing in as `agus` and
finding no Finance link is the entitlement model made visible without reading any
code. Details: [`docs/reference/seed-data.md`](docs/reference/seed-data.md).

### Resetting to clean data

```bash
docker compose down -v && make up && make migrate && make seed
```

Destroys the volume and rebuilds from scratch. Seeded UUIDs are derived from what
each row *is* (UUIDv5), so document URLs stay stable across a rebuild.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Every screen is empty after a successful sign-in | Frontend is on `:5174`, outside `CORS_ORIGINS` | Free port 5173, or add the new origin to `backend/.env` |
| API starts, then every request 500s | Database never migrated | `make migrate` |
| `make seed` stops at the first user | No/invalid Firebase service account key | Check `GOOGLE_APPLICATION_CREDENTIALS` points at a real file |
| Sign-in fails with `auth/operation-not-allowed` | Email/password sign-in not enabled | Enable it in the Firebase console |
| `docker compose up` hangs on healthcheck | Port 5432 already in use | Stop the other PostgreSQL, or change the published port |
| Tests fail instantly at `0.00s` | Docker not running, or too many containers at once | Start Docker; the suite already runs `-p 1` for this reason |
| `go: cannot find main module` | Ran a Go command from the repo root | `cd backend` first |

`make verify-db` is the fastest way to confirm the database itself is sane — see
below.

---

## Make targets

Every target `cd`s into the correct application, which is the whole point of the
file: it stops you (and an agent) running a command in the wrong directory.

| Target | Runs | Does |
|---|---|---|
| `make help` | — | Lists these |
| `make up` | `docker compose up -d --wait` | PostgreSQL only |
| `make down` | `docker compose down` | Stop it — data survives |
| `make dev` | both servers | Database + API + frontend together |
| `make dev-api` | `cd backend && go run ./cmd/api` | API on `:8080` |
| `make dev-web` | `cd frontend && npm run dev` | Frontend on `:5173` |
| `make migrate` | `cd backend && go run ./cmd/migrate` | Apply migrations, re-apply role grants |
| `make seed` | `cd backend && go run ./cmd/seed` | Load demo data |
| `make verify-db` | `cd backend && go run ./cmd/dbverify` | Assert the database invariants |
| `make test` | `go test ./... -p 1`, then frontend lint + test + build | The full suite |
| `make cover` | coverage for both applications | Against the §12.6 targets |
| `make fmt` | `gofmt -w .` | Format the backend |

The three database commands take an env file, so the same target points at a
deployed database:

```bash
cd backend && go run ./cmd/dbverify .env.production
```

---

## Tests

```bash
make test          # everything
cd backend  && go test ./... -p 1
cd frontend && npm run test
```

**376 backend tests and 148 frontend tests**, all green.

The backend suite runs against **real PostgreSQL in containers**, never a mock or
SQLite. That is not thoroughness for its own sake: RLS is a property of Postgres
policy evaluation, so a suite that stubs the database proves nothing about the
one mechanism this project exists to demonstrate. `-p 1` runs one package at a
time because each database-touching package starts its own container.

Test groups A–J and FE1–FE26 are catalogued in
[`docs/reference/tests.md`](docs/reference/tests.md). FE27–FE32 came later, with
the 2026-07-27 interface passes — pagination, the account menu, the filter row
and the dashboard — and are described in the log entries for those passes in
[`docs/PROGRESS.md`](docs/PROGRESS.md).

### Verifying the database itself

```bash
make verify-db
```

Asserts the invariants that live in the database rather than in Go — the ones a
test suite running against a throwaway container can never confirm about a real
one:

```
ok    A10 erp_app/erp_admin not elevated           erp_admin, erp_app
ok    J1  erp_app session timezone                 UTC
ok    I4 views are security_invoker                stock_balances, po_line_status
ok    RLS enabled+forced on 14 tenant tables       14 tables
ok    no tenant context returns no rows            0 without tenant context, 10 with it
ok    migrations have been applied                 6 applied
...
all 11 checks passed
```

Locally it warns that `erp_migrate` is elevated — expected, it is the container's
superuser. On a deployed database that warning means the owner was not created by
`deploy/neon-bootstrap.sql`.

---

## Project layout

```
mini-erp/
├── docker-compose.yml          # PostgreSQL only
├── Makefile                    # every target cd's into the right application
├── render.yaml                 # Render deployment (configured, not deployed)
├── deploy/                     # Cloud Run script + Neon bootstrap SQL
├── backend/                    # ← the Go module root is HERE
│   ├── go.mod
│   ├── Dockerfile              # multi-stage → distroless
│   ├── secrets/                # gitignored — service account keys
│   ├── migrations/             # 6 versioned + 000_roles.sql
│   ├── cmd/
│   │   ├── api/                # the HTTP server
│   │   ├── migrate/            # schema migrations
│   │   ├── seed/               # demo data (UUIDv5 ids — stable across reseeds)
│   │   └── dbverify/           # the 11 invariant checks
│   └── internal/
│       ├── db/                 # the two pools, and WithTenant
│       ├── auth/               # Firebase verifier (interface + fake for tests)
│       ├── middleware/         # the six-step chain
│       └── api/                # handlers, one file per resource
├── frontend/                   # ← the npm project root is HERE
│   ├── package.json
│   └── src/
│       ├── components/         # AppShell, ResponsiveList, Filters, UserMenu…
│       ├── hooks/              # useAuth, useTheme, useCompact, usePagination
│       └── pages/              # one directory per module
└── docs/                       # build documentation — start at docs/PROGRESS.md
```

---

## Documentation

The build plan was one 2,900-line document, split so that any single phase needs
only three or four files. **[`docs/README.md`](docs/README.md) is the index.**

| Need | File |
|---|---|
| Current state, phase by phase | [`docs/PROGRESS.md`](docs/PROGRESS.md) |
| What is and is not MVP | [`docs/00-scope.md`](docs/00-scope.md) |
| Isolation model in depth | [`docs/reference/tenancy-and-rls.md`](docs/reference/tenancy-and-rls.md) |
| Tables, constraints, triggers | [`docs/reference/schema.md`](docs/reference/schema.md) · [`constraints-and-indexes.md`](docs/reference/constraints-and-indexes.md) |
| Entitlement × role × record rule | [`docs/reference/permissions.md`](docs/reference/permissions.md) |
| Numbering, PR lifecycle, receipts, concurrency | [`docs/reference/business-logic.md`](docs/reference/business-logic.md) |
| Every route and error code | [`docs/reference/api.md`](docs/reference/api.md) |
| Env vars and secrets | [`docs/reference/env-setup.md`](docs/reference/env-setup.md) |
| Test groups A–J, FE1–FE26 | [`docs/reference/tests.md`](docs/reference/tests.md) |
| Deployment runbook | [`docs/DEPLOY.md`](docs/DEPLOY.md) |
| Why a decision was made | [`docs/decisions/`](docs/decisions/) |

The three decision records are the ones worth reading before disagreeing with
something: [modular monolith](docs/decisions/001-modular-monolith.md),
[security posture](docs/decisions/002-security-posture.md),
[time and timezone](docs/decisions/003-time-and-timezone.md).

---

## Invariants — not negotiable

These hold in every phase. Most have a test that fails if they stop holding.

| # | Invariant |
|---|---|
| I1 | Every tenant-scoped query runs inside a transaction that has set `app.current_tenant` via `db.WithTenant`. A handler that touches tenant data outside that helper is a bug. |
| I2 | `SET LOCAL`, never plain `SET`. A session-scoped set leaks tenant context across pooled connections. |
| I3 | No database role has `BYPASSRLS` or `SUPERUSER`. |
| I4 | Both views are created `WITH (security_invoker = true)`. Without it they leak every tenant. |
| I5 | There is no `DELETE` in business logic. Master data soft-deletes, documents cancel, ledgers append. |
| I6 | Stock on hand and received quantity are **derived**. Never a stored counter column. |
| I7 | All timestamps `TIMESTAMPTZ`, stored UTC. Business *dates* use the tenant's timezone. |
| I8 | Money is `NUMERIC(18,2)`, quantities `NUMERIC(18,4)`. Never float. |
| I9 | Authorization is resolved from the database on every request. Never from a token claim. |
| I10 | Triggers state that something is illegal. Services state what happens next. A trigger never `INSERT`s. |
| I11 | Tests are written inside the phase that introduces the behaviour, never as a cleanup pass. |
| I12 | Frontend hiding is cosmetic. Every hidden control is independently enforced server-side. |
| I13 | One repo, two applications. `go.mod` lives at `backend/go.mod`, `package.json` at `frontend/package.json` — never at the repository root. |

Append-only is enforced by `REVOKE UPDATE, DELETE` on the ledger tables rather
than by a trigger — a grant cannot be switched off by
`ALTER TABLE … DISABLE TRIGGER`, and there is nothing to forget on a new code
path.

---

## Scope — what is built and what is not

| Area | Status | Notes |
|---|---|---|
| Tenancy & entitlements | ✅ Built | Workspaces, per-module entitlement, suspension, platform superadmin |
| Identity & permissions | ✅ Built | Firebase for identity only; roles resolved from the DB per request |
| Procurement | ✅ Built | Requisitions → approval → orders → receipts, with over-receipt refusal |
| Inventory | ✅ Built | Append-only ledger, derived stock on hand, manual adjustments |
| Cross-module posting | ✅ Built | One transaction, idempotent on a key |
| Finance | 🟡 Stub | Chart of accounts and journal entries only. No invoicing, payment cycle, period close or reporting |
| Approvals | 🟡 Single level | No delegation, value thresholds or multi-step chains |
| Audit log | ⬜ Not built | Designed and specified; deliberately post-MVP ([`docs/post-mvp/audit-log.md`](docs/post-mvp/audit-log.md)) |
| Reversal documents | ⬜ Not built | Ledger corrections are new entries for now |

Saying so is worth more than hiding it — the limits are where the interesting
conversation about what comes next starts.

---

## Deployment

`docs/DEPLOY.md` is the runbook. Current state:

- **Database — live.** Neon PostgreSQL 17 in Singapore, migrated and seeded, with
  `cmd/dbverify` passing all 11 checks against it.
- **Frontend — deployed** to Firebase Hosting, but built against a placeholder API
  URL, so it stops at the login screen. Not linked anywhere for that reason.
- **Backend — not deployed.** The image builds and runs. Both a Cloud Run
  (`deploy/deploy-api.sh`) and a Render (`render.yaml`) path are configured and
  committed; both require a verified payment method, which is where this stops.

```bash
make deploy-api    # Cloud Run, builds from source
make deploy-web    # Firebase Hosting — run second, VITE_API_BASE_URL is baked in at build time
```

The order is forced: the API goes first, because the frontend bundle embeds the
API URL at build time and cannot be edited afterwards.
