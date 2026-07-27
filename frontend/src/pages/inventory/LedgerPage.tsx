import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { FilterBar, FilterDropdown, SearchInput } from "../../components/Filters";
import {
  EmptyState,
  ErrorNotice,
  frozenCell,
  Pagination,
  ScrollableTable,
  SkeletonRows,
  SourceFilterNotice,
  TableHead,
  tableRow,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { usePagination } from "../../hooks/usePagination";
import {
  listLedger,
  listWarehouses,
  type EntryType,
  type SourceType,
} from "../../lib/api";
import { formatDateTime, formatDelta, formatMoney } from "../../lib/format";

const COLUMNS = 5;

// The "all" row is not here: `FilterDropdown` owns it, so that every filter on
// every screen spells the unfiltered case the same way and puts it in the same
// place.
const ENTRY_TYPES: { value: EntryType; label: string }[] = [
  { value: "receipt", label: "Receipts" },
  { value: "issue", label: "Issues" },
  { value: "adjustment", label: "Adjustments" },
];

const SOURCE_TYPES: { value: SourceType; label: string }[] = [
  { value: "goods_receipt", label: "Goods receipts" },
  { value: "manual_adjustment", label: "Manual adjustments" },
];

/**
 * `/inventory/ledger` — the full, filterable ledger (§10.4).
 *
 * Append-only, and it reads that way: there is no edit and no delete on any row,
 * because a correction is a new, opposite entry (§6.9.3). Rows whose product has
 * since been deleted are still here and still name it — that is the whole reason
 * master data is soft-deleted rather than removed.
 *
 * Timestamps render in the tenant's business timezone, never the browser's
 * (I7): two colleagues in different countries must agree about which day a
 * movement fell on.
 */
export function LedgerPage() {
  const me = useMe();
  const timezone = me.tenant?.timezone ?? "UTC";

  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [search, setSearch] = useState("");
  const [warehouseId, setWarehouseId] = useState("");
  const [entryType, setEntryType] = useState<EntryType | "">("");
  const [sourceType, setSourceType] = useState<SourceType | "">("");

  // `?sourceId=` narrows the ledger to the rows one document wrote. It comes from
  // the URL rather than from a control, because it is not a filter anybody would
  // type — it is what "2 stock ledger entries created" links to on the goods
  // receipt confirmation panel, and it has to keep working when that link is
  // pasted to a colleague.
  const [params] = useSearchParams();
  const sourceId = params.get("sourceId") ?? "";

  const warehouses = useAsync("ledger-warehouses", () =>
    listWarehouses({ pageSize: 100, sort: "code" }),
  );

  const { state, reload } = useAsync(
    `ledger:${key}:${search}:${warehouseId}:${entryType}:${sourceType}:${sourceId}`,
    () =>
      listLedger({
        page,
        pageSize,
        q: search,
        warehouseId: warehouseId || undefined,
        entryType,
        sourceType,
        sourceId: sourceId || undefined,
        sort: "-occurredAt",
      }),
  );

  const filtered =
    search !== "" ||
    warehouseId !== "" ||
    entryType !== "" ||
    sourceType !== "" ||
    sourceId !== "";

  return (
    <AppShell title="Stock ledger">
      <FilterBar>
        <SearchInput
          label="Search the ledger"
          value={search}
          onChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          placeholder="SKU, product, or note"
        />
        <FilterDropdown
          label="Warehouse"
          value={warehouseId}
          allLabel="All warehouses"
          options={
            warehouses.state.status === "ready"
              ? warehouses.state.data.data.map((warehouse) => ({
                  value: warehouse.id,
                  label: `${warehouse.code} — ${warehouse.name}`,
                }))
              : []
          }
          onChange={(next) => {
            setWarehouseId(next);
            setPage(1);
          }}
        />
        <FilterDropdown
          label="Movement"
          value={entryType}
          allLabel="All movements"
          options={ENTRY_TYPES}
          onChange={(next) => {
            setEntryType(next as EntryType | "");
            setPage(1);
          }}
        />
        <FilterDropdown
          label="Source"
          value={sourceType}
          allLabel="All sources"
          options={SOURCE_TYPES}
          onChange={(next) => {
            setSourceType(next as SourceType | "");
            setPage(1);
          }}
        />
      </FilterBar>

      <SourceFilterNotice
        showing="the movements from one document"
        clearLabel="Show all movements"
        sourceNumber={
          (state.status === "ready" && state.data.data[0]?.sourceNumber) || null
        }
        onCleared={() => setPage(1)}
      />

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          {/* Horizontal scroll with a frozen first column, not cards (§10.7.4).
              A ledger is read by scanning down the times and across to what
              changed, and the moment is what identifies the row — so it is the
              moment that stays put while the rest scrolls under it. */}
          <ScrollableTable>
            <table className="w-full min-w-[48rem] text-left text-sm">
              <TableHead
                sticky
                columns={[
                  { label: "When", sticky: true },
                  { label: "Product" },
                  { label: "Warehouse" },
                  { label: "Change", align: "right" },
                  { label: "Source" },
                ]}
              />

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={filtered}
                  firstRun="The ledger is empty. Every receipt and adjustment lands here, permanently."
                  noResults="No movements match those filters."
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((entry) => (
                    <tr
                      key={entry.id}
                      className={`group ${tableRow}`}
                    >
                      <td className={`px-3 py-3 text-secondary ${frozenCell}`}>
                        {formatDateTime(entry.occurredAt, timezone)}
                      </td>
                      <td className="px-3 py-3">
                        <Link
                          to={`/inventory/products/${entry.productId}`}
                          className="tabular underline decoration-hairline underline-offset-2"
                        >
                          {entry.sku}
                        </Link>
                        <div className="text-xs text-secondary">
                          {entry.productName}
                          {/* The product was deleted after this movement. The
                              row stays, and says so, rather than vanishing. */}
                          {entry.productDeleted && " (deleted)"}
                        </div>
                      </td>
                      <td className="tabular px-3 py-3">
                        {entry.warehouseCode}
                      </td>
                      <td
                        className={`tabular px-3 py-3 text-right font-medium ${
                          entry.qtyDelta < 0 ? "text-danger" : "text-success"
                        }`}
                      >
                        {formatDelta(entry.qtyDelta)}
                        <div className="text-xs font-normal text-secondary">
                          at {formatMoney(entry.unitCost)}
                        </div>
                      </td>
                      <td className="px-3 py-3">
                        {/* A receipt names the document behind it and links to
                            the order it arrived against (§10.4). An adjustment
                            names the person, because that is its source (§6.3). */}
                        {entry.sourceNumber && entry.sourcePoId ? (
                          <Link
                            to={`/procurement/orders/${entry.sourcePoId}`}
                            className="tabular underline decoration-hairline underline-offset-2"
                          >
                            {entry.sourceNumber}
                          </Link>
                        ) : (
                          entry.entryType
                        )}
                        <div className="text-xs text-secondary">
                          {entry.note ?? entry.createdByName}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              )}
            </table>
          </ScrollableTable>

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
