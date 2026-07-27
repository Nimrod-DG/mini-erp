import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { DocumentCard } from "../../components/CardList";
import { FilterBar, FilterDropdown, SearchInput } from "../../components/Filters";
import { tableRow } from "../../components/ListStates";
import { ResponsiveList } from "../../components/ResponsiveList";
import { StatusChip } from "../../components/StatusChip";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { usePagination } from "../../hooks/usePagination";
import {
  listRequisitions,
  listSuppliers,
  type RequisitionStatus,
} from "../../lib/api";
import { formatDateTime, formatMoney, statusLabel } from "../../lib/format";
import { holds } from "../../lib/levels";

const STATUSES: readonly RequisitionStatus[] = [
  "draft",
  "submitted",
  "approved",
  "rejected",
  "cancelled",
];

const STATUS_OPTIONS = STATUSES.map((status) => ({
  value: status,
  label: statusLabel(status),
}));

/**
 * `/procurement/requisitions` — the list, with the §10.3 status filter.
 *
 * The filter is a server parameter, not a client-side `.filter()`. Filtering the
 * fetched page would report "3 drafts" for a tenant with forty, because only 25
 * rows arrived — the count in the pagination line and the rows in the table have
 * to be answers to the same question.
 *
 * Below `md` the table becomes cards (§10.7.4). A requisition is read one at a
 * time — what it is, who raised it, how much — and it is one of the two screens
 * §10.7.1 puts on a phone. `ResponsiveList` owns that switch and the four
 * §10.7.6 states; the cells and the card fields stay here, because that is where
 * the decisions are.
 */
export function RequisitionList() {
  const me = useMe();
  const timezone = me.tenant?.timezone ?? "UTC";

  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [search, setSearch] = useState("");
  const [supplierId, setSupplierId] = useState("");

  // `?status=` seeds the filter, because the dashboard's "Awaiting approval"
  // tile links here with it set. Until this existed the tile was a broken
  // promise: it said three were waiting, and landed the reader on all thirteen.
  //
  // Seeded, not bound: changing the dropdown afterwards does not rewrite the
  // URL. Binding it would be nicer — the filter would be shareable — but it is
  // a bigger change than the bug needs, and a stale URL after the reader has
  // moved on is not a wrong answer, only a less useful one.
  const [params] = useSearchParams();
  const [status, setStatus] = useState<RequisitionStatus | "">(() => {
    const asked = params.get("status");
    // Validated against the contract, not trusted: the server rejects an
    // unknown status with a 400, so an unfiltered list beats a broken screen
    // for anybody who edits the URL by hand.
    return STATUSES.includes(asked as RequisitionStatus)
      ? (asked as RequisitionStatus)
      : "";
  });

  const canRaise = holds(me.moduleRoles, "procurement", "user");

  const { state, reload } = useAsync(
    `requisitions:${key}:${search}:${status}:${supplierId}`,
    () =>
      listRequisitions({
        page,
        pageSize,
        q: search,
        status,
        supplierId: supplierId || undefined,
        sort: "-createdAt",
      }),
  );

  // The supplier filter's options. A failure here leaves the filter empty rather
  // than breaking the list, which is the screen the user came for.
  const { state: suppliers } = useAsync("requisition-filter-suppliers", () =>
    listSuppliers({ pageSize: 100, sort: "code" }),
  );

  const newButton = canRaise ? (
    <Link
      to="/procurement/requisitions/new"
      className="inline-flex min-h-11 items-center rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      New requisition
    </Link>
  ) : null;

  /** The order approval generated, on both the row and the card. */
  const orderLink = (row: {
    purchaseOrderId: string | null;
    purchaseOrderNumber: string | null;
  }) =>
    row.purchaseOrderNumber && (
      <Link
        to={`/procurement/orders/${row.purchaseOrderId}`}
        className="tabular text-accent"
      >
        {row.purchaseOrderNumber}
      </Link>
    );

  return (
    <AppShell title="Requisitions" actions={newButton}>
      <FilterBar>
        <SearchInput
          label="Search requisitions"
          value={search}
          onChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          placeholder="Number, supplier, or notes"
        />
        <FilterDropdown
          label="Status"
          value={status}
          options={STATUS_OPTIONS}
          allLabel="All statuses"
          onChange={(next) => {
            setStatus(next as RequisitionStatus | "");
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
        firstRun="No requisitions yet. A requisition is how buying starts here: raise one, have it approved, and a purchase order is created for you."
        noResults="No requisitions match those filters."
        action={newButton}
        columns={[
          { label: "Number" },
          { label: "Status" },
          { label: "Supplier" },
          { label: "Raised by" },
          { label: "Lines", align: "right" },
          { label: "Estimated", align: "right" },
        ]}
        row={(row) => (
          <tr key={row.id} className={tableRow}>
            <td className="px-3 py-3">
              <Link
                to={`/procurement/requisitions/${row.id}`}
                className="tabular font-medium text-accent"
              >
                {row.prNumber}
              </Link>
              <div className="text-xs text-secondary">
                {formatDateTime(row.createdAt, timezone)}
              </div>
            </td>
            <td className="px-3 py-3">
              <StatusChip status={row.status} />
              {row.purchaseOrderNumber && (
                <div className="mt-1 text-xs">{orderLink(row)}</div>
              )}
            </td>
            <td className="px-3 py-3">
              {row.supplierName ?? (
                <span className="text-secondary">not chosen yet</span>
              )}
              <div className="text-xs text-secondary">{row.warehouseCode}</div>
            </td>
            <td className="px-3 py-3">{row.requestedByName}</td>
            <td className="tabular px-3 py-3 text-right">{row.lineCount}</td>
            <td className="tabular px-3 py-3 text-right">
              {formatMoney(row.estimatedTotal)}
            </td>
          </tr>
        )}
        card={(row) => (
          <DocumentCard
            key={row.id}
            to={`/procurement/requisitions/${row.id}`}
            number={row.prNumber}
            caption={formatDateTime(row.createdAt, timezone)}
            chip={<StatusChip status={row.status} />}
            fields={[
              {
                label: "Supplier",
                value: row.supplierName ?? (
                  <span className="text-secondary">not chosen yet</span>
                ),
              },
              {
                label: "Estimated",
                value: formatMoney(row.estimatedTotal),
                align: "right",
              },
              { label: "Raised by", value: row.requestedByName },
              { label: "Lines", value: row.lineCount, align: "right" },
            ]}
            footer={orderLink(row)}
          />
        )}
      />
    </AppShell>
  );
}
