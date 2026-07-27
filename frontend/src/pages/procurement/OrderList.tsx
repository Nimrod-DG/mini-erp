import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { DocumentCard } from "../../components/CardList";
import { FilterBar, FilterDropdown, SearchInput } from "../../components/Filters";
import { tableRow } from "../../components/ListStates";
import { ResponsiveList } from "../../components/ResponsiveList";
import { StatusChip } from "../../components/StatusChip";
import { useAsync } from "../../hooks/useAsync";
import { usePagination } from "../../hooks/usePagination";
import {
  listPurchaseOrders,
  listSuppliers,
  type PurchaseOrderStatus,
} from "../../lib/api";
import { formatMoney, formatQty, statusLabel } from "../../lib/format";

const STATUSES: readonly PurchaseOrderStatus[] = [
  "open",
  "partially_received",
  "received",
  "cancelled",
];

const STATUS_OPTIONS = STATUSES.map((status) => ({
  value: status,
  label: statusLabel(status),
}));

/**
 * `/procurement/orders` — the PO list with the §10.3 status and supplier filters.
 *
 * The received/ordered figures come from `po_line_status`, which sums the goods
 * receipt lines on every read. There is no stored counter anywhere behind this
 * column (I6), so it cannot disagree with the receipts it is describing.
 *
 * Cards below `md`, for the same reason as the requisition list: an order is
 * read one at a time, and this is a screen a manager checks on the move
 * (§10.7.1, §10.7.4).
 */
export function OrderList() {
  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<PurchaseOrderStatus | "">("");
  const [supplierId, setSupplierId] = useState("");

  const { state, reload } = useAsync(
    `orders:${key}:${search}:${status}:${supplierId}`,
    () =>
      listPurchaseOrders({
        page,
        pageSize,
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

  /** The requisition this order came from — on both the row and the card. */
  const fromRequisition = (row: {
    requisitionId: string | null;
    requisitionNumber: string | null;
  }) =>
    row.requisitionNumber && (
      <Link
        to={`/procurement/requisitions/${row.requisitionId}`}
        className="tabular text-secondary"
      >
        from {row.requisitionNumber}
      </Link>
    );

  return (
    <AppShell title="Purchase orders">
      <FilterBar>
        <SearchInput
          label="Search purchase orders"
          value={search}
          onChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          placeholder="Order number or supplier"
        />
        <FilterDropdown
          label="Status"
          value={status}
          options={STATUS_OPTIONS}
          allLabel="All statuses"
          onChange={(next) => {
            setStatus(next as PurchaseOrderStatus | "");
            setPage(1);
          }}
        />
        <FilterDropdown
          label="Supplier"
          value={supplierId}
          allLabel="All suppliers"
          options={
            suppliers.status === "ready"
              ? suppliers.data.data.map((supplier) => ({
                  value: supplier.id,
                  label: `${supplier.code} — ${supplier.name}`,
                }))
              : []
          }
          onChange={(next) => {
            setSupplierId(next);
            setPage(1);
          }}
        />
      </FilterBar>

      <ResponsiveList
        state={state}
        onRetry={reload}
        onPage={setPage}
        onPageSize={setPageSize}
        minWidth="min-w-[52rem]"
        filtered={search !== "" || status !== "" || supplierId !== ""}
        firstRun="No purchase orders yet. They are not created by hand — approving a requisition creates one."
        noResults="No orders match those filters."
        columns={[
          { label: "Number" },
          { label: "Status" },
          { label: "Supplier" },
          { label: "Expected" },
          { label: "Received", align: "right" },
          { label: "Total", align: "right" },
        ]}
        row={(row) => (
          <tr key={row.id} className={tableRow}>
            <td className="px-3 py-3">
              <Link
                to={`/procurement/orders/${row.id}`}
                className="tabular font-medium text-accent"
              >
                {row.poNumber}
              </Link>
              {row.requisitionNumber && (
                <div className="text-xs">{fromRequisition(row)}</div>
              )}
            </td>
            <td className="px-3 py-3">
              <StatusChip status={row.status} />
            </td>
            <td className="px-3 py-3">
              {row.supplierName}
              <div className="text-xs text-secondary">{row.warehouseCode}</div>
            </td>
            {/* A DATE, rendered as it arrived. Putting it through a timezone
                would move it a day for half the world (§2.5.3). */}
            <td className="tabular px-3 py-3">{row.expectedAt ?? "—"}</td>
            <td className="tabular px-3 py-3 text-right">
              {formatQty(row.qtyReceived)} / {formatQty(row.qtyOrdered)}
            </td>
            <td className="tabular px-3 py-3 text-right">
              {formatMoney(row.totalAmount)}
            </td>
          </tr>
        )}
        card={(row) => (
          <DocumentCard
            key={row.id}
            to={`/procurement/orders/${row.id}`}
            number={row.poNumber}
            caption={row.supplierName}
            chip={<StatusChip status={row.status} />}
            fields={[
              { label: "Expected", value: row.expectedAt ?? "—" },
              {
                label: "Total",
                value: formatMoney(row.totalAmount),
                align: "right",
              },
              { label: "Warehouse", value: row.warehouseCode },
              {
                label: "Received",
                value: `${formatQty(row.qtyReceived)} / ${formatQty(row.qtyOrdered)}`,
                align: "right",
              },
            ]}
            footer={fromRequisition(row)}
          />
        )}
      />
    </AppShell>
  );
}
