# Phase 9.5 — Put the API on Vercel · *post-MVP*

**MVP:** no · **Estimate:** 3–4h, most of it on steps 2 and 6 · **Depends on:** Phase 9

> **Why this phase exists.** Phase 9 finished the deployment work and then stopped
> at a payment wall: Cloud Run and Render both demand a verified card, and an
> Indonesian debit card is refused by Google and by Stripe. Nothing in the code is
> the blocker. **Vercel's Hobby tier does not ask for a card**, which is the only
> reason this phase is worth doing — it is a hosting workaround, not an
> architectural improvement.
>
> Everything here is **additive**. The Cloud Run path (`deploy/deploy-api.sh`) and
> the Render path (`render.yaml`) must still work when this is finished, because
> either may become available later and both are better fits than serverless.

## Load only these

1. This file.
2. [`../reference/env-setup.md`](../reference/env-setup.md) — every variable, and which ones the API actually requires.
3. [`../DEPLOY.md`](../DEPLOY.md) — the Neon half is unchanged and still applies.
4. [`../reference/tenancy-and-rls.md`](../reference/tenancy-and-rls.md) — only if step 6 surprises you.

Do not read the rest of `docs/`. This phase touches four files.

---

## 1. Understand the shape of the problem first

The API is a **long-lived process**. Vercel runs **serverless functions**. Four
consequences, in descending order of how likely they are to cost you an evening:

| # | Issue | Why it bites |
|---|---|---|
| 1 | **Prepared statements vs. PgBouncer** | Neon's pooled endpoint is PgBouncer in *transaction* mode, which does not support server-side prepared statements. GORM + pgx v5 uses the extended protocol by default. Symptom: intermittent `prepared statement "lrupsc_1_0" already exists` under concurrency, and **it never reproduces locally**. |
| 2 | **Connection exhaustion** | `internal/db/pool.go` hardcodes `SetMaxOpenConns(10)` / `SetMaxIdleConns(5)`. Serverless scales *instances* sideways, so ten concurrent cold starts is 100 connections against a free-tier cap far below that. |
| 3 | **Routing** | Every route is under `/api` (`internal/api/router.go:86`), and Vercel also treats `api/` as its functions directory. Getting a catch-all to work *and* preserve the original path is the unknown worth spiking. |
| 4 | **Credentials as a file** | `auth.NewFirebase` relies on `GOOGLE_APPLICATION_CREDENTIALS` being a **path**. There is no file on Vercel. |

Two things are already in your favour, and you should not undo them:

- **Pools are lazy** (`sync.Once`, `internal/db/pool.go`). A normal request opens
  only the `erp_app` pool; the `erp_admin` pool opens only for `/api/admin/*`.
  Cold starts therefore cost one pool, not two.
- **Tenant scope is `SET LOCAL` inside an explicit transaction** (invariant I2).
  Transaction-mode pooling destroys session state, so a plain
  `SET app.current_tenant` would leak between tenants or silently vanish. `SET
  LOCAL` is scoped to the transaction, which is exactly the unit PgBouncer keeps
  intact. **The isolation model survives the pooler unchanged** — verify this in
  step 7 rather than assuming it, and if it holds, it is worth a paragraph in the
  case study.

---

## 2. Spike before you port

**Do not start by editing this codebase.** Three unknowns decide whether the rest
is worth doing, and all three are answerable in about thirty minutes with a
throwaway repository.

Create a scratch repo containing one Go function and a `vercel.json`, deploy it,
and answer:

- [ ] **Does Hobby actually deploy without a card?** Terms change; this whole
      phase is pointless if the answer is no. Check this first.
- [ ] **Does a catch-all reach one function, and does `r.URL.Path` still hold the
      original request path?** Try `GET /api/procurement/orders?page=2` and log
      `r.URL.Path` and `r.URL.RawQuery`. The Fiber app routes on that path, so if
      a rewrite rewrites it, the port needs a path fix-up before
      `adaptor.FiberApp` sees the request.
- [ ] **What is the current max function duration on Hobby, and the cold-start
      cost of a Go binary that dials Postgres?** A first request that opens a pool
      to Singapore has a real budget to fit inside.

Write the three answers into the Phase 9.5 entry in `PROGRESS.md` before going
further. If the first is "no", stop and record that — the phase is closed, not
failed.

---

## 3. The trap that would violate I13

Vercel looks for functions in `api/` **relative to the project's root
directory**, and it needs a `go.mod`. The obvious move — put `api/index.go` and a
`go.mod` at the repository root — **breaks invariant I13** (one repo, two
applications; `go.mod` lives at `backend/go.mod`, never at the repository root).

Instead: set the Vercel project's **Root Directory to `backend`**. The function
then lives at `backend/api/index.go`, `vercel.json` at `backend/vercel.json`, and
`backend/go.mod` is exactly where Vercel expects it. Nothing moves.

Note the resulting shape: `backend/api/` (the Vercel function) sits beside
`backend/internal/api/` (the Fiber handlers). Different things, confusingly
similar names — give the new package a header comment saying so.

---

## 4. What to change

Four files. Everything is additive except one small edit to `firebase.go` and one
to `pool.go`.

### 4.1 `backend/api/index.go` — new

The Vercel entrypoint. Model it on [`../../backend/cmd/api/main.go`](../../backend/cmd/api/main.go),
which is the same wiring for a long-lived process:

```go
// Package handler is the Vercel serverless entrypoint. It is NOT
// internal/api — that package holds the Fiber handlers this one wraps.
package handler

import (
    "net/http"
    "sync"

    "github.com/gofiber/fiber/v2/middleware/adaptor"
    // …
)

var (
    once sync.Once
    h    http.HandlerFunc
    initErr error
)

func Handler(w http.ResponseWriter, r *http.Request) {
    once.Do(build)          // built once per warm instance, reused after
    if initErr != nil { http.Error(w, "…", 500); return }
    h(w, r)
}
```

`build()` does what `main()` does — `config.Load`, the Firebase client, `NewPools`,
`api.New(...)` — and ends with `h = adaptor.FiberApp(app)`. **Confirmed present in
the pinned version:** `adaptor.FiberApp(*fiber.App) http.HandlerFunc` exists in
`gofiber/fiber/v2@v2.52.14`.

`sync.Once` is the whole performance story: a warm invocation must not rebuild
the app or reopen the pool.

`config.Load()` calls `godotenv.Load()` and ignores a missing file, so it is
already safe with no `.env` present. It requires `DATABASE_URL`,
`ADMIN_DATABASE_URL` and `FIREBASE_PROJECT_ID`; `MIGRATE_DATABASE_URL` is
deliberately **not** required and must not be set on the deployed service.

### 4.2 `backend/vercel.json` — new

The catch-all worked out in step 2, plus the Go runtime. Keep it minimal.

### 4.3 `backend/internal/auth/firebase.go` — edit

`NewFirebase` currently lets the Admin SDK find `GOOGLE_APPLICATION_CREDENTIALS`
by itself. Add a path for the key arriving as **JSON in an environment
variable**, without breaking the file-path behaviour every other environment
uses:

- If `FIREBASE_CREDENTIALS_JSON` is set and non-empty, pass
  `option.WithCredentialsJSON([]byte(v))` to `firebase.NewApp`.
- Otherwise behave exactly as today.

Keep the existing doc comment's explanation and extend it. Local development,
CI and Cloud Run must all be unaffected — that is the test.

### 4.4 `backend/internal/db/pool.go` — edit

`open()` hardcodes `SetMaxOpenConns(10)` / `SetMaxIdleConns(5)`. Make both
readable from the environment with the current values as defaults, e.g.
`DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS`. On Vercel set them to `2` and `1`.

Do not change the defaults — a long-lived container still wants 10.

---

## 5. Environment on Vercel

Set these in the project settings. **Use Neon's pooled (`-pooler`) host** for
both URLs.

| Variable | Value |
|---|---|
| `DATABASE_URL` | Neon **pooled**, `erp_app`, `sslmode=require`, plus the pgx setting from step 6 |
| `ADMIN_DATABASE_URL` | Neon **pooled**, `erp_admin`, same |
| `FIREBASE_PROJECT_ID` | `erp-project-b66ce` |
| `FIREBASE_CREDENTIALS_JSON` | the whole service-account JSON, pasted |
| `CORS_ORIGINS` | `https://erp-project-b66ce.web.app` — and nothing else in production |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | `2` / `1` |
| `TZ` | `UTC` |

`MIGRATE_DATABASE_URL` is **not** set here, deliberately: the deployed service
must not carry a credential that can drop its own tables. Migrations keep running
from a laptop — `cd backend && go run ./cmd/migrate .env.production`, unchanged.

---

## 6. The pgx / PgBouncer fix

This is the one that fails in production and not locally, so do it deliberately
rather than reactively.

Neon's pooled endpoint cannot do server-side prepared statements. GORM's postgres
driver sits on pgx v5, which uses the extended protocol by default. Pick one:

- Append `default_query_exec_mode=simple_protocol` to the pooled DSNs, or
- Configure the pgx pool explicitly and disable statement caching, or
- Set GORM's `PrepareStmt: false` **and** confirm pgx is not still preparing
  underneath — it usually is, so this alone is typically not enough.

Then **prove it**: drive at least twenty concurrent authenticated requests
against the deployed URL and confirm no `prepared statement … already exists`.
One request proves nothing here.

---

## 7. Verify

In this order. Stop at the first failure.

- [ ] `make test` — 376 backend and 102 frontend still green. The port must not
      change behaviour.
- [ ] `curl https://<app>.vercel.app/api/health` → `{"status":"ok"}`
- [ ] Sign in from the deployed frontend and load a list. This is the real CORS
      and cold-start test.
- [ ] `cd backend && go run ./cmd/dbverify .env.production` → all 11 checks.
- [ ] **Isolation still holds through the pooler.** Sign in as `rina@nusantara.test`
      and `agus@bahari.test` in two browsers and confirm neither sees the other's
      data, with requests interleaved. This is the `SET LOCAL` claim from step 1 —
      if it is going to break, transaction pooling is where.
- [ ] Concurrency: twenty-plus parallel requests, no prepared-statement errors and
      no connection-limit errors from Neon.
- [ ] Post a goods receipt end to end. It is the cross-module transaction and the
      one write worth confirming against a pooled connection.

---

## 8. Afterwards

The API URL changes, and three things depend on it:

1. **Frontend** — `VITE_API_BASE_URL` is baked into the bundle at build time and
   cannot be edited afterwards. Update `frontend/.env.production`, then
   `make deploy-web`. Until this is done the deployed frontend still points at a
   placeholder and still stops at the login screen.
2. **`CORS_ORIGINS`** on Vercel must name the Firebase Hosting origin exactly.
3. **The portfolio case study** — `D:\Work\lw-sports-portfolio\src\page.html`.
   The `#erp-status` section says "deploy-ready, not deployed" throughout, and the
   `.live` nav slot for `data-nav-for="mini-erp"` points at the repository. If
   this phase succeeds, **that wording is now wrong** and the slot should become a
   real live link. Rebuild with `node build.mjs`, push to `main`, and GitHub Pages
   republishes in about a minute.

Also update `docs/DEPLOY.md` with a Vercel section, and append the Phase 9.5
entry to `PROGRESS.md`.

---

## Do not

- **Do not put a `go.mod` at the repository root.** See step 3 — set the Vercel
  root directory to `backend` instead. I13 is not negotiable.
- **Do not delete or break `render.yaml` or `deploy/deploy-api.sh`.** Serverless
  is the compromise here; a container is the better fit and may become available.
- **Do not point the deployed service at Neon's direct (unpooled) endpoint.**
  It will work for one user and fall over on the third.
- **Do not set `MIGRATE_DATABASE_URL` on Vercel.**
- **Do not commit the service-account JSON.** It goes in the Vercel dashboard,
  nowhere else.
- **Do not weaken `SET LOCAL` to plain `SET`** if something looks like it is not
  persisting. That instinct is exactly backwards and would defeat tenant
  isolation across pooled connections.

---

## Done when

- [ ] `https://<app>.vercel.app/api/health` returns `{"status":"ok"}`
- [ ] The deployed frontend signs in and loads real data from it
- [ ] Two tenants, interleaved, cannot see each other's rows
- [ ] Twenty concurrent requests produce no prepared-statement or connection errors
- [ ] `make test` still green; Cloud Run and Render configs untouched
- [ ] The portfolio no longer says "not deployed", and links the live API
- [ ] `PROGRESS.md` and `DEPLOY.md` record what was done

If step 2 says Hobby now wants a card, write that in `PROGRESS.md` and stop.
That is a complete and useful outcome, not a failure.
