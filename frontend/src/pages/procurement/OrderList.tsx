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
import {
  listPurchaseOrders,
  listSuppliers,
  type PurchaseOrderStatus,
} from "../../lib/api";
import { formatMoney, formatQty } from "../../lib/format";

const COLUMNS = 6;

const STATUSES: readonly PurchaseOrderStatus[] = [
  "open",
  "partially_received",
  "received",
  "cancelled",
];

/**
 * `/procurement/orders` — the PO list with the §10.3 status and supplier filters.
 *
 * The received/ordered figures come from `po_line_status`, which sums the goods
 * receipt lines on every read. There is no stored counter anywhere behind this
 * column (I6), so it cannot disagree with the receipts it is describing.
 */
export function OrderList() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<PurchaseOrderStatus | "">("");
  const [supplierId, setSupplierId] = useState("");

  const { state, reload } = useAsync(
    `orders:${page}:${search}:${status}:${supplierId}`,
    () =>
      listPurchaseOrders({
        page,
        q: search,
        status,
        supplierId: supplierId || undefined,
        sort: "-orderedAt",
      }),
  );

  // The supplier filter's options. A failure here leaves the filter empty rather
  // than breaking the list, which is the screen the user came for.
  const { state: suppliers } = useAsync("order-filter-suppliers", () =>
    listSuppliers({ pageSize: 100, sort: "code" }),
  );

  return (
    <AppShell title="Purchase orders">
      <div className="mb-4 space-y-4">
        <StatusFilter
          value={status}
          options={STATUSES}
          onChange={(next) => {
            setStatus(next);
            setPage(1);
          }}
        />

        <div className="flex flex-wrap items-end gap-4">
          <label className="block max-w-sm grow">
            <span className="mb-1 block text-sm text-secondary">Search</span>
            <input
              type="search"
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setPage(1);
              }}
              placeholder="Order number or supplier"
              className="min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm"
            />
          </label>

          <label className="block">
            <span className="mb-1 block text-sm text-secondary">Supplier</span>
            <select
              value={supplierId}
              onChange={(event) => {
                setSupplierId(event.target.value);
                setPage(1);
              }}
              className="min-h-11 rounded-md border border-hairline bg-surface px-3 text-sm"
            >
              <option value="">All suppliers</option>
              {suppliers.status === "ready" &&
                suppliers.data.data.map((supplier) => (
                  <option key={supplier.id} value={supplier.id}>
                    {supplier.code} — {supplier.name}
                  </option>
                ))}
            </select>
          </label>
        </div>
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
                  { label: "Expected" },
                  { label: "Received", align: "right" },
                  { label: "Total", align: "right" },
                ]}
              />

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={search !== "" || status !== "" || supplierId !== ""}
                  firstRun="No purchase orders yet. They are not created by hand — approving a requisition creates one."
                  noResults="No orders match those filters."
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
                          to={`/procurement/orders/${row.id}`}
                          className="tabular font-medium text-accent"
                        >
                          {row.poNumber}
                        </Link>
                        {row.requisitionNumber && (
                          <div className="text-xs">
                            <Link
                              to={`/procurement/requisitions/${row.requisitionId}`}
                              className="tabular text-secondary"
                            >
                              from {row.requisitionNumber}
                            </Link>
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-3">
                        <StatusChip status={row.status} />
                      </td>
                      <td className="px-3 py-3">
                        {row.supplierName}
                        <div className="text-xs text-secondary">
                          {row.warehouseCode}
                        </div>
                      </td>
                      {/* A DATE, rendered as it arrived. Putting it through a
                          timezone would move it a day for half the world (§2.5.3). */}
                      <td className="px-3 py-3 tabular">
                        {row.expectedAt ?? "—"}
                      </td>
                      <td className="px-3 py-3 text-right tabular">
                        {formatQty(row.qtyReceived)} /{" "}
                        {formatQty(row.qtyOrdered)}
                      </td>
                      <td className="px-3 py-3 text-right tabular">
                        {formatMoney(row.totalAmount)}
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
