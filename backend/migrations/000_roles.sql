-- 000_roles.sql — the three database roles.
--
-- Runs automatically on the container's FIRST boot, via
-- /docker-entrypoint-initdb.d. It is also re-run by `make migrate` after the
-- schema migrations, because the platform-table grants at the bottom cannot be
-- applied on first boot: the tables do not exist yet. The whole file is
-- therefore written to be idempotent.
--
-- erp_migrate  the container superuser — owns the schema, runs migrations only
-- erp_app      the application role — RLS applies to it, always
-- erp_admin    the platform-admin role — see reference/tenancy-and-rls.md
--
-- Neither erp_app nor erp_admin may ever hold BYPASSRLS or SUPERUSER (I3).

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'erp_app') THEN
    CREATE ROLE erp_app   LOGIN PASSWORD 'localdev' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'erp_admin') THEN
    CREATE ROLE erp_admin LOGIN PASSWORD 'localdev' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
END
$$;

-- Re-asserted on every run: a role that somehow acquired either attribute is a
-- silent, total loss of tenant isolation, not a warning.
ALTER ROLE erp_app   NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
ALTER ROLE erp_admin NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;

GRANT CONNECT ON DATABASE erp TO erp_app, erp_admin;
GRANT USAGE ON SCHEMA public TO erp_app, erp_admin;

-- Pin timezone to the role so it travels with the connection (Section 2.5.2)
ALTER ROLE erp_app     SET timezone = 'UTC';
ALTER ROLE erp_admin   SET timezone = 'UTC';
ALTER ROLE erp_migrate SET timezone = 'UTC';

-- --------------------------------------------------------------------------
-- Platform-table grants (AUDIT A1).
--
-- Without these, Phase 2 fails on its first request with
-- `permission denied for table users`. These five tables carry no RLS (§6.8);
-- scoping is application-side, filtered by the tenant derived from the
-- verified Firebase UID.
--
-- Skipped on first boot — the tables arrive with the Phase 1 migrations — and
-- applied when `make migrate` re-runs this file afterwards.
-- --------------------------------------------------------------------------
DO $$
BEGIN
  IF to_regclass('public.users') IS NULL THEN
    RAISE NOTICE 'platform tables absent — grants skipped; re-run after migrations (make migrate does this)';
    RETURN;
  END IF;

  GRANT SELECT                 ON tenants, modules, tenant_modules TO erp_app;
  GRANT SELECT, INSERT, UPDATE ON users, user_module_roles         TO erp_app;

  -- Superadmins touch platform tables, and only platform tables. The matching
  -- REVOKE on every tenant business table lives in 005_rls_grants.sql, which
  -- is what makes "no access to tenant data" a property rather than a promise.
  GRANT SELECT, INSERT, UPDATE
    ON tenants, modules, tenant_modules, users, user_module_roles TO erp_admin;
END
$$;
