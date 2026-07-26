# Phase 8 — Frontend tests, CI, polish · *post-MVP*

**MVP:** no · **Estimate:** 4h · **Depends on:** the acceptance test passing locally

Do not start this phase until the MVP gate in Phase 7 is genuinely crossed.

## Load only these

1. [`../reference/tests.md`](../reference/tests.md) — **§12.5 and §12.7 only**.
2. [`../reference/responsive.md`](../reference/responsive.md) — for FE7–FE9.
3. [`../reference/design-system.md`](../reference/design-system.md) — for FE10–FE13, FE26.

## Build

- **FE1–FE26** with Vitest, React Testing Library, and MSW mocking `/api/*`.
  All twenty-six, including FE22–FE26, which the original document defined but
  never assigned to a phase (AUDIT C3).
- Empty states: **first-run** ("no requisitions yet" + the create action) and
  **no results** ("nothing matches these filters" + clear filters) are different
  copy and different actions. A blank panel for either reads as broken.
- Loading skeletons matching real row height, so nothing lurches on arrival.
- Error toasts that say what happened and what to do — `409 in_use` renders as
  "This supplier has 3 open purchase orders. Close or cancel them before
  deleting", not "Operation failed".
- GitHub Actions: `go vet` → `golangci-lint` → `govulncheck` → `go test -race`
  → frontend lint and tests → build both containers. `TZ=UTC` in the workflow env.
  **Do not deploy from CI** — manual deploy, no portfolio value in automating it.

## Done when

- [ ] FE1–FE26 green
- [ ] CI green on a pull request
- [ ] Coverage meets §12.6 targets, with `internal/procurement` and `internal/db` at 90%+
