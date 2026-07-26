import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import {
  ApiError,
  getTenantUser,
  patchTenantUser,
  setTenantUserModules,
  type ModuleCode,
  type RoleLevelOrNone,
  type TenantUserDetail,
} from "../../lib/api";

/** The five levels, ranked, lowest first — the order of the dropdown is the
 *  order of the model (§5.3). `none` is offered even though it is never stored:
 *  choosing it deletes the row. */
const LEVELS: { value: RoleLevelOrNone; label: string; hint: string }[] = [
  { value: "none", label: "None", hint: "No access; the module is hidden" },
  { value: "viewer", label: "Viewer", hint: "Read-only" },
  { value: "user", label: "User", hint: "Create and edit own drafts" },
  { value: "approver", label: "Approver", hint: "Approve, reject, post receipts" },
  { value: "admin", label: "Admin", hint: "Manage the module's master data" },
];

type Matrix = Partial<Record<ModuleCode, RoleLevelOrNone>>;

function matrixOf(user: TenantUserDetail): Matrix {
  const matrix: Matrix = {};
  for (const cell of user.modules) matrix[cell.code] = cell.roleLevel;
  return matrix;
}

function sameMatrix(a: Matrix, b: Matrix): boolean {
  const codes = new Set([...Object.keys(a), ...Object.keys(b)]) as Set<ModuleCode>;
  for (const code of codes) {
    if ((a[code] ?? "none") !== (b[code] ?? "none")) return false;
  }
  return true;
}

/**
 * `/settings/users/:id` — the per-module role matrix (§10.6).
 *
 * Worth building carefully: it is the clearest visual explanation of the
 * permission model in the whole application. Three things it has to get right:
 *
 *  - One dropdown per **entitled** module. A module the workspace does not have
 *    is absent, not disabled — there is nothing to allocate (§5.7).
 *  - Saving is **one request** for the whole matrix, not one per dropdown, so
 *    six changes cannot half-fail and leave three applied (§9.3).
 *  - For a workspace admin the matrix is read-only, because their levels are
 *    implicit. Any stored levels are shown as what a demotion would restore,
 *    which is the honest description of what the rows are for (§5.4).
 */
export function UserDetailPage() {
  const { id = "" } = useParams();
  const me = useMe();
  const { state, reload } = useAsync(`user:${id}`, () => getTenantUser(id));

  const [user, setUser] = useState<TenantUserDetail | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<Error | null>(null);
  const [saved, setSaved] = useState(false);

  const current = user ?? (state.status === "ready" ? state.data : null);

  // The dropdowns are *derived* from the server's matrix with an optional local
  // override on top, rather than copied into state by an effect. Copying needs
  // an effect to re-seed on every reload, and that effect is where this pattern
  // usually goes wrong: it either misses a case and shows stale levels, or fires
  // mid-edit and discards what the admin just chose. `null` means "no local
  // edits", so discarding is one assignment and there is nothing to keep in sync.
  const [override, setOverride] = useState<Matrix | null>(null);
  const serverMatrix = current ? matrixOf(current) : {};
  const draft = override ?? serverMatrix;

  async function run(action: () => Promise<TenantUserDetail>) {
    setActionError(null);
    setSaved(false);
    setBusy(true);
    try {
      const next = await action();
      setUser(next);
      setOverride(null);
      setSaved(true);
    } catch (caught) {
      setActionError(caught instanceof Error ? caught : new Error(String(caught)));
    } finally {
      setBusy(false);
    }
  }

  if (state.status === "loading" && !current) {
    return (
      <AppShell title="User">
        <p className="text-sm text-secondary">Loading…</p>
      </AppShell>
    );
  }
  if (state.status === "error" && !current) {
    return (
      <AppShell title="User">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (!current) return null;

  const isAdmin = current.tenantRole === "admin";
  const isSelf = current.id === me.user.id;
  const dirty = !sameMatrix(draft, matrixOf(current));
  const lastAdmin =
    actionError instanceof ApiError && actionError.code === "last_admin";

  return (
    <AppShell
      title={current.fullName}
      actions={
        <Link
          to="/settings/users"
          className="min-h-11 rounded-md border border-hairline px-3 text-sm leading-[2.75rem]"
        >
          All users
        </Link>
      }
    >
      <div className="max-w-2xl space-y-6">
        <section className="rounded-lg border border-hairline bg-surface p-5">
          <dl className="grid grid-cols-[auto_1fr] gap-x-8 gap-y-2 text-sm">
            <dt className="text-secondary">Email</dt>
            <dd>{current.email}</dd>

            <dt className="text-secondary">Workspace role</dt>
            <dd className="tabular">{current.tenantRole}</dd>

            <dt className="text-secondary">Status</dt>
            <dd>{current.isActive ? "active" : "deactivated"}</dd>
          </dl>
        </section>

        {/* ---------------------------------------------------------------- */}
        <section className="rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">Module roles</h2>

          {isAdmin ? (
            <p className="mt-1 text-sm text-secondary">
              Workspace administrators hold{" "}
              <span className="tabular">admin</span> in every enabled module
              implicitly, so there is nothing to set here.
              {Object.keys(current.moduleRoles).length > 0 && (
                <>
                  {" "}
                  The levels below are kept from before the promotion and would be
                  restored on demotion.
                </>
              )}
            </p>
          ) : (
            <p className="mt-1 text-sm text-secondary">
              Each level includes everything below it, so an approver can also do
              what a user and a viewer can. Modules this workspace is not entitled
              to are not listed at all.
            </p>
          )}

          <ul className="mt-4 divide-y divide-hairline">
            {current.modules.map((cell) => (
              <li
                key={cell.code}
                className="flex flex-wrap items-center justify-between gap-3 py-3"
              >
                <div className="min-w-0">
                  <span className="text-sm font-medium">{cell.name}</span>
                  <p className="text-xs text-secondary">
                    {isAdmin ? (
                      <>
                        effective <span className="tabular">admin</span>
                        {cell.roleLevel !== "none" && (
                          <>
                            {" · stored "}
                            <span className="tabular">{cell.roleLevel}</span>
                          </>
                        )}
                      </>
                    ) : (
                      LEVELS.find((l) => l.value === (draft[cell.code] ?? "none"))
                        ?.hint
                    )}
                  </p>
                </div>

                <select
                  aria-label={`${cell.name} role level`}
                  value={draft[cell.code] ?? "none"}
                  disabled={isAdmin || busy}
                  onChange={(event) =>
                    setOverride(() => ({
                      ...draft,
                      [cell.code]: event.target.value as RoleLevelOrNone,
                    }))
                  }
                  className="min-h-11 w-40 shrink-0 rounded-md border border-hairline bg-surface px-3 text-sm disabled:opacity-50"
                >
                  {LEVELS.map((level) => (
                    <option key={level.value} value={level.value}>
                      {level.label}
                    </option>
                  ))}
                </select>
              </li>
            ))}
          </ul>

          {current.modules.length === 0 && (
            <p className="mt-4 text-sm text-secondary">
              This workspace has no modules enabled. Its platform administrator
              controls that.
            </p>
          )}

          {!isAdmin && current.modules.length > 0 && (
            <div className="mt-5 flex flex-wrap items-center gap-3">
              <button
                type="button"
                disabled={!dirty || busy}
                onClick={() => void run(() => setTenantUserModules(id, draft))}
                className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
              >
                {busy ? "Saving…" : "Save all module roles"}
              </button>
              {dirty && (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => setOverride(null)}
                  className="min-h-11 rounded-md border border-hairline px-3 text-sm"
                >
                  Discard changes
                </button>
              )}
              <span className="text-xs text-secondary">
                {dirty
                  ? "Unsaved — the whole matrix is saved in one request."
                  : saved
                    ? "Saved."
                    : ""}
              </span>
            </div>
          )}
        </section>

        {/* ---------------------------------------------------------------- */}
        <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
          <h2 className="text-base font-semibold">Workspace role and access</h2>

          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              disabled={busy}
              onClick={() =>
                void run(() =>
                  patchTenantUser(id, {
                    tenantRole: isAdmin ? "staff" : "admin",
                  }),
                )
              }
              className="min-h-11 rounded-md border border-hairline px-4 text-sm"
            >
              {isAdmin ? "Demote to staff" : "Promote to administrator"}
            </button>

            <button
              type="button"
              disabled={busy}
              onClick={() =>
                void run(() =>
                  patchTenantUser(id, { isActive: !current.isActive }),
                )
              }
              className={`min-h-11 rounded-md px-4 text-sm ${
                current.isActive
                  ? "border border-danger/50 text-danger"
                  : "bg-accent font-medium text-white"
              }`}
            >
              {current.isActive ? "Deactivate" : "Reactivate"}
            </button>
          </div>

          <p className="text-xs text-secondary">
            Users are deactivated, never deleted — their name stays on the
            documents they raised. A workspace must always keep at least one
            active administrator.
            {isSelf && " This is your own account."}
          </p>
        </section>

        {actionError && <ErrorNotice error={actionError} />}
        {lastAdmin && (
          <p className="text-sm text-secondary">
            Promote someone else to administrator first, then try again. Without
            the rule, a workspace could be left with nobody able to manage it and
            no way to recover in the application.
          </p>
        )}
      </div>
    </AppShell>
  );
}
