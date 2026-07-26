import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import {
  getTenant,
  patchTenant,
  setTenantModule,
  type ModuleCode,
  type TenantDetail as Tenant,
} from "../../lib/api";

/**
 * `/admin/tenants/:id` — the module entitlement toggle matrix (§10.6).
 *
 * Each toggle is its own request, deliberately: unlike the per-user role matrix,
 * these are independent decisions a superadmin makes one at a time, and there is
 * no half-failure to prevent. The effect is immediate — the tenant's very next
 * request reflects it, with no restart and nothing to invalidate, because
 * entitlement is read from the database during identity resolution on every
 * request (B5).
 */
export function TenantDetailPage() {
  const { id = "" } = useParams();
  const { state, reload } = useAsync(`tenant:${id}`, () => getTenant(id));

  const [pending, setPending] = useState<ModuleCode | "status" | null>(null);
  const [actionError, setActionError] = useState<Error | null>(null);
  const [tenant, setTenant] = useState<Tenant | null>(null);

  // The fetched value is the source of truth until an action replaces it.
  const current = tenant ?? (state.status === "ready" ? state.data : null);

  async function run<T>(key: ModuleCode | "status", action: () => Promise<T>) {
    setActionError(null);
    setPending(key);
    try {
      await action();
      // Re-read rather than patching local state from the response: the counts
      // and the matrix both move, and one round trip is cheaper than a bug
      // where they disagree.
      setTenant(await getTenant(id));
    } catch (caught) {
      setActionError(caught instanceof Error ? caught : new Error(String(caught)));
    } finally {
      setPending(null);
    }
  }

  if (state.status === "loading" && !current) {
    return (
      <AppShell title="Workspace">
        <p className="text-sm text-secondary">Loading…</p>
      </AppShell>
    );
  }

  if (state.status === "error" && !current) {
    return (
      <AppShell title="Workspace">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }

  if (!current) return null;

  const suspended = current.status === "suspended";

  return (
    <AppShell
      title={current.name}
      actions={
        <Link
          to="/admin/tenants"
          className="min-h-11 rounded-md border border-hairline px-3 text-sm leading-[2.75rem]"
        >
          All workspaces
        </Link>
      }
    >
      <div className="max-w-2xl space-y-6">
        <section className="rounded-lg border border-hairline bg-surface p-5">
          <dl className="grid grid-cols-[auto_1fr] gap-x-8 gap-y-2 text-sm">
            <dt className="text-secondary">Slug</dt>
            <dd className="tabular">{current.slug}</dd>

            <dt className="text-secondary">Status</dt>
            <dd>{current.status}</dd>

            <dt className="text-secondary">Timezone</dt>
            <dd className="tabular">{current.timezone}</dd>

            <dt className="text-secondary">Active users</dt>
            <dd className="tabular">
              {current.userCount} ({current.adminCount} admin
              {current.adminCount === 1 ? "" : "s"})
            </dd>
          </dl>
        </section>

        <section className="rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">Modules</h2>
          <p className="mt-1 text-sm text-secondary">
            The ceiling this workspace's administrator allocates within. Turning a
            module off takes effect on their next request, and leaves everyone's
            role levels untouched — turning it back on restores them.
          </p>

          <ul className="mt-4 divide-y divide-hairline">
            {current.modules.map((module) => (
              <li
                key={module.code}
                className="flex items-center justify-between gap-4 py-3"
              >
                <div>
                  <span className="text-sm font-medium">{module.name}</span>
                  <p className="text-xs text-secondary">{module.description}</p>
                </div>
                <label className="flex min-h-11 shrink-0 items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={module.enabled}
                    disabled={pending !== null}
                    onChange={(event) =>
                      void run(module.code, () =>
                        setTenantModule(id, module.code, event.target.checked),
                      )
                    }
                    className="size-4 accent-accent"
                  />
                  <span className={module.enabled ? "" : "text-secondary"}>
                    {module.enabled ? "Enabled" : "Off"}
                  </span>
                </label>
              </li>
            ))}
          </ul>
        </section>

        <section className="rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">
            {suspended ? "Reactivate" : "Suspend"}
          </h2>
          <p className="mt-1 text-sm text-secondary">
            {suspended
              ? "The workspace's users are currently turned away at sign-in. Reactivating restores access immediately."
              : "Every user of this workspace is turned away at sign-in with a clear message, rather than signing in to an empty application. Nothing is deleted, and it is reversible."}
          </p>

          <button
            type="button"
            disabled={pending !== null}
            onClick={() =>
              void run("status", () =>
                patchTenant(id, { status: suspended ? "active" : "suspended" }),
              )
            }
            className={`mt-4 min-h-11 rounded-md px-4 text-sm font-medium disabled:opacity-50 ${
              suspended
                ? "bg-accent text-white"
                : "border border-danger/50 text-danger"
            }`}
          >
            {pending === "status"
              ? "Saving…"
              : suspended
                ? "Reactivate workspace"
                : "Suspend workspace"}
          </button>

          {/* There is no delete. Tenants are suspended, never deleted (§6.9.4)
              — the API has no such route to call. */}
        </section>

        {actionError && <ErrorNotice error={actionError} />}
      </div>
    </AppShell>
  );
}
