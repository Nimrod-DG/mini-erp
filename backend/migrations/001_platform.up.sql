-- 001_platform — the five platform tables.
--
-- These carry no tenant_id (except users, which carries a nullable one) and
-- NO RLS: they are read during identity resolution, before tenant context
-- exists. Scoping is application-side, always by the tenant_id derived from a
-- verified Firebase UID, never from a client parameter (schema.md §6.2).

-- --------------------------------------------------------------------------
-- updated_at maintenance. Defined here because 001 is the first migration that
-- needs it; attached to every mutable table in 001 and 002 (§6.10.9).
-- --------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TABLE tenants (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL UNIQUE,
  status      TEXT NOT NULL DEFAULT 'active',        -- active | suspended
  timezone    TEXT NOT NULL DEFAULT 'Asia/Jakarta',  -- business dates (§2.5.3)
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE tenants ADD CONSTRAINT tenant_status_valid
  CHECK (status IN ('active','suspended'));

CREATE TABLE modules (
  code         TEXT PRIMARY KEY,                -- procurement | inventory | finance
  name         TEXT NOT NULL,
  description  TEXT NOT NULL,
  is_available BOOLEAN NOT NULL DEFAULT true,
  sort_order   INT NOT NULL DEFAULT 0
);

-- Reference data, not seed data: `modules` is the enumeration the naming
-- contract fixes, and every tenant_modules row needs a parent here.
INSERT INTO modules (code, name, description, sort_order) VALUES
  ('procurement', 'Procurement', 'Requisitions, purchase orders, and goods receipts', 1),
  ('inventory',   'Inventory',   'Warehouses, products, and the stock ledger',        2),
  ('finance',     'Finance',     'Chart of accounts and journal entries',             3)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE tenant_modules (
  tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  module_code  TEXT NOT NULL REFERENCES modules(code),
  enabled      BOOLEAN NOT NULL DEFAULT false,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, module_code)
);

CREATE TABLE users (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID REFERENCES tenants(id) ON DELETE CASCADE,  -- NULL = superadmin
  firebase_uid TEXT NOT NULL UNIQUE,
  email        TEXT NOT NULL UNIQUE,
  full_name    TEXT NOT NULL,
  tenant_role  TEXT NOT NULL DEFAULT 'staff',  -- staff | admin | superadmin
  is_active    BOOLEAN NOT NULL DEFAULT true,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users ADD CONSTRAINT users_superadmin_has_no_tenant
  CHECK ((tenant_role =  'superadmin' AND tenant_id IS NULL)
      OR (tenant_role <> 'superadmin' AND tenant_id IS NOT NULL));

ALTER TABLE users ADD CONSTRAINT users_tenant_role_valid
  CHECK (tenant_role IN ('staff','admin','superadmin'));

-- The "at least one active admin per tenant" rule is a cross-row invariant and
-- is NOT expressible here — it is enforced in the service layer with a
-- SELECT ... FOR UPDATE count, in Phase 3 (§6.2).

CREATE TABLE user_module_roles (
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  module_code  TEXT NOT NULL REFERENCES modules(code),
  role_level   TEXT NOT NULL,   -- viewer | user | approver | admin
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, module_code),
  -- Four values, not five: `none` is the absence of a row (§5.3). Do not
  -- "fix" this constraint by adding it.
  CHECK (role_level IN ('viewer','user','approver','admin'))
);

-- Identity resolution runs on every request.
CREATE INDEX idx_users_tenant ON users (tenant_id) WHERE is_active = true;

CREATE TRIGGER tenants_touch_updated_at
  BEFORE UPDATE ON tenants FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER tenant_modules_touch_updated_at
  BEFORE UPDATE ON tenant_modules FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER users_touch_updated_at
  BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER user_module_roles_touch_updated_at
  BEFORE UPDATE ON user_module_roles FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
