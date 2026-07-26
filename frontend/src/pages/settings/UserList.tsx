import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { listTenantUsers, type ModuleCode, type TenantUser } from "../../lib/api";

const COLUMNS = 4;

const MODULES: { code: ModuleCode; short: string }[] = [
  { code: "procurement", short: "Proc" },
  { code: "inventory", short: "Inv" },
  { code: "finance", short: "Fin" },
];

/**
 * EffectiveLevels renders what the user actually holds, not what is stored.
 *
 * The difference matters for every admin: their stored matrix is empty and their
 * effective one is `admin` everywhere entitled. Showing the stored map would
 * display "no access" next to the person who has all of it — and the level comes
 * from the server's own LevelFor, so this screen cannot disagree with the gate
 * that enforces it.
 */
function EffectiveLevels({
  user,
  entitled,
}: {
  user: TenantUser;
  entitled: ModuleCode[];
}) {
  const visible = MODULES.filter((m) => entitled.includes(m.code));
  if (visible.length === 0) {
    return <span className="text-xs text-secondary">no modules enabled</span>;
  }

  return (
    <ul className="flex flex-wrap gap-1">
      {visible.map((module) => {
        const level = user.effectiveRoles[module.code];
        return (
          <li
            key={module.code}
            className={`rounded border px-1.5 py-0.5 text-xs ${
              level
                ? "border-hairline text-secondary"
                : "border-transparent text-secondary/50"
            }`}
            title={`${module.code}: ${level ?? "none"}`}
          >
            {module.short} <span className="tabular">{level ?? "—"}</span>
          </li>
        );
      })}
    </ul>
  );
}

/** `/settings/users` — the tenant user list (§10.6). */
export function UserList() {
  const me = useMe();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");

  const { state, reload } = useAsync(`users:${page}:${search}`, () =>
    listTenantUsers({ page, q: search, sort: "fullName" }),
  );

  // Only modules this tenant is entitled to have a column at all: a level in a
  // module the workspace does not have is not access to it (§5.7).
  const entitled = Object.keys(me.moduleRoles) as ModuleCode[];

  const newUser = (
    <Link
      to="/settings/users/new"
      className="flex min-h-11 items-center rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      Add user
    </Link>
  );

  return (
    <AppShell title="Users" actions={newUser}>
      <label className="mb-4 block max-w-sm">
        <span className="mb-1 block text-sm text-secondary">Search</span>
        <input
          type="search"
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
            setPage(1);
          }}
          placeholder="Name or email"
          className="min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm"
        />
      </label>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[38rem] text-left text-sm">
              <thead className="text-xs uppercase tracking-wide text-secondary">
                <tr>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Name
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Workspace role
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Effective module levels
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Status
                  </th>
                </tr>
              </thead>

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={search !== ""}
                  firstRun="No users yet. Add your colleagues and give each of them a level in the modules they work in."
                  noResults="No users match that search."
                  action={newUser}
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((user) => (
                    <tr
                      key={user.id}
                      className="border-t border-hairline hover:bg-raised"
                    >
                      <td className="px-3 py-3">
                        <Link
                          to={`/settings/users/${user.id}`}
                          className="font-medium underline decoration-hairline underline-offset-2"
                        >
                          {user.fullName}
                        </Link>
                        <div className="text-xs text-secondary">{user.email}</div>
                        {user.id === me.user.id && (
                          <span className="text-xs text-secondary">(you)</span>
                        )}
                      </td>
                      <td className="px-3 py-3">
                        <span className="tabular">{user.tenantRole}</span>
                        {user.tenantRole === "admin" && (
                          <p className="text-xs text-secondary">
                            admin in every enabled module
                          </p>
                        )}
                      </td>
                      <td className="px-3 py-3">
                        <EffectiveLevels user={user} entitled={entitled} />
                      </td>
                      <td className="px-3 py-3">
                        <span
                          className={
                            user.isActive
                              ? "rounded border border-success/40 px-2 py-0.5 text-xs text-success"
                              : "rounded border border-hairline px-2 py-0.5 text-xs text-secondary"
                          }
                        >
                          {user.isActive ? "active" : "deactivated"}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              )}
            </table>
          </div>

          {state.status === "ready" && (
            <Pagination
              page={state.data.page}
              pageSize={state.data.pageSize}
              totalItems={state.data.totalItems}
              totalPages={state.data.totalPages}
              onPage={setPage}
            />
          )}
        </>
      )}
    </AppShell>
  );
}
