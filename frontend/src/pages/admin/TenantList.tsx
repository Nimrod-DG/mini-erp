import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { FilterBar, SearchInput } from "../../components/Filters";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
  TableFrame,
  TableHead,
  tableRow,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { usePagination } from "../../hooks/usePagination";
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
    <span className={`rounded border px-2 py-0.5 text-xs ${tone}`}>
      {status}
    </span>
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
  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [search, setSearch] = useState("");

  const { state, reload } = useAsync(`tenants:${key}:${search}`, () =>
    listTenants({ page, pageSize, q: search, sort: "name" }),
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
      <FilterBar>
        <SearchInput
          label="Search workspaces"
          value={search}
          onChange={(next) => {
            setSearch(next);
            setPage(1); // or page 3 of a 1-page result set strands the user
          }}
          placeholder="Name or slug"
        />
      </FilterBar>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          {/* Horizontal scroll rather than card transformation: this is a dense
              administrative grid read on a desktop, and the counts are compared
              across rows (§10.7.4). */}
          <TableFrame>
            <table className="w-full min-w-[42rem] text-left text-sm">
              <TableHead
                columns={[
                  { label: "Workspace" },
                  { label: "Status" },
                  { label: "Users" },
                  { label: "Modules" },
                  { label: "Timezone" },
                ]}
              />

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
                    <tr key={tenant.id} className={tableRow}>
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
          </TableFrame>

          {state.status === "ready" && (
            <Pagination
              page={state.data.page}
              pageSize={state.data.pageSize}
              totalItems={state.data.totalItems}
              totalPages={state.data.totalPages}
              onPage={setPage}
              onPageSize={setPageSize}
            />
          )}
        </>
      )}
    </AppShell>
  );
}
