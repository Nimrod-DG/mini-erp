# mini-erp

A multi-tenant ERP: procurement, inventory, and finance, with tenant isolation
enforced in the database by row-level security rather than by application code.

One repository, two applications. **`go.mod` lives at `backend/go.mod` and
`package.json` at `frontend/package.json` — never at the repository root.** Go
commands run from `backend/`, npm commands from `frontend/`.

## Running it

```bash
cp backend/.env.example  backend/.env          # then fill in
cp frontend/.env.example frontend/.env.local   # then fill in
make dev                                       # database + API + frontend
```

`make dev` brings up Postgres in Docker, the API on `:8080`, and the frontend on
`:5173`. `make help` lists the rest.

| | |
|---|---|
| API health | <http://localhost:8080/api/health> |
| Frontend | <http://localhost:5173> |

Docker is required even if you never run `make dev`: the test suite runs against
real Postgres, because row-level security cannot be tested against a mock.

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

The remaining invariants are in [`CLAUDE.md`](CLAUDE.md).
