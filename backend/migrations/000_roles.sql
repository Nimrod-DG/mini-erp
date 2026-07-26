-- 000_roles.sql — the three database roles.
--
-- Runs automatically on the container's FIRST boot, via
-- /docker-entrypoint-initdb.d. It is also re-run by `make migrate` after the
-- schema migrations, because the platform-table grants at the bottom cannot be
-- applied on first boot: the tables do not exist yet. The whole file is
-- therefore written to be idempotent.
--
-- erp_migrate  the schema owner — owns the DDL, runs migrations only
-- erp_app      the application role — RLS applies to it, always
-- erp_admin    the platform-admin role — see reference/tenancy-and-rls.md
--
-- Neither erp_app nor erp_admin may ever hold BYPASSRLS or SUPERUSER (I3).
--
-- ---------------------------------------------------------------------------
-- WHY THIS FILE IS DEFENSIVE (Phase 9)
--
-- It runs in two places that grant very different privileges:
--
--   * locally, as the container superuser, where every statement below is
--     permitted; and
--   * against a managed host, where `make migrate` re-runs it as erp_migrate —
--     which is NOT a superuser there, and PostgreSQL requires superuser to
--     create a role or to touch the SUPERUSER/BYPASSRLS attributes *even to
--     turn them off*. Run as written, the previous version of this file
--     aborted the migration on a managed host with `must be superuser to alter
--     superuser roles`, which reads like a schema problem and is not one.
--
-- So every privileged step is attempted and skipped when refused, and the part
-- that actually matters — I3 — is asserted from pg_roles afterwards, which
-- needs no privilege at all. That is the stronger arrangement anyway: forcing
-- an attribute and asserting it are not the same claim, and only the assertion
-- fails loudly when a role was provisioned by hand through a console.
--
-- `deploy/neon-bootstrap.sql` performs the skipped steps once, as the
-- provider's own owner role. See docs/DEPLOY.md step 3.
-- ---------------------------------------------------------------------------

-- 1. The roles themselves.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'erp_app') THEN
    CREATE ROLE erp_app   LOGIN PASSWORD 'localdev' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'erp_admin') THEN
    CREATE ROLE erp_admin LOGIN PASSWORD 'localdev' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
EXCEPTION WHEN insufficient_privilege THEN
  RAISE NOTICE 'cannot CREATE ROLE here — expected on a managed host, where deploy/neon-bootstrap.sql creates them';
END
$$;

-- 2. Re-assert the attributes. Superuser-only, so attempted and skipped; step 3
-- is what catches a role that acquired either attribute anyway.
DO $$
BEGIN
  ALTER ROLE erp_app   NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
  ALTER ROLE erp_admin NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
EXCEPTION WHEN insufficient_privilege THEN
  RAISE NOTICE 'cannot ALTER ROLE attributes here — asserted instead, below';
END
$$;

-- 3. THE ASSERTION, and the reason the two blocks above are allowed to fail.
--
-- A role that somehow acquired either attribute is a silent, total loss of
-- tenant isolation — nothing looks wrong, every policy simply stops applying.
-- Test A10 asserts the same thing from Go against the test container;
-- `cmd/dbverify` runs it against a deployed database (Phase 9).
DO $$
DECLARE
  missing  text;
  elevated text;
BEGIN
  SELECT string_agg(r, ', ') INTO missing
  FROM unnest(ARRAY['erp_app', 'erp_admin']) AS r
  WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r);

  IF missing IS NOT NULL THEN
    RAISE EXCEPTION 'role(s) % do not exist and could not be created — run deploy/neon-bootstrap.sql against this database first', missing;
  END IF;

  SELECT string_agg(rolname, ', ') INTO elevated
  FROM pg_roles
  WHERE rolname IN ('erp_app', 'erp_admin') AND (rolsuper OR rolbypassrls);

  IF elevated IS NOT NULL THEN
    RAISE EXCEPTION 'I3 violated: % holds SUPERUSER or BYPASSRLS — every RLS policy in this schema is decorative until that is fixed', elevated;
  END IF;
END
$$;

-- 4. Connect and schema usage.
--
-- current_database() rather than the literal `erp`: the local container's
-- database is named erp, and a managed host's may not be. Hard-coding it makes
-- this file fail with `database "erp" does not exist` on a provider whose
-- database is called something else.
DO $$
BEGIN
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO erp_app, erp_admin', current_database());
  GRANT USAGE ON SCHEMA public TO erp_app, erp_admin;
EXCEPTION WHEN insufficient_privilege THEN
  RAISE NOTICE 'cannot grant CONNECT/USAGE here — deploy/neon-bootstrap.sql does it once';
END
$$;

-- 5. Pin timezone to the role so it travels with the connection (Section 2.5.2).
-- This is the half of J1 that survives a managed host whose containers you do
-- not control, so it warns rather than skipping silently.
DO $$
BEGIN
  ALTER ROLE erp_app     SET timezone = 'UTC';
  ALTER ROLE erp_admin   SET timezone = 'UTC';
  ALTER ROLE erp_migrate SET timezone = 'UTC';
EXCEPTION
  WHEN insufficient_privilege THEN
    RAISE WARNING 'cannot pin role timezones here — deploy/neon-bootstrap.sql must have done it; confirm with cmd/dbverify (J1)';
  WHEN undefined_object THEN
    RAISE WARNING 'erp_migrate does not exist — the schema owner is named something else on this host; pin its timezone by hand (J1)';
END
$$;

-- --------------------------------------------------------------------------
-- Platform-table grants (AUDIT A1).
--
-- Without these, Phase 2 fails on its first request with
-- `permission denied for table users`. These five tables carry no RLS (§6.8);
-- scoping is application-side, filtered by the tenant derived from the
-- verified Firebase UID.
--
-- Skipped on first boot — the tables arrive with the Phase 1 migrations — and
-- applied when `make migrate` re-runs this file afterwards. These need only
-- table ownership, which erp_migrate has on every host, so they are not
-- wrapped in a privilege guard: a failure here is real.
-- --------------------------------------------------------------------------
DO $$
BEGIN
  IF to_regclass('public.users') IS NULL THEN
    RAISE NOTICE 'platform tables absent — grants skipped; re-run after migrations (make migrate does this)';
    RETURN;
  END IF;

  GRANT SELECT                 ON tenants, modules, tenant_modules TO erp_app;
  GRANT SELECT, INSERT, UPDATE ON users, user_module_roles         TO erp_app;

  -- The one DELETE erp_app holds anywhere, and the one exception to I5.
  --
  -- Setting a module level to `none` *deletes* the row: `none` is the absence
  -- of a row, and the CHECK on role_level refuses to store it (§5.3). A grant
  -- table row is not master data, a document, or a ledger entry — it is a
  -- present-tense grant, and revoking it has no history to preserve. Every
  -- other table in this schema soft-deletes, cancels, or appends.
  --
  -- Deliberately not extended to `users`: users are deactivated, never
  -- deleted (§6.9.4), and there is no DELETE /tenant/users/:id to serve.
  GRANT DELETE                 ON user_module_roles                TO erp_app;

  -- Superadmins touch platform tables, and only platform tables. The matching
  -- REVOKE on every tenant business table lives in 005_rls_grants.sql, which
  -- is what makes "no access to tenant data" a property rather than a promise.
  GRANT SELECT, INSERT, UPDATE
    ON tenants, modules, tenant_modules, users, user_module_roles TO erp_admin;
END
$$;
