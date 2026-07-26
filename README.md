# mini-erp

A multi-tenant ERP: procurement, inventory, and finance, with tenant isolation
enforced in the database by row-level security rather than by application code.

One repository, two applications. **`go.mod` lives at `backend/go.mod` and
`package.json` at `frontend/package.json` — never at the repository root.** Go
commands run from `backend/`, npm commands from `frontend/`.

## Running it

From a clean clone, in order:

```bash
cp backend/.env.example  backend/.env          # then fill in
cp frontend/.env.example frontend/.env.local   # then fill in

make up          # Postgres 17 in Docker
make migrate     # apply the migrations, then re-apply the role grants
make seed        # two demo workspaces, with documents already posted
make dev         # database + API :8080 + frontend :5173
```

**`make migrate` and `make seed` are separate steps deliberately** — nothing
migrates a database implicitly on boot. `make dev` against a database that has
never been migrated starts an API that then fails every query, which looks like a
code fault and is not one.

Both are idempotent and self-verifying, so re-running them is safe. To get back
to a known-good dataset after experimenting:

```bash
docker compose down -v && make up && make migrate && make seed
```

`make help` lists the rest.

### What you need first

- **Docker**, running. Required even if you never run `make dev`: the test suite
  starts a real Postgres per package, because row-level security cannot be tested
  against a mock.
- **A Firebase project** with email/password sign-in enabled, plus a service
  account key on disk. `backend/.env` points at it through
  `GOOGLE_APPLICATION_CREDENTIALS`, and `frontend/.env.local` carries the matching
  web config. That key is a credential: it is gitignored and belongs nowhere near
  a commit. `make seed` creates the demo users *in that Firebase project*, so
  without it seeding stops at the first account.

### Signing in

`make seed` prints what it created. Every demo account uses the password
`password123` — acceptable only because this is a local database and none of
these people exist.

| Account | Who they are |
|---|---|
| `rina@nusantara.test` | Nusantara workspace admin — implicitly admin in all three modules |
| `budi@nusantara.test` | staff · procurement **approver**, inventory viewer |
| `sari@nusantara.test` | staff · procurement **user** — useful for proving a user cannot approve |
| `dewi@nusantara.test` | staff · finance **admin**, procurement viewer |
| `agus@bahari.test` | Bahari workspace admin — **Bahari has no Finance entitlement** |
| `super@erp.test` | platform superadmin, belongs to no tenant |

Nusantara Retail runs all three modules in `Asia/Jakarta`; Bahari Logistics runs
two, in `Asia/Makassar`. The contrast is the point — signing in as `agus` and
finding no Finance link is the entitlement model made visible without reading any
code.

| | |
|---|---|
| API health | <http://localhost:8080/api/health> |
| Frontend | <http://localhost:5173> |

**The frontend has to be on `:5173`.** `CORS_ORIGINS` in `backend/.env` names
that exact origin, and Vite moves to `:5174` without failing when the port is
already taken — after which every API call is refused by CORS and the app looks
broken for a reason that appears in no log of its own. If sign-in succeeds and
every screen is then empty, check the port before anything else.

### Checking the database, and the tests

```bash
make verify-db   # the invariants that live in the database, not in Go
make test        # go test ./... plus the frontend lint, tests and build
```

`verify-db` asserts what a test suite running against a throwaway container never
can about a real one: that no application role holds `BYPASSRLS` or `SUPERUSER`,
that every role's session timezone is UTC, that both views are
`security_invoker`, and that RLS is enabled *and forced* on all fourteen tenant
tables. It takes an env file, so the same command points at a deployed database:

```bash
cd backend && go run ./cmd/dbverify .env.production
```

## Layout

```
backend/          Go — Fiber, GORM, Postgres
  cmd/api/        the HTTP server
  internal/db/    the two pools, and WithTenant
  migrations/     000_roles.sql runs on the container's first boot
frontend/         React + TypeScript + Vite + Tailwind v4
docs/             build documentation — start at docs/PROGRESS.md
```

## Two rules that are not negotiable

**Every tenant-scoped query goes through `db.WithTenant`.** It opens a
transaction and sets `app.current_tenant`, which is what every RLS policy reads.
A handler that touches tenant data outside that helper is a bug.

**No role has `BYPASSRLS` or `SUPERUSER`.** The application connects as
`erp_app`, never as the schema owner — an owner bypasses RLS policies, so
developing as one means building the whole system without ever exercising
isolation, and finding out at deploy time that it never worked.
