# One repo, two applications. Every target cd's into the right one:
# go commands only work from backend/, npm only from frontend/. That is the
# whole point of this file -- it stops both you and the agent running a command
# in the wrong place for the rest of the build.

.PHONY: dev dev-api dev-web up down test cover migrate seed fmt help \
        verify-db deploy-api deploy-web

help:
	@echo "make dev        database + API + frontend together"
	@echo "make up         database only"
	@echo "make down       stop the database (data survives)"
	@echo "make test       go test ./... plus the frontend lint, tests and build"
	@echo "make cover      coverage for both applications, against the §12.6 targets"
	@echo "make migrate    apply migrations, then re-apply role grants"
	@echo "make seed       load demo data"
	@echo "make verify-db  assert A10, J1 and I4 against the database in backend/.env"
	@echo "make fmt        gofmt the backend"
	@echo ""
	@echo "deployment (Phase 9 — docs/DEPLOY.md):"
	@echo "make deploy-api  build and deploy the backend to Cloud Run"
	@echo "make deploy-web  build and deploy the frontend to Firebase Hosting"
	@echo ""
	@echo "  the three database commands take an env file, so the same target"
	@echo "  points at the deployed database:"
	@echo "    cd backend && go run ./cmd/migrate   .env.production"
	@echo "    cd backend && go run ./cmd/seed      .env.production"
	@echo "    cd backend && go run ./cmd/dbverify  .env.production"

up:
	docker compose up -d --wait

down:
	docker compose down

# Runs all three. The two servers are backgrounded; Ctrl-C stops both.
dev: up
	@echo "api :8080  web :5173  --  Ctrl-C to stop"
	@trap 'kill 0' INT TERM; \
		$(MAKE) dev-api & \
		$(MAKE) dev-web & \
		wait

dev-api:
	cd backend && go run ./cmd/api

dev-web:
	cd frontend && npm run dev

# -p 1 runs one package at a time. Each package that touches the database starts
# its own PostgreSQL container (one per test process), and Phase 2 saw a whole
# package fail at 0.00s because a container never came up under the load of two
# in parallel. Phase 3 makes it three, so serialising is no longer optional. The
# suite takes about ten seconds either way.
test:
	cd backend && go test ./... -p 1
	cd frontend && npm run lint
	cd frontend && npm run test
	cd frontend && npm run build

# Coverage, in the shape §12.6 asks about. `-coverpkg=./...` is the whole
# difference: without it, each package reports only what its *own* tests reached,
# so internal/db shows 42% while the api tests are exercising most of it. With it,
# every test binary reports the whole module -- so the per-binary profiles have to
# be unioned rather than concatenated, which is what cmd/covreport does.
cover:
	cd backend && go test ./... -p 1 -coverpkg=./... -coverprofile=coverage-all.out -covermode=atomic
	cd backend && go run ./cmd/covreport coverage-all.out
	cd frontend && npm run test -- --coverage

# cmd/migrate applies the versioned migrations and then re-applies
# 000_roles.sql, whose grants on the five platform tables cannot land on the
# container's first boot -- the tables do not exist yet. The file is
# idempotent, and doing it in Go rather than `docker compose exec psql` keeps
# the target working on Windows, where the shell rewrites the container path.
migrate:
	cd backend && go run ./cmd/migrate

seed:
	cd backend && go run ./cmd/seed

# The invariants that live in the database rather than in Go: A10 (no
# application role holds BYPASSRLS or SUPERUSER), J1 (every role's session
# timezone is UTC) and I4 (both views are security_invoker). The Go suite
# asserts these against a testcontainer, which says nothing about the database
# a deployed service actually talks to -- and a role provisioned by hand
# through a managed provider's console is exactly what they exist to catch.
verify-db:
	cd backend && go run ./cmd/dbverify

fmt:
	cd backend && gofmt -w .

# --------------------------------------------------------------------------
# Deployment (Phase 9). Both are one-liners with a great deal of first-time
# setup behind them -- read docs/DEPLOY.md before the first run of either.
#
# The order is forced: the API goes first, because VITE_API_BASE_URL is baked
# into the frontend bundle at build time and cannot be edited afterwards.
# --------------------------------------------------------------------------

# Builds from source with Cloud Build, using backend/Dockerfile. Everything
# that varies -- the secrets, the CORS origins, the region -- was set once by
# the `gcloud run deploy` in DEPLOY.md step 6 and is carried forward by the
# service, so a redeploy is only ever a new image.
deploy-api:
	cd backend && gcloud run deploy mini-erp-api --source . --region asia-southeast1

# Reads frontend/.env.production for VITE_API_BASE_URL. Vite loads it on top of
# .env.local, so the dev URL cannot leak into a release.
deploy-web:
	cd frontend && npm run build
	cd frontend && firebase deploy --only hosting
