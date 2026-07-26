# Phase 9 — Deployment · *post-MVP*

**MVP:** no · **Estimate:** 4h · **Depends on:** Phase 8

## Load only these

1. **[`../DEPLOY.md`](../DEPLOY.md) — the runbook.** Written for this
   deployment specifically, with the commands and the values filled in. Start
   there; the three below are the "why" behind it.
2. [`../reference/deployment.md`](../reference/deployment.md) — §2.3.2 and
   §2.3.3 are what DEPLOY.md's Neon steps implement.
3. [`../reference/auth.md`](../reference/auth.md) — **§3.5.1 and §3.5.2**. Note
   that §3.5.1's dev/prod split was **not** followed; see *Deviations* below.
4. [`../decisions/002-security-posture.md`](../decisions/002-security-posture.md)
   — before writing anything about the deployment in the README.

## The topology that was chosen

Nothing before this phase depended on the hosting choice — that was deliberate.

| Piece | Where |
|---|---|
| Frontend | Firebase Hosting, project `erp-project-b66ce` |
| Backend | Cloud Run `mini-erp-api`, project `banded-torus-476311-q1`, `asia-southeast1` |
| Database | Neon Postgres 17, AWS `ap-southeast-1`, database `erp` |
| Auth | Firebase Authentication, `erp-project-b66ce` |
| Secrets | Secret Manager, `banded-torus-476311-q1` |

**The two projects are owned by different Google accounts**, which is the one
structural difference from what `reference/deployment.md` assumes. It costs
exactly one thing: the Firebase service-account key has to be stored in the
Cloud Run project's Secret Manager and mounted as a file, because Cloud Run's own
identity belongs to the wrong project to be granted anything in the other one.
DEPLOY.md §5.

## Build

1. Rename the Cloud Run project's display name; note the **project ID is
   immutable** and appears nowhere a visitor sees. DEPLOY.md §1.
2. Neon project on Postgres 17, **same region as Cloud Run**.
3. **Create `erp_app`, `erp_admin` and `erp_migrate` with SQL, never through the
   Console, API, or CLI** — `deploy/neon-bootstrap.sql`. Roles created through
   Neon's tooling get `neon_superuser`, which carries `BYPASSRLS`; every policy
   in this system would silently stop applying with nothing visibly wrong.
4. Migrate, seed and verify from the laptop against the **direct** endpoint.
5. Secret Manager: both **pooled** connection strings and the Firebase service
   account key. Never baked into the image, never a constant in the binary.
6. Cloud Run from source, `--env-vars-file` + `--set-secrets`. The service is
   **not** given `MIGRATE_DATABASE_URL` — `config.Load` no longer requires it.
7. CORS locked to the two Firebase Hosting origins. No wildcards.
8. Frontend build **after** the API URL exists, then Firebase Hosting with the
   `**` → `/index.html` rewrite.
9. The three Firebase console settings, including the password-reset action URL
   that has pointed at Firebase's hosted page since Phase 2.

## Done when

- [ ] The acceptance test passes against the **deployed** URLs, not just locally
- [ ] **Test A10 run against the production database** — confirming the real
      `erp_app` has no `BYPASSRLS`. This is the check that catches a role
      provisioned through a console.
- [ ] **Test J1 run against the production database** — `SHOW timezone` returns UTC

The last two are `cd backend && go run ./cmd/dbverify .env.production`, which
also covers I4 and RLS-forced-on-all-fourteen-tables, and ends in
`all 11 checks passed`. Neither test could be pointed at a deployed database:
both run against a testcontainer by construction.

## Deviations, and where they are recorded

- **No `erp-prod` Firebase project.** The deployment authenticates against the
  dev project so that the seeded demo accounts work on the live URL. §3.5.1 and
  step 7 of the original plan say the opposite. Recorded as a Decision in
  `PROGRESS.md`.
- **I3 now has a precise wording for the schema owner.** `erp_app` and
  `erp_admin`: never elevated, asserted everywhere. `erp_migrate`: unprivileged
  on any host that allows it, reported rather than asserted, because locally it
  *is* the container's superuser. This was the open question Phase 8 wrote down.
