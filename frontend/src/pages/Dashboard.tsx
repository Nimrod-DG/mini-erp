import { Link } from "react-router-dom";

import { AppShell } from "../components/AppShell";
import { useMe } from "../hooks/useAuth";
import type { ModuleCode, RoleLevel } from "../lib/api";

/** Written out rather than built from a template string: Tailwind scans source
 *  text, so a class it cannot see literally is a class it does not generate. */
const MODULES: { code: ModuleCode; label: string }[] = [
  { code: "procurement", label: "Procurement" },
  { code: "inventory", label: "Inventory" },
  { code: "finance", label: "Finance" },
];

/**
 * The dashboard. Its four real widgets (§10.2) need documents to count, so they
 * arrive with the module screens in Phases 4-6. Until then it shows what has
 * actually been built: the identity and the levels the backend resolved from the
 * database for this request.
 */
export function Dashboard() {
  const me = useMe();

  // A module absent from the map is one the tenant is not entitled to, or one
  // this user holds no level in. Both are hidden entirely, not disabled (§10.1)
  // — and the hiding is cosmetic: RequireModule enforces it server-side (I12).
  const visible = MODULES.filter((m) => me.moduleRoles[m.code]);

  return (
    <AppShell title="Dashboard">
      <div className="max-w-3xl">
        <p className="text-sm text-secondary">
          Everything below came from <span className="tabular">/api/me</span>,
          resolved from the database against the verified token.
        </p>

        <section className="mt-6 rounded-lg border border-hairline bg-surface p-6">
          <dl className="grid grid-cols-[auto_1fr] gap-x-8 gap-y-2 text-sm">
            <dt className="text-secondary">Name</dt>
            <dd>{me.user.fullName}</dd>

            <dt className="text-secondary">Email</dt>
            <dd>{me.user.email}</dd>

            <dt className="text-secondary">Workspace role</dt>
            <dd className="tabular">{me.user.tenantRole}</dd>

            <dt className="text-secondary">Workspace</dt>
            <dd>{me.tenant ? me.tenant.name : "— platform superadmin"}</dd>

            <dt className="text-secondary">Timezone</dt>
            <dd className="tabular">{me.tenant?.timezone ?? "—"}</dd>
          </dl>
        </section>

        <section className="mt-6">
          <h2 className="text-base font-semibold">Modules</h2>
          {visible.length === 0 ? (
            <p className="mt-2 text-sm text-secondary">
              No modules yet. Your administrator assigns these.
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
          {me.user.tenantRole === "admin" && (
            <p className="mt-3 text-sm text-secondary">
              You hold <span className="tabular">admin</span> in every enabled
              module implicitly.{" "}
              <Link
                to="/settings/users"
                className="underline decoration-hairline underline-offset-2"
              >
                Manage who else can do what
              </Link>
              .
            </p>
          )}
        </section>
      </div>
    </AppShell>
  );
}
