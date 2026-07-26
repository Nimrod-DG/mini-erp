import { ThemeToggle } from "../components/ThemeToggle";
import { useAuth, useMe } from "../hooks/useAuth";
import type { ModuleCode, RoleLevel } from "../lib/api";

/** Written out rather than built from a template string: Tailwind scans source
 *  text, so a class it cannot see literally is a class it does not generate. */
const MODULES: { code: ModuleCode; label: string }[] = [
  { code: "procurement", label: "Procurement" },
  { code: "inventory", label: "Inventory" },
  { code: "finance", label: "Finance" },
];

/**
 * Phase 2's screen. It exists to show that the round trip works end to end:
 * Firebase issued a token, the backend verified it, resolved the user against
 * the database, and returned their tenant and levels. The real module screens
 * arrive from Phase 4.
 */
export function Dashboard() {
  const me = useMe();
  const { signOut } = useAuth();

  // A module absent from the map is one the tenant is not entitled to, or one
  // this user holds no level in. Both are hidden entirely, not disabled (§10.1)
  // — and the hiding is cosmetic: RequireModule enforces it server-side (I12).
  const visible = MODULES.filter((m) => me.moduleRoles[m.code]);

  return (
    <div className="min-h-screen bg-canvas text-primary">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b border-hairline px-6 py-4">
        <div>
          <span className="text-lg font-semibold">mini-erp</span>
          {me.tenant && (
            <span className="ml-3 text-sm text-secondary">
              {me.tenant.name}
            </span>
          )}
        </div>
        <div className="flex items-center gap-3">
          <ThemeToggle />
          <button
            type="button"
            onClick={() => void signOut()}
            className="min-h-11 rounded-md border border-hairline px-3 text-sm"
          >
            Sign out
          </button>
        </div>
      </header>

      <main className="mx-auto max-w-3xl px-6 py-10">
        <h1 className="text-xl font-semibold">Signed in</h1>
        <p className="mt-1 text-sm text-secondary">
          Everything below came from <span className="tabular">/api/me</span>,
          resolved from the database against the verified token.
        </p>

        <section className="mt-6 rounded-lg border border-hairline bg-surface p-6">
          <dl className="grid grid-cols-[auto_1fr] gap-x-8 gap-y-2 text-sm">
            <dt className="text-secondary">Name</dt>
            <dd>{me.user.fullName}</dd>

            <dt className="text-secondary">Email</dt>
            <dd>{me.user.email}</dd>

            <dt className="text-secondary">Tenant role</dt>
            <dd>{me.user.tenantRole}</dd>

            <dt className="text-secondary">Tenant</dt>
            <dd>{me.tenant ? me.tenant.name : "— platform superadmin"}</dd>

            <dt className="text-secondary">Timezone</dt>
            <dd className="tabular">{me.tenant?.timezone ?? "—"}</dd>
          </dl>
        </section>

        <section className="mt-6">
          <h2 className="text-base font-semibold">Modules</h2>
          {visible.length === 0 ? (
            <p className="mt-2 text-sm text-secondary">
              {me.user.tenantRole === "superadmin"
                ? "Superadmins hold no module roles — they administer tenants, not business data."
                : "No modules yet. Your administrator assigns these."}
            </p>
          ) : (
            <ul className="mt-2 flex flex-wrap gap-2">
              {visible.map((module) => (
                <li
                  key={module.code}
                  className="rounded-md border border-hairline bg-surface px-3 py-1.5 text-sm"
                >
                  {module.label}{" "}
                  <span className="text-secondary">
                    · {me.moduleRoles[module.code] as RoleLevel}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </main>
    </div>
  );
}
