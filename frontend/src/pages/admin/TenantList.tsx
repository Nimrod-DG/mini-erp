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
import { listTenants, type TenantSummary } from "../../lib/api";

const COLUMNS = 5;

/** StatusPill: colour is never the only signal — the word is there too, for the
 *  8% of men with a colour vision deficiency and for a monochrome print. */
function StatusPill({ status }: { status: TenantSummary["status"] }) {
  const tone =
    status === "active"
      ? "border-success/40 text-success"
      : "border-warning/40 text-warning";
  return (
    <span className={`rounded border px-2 py-0.5 text-xs ${tone}`}>{status}</span>
  );
}

function ModulePills({ codes }: { codes: string[] }) {
  if (codes.length === 0) {
    return <span className="text-xs text-secondary">none</span>;
  }
  return (
    <ul className="flex flex-wrap gap-1">
      {codes.map((code) => (
        <li
          key={code}
          className="rounded border border-hairline px-1.5 py-0.5 text-xs text-secondary"
        >
          {code}
        </li>
      ))}
    </ul>
  );
}

/**
 * `/admin/tenants` — the platform plane's home, and where a superadmin lands
 * (§10.1). Status, user count, and module pills (§10.6).
 */
export function TenantList() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");

  const { state, reload } = useAsync(`tenants:${page}:${search}`, () =>
    listTenants({ page, q: search, sort: "name" }),
  );

  const newTenant = (
    <Link
      to="/admin/tenants/new"
      className="flex min-h-11 items-center rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      New workspace
    </Link>
  );

  return (
    <AppShell title="Workspaces" actions={newTenant}>
      <label className="mb-4 block max-w-sm">
        <span className="mb-1 block text-sm text-secondary">Search</span>
        <input
          type="search"
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
            setPage(1); // or page 3 of a 1-page result set strands the user
          }}
          placeholder="Name or slug"
          className="min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm"
        />
      </label>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          {/* Horizontal scroll rather than card transformation: this is a dense
              administrative grid read on a desktop, and the counts are compared
              across rows (§10.7.4). */}
          <div className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[42rem] text-left text-sm">
              <thead className="text-xs uppercase tracking-wide text-secondary">
                <tr>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Workspace
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Status
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Users
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Modules
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Timezone
                  </th>
                </tr>
              </thead>

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={search !== ""}
                  firstRun="No workspaces yet. Create one to get started — it comes with its first administrator and a chart of accounts."
                  noResults="No workspaces match that search."
                  action={newTenant}
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((tenant) => (
                    <tr
                      key={tenant.id}
                      className="border-t border-hairline hover:bg-raised"
                    >
                      <td className="px-3 py-3">
                        <Link
                          to={`/admin/tenants/${tenant.id}`}
                          className="font-medium underline decoration-hairline underline-offset-2"
                        >
                          {tenant.name}
                        </Link>
                        <div className="tabular text-xs text-secondary">
                          {tenant.slug}
                        </div>
                      </td>
                      <td className="px-3 py-3">
                        <StatusPill status={tenant.status} />
                      </td>
                      <td className="px-3 py-3">
                        <span className="tabular">{tenant.userCount}</span>
                        <span className="text-secondary">
                          {" "}
                          ({tenant.adminCount} admin
                          {tenant.adminCount === 1 ? "" : "s"})
                        </span>
                      </td>
                      <td className="px-3 py-3">
                        <ModulePills codes={tenant.enabledModules} />
                      </td>
                      <td className="tabular px-3 py-3 text-secondary">
                        {tenant.timezone}
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
