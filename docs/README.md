# Documentation map

The build plan was originally one 2,900-line document. It is split here so that
an agent loads only what the current phase needs. Content is unchanged from the
original except for the sixteen corrections listed in [`AUDIT.md`](AUDIT.md).

## Start here

| File | When |
|---|---|
| [`PROGRESS.md`](PROGRESS.md) | Every session, first thing. Current state. |
| [`DEPLOY.md`](DEPLOY.md) | **The Phase 9 runbook.** Neon + Cloud Run + Firebase Hosting, across two Google accounts, step by step. Read it instead of deriving the commands from the reference docs. |
| [`AUDIT.md`](AUDIT.md) | Optional. Record of the 16 problems found in the original plan and how each was fixed. Nothing outstanding. |
| [`00-scope.md`](00-scope.md) | Once, at Phase 0. What is and is not MVP. |
| [`phases/`](phases/) | One file per phase. The build instructions. |

## Phases

| Phase | File | MVP | Est. |
|---|---|---|---|
| 0 | [Foundations](phases/phase-0-foundations.md) | ✅ | 2h |
| 1 | [Schema, RLS, invariants](phases/phase-1-schema-rls.md) | ✅ | 5h |
| 2 | [Auth and identity](phases/phase-2-auth.md) | ✅ | 3h |
| 3 | [Permissions](phases/phase-3-permissions.md) | ✅ | 3h |
| 4 | [Inventory core](phases/phase-4-inventory.md) | ✅ | 4h |
| 5 | [Procurement](phases/phase-5-procurement.md) | ✅ | 8h |
| 6 | [Finance stub](phases/phase-6-finance.md) | ✅ | 2h |
| 7 | [Dashboard, seed, responsive](phases/phase-7-dashboard-seed.md) | ✅ | 6h |
| 7.5 | **[The acceptance walk](phases/phase-7.5-acceptance-walk.md)** — the MVP gate. How to walk [`acceptance-test.md`](acceptance-test.md) in a browser, and what is already verified without one | ✅ | 1.5h |
| 8 | [Frontend tests, CI, polish](phases/phase-8-frontend-tests.md) | ➖ | 4h |
| 9 | [Deployment](phases/phase-9-deployment.md) | ➖ | 4h |
| 10 | [Documentation](phases/phase-10-documentation.md) | ➖ | 2h |
| 11 | [Audit log](phases/phase-11-audit-log.md) | ➖ | 3h |

## Reference — look up, do not read end to end

| File | Owns |
|---|---|
| [`reference/tenancy-and-rls.md`](reference/tenancy-and-rls.md) | The three roles, `SET LOCAL`, the policy template |
| [`reference/schema.md`](reference/schema.md) | Table definitions, which tables get RLS |
| [`reference/constraints-and-indexes.md`](reference/constraints-and-indexes.md) | Composite FKs, CHECKs, the four triggers, index set |
| [`reference/deletion-policy.md`](reference/deletion-policy.md) | Soft delete vs cancel vs append-only |
| [`reference/auth.md`](reference/auth.md) | Firebase split, provisioning, local setup |
| [`reference/env-setup.md`](reference/env-setup.md) | Every env var and secret, and where it lives |
| [`reference/permissions.md`](reference/permissions.md) | Entitlement × module role × record rule |
| [`reference/middleware.md`](reference/middleware.md) | The six-step chain and its order |
| [`reference/business-logic.md`](reference/business-logic.md) | Numbering, PR lifecycle, goods receipt, concurrency |
| [`reference/api.md`](reference/api.md) | Every route, min level, error format |
| [`reference/design-system.md`](reference/design-system.md) | Palette, type, tokens, dark mode |
| [`reference/screens.md`](reference/screens.md) | Route → screen inventory |
| [`reference/responsive.md`](reference/responsive.md) | Breakpoints, mobile table strategies |
| [`reference/project-structure.md`](reference/project-structure.md) | Directory layout, `WithTenant` |
| [`reference/tests.md`](reference/tests.md) | Groups A–J and FE1–FE26 |
| [`reference/seed-data.md`](reference/seed-data.md) | Tenants, users, volumes |
| [`reference/deployment.md`](reference/deployment.md) | Local Docker (Phase 0), hosting (Phase 9) |
| [`reference/discipline.md`](reference/discipline.md) | Avoiding rework, linting, boundaries |

## Decisions — the "why". Read when tempted to deviate.

- [001 — Modular monolith](decisions/001-modular-monolith.md)
- [002 — Security posture](decisions/002-security-posture.md)
- [003 — Time and timezone](decisions/003-time-and-timezone.md)

## Post-MVP

- [`post-mvp/audit-log.md`](post-mvp/audit-log.md) — schema is designed, do not build until Phase 11
- [`narrative.md`](narrative.md) — portfolio write-up and future work
