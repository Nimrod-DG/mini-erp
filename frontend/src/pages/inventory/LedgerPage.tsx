import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
  SourceFilterNotice,
  TableHead,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import {
  listLedger,
  listWarehouses,
  type EntryType,
  type SourceType,
} from "../../lib/api";
import { formatDateTime, formatDelta, formatMoney } from "../../lib/format";

const COLUMNS = 5;

const ENTRY_TYPES: { value: EntryType | ""; label: string }[] = [
  { value: "", label: "All movements" },
  { value: "receipt", label: "Receipts" },
  { value: "issue", label: "Issues" },
  { value: "adjustment", label: "Adjustments" },
];

const SOURCE_TYPES: { value: SourceType | ""; label: string }[] = [
  { value: "", label: "All sources" },
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

  const [page, setPage] = useState(1);
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
    `ledger:${page}:${search}:${warehouseId}:${entryType}:${sourceType}:${sourceId}`,
    () =>
      listLedger({
        page,
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

  const select =
    "min-h-11 rounded-md border border-hairline bg-surface px-3 text-sm";

  return (
    <AppShell title="Stock ledger">
      <div className="mb-4 flex flex-wrap items-end gap-4">
        <label className="block max-w-xs grow">
          <span className="mb-1 block text-sm text-secondary">Search</span>
          <input
            type="search"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
            placeholder="SKU, product, or note"
            className={`${select} w-full`}
          />
        </label>

        <label className="block">
          <span className="mb-1 block text-sm text-secondary">Warehouse</span>
          <select
            value={warehouseId}
            onChange={(event) => {
              setWarehouseId(event.target.value);
              setPage(1);
            }}
            className={select}
          >
            <option value="">All warehouses</option>
            {warehouses.state.status === "ready" &&
              warehouses.state.data.data.map((warehouse) => (
                <option key={warehouse.id} value={warehouse.id}>
                  {warehouse.code}
                </option>
              ))}
          </select>
        </label>

        <label className="block">
          <span className="mb-1 block text-sm text-secondary">Movement</span>
          <select
            value={entryType}
            onChange={(event) => {
              setEntryType(event.target.value as EntryType | "");
              setPage(1);
            }}
            className={select}
          >
            {ENTRY_TYPES.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>

        <label className="block">
          <span className="mb-1 block text-sm text-secondary">Source</span>
          <select
            value={sourceType}
            onChange={(event) => {
              setSourceType(event.target.value as SourceType | "");
              setPage(1);
            }}
            className={select}
          >
            {SOURCE_TYPES.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
      </div>

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
          <div className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[48rem] text-left text-sm">
              <TableHead
                columns={[
                  { label: "When" },
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
                      className="border-t border-hairline hover:bg-raised"
                    >
                      <td className="px-3 py-3 text-secondary">
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
