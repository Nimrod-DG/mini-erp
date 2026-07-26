# Phase 9 — Deployment · *post-MVP*

**MVP:** no · **Estimate:** 4h · **Depends on:** Phase 8

## Load only these

1. [`../reference/deployment.md`](../reference/deployment.md) — in full. **Now** is when §2.3.2 and §2.3.3 become relevant.
2. [`../reference/auth.md`](../reference/auth.md) — **§3.5.1 and §3.5.2** (switching to `erp-prod`).
3. [`../decisions/002-security-posture.md`](../decisions/002-security-posture.md) — before writing anything about the deployment in the README.

## Build

Nothing before this phase depended on the hosting choice — that was deliberate.
The requirements are modest: Postgres 15+, the ability to create roles via SQL,
and a region near the Cloud Run service.

Assuming Neon:

1. Neon project on Postgres 16/17, **same region as Cloud Run** — every request
   makes several round trips, so cross-region latency multiplies rather than adds.
2. **Create `erp_app` and `erp_admin` with SQL, never through the Console, API,
   or CLI.** Roles created through Neon's tooling get `neon_superuser`, which
   carries `BYPASSRLS` — every policy in this system would silently stop applying
   with nothing visibly wrong.
3. Secret Manager: both connection strings and the Firebase service account key.
   Never baked into the image, never a constant in the binary.
4. Backend container to Artifact Registry; Cloud Run service with the **pooled**
   `DATABASE_URL` and the **direct** `MIGRATE_DATABASE_URL` — `golang-migrate`
   takes session-level advisory locks that do not survive transaction-mode pooling.
5. CORS locked to the Firebase Hosting origin. No wildcards.
6. Frontend build to Firebase Hosting with SPA rewrites.
7. Switch to the `erp-prod` Firebase project. Do not seed demo accounts into it.

## Done when

- [ ] The acceptance test passes against the **deployed** URLs, not just locally
- [ ] **Test A10 run against the production database** — confirming the real
      `erp_app` has no `BYPASSRLS`. This is the check that catches a role
      provisioned through a console.
- [ ] **Test J1 run against the production database** — `SHOW timezone` returns UTC
