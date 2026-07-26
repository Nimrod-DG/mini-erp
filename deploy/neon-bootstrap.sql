-- deploy/neon-bootstrap.sql — run ONCE, by hand, against a managed Postgres
-- before the first `make migrate` reaches it. Phase 9. See docs/DEPLOY.md §3.
--
-- ---------------------------------------------------------------------------
-- WHY THIS FILE EXISTS AT ALL
--
-- Locally the database container boots as a superuser and 000_roles.sql does
-- everything. A managed host gives you an owner role that is NOT a superuser
-- (Neon's is `neondb_owner`, a member of `neon_superuser`), and PostgreSQL
-- reserves CREATE ROLE and the SUPERUSER/BYPASSRLS attributes for real
-- superusers. So the three roles have to be created once, here, as the
-- provider's owner role — after which 000_roles.sql is re-runnable as
-- erp_migrate on every deploy, and asserts rather than forces (I3).
--
-- THE ONE RULE THAT MATTERS: create these roles with SQL, exactly as written.
-- A role created through the Neon Console, API, or CLI is granted membership in
-- `neon_superuser`, which carries BYPASSRLS. Every RLS policy in this schema
-- would then stop applying with nothing visibly wrong — tenant isolation
-- becomes decorative. `cmd/dbverify` is the check that catches it; do not treat
-- a green run as a substitute for creating the roles correctly in the first
-- place (reference/deployment.md §2.3.3).
-- ---------------------------------------------------------------------------
--
-- BEFORE RUNNING: replace the three placeholders below. Generate them; do not
-- reuse the Neon owner password, and do not reuse `localdev`.
--
--     __ERP_MIGRATE_PASSWORD__
--     __ERP_APP_PASSWORD__
--     __ERP_ADMIN_PASSWORD__
--
-- Keep the filled-in copy out of the repository. `.gitignore` already ignores
-- `deploy/*.filled.sql`.

-- ===========================================================================
-- Run the whole file on ONE connection: the provider's DEFAULT database
-- (Neon: `neondb`), as the provider's owner role (Neon: `neondb_owner`).
--
-- Roles are cluster-wide, so they are visible from the new `erp` database
-- without reconnecting. Everything that belongs to `erp` itself is erp_migrate's
-- to do, and 000_roles.sql does it on the first `migrate` run.
-- ===========================================================================

-- The three roles. NOBYPASSRLS and NOSUPERUSER are stated explicitly even
-- though they are the defaults: this file is also the record of what these
-- roles are allowed to be.
CREATE ROLE erp_migrate LOGIN PASSWORD '__ERP_MIGRATE_PASSWORD__' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE erp_app     LOGIN PASSWORD '__ERP_APP_PASSWORD__'     NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE erp_admin   LOGIN PASSWORD '__ERP_ADMIN_PASSWORD__'   NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;

-- Pin the session timezone to each role, so it travels with the connection
-- rather than depending on a container you do not control (J1, §2.5.2).
ALTER ROLE erp_migrate SET timezone = 'UTC';
ALTER ROLE erp_app     SET timezone = 'UTC';
ALTER ROLE erp_admin   SET timezone = 'UTC';

-- A database named `erp`, owned by erp_migrate. The name matches every .env
-- example, the local compose file, and the test container, so a connection
-- string can be moved between them by changing only host and credentials.
--
-- erp_migrate must OWN it: 005_rls_grants.sql runs ALTER TABLE … FORCE ROW
-- LEVEL SECURITY, which requires table ownership, and in PostgreSQL 15+ the
-- owner of `public` is the database owner.
--
-- PostgreSQL 16+ automatically grants a CREATEROLE role ADMIN on the roles it
-- creates, but NOT SET — and `ALTER DATABASE … OWNER TO` requires the ability
-- to SET ROLE to the incoming owner. Without this line the next statement fails
-- with `must be able to SET ROLE "erp_migrate"`, which reads like a permissions
-- dead end and is one line from being fixed.
GRANT erp_migrate TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;

-- erp_migrate re-runs 000_roles.sql on every deploy, and that file pins the
-- other two roles' timezones. Altering a role requires ADMIN on it, which
-- erp_migrate does not have by default here.
--
-- INHERIT FALSE and SET FALSE are the important half: erp_migrate gets the
-- right to administer these roles WITHOUT gaining their privileges and without
-- being able to SET ROLE into them. Plain membership would make erp_migrate a
-- member of erp_app, which changes which RLS policies apply to it.
GRANT erp_app   TO erp_migrate WITH ADMIN OPTION, INHERIT FALSE, SET FALSE;
GRANT erp_admin TO erp_migrate WITH ADMIN OPTION, INHERIT FALSE, SET FALSE;

-- CREATE DATABASE cannot run inside a transaction or a DO block. If your SQL
-- editor wraps statements in one, run these two lines on their own.
--
-- Nothing is granted on the new database here, and that is not an omission:
-- after the ALTER, the provider's role no longer owns `erp` or its `public`
-- schema, so a GRANT issued by it grants nothing and PostgreSQL says so only in
-- a WARNING. CONNECT and USAGE are erp_migrate's to give, and 000_roles.sql
-- gives them on the very next step — the first `migrate` run.
CREATE DATABASE erp;
ALTER DATABASE erp OWNER TO erp_migrate;

-- ===========================================================================
-- VERIFY — both columns must be false on all three rows. If any is true, the
-- role was created through the console rather than by this file: drop it and
-- start again. `make verify-db` runs this and more against the live database.
-- ===========================================================================
SELECT rolname, rolsuper, rolbypassrls
FROM pg_roles
WHERE rolname IN ('erp_migrate', 'erp_app', 'erp_admin')
ORDER BY rolname;
