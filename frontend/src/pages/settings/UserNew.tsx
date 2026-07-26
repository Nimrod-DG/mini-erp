import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { PasswordField } from "../../components/PasswordField";
import { useMe } from "../../hooks/useAuth";
import {
  createTenantUser,
  type ModuleCode,
  type RoleLevelOrNone,
} from "../../lib/api";

const LEVELS: RoleLevelOrNone[] = ["none", "viewer", "user", "approver", "admin"];

const MODULE_LABELS: Record<ModuleCode, string> = {
  procurement: "Procurement",
  inventory: "Inventory",
  finance: "Finance",
};

/**
 * `/settings/users/new` — add a colleague.
 *
 * Backend-first provisioning (§3.3): one request creates the identity-provider
 * account, the `users` row, and the initial module roles together, and deletes
 * the provider account again if the database refuses the row. Nothing here has
 * to orchestrate that — the point of doing it server-side is that a browser tab
 * closing halfway cannot leave an account nobody can use.
 */
export function UserNew() {
  const me = useMe();
  const navigate = useNavigate();

  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [tenantRole, setTenantRole] = useState<"staff" | "admin">("staff");
  const [roles, setRoles] = useState<Partial<Record<ModuleCode, RoleLevelOrNone>>>(
    {},
  );

  const [error, setError] = useState<Error | null>(null);
  const [saving, setSaving] = useState(false);

  // Only entitled modules can be allocated — a tenant admin can never grant what
  // the workspace has not been given (§5.7). The server refuses it too.
  const entitled = Object.keys(me.moduleRoles) as ModuleCode[];

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    setSaving(true);
    try {
      const created = await createTenantUser({
        email: email.trim(),
        fullName: fullName.trim(),
        password,
        tenantRole,
        moduleRoles: roles,
      });
      navigate(`/settings/users/${created.id}`, { replace: true });
    } catch (caught) {
      setError(caught instanceof Error ? caught : new Error(String(caught)));
    } finally {
      setSaving(false);
    }
  }

  const field = "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";
  const label = "mb-1 block text-sm text-secondary";

  return (
    <AppShell title="Add user">
      <form onSubmit={submit} className="max-w-xl space-y-8">
        <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
          <label className="block">
            <span className={label}>Full name</span>
            <input
              required
              value={fullName}
              onChange={(event) => setFullName(event.target.value)}
              className={field}
              placeholder="Budi Santoso"
            />
          </label>

          <label className="block">
            <span className={label}>Email</span>
            <input
              required
              type="email"
              autoComplete="off"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              className={field}
            />
          </label>

          <PasswordField
            label="Initial password"
            name="new-user-password"
            value={password}
            onChange={setPassword}
            autoComplete="new-password"
          />
          <p className="text-xs text-secondary">
            At least 8 characters. They can change it themselves from{" "}
            <span className="whitespace-nowrap">Forgot your password?</span> on the
            sign-in screen.
          </p>

          <label className="block">
            <span className={label}>Workspace role</span>
            <select
              value={tenantRole}
              onChange={(event) =>
                setTenantRole(event.target.value as "staff" | "admin")
              }
              className={field}
            >
              <option value="staff">Staff — levels set per module below</option>
              <option value="admin">
                Administrator — admin in every enabled module
              </option>
            </select>
          </label>
        </section>

        <section className="rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">Module roles</h2>
          <p className="mt-1 text-sm text-secondary">
            {tenantRole === "admin"
              ? "An administrator holds admin everywhere implicitly. Anything set here is stored and takes effect only if they are later demoted to staff."
              : "Leave a module at None and it stays hidden for them."}
          </p>

          <ul className="mt-4 divide-y divide-hairline">
            {entitled.map((code) => (
              <li
                key={code}
                className="flex items-center justify-between gap-3 py-3"
              >
                <span className="text-sm">{MODULE_LABELS[code]}</span>
                <select
                  aria-label={`${MODULE_LABELS[code]} role level`}
                  value={roles[code] ?? "none"}
                  onChange={(event) =>
                    setRoles((previous) => ({
                      ...previous,
                      [code]: event.target.value as RoleLevelOrNone,
                    }))
                  }
                  className="min-h-11 w-40 shrink-0 rounded-md border border-hairline bg-surface px-3 text-sm"
                >
                  {LEVELS.map((level) => (
                    <option key={level} value={level}>
                      {level}
                    </option>
                  ))}
                </select>
              </li>
            ))}
          </ul>

          {entitled.length === 0 && (
            <p className="mt-4 text-sm text-secondary">
              This workspace has no modules enabled, so there are no levels to
              allocate yet.
            </p>
          )}
        </section>

        {error && <ErrorNotice error={error} />}

        <div className="flex flex-wrap gap-3">
          <button
            type="submit"
            disabled={saving}
            className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            {saving ? "Adding…" : "Add user"}
          </button>
          <button
            type="button"
            onClick={() => navigate("/settings/users")}
            className="min-h-11 rounded-md border border-hairline px-4 text-sm"
          >
            Cancel
          </button>
        </div>
      </form>
    </AppShell>
  );
}
