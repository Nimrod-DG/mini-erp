# One repo, two applications. Every target cd's into the right one:
# go commands only work from backend/, npm only from frontend/. That is the
# whole point of this file -- it stops both you and the agent running a command
# in the wrong place for the rest of the build.

.PHONY: dev dev-api dev-web up down test migrate seed fmt help

MIGRATE_URL ?= postgres://erp_migrate:localdev@localhost:5432/erp?sslmode=disable

help:
	@echo "make dev      database + API + frontend together"
	@echo "make up       database only"
	@echo "make down     stop the database (data survives)"
	@echo "make test     go test ./... and the frontend build"
	@echo "make migrate  apply migrations, then re-apply role grants"
	@echo "make seed     load demo data"
	@echo "make fmt      gofmt the backend"

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

test:
	cd backend && go test ./...
	cd frontend && npm run build

# 000_roles.sql is re-applied after the schema migrations because its grants on
# the five platform tables cannot be applied on the container's first boot --
# the tables do not exist yet. The file is idempotent.
migrate:
	cd backend && go run ./cmd/migrate
	docker compose exec -T postgres psql "$(MIGRATE_URL)" -f /docker-entrypoint-initdb.d/000_roles.sql

seed:
	cd backend && go run ./cmd/seed

fmt:
	cd backend && gofmt -w .
