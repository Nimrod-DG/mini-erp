# Phase 0 — Foundations

**MVP:** yes · **Estimate:** 2h · **Depends on:** nothing

## Load only these

1. [`../reference/deployment.md`](../reference/deployment.md) — **§2.3.1 only**. Ignore §2.3.2 and §2.3.3; hosting is Phase 9.
2. [`../reference/auth.md`](../reference/auth.md) — **§3.5 only** (two Firebase projects, config, gitignore).
3. [`../reference/env-setup.md`](../reference/env-setup.md) — where every env var and secret lives. Consolidates §2.3.1 and §3.5.2.
4. [`../reference/project-structure.md`](../reference/project-structure.md) — in full.
5. [`../reference/design-system.md`](../reference/design-system.md) — palette, typography, and the token/theme wiring.
6. [`../reference/discipline.md`](../reference/discipline.md) — read once, now.
7. [`../AUDIT.md`](../AUDIT.md) — skim only. Items A1 and A3 were corrections to this phase's files; the corrected versions are already in the reference docs above.

Do not load the schema, permissions, API, or business logic docs. Nothing in this
phase touches them.

## Build

**Order matters — the `.gitignore` line comes before the file it protects exists.**

1. `.gitignore` with `secrets/` and `*-service-account.json`. **Commit this first.**
2. `docker-compose.yml` — Postgres 17 only, `TZ=UTC`, `PGTZ=UTC`, superuser named
   `erp_migrate`. Use the file in §2.3.1 verbatim.
3. `backend/migrations/000_roles.sql` — the three roles, `NOBYPASSRLS NOSUPERUSER`,
   role-level `SET timezone = 'UTC'`.
   Include the `erp_app` grants on `tenants`, `modules`, `tenant_modules`,
   `users`, `user_module_roles` — without them Phase 2 fails with
   `permission denied for table users` on the first request.
4. Firebase projects for the `erp-dev` and `erp-prod` roles (real IDs will differ —
   IDs are globally unique and immutable; record the actual ones in `PROGRESS.md`).
   Add a **Web app** to the dev project, enable **Email/Password** as the only
   sign-in provider, download the service account key to `backend/secrets/`, and
   write `backend/.env` and `frontend/.env.local` per `env-setup.md`.
   `erp-prod` can wait until Phase 9 if you prefer — but never seed into it.
5. **`backend/`** — `go mod init` **inside this directory**, not at the repo root.
   Fiber server at `backend/cmd/api/main.go`, `GET /api/health` returning 200,
   three DB URLs read from env in `backend/internal/config/`.
6. `backend/internal/db/pool.go` — the app pool and the admin pool, opened lazily.
7. `backend/internal/db/tenant.go` — `WithTenant`, exactly as written in
   `project-structure.md`. It uses `SELECT set_config('app.current_tenant', ?, true)`;
   `SET LOCAL … = ?` is a syntax error, since PostgreSQL takes no bind parameters
   in `SET`.
8. **`frontend/`** — `npm create vite@latest` **inside this directory**. React +
   TypeScript + Tailwind + Router. `package.json` must end up at
   `frontend/package.json`, never at the repository root.
9. **Theming, now — not later.** Semantic tokens in `frontend/src/globals.css`,
   the non-inline `@theme` mapping, the pre-paint script in
   `frontend/index.html`, and the light/dark/system toggle. Retrofitting this after fifty components exist means
   auditing every one of them.
10. Root `Makefile`: `dev`, `test`, `migrate`, `seed`. Each target `cd`s into the
    right application — `go` commands only work from `backend/`, `npm` only from
    `frontend/`. This is what stops you (and the agent) running them in the wrong
    place for the rest of the build.

## Do not build

Schema, models, handlers beyond `/health`, any UI screen beyond a sample page,
any auth wiring. Do not run `golangci-lint` config tuning yet — Phase 1.

## Done when

- [ ] `make dev` brings up database, API, and frontend together
- [ ] `GET /api/health` returns 200
- [ ] Theme toggle switches a sample page light↔dark, and a reload in dark mode
      shows **no white flash**
- [ ] `SELECT rolbypassrls, rolsuperuser FROM pg_roles WHERE rolname IN ('erp_app','erp_admin')`
      returns false in all four cells
- [ ] `git check-ignore backend/secrets/ backend/.env frontend/.env.local` matches
      all three, and the service account key is not in `git log`
- [ ] `backend/.env.example` and `frontend/.env.example` are committed
- [ ] `backend/go.mod` and `frontend/package.json` both exist, and **neither
      `go.mod` nor `package.json` exists at the repository root**

## Then

Append the Phase 0 block to [`../PROGRESS.md`](../PROGRESS.md) and stop. Do not
start Phase 1 in the same session — the context you built here is not what
Phase 1 needs.
