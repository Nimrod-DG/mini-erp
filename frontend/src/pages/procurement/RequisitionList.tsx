import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { DocumentCard } from "../../components/CardList";
import { ResponsiveList } from "../../components/ResponsiveList";
import { StatusChip, StatusFilter } from "../../components/StatusChip";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { listRequisitions, type RequisitionStatus } from "../../lib/api";
import { formatDateTime, formatMoney } from "../../lib/format";
import { holds } from "../../lib/levels";

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

      <ResponsiveList
        state={state}
        onRetry={reload}
        onPage={setPage}
        minWidth="min-w-[52rem]"
        filtered={search !== "" || status !== ""}
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
          <tr key={row.id} className="border-t border-hairline hover:bg-raised">
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
