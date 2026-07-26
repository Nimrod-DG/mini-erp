# Reference — Deployment topology and hosting

> §2.3.1 (local Docker) in Phase 0. Everything else is Phase 9 — do not read it before then.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 2.3 Deployment topology

| Component | Platform | Notes |
|---|---|---|
| Frontend SPA | **Firebase Hosting** | Static build output, CDN-served, SPA rewrite to `index.html` |
| Backend API | **Cloud Run** | Container, `PORT=8080`, min instances 0, scales to zero |
| Database | **Neon** (serverless Postgres) | Free tier; see Section 2.3.3. Cloud SQL is a drop-in alternative |
| Auth | **Firebase Authentication** | Identity provider; see Section 3 |
| Secrets | **Secret Manager** | DB password, Firebase service account |

Request flow: browser → Firebase Hosting (static assets) → `fetch()` to the Cloud Run URL → Go API → Neon.

CORS: the API allows exactly the Firebase Hosting origin(s) and `http://localhost:5173` for development. No wildcard origins.

Scale-to-zero on Cloud Run means cold starts. Keep the container small (multi-stage build on `golang:1.22-alpine` → `gcr.io/distroless/static`), and open one database connection lazily rather than eagerly on boot.

#### 2.3.1 Local development first — decide hosting later

**Build the entire MVP against Postgres in Docker.** Phases 0–7 require no cloud account, no credit card, and no hosting decision. Deployment is Phase 9, and by then you will know what you actually need.

This is safe because Section 4.2 removed the `BYPASSRLS` dependency: nothing in this design is provider-specific, so Postgres 17 in a container and Postgres 17 on any managed host behave identically for every feature used here.

Docker is required regardless — the test suite runs against real Postgres via `testcontainers` (Section 12.2), because RLS cannot be tested against a mock.

Authentication is the exception to local-first: use a real Firebase project immediately (Section 3.5). It is faster to provision than a database, and the tests never touch it regardless.

**`docker-compose.yml`:**

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: erp_migrate
      POSTGRES_PASSWORD: localdev
      POSTGRES_DB: erp
      TZ: UTC
      PGTZ: UTC
    ports: ["5432:5432"]
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./backend/migrations/000_roles.sql:/docker-entrypoint-initdb.d/000_roles.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U erp_migrate -d erp"]
      interval: 5s
      retries: 10

volumes:
  pgdata:
```

Postgres is the only service that needs containerising. Authentication uses a real Firebase project from the start (Section 3.5) — one fewer container, and no emulator-versus-production divergence to discover later.

Note the container's superuser is named `erp_migrate`, so the schema owner matches production naming from the first commit.

**`backend/migrations/000_roles.sql`** — runs once on first boot:

```sql
CREATE ROLE erp_app   LOGIN PASSWORD 'localdev' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE erp_admin LOGIN PASSWORD 'localdev' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
GRANT CONNECT ON DATABASE erp TO erp_app, erp_admin;
GRANT USAGE ON SCHEMA public TO erp_app, erp_admin;

-- Pin timezone to the role so it travels with the connection (Section 2.5.2)
ALTER ROLE erp_app     SET timezone = 'UTC';
ALTER ROLE erp_admin   SET timezone = 'UTC';
ALTER ROLE erp_migrate SET timezone = 'UTC';
```

> **Develop as `erp_app`, never as the owner.** The owner bypasses RLS policies unless `FORCE ROW LEVEL SECURITY` is set (Section 4.4), so working as `erp_migrate` would let you build the entire application without ever exercising isolation — and discover at deploy time that it never worked. Only migrations use the owner connection.

**Environment:**

```
DATABASE_URL=postgres://erp_app:localdev@localhost:5432/erp?sslmode=disable
ADMIN_DATABASE_URL=postgres://erp_admin:localdev@localhost:5432/erp?sslmode=disable
MIGRATE_DATABASE_URL=postgres://erp_migrate:localdev@localhost:5432/erp?sslmode=disable

FIREBASE_PROJECT_ID=erp-dev
GOOGLE_APPLICATION_CREDENTIALS=./secrets/erp-dev-service-account.json
```

Three URLs locally, three in production. The only change at Phase 9 is their values — plus `sslmode=require`, and pointing `MIGRATE_DATABASE_URL` at an unpooled endpoint if the host offers pooling.

#### 2.3.2 Choosing a host at Phase 9

Deferred deliberately. When you get there, the requirements are modest: **Postgres 15+** (for `security_invoker`), the ability to **create roles via SQL**, and a region near your Cloud Run service.

| Option | Free tier | Notes |
|---|---|---|
| **Neon** | Yes, ongoing | Recommended default. Serverless, scales to zero. See 2.3.3. |
| Supabase | Yes, ongoing | Fine for the database alone; its auth overlaps with Firebase, so ignore that half |
| Railway | Trial credit | Simplest deploy story; becomes paid quickly |
| Render | 90 days only | **The free Postgres instance expires and is deleted.** Avoid for anything you want to keep |
| Cloud SQL | No | Cleanest integration with Cloud Run, but there is no free tier |

Neon is the default in this document because the free tier persists and the design already avoids the one feature managed hosts restrict.

#### 2.3.3 Neon specifics

Neon is serverless Postgres with a usable free tier, which is why it is the default choice here. Five things about it materially affect this project.

**1. Create `erp_app` with SQL, never through the Neon Console, API, or CLI.**

Roles created through Neon's Console/API/CLI are granted membership in `neon_superuser`, whereas roles created with plain SQL receive only ordinary public-schema privileges. `neon_superuser` carries `BYPASSRLS`.

If `erp_app` ends up with elevated privileges, **every RLS policy in this system silently stops applying** and tenant isolation becomes decorative — with nothing visibly wrong. Create application roles by connecting with the Neon-provided owner role and running `CREATE ROLE … NOBYPASSRLS` yourself.

Guard it with an assertion, not a habit — test A10:

```sql
SELECT rolbypassrls, rolsuperuser
FROM pg_roles WHERE rolname IN ('erp_app','erp_admin');
-- both columns must be false for both roles
```

**2. Choose Postgres 16 or 17 when creating the project.** `security_invoker` views (Section 6.3) require 15+, and there is no reason to start on an older major version.

**3. Use the direct connection string for migrations, the pooled one for the app.**

Neon offers a pooled endpoint (PgBouncer, transaction mode) and a direct one. Transaction-mode pooling is compatible with this design specifically because Section 4.3 mandates `SET LOCAL` inside an explicit transaction — a session-scoped `SET` would leak tenant context between requests through a shared connection. This is the second reason `SET LOCAL` is non-negotiable.

Migrations must use the **direct** connection: `golang-migrate` takes session-level advisory locks, which do not survive transaction-mode pooling.

```
DATABASE_URL=postgres://…-pooler.region.aws.neon.tech/erp?sslmode=require
MIGRATE_DATABASE_URL=postgres://…       .region.aws.neon.tech/erp?sslmode=require
```

**4. Scale-to-zero compounds with Cloud Run's.** Free-tier Neon suspends the compute after a few minutes idle; the next query pays a wake-up cost. Stacked on a Cloud Run cold start, a demo opened after lunch can take several seconds to render its first screen — which reads as "slow app" to someone evaluating your work. Two cheap mitigations: set Cloud Run `min-instances=1` while actively demoing, and hit the app once before showing it to anyone.

**5. Co-locate regions.** Put the Neon project in the same cloud region as the Cloud Run service. Every request in this design makes several round trips (identity resolution, then the transaction), so cross-region latency multiplies rather than adds.

**Branching is a genuine bonus.** Neon can branch a database copy-on-write in seconds. It is useful for trying a destructive migration against production-shaped data. Do **not** use it for CI — tests stay on `testcontainers` (Section 12.2), which is deterministic, offline, and free of shared state.

**If you outgrow the free tier or want managed backups with a longer retention window, Cloud SQL is a drop-in swap** — nothing in this design depends on Neon-specific features, precisely because Section 4.2 avoids `BYPASSRLS`.
