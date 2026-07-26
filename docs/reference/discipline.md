# Reference — Development discipline

> Read once at Phase 0. Re-read before any refactor.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 12A. Development discipline — avoiding rework

This project is specified in enough detail that most refactoring is avoidable. The rework that does happen usually comes from three causes, all preventable.

### 12A.1 Follow the specified names exactly

Table names, column names, status strings, role levels, endpoint paths, and test IDs in this document are **contracts**, not suggestions. Later phases reference them directly, and the acceptance test asserts on them. Renaming `partially_received` to `partial` in Phase 5 means touching migrations, models, handlers, the frontend, and eight tests.

If a name genuinely seems wrong, change it in Phase 1 — before anything depends on it — or not at all.

### 12A.2 Do not abstract before the second use case

The strongest driver of rework in a project this size is premature abstraction: a generic `BaseRepository`, a configurable workflow engine, a shared form builder — all written before there is a second caller to shape them. They then get rewritten once the second case arrives and does not fit.

Write the second implementation concretely, look at both, then extract if a shape is obvious. Sections 8.2–8.4 describe three status transitions that *look* like they should share machinery; approve, reject, and cancel have different participants, different validation, and different side effects. Keeping them separate is the right call.

### 12A.3 Respect the module boundaries mechanically

Section 2.2 defines boundaries that make "modular monolith" meaningful. Enforce them with a linter rather than discipline — `depguard` in `golangci-lint` can fail the build when one module imports another's internals:

```yaml
# .golangci.yml
linters-settings:
  depguard:
    rules:
      procurement-isolation:
        files: ["**/internal/inventory/**", "**/internal/finance/**"]
        deny:
          - pkg: "**/internal/procurement/handler"
            desc: "modules must not import another module's HTTP layer"
```

This is the cheapest possible protection for the architectural claim in Section 16.

### 12A.4 Tooling

**Go backend** — `golangci-lint` with `unused`, `dupl`, `gocyclo`, and `depguard` enabled covers dead code, duplication, complexity, and boundary drift. Add `govulncheck` (Section 12.7). Run before each phase's "done when" check, not just in CI.

**TypeScript frontend** — [Fallow](https://github.com/fallow-rs/fallow) does the equivalent job and more: unused code, duplication, circular dependencies, complexity hotspots, and architecture drift, with 100+ framework plugins auto-detected from `package.json`.

```bash
npx fallow                 # full pass: dead code + duplication + health
npx fallow audit           # gate only what changed — use before each commit
npx fallow dead-code       # cleanup candidates
npx fallow dupes           # clone families, with refactor suggestions
```

> **Fallow analyses TypeScript and JavaScript only.** It will not see your Go backend at all. Use it for `frontend/`, and rely on `golangci-lint` for `backend/`. Do not read a clean Fallow report as a clean codebase.

Because this build uses Claude Code, two integrations are worth the setup cost:

- `npx fallow init --agents` scaffolds an `AGENTS.md` with a task-to-command matrix
- Fallow ships an MCP server (`fallow-mcp`) and a version-matched agent skill, so Claude Code can query the analysis directly instead of you pasting reports

Run `npx fallow audit` at the end of Phases 4, 5, and 7 — the points where the frontend grows fastest. Catching a duplicated form component after two copies is a ten-minute fix; after five it is an afternoon.

### 12A.5 When rework is correct

Do not over-apply the above. Two changes in this document were the result of finding a design wrong mid-review and are worth making even late: removing the stored `qty_received` column (Section 6.10.6) and dropping `BYPASSRLS` (Section 4.2). Both fixed correctness, not aesthetics.

The test to apply: **does the change fix a bug, close a security gap, or remove a guarantee-weakening inconsistency?** Refactor. **Does it make the code prettier?** Note it and move on.
