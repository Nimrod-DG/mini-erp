import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
  TableHead,
} from "../../components/ListStates";
import { StatusChip, StatusFilter } from "../../components/StatusChip";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { listRequisitions, type RequisitionStatus } from "../../lib/api";
import { formatDateTime, formatMoney } from "../../lib/format";
import { holds } from "../../lib/levels";

const COLUMNS = 6;

const STATUSES: readonly RequisitionStatus[] = [
  "draft",
  "submitted",
  "approved",
  "rejected",
  "cancelled",
];

/**
 * `/procurement/requisitions` — the list, with the §10.3 status filter chips.
 *
 * The filter is a server parameter, not a client-side `.filter()`. Filtering the
 * fetched page would report "3 drafts" for a tenant with forty, because only 25
 * rows arrived — the count in the pagination line and the rows in the table have
 * to be answers to the same question.
 */
export function RequisitionList() {
  const me = useMe();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<RequisitionStatus | "">("");

  const canRaise = holds(me.moduleRoles, "procurement", "user");

  const { state, reload } = useAsync(
    `requisitions:${page}:${search}:${status}`,
    () => listRequisitions({ page, q: search, status, sort: "-createdAt" }),
  );

  const newButton = canRaise ? (
    <Link
      to="/procurement/requisitions/new"
      className="inline-flex min-h-11 items-center rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      New requisition
    </Link>
  ) : null;

  return (
    <AppShell title="Requisitions" actions={newButton}>
      <div className="mb-4 space-y-4">
        <StatusFilter
          value={status}
          options={STATUSES}
          onChange={(next) => {
            setStatus(next);
            setPage(1);
          }}
        />

        <label className="block max-w-sm">
          <span className="mb-1 block text-sm text-secondary">Search</span>
          <input
            type="search"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
            placeholder="Number, supplier, or notes"
            className="min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm"
          />
        </label>
      </div>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[52rem] text-left text-sm">
              <TableHead
                columns={[
                  { label: "Number" },
                  { label: "Status" },
                  { label: "Supplier" },
                  { label: "Raised by" },
                  { label: "Lines", align: "right" },
                  { label: "Estimated", align: "right" },
                ]}
              />

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={search !== "" || status !== ""}
                  firstRun="No requisitions yet. A requisition is how buying starts here: raise one, have it approved, and a purchase order is created for you."
                  noResults="No requisitions match those filters."
                  action={newButton}
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((row) => (
                    <tr
                      key={row.id}
                      className="border-t border-hairline hover:bg-raised"
                    >
                      <td className="px-3 py-3">
                        <Link
                          to={`/procurement/requisitions/${row.id}`}
                          className="tabular font-medium text-accent"
                        >
                          {row.prNumber}
                        </Link>
                        <div className="text-xs text-secondary">
                          {formatDateTime(
                            row.createdAt,
                            me.tenant?.timezone ?? "UTC",
                          )}
                        </div>
                      </td>
                      <td className="px-3 py-3">
                        <StatusChip status={row.status} />
                        {row.purchaseOrderNumber && (
                          <div className="mt-1 text-xs">
                            <Link
                              to={`/procurement/orders/${row.purchaseOrderId}`}
                              className="tabular text-accent"
                            >
                              {row.purchaseOrderNumber}
                            </Link>
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-3">
                        {row.supplierName ?? (
                          <span className="text-secondary">not chosen yet</span>
                        )}
                        <div className="text-xs text-secondary">
                          {row.warehouseCode}
                        </div>
                      </td>
                      <td className="px-3 py-3">{row.requestedByName}</td>
                      <td className="px-3 py-3 text-right tabular">
                        {row.lineCount}
                      </td>
                      <td className="px-3 py-3 text-right tabular">
                        {formatMoney(row.estimatedTotal)}
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
