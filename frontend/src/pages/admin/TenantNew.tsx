import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { PasswordField } from "../../components/PasswordField";
import { useAsync } from "../../hooks/useAsync";
import {
  ApiError,
  createTenant,
  listModuleCatalogue,
  type ModuleCode,
} from "../../lib/api";

/** A handful of zones rather than the full IANA list: this is a demo console for
 *  an Indonesian ERP, and a 400-entry select is worse than five right answers.
 *  The backend validates against the real zone database either way (I7). */
const TIMEZONES = [
  "Asia/Jakarta",
  "Asia/Makassar",
  "Asia/Jayapura",
  "Asia/Singapore",
  "UTC",
];

/** slugify offers a starting point; the field stays editable because the slug
 *  appears in URLs and is the one thing PATCH cannot change later. */
function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/**
 * `/admin/tenants/new` — create a workspace and its first administrator (§10.6).
 *
 * The two halves are one request on purpose: a brand-new workspace has no users,
 * so nobody inside it can create the first one. This is the only place a
 * superadmin writes a row scoped to a tenant (§5.7).
 */
export function TenantNew() {
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [timezone, setTimezone] = useState(TIMEZONES[0]);

  // The catalogue comes from `GET /admin/modules` rather than from a list in
  // this file. The three codes were hardcoded here until Phase 5, which worked
  // and was still wrong twice over: the names and descriptions were a second
  // copy of the `modules` rows that nothing kept in step, and a fourth module
  // added to the catalogue would have been invisible to the one screen whose job
  // is choosing modules.
  //
  // `modules` starts empty and is filled once the catalogue arrives, so the
  // default stays "everything available" without this file knowing what that is.
  const [modules, setModules] = useState<ModuleCode[]>([]);
  const { state: catalogue } = useAsync("module-catalogue", async () => {
    const entries = await listModuleCatalogue();
    setModules(entries.map((entry) => entry.code));
    return entries;
  });

  const [adminName, setAdminName] = useState("");
  const [adminEmail, setAdminEmail] = useState("");
  const [password, setPassword] = useState("");

  const [error, setError] = useState<Error | null>(null);
  const [saving, setSaving] = useState(false);

  const effectiveSlug = slugEdited ? slug : slugify(name);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    setSaving(true);
    try {
      const created = await createTenant({
        name: name.trim(),
        slug: effectiveSlug,
        timezone,
        modules,
        admin: {
          email: adminEmail.trim(),
          fullName: adminName.trim(),
          password,
        },
      });
      navigate(`/admin/tenants/${created.tenant.id}`, { replace: true });
    } catch (caught) {
      setError(caught instanceof Error ? caught : new Error(String(caught)));
    } finally {
      setSaving(false);
    }
  }

  const field = "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";
  const label = "mb-1 block text-sm text-secondary";

  return (
    <AppShell title="New workspace">
      {/* Single column and full-width inputs: this is a form, and below `md`
          §10.7.5 wants exactly this shape anyway. */}
      <form onSubmit={submit} className="max-w-xl space-y-8">
        <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">Workspace</h2>

          <label className="block">
            <span className={label}>Name</span>
            <input
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              className={field}
              placeholder="Nusantara Trading"
            />
          </label>

          <label className="block">
            <span className={label}>
              Slug{" "}
              <span className="text-xs">
                — lowercase, hyphens; appears in URLs and cannot be changed later
              </span>
            </span>
            <input
              required
              pattern="[a-z0-9]+(-[a-z0-9]+)*"
              value={effectiveSlug}
              onChange={(event) => {
                setSlugEdited(true);
                setSlug(event.target.value);
              }}
              className={`${field} tabular`}
              placeholder="nusantara-trading"
            />
          </label>

          <label className="block">
            <span className={label}>
              Business timezone{" "}
              <span className="text-xs">— every business date renders in it</span>
            </span>
            <select
              value={timezone}
              onChange={(event) => setTimezone(event.target.value)}
              className={field}
            >
              {TIMEZONES.map((zone) => (
                <option key={zone} value={zone}>
                  {zone}
                </option>
              ))}
            </select>
          </label>
        </section>

        <section className="space-y-3 rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">Modules</h2>
          <p className="text-sm text-secondary">
            The entitlement ceiling. The workspace administrator allocates levels
            within it and can never grant what is not enabled here.
          </p>

          {catalogue.status === "loading" && (
            <div className="h-24 animate-pulse rounded bg-raised" />
          )}
          {catalogue.status === "error" && (
            <ErrorNotice error={catalogue.error} />
          )}
          {catalogue.status === "ready" &&
            catalogue.data.map((module) => (
              <label
                key={module.code}
                className="flex min-h-11 items-start gap-3 py-1"
              >
                <input
                  type="checkbox"
                  checked={modules.includes(module.code)}
                  onChange={(event) => {
                    setModules((current) =>
                      event.target.checked
                        ? [...current, module.code]
                        : current.filter((code) => code !== module.code),
                    );
                  }}
                  className="mt-1 size-4 accent-accent"
                />
                <span>
                  <span className="block text-sm">{module.name}</span>
                  <span className="block text-xs text-secondary">
                    {module.description}
                  </span>
                </span>
              </label>
            ))}
        </section>

        <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">First administrator</h2>
          <p className="text-sm text-secondary">
            They will hold <span className="tabular">admin</span> in every enabled
            module, and can add the rest of the team themselves.
          </p>

          <label className="block">
            <span className={label}>Full name</span>
            <input
              required
              value={adminName}
              onChange={(event) => setAdminName(event.target.value)}
              className={field}
              placeholder="Rina Wijaya"
            />
          </label>

          <label className="block">
            <span className={label}>Email</span>
            <input
              required
              type="email"
              autoComplete="off"
              value={adminEmail}
              onChange={(event) => setAdminEmail(event.target.value)}
              className={field}
              placeholder="rina@nusantara.example"
            />
          </label>

          <PasswordField
            label="Initial password"
            name="new-admin-password"
            value={password}
            onChange={setPassword}
            autoComplete="new-password"
          />
          <p className="text-xs text-secondary">
            At least 8 characters. They can change it themselves from{" "}
            <span className="whitespace-nowrap">Forgot your password?</span> on the
            sign-in screen — this console cannot read it back.
          </p>
        </section>

        {error && <ErrorNotice error={error} />}

        <div className="flex flex-wrap gap-3">
          <button
            type="submit"
            disabled={saving}
            className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            {saving ? "Creating…" : "Create workspace"}
          </button>
          <button
            type="button"
            onClick={() => navigate("/admin/tenants")}
            className="min-h-11 rounded-md border border-hairline px-4 text-sm"
          >
            Cancel
          </button>
        </div>

        {error instanceof ApiError && error.code === "in_use" && (
          <p className="text-sm text-secondary">
            Both the slug and the email address have to be unique across the whole
            platform — Firebase has one user pool per project.
          </p>
        )}
      </form>
    </AppShell>
  );
}
