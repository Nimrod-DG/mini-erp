# Reference — Project structure

> Phase 0. The layout is a contract; do not reorganise it later.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## 11. Project structure

**One git repository, two applications.** `backend/` and `frontend/` are sibling
directories inside a single repo — not two repos, not a workspace tool, no shared
`node_modules`. They are built and deployed separately (Cloud Run and Firebase
Hosting), which is deployment topology, not service decomposition (Section 2.1).

Two consequences worth fixing in your head before Phase 0:

- **The Go module root is `backend/`, not the repository root.** `go.mod` lives at
  `backend/go.mod`, so every Go command runs from inside `backend/` —
  `cd backend && go test ./...`. Import paths are
  `github.com/<you>/mini-erp/backend/internal/...`.
- **The npm project root is `frontend/`.** `package.json`, `vite.config.ts`,
  `index.html`, and `tsconfig.json` all live there. Never run `npm init` at the
  repository root.

Paths written elsewhere in these docs as `internal/db/tenant.go` or `src/App.tsx`
are relative to their own application root. Paths in this tree are absolute from
the repository root.

```
mini-erp/
├── docker-compose.yml            # postgres only
├── .gitignore                    # secrets/, *-service-account.json — commit this FIRST
├── Makefile                      # dev, test, migrate, seed, deploy — targets cd into the right dir
├── README.md
├── backend/
│   ├── go.mod                    # ← the Go module root is HERE, not at the repo root
│   ├── go.sum
│   ├── Dockerfile                # multi-stage → distroless
│   ├── secrets/                  # gitignored — service account keys
│   ├── cmd/
│   │   ├── api/main.go
│   │   └── seed/main.go
│   ├── internal/
│   │   ├── config/
│   │   ├── db/
│   │   │   ├── pool.go           # app pool + admin pool
│   │   │   └── tenant.go         # WithTenant helper
│   │   ├── auth/
│   │   │   ├── verifier.go       # interface — real + fake impls
│   │   │   └── firebase.go       # Admin SDK implementation
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   ├── identity.go
│   │   │   ├── tenanttx.go
│   │   │   └── module.go         # RequireModule(module, minLevel)
│   │   ├── models/
│   │   ├── procurement/          # handler.go, service.go, receipt.go, *_test.go
│   │   ├── inventory/
│   │   ├── finance/
│   │   ├── admin/
│   │   ├── tenantusers/
│   │   └── shared/
│   │       ├── docnumber/
│   │       ├── httperr/
│   │       └── audit/
│   ├── migrations/               # 001_platform.up.sql, ...
│   └── testsupport/              # testcontainers harness, fixtures
└── frontend/
    ├── package.json              # ← the npm project root is HERE
    ├── vite.config.ts
    ├── tsconfig.json
    ├── index.html                # the pre-paint theme script goes here (Section 10.8.3)
    ├── .env.local                # VITE_FIREBASE_* — public identifiers, not secrets
    ├── Dockerfile                # build only; output → Firebase Hosting
    ├── firebase.json
    └── src/
        ├── globals.css           # semantic colour tokens + @theme (Section 10.8.1)
        ├── api/
        ├── components/
        ├── modules/{procurement,inventory,finance,admin,settings}/
        ├── hooks/
        ├── lib/firebase.ts
        └── App.tsx
```

The root `.gitignore` covers both applications. `secrets/` with no leading slash
matches at any depth, so it catches `backend/secrets/` without a second file.

**Key file:** `internal/db/tenant.go` exposes the one helper every tenant-scoped handler uses:

```go
func WithTenant(ctx context.Context, db *gorm.DB, tenantID uuid.UUID,
                fn func(tx *gorm.DB) error) error {
    return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // set_config(..., true) is transaction-local -- identical in effect to
        // SET LOCAL, but it is a function call, so it accepts a bind parameter.
        // `SET LOCAL app.current_tenant = ?` is a syntax error under prepared
        // statements: PostgreSQL does not allow parameters in SET.
        if err := tx.Exec(
            "SELECT set_config('app.current_tenant', ?, true)",
            tenantID.String()).Error; err != nil {
            return err
        }
        return fn(tx)
    })
}
```

If a handler touches tenant data without going through this helper, it is a bug.
