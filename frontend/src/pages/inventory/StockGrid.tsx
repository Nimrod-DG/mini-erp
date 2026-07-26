import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import {
  EmptyState,
  ErrorNotice,
  frozenCell,
  Pagination,
  ScrollableTable,
  SkeletonRows,
  TableHead,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { listLowStock, listStock, listWarehouses } from "../../lib/api";
import { formatQty } from "../../lib/format";

const COLUMNS = 3;

/**
 * `/inventory/stock` — the stock-on-hand grid, product × warehouse (§10.4).
 *
 * Every cell is `SUM(qty_delta)` from `stock_balances`, computed on this
 * request. A product that has never moved has no row at all; one whose movements
 * cancel out has a row of zero, and it is shown — "we hold none of this here" is
 * an answer, and hiding it would make this screen disagree with the ledger.
 */
export function StockGrid() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [warehouseId, setWarehouseId] = useState("");

  const warehouses = useAsync("stock-warehouses", () =>
    listWarehouses({ pageSize: 100, sort: "code" }),
  );

  const { state, reload } = useAsync(
    `stock:${page}:${search}:${warehouseId}`,
    () =>
      listStock({
        page,
        q: search,
        warehouseId: warehouseId || undefined,
        sort: "sku",
      }),
  );

  const filtered = search !== "" || warehouseId !== "";

  return (
    <AppShell title="Stock on hand">
      <LowStockBanner />

      <div className="mb-4 flex flex-wrap items-end gap-4">
        <label className="block max-w-sm grow">
          <span className="mb-1 block text-sm text-secondary">Search</span>
          <input
            type="search"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
            placeholder="SKU, product, or warehouse"
            className="min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm"
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
            className="min-h-11 rounded-md border border-hairline bg-surface px-3 text-sm"
          >
            <option value="">All warehouses</option>
            {warehouses.state.status === "ready" &&
              warehouses.state.data.data.map((warehouse) => (
                <option key={warehouse.id} value={warehouse.id}>
                  {warehouse.code} — {warehouse.name}
                </option>
              ))}
          </select>
        </label>
      </div>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          {/* Horizontal scroll with a frozen first column, not cards (§10.7.4).
              This grid's power is comparing a product across warehouses, and a
              stack of cards throws that away — whereas a row of quantities whose
              SKU has scrolled out of sight is numbers about nothing. */}
          <ScrollableTable>
            <table className="w-full min-w-[28rem] table-fixed text-left text-sm sm:min-w-[34rem]">
              <colgroup>
                <col className="w-44 sm:w-60" />
                <col className="w-36 sm:w-40" />
                <col className="w-28 sm:w-32" />
              </colgroup>
              <TableHead
                sticky
                columns={[
                  { label: "Product", sticky: true },
                  { label: "Warehouse" },
                  { label: "On hand", align: "right" },
                ]}
              />

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={filtered}
                  firstRun="Nothing has moved yet. Receive a purchase order, or post an adjustment from a product, and balances will appear here."
                  noResults="No stock matches those filters."
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((cell) => (
                    <tr
                      key={`${cell.productId}:${cell.warehouseId}`}
                      className="group border-t border-hairline hover:bg-raised"
                    >
                      <td className={`px-3 py-3 align-top ${frozenCell}`}>
                        <Link
                          to={`/inventory/products/${cell.productId}`}
                          className="tabular break-words underline decoration-hairline underline-offset-2"
                        >
                          {cell.sku}
                        </Link>
                        <div className="break-words text-xs text-secondary">
                          {cell.productName}
                          {cell.productDeleted && " (deleted)"}
                        </div>
                      </td>
                      <td className="px-3 py-3 align-top">
                        <span className="tabular">{cell.warehouseCode}</span>
                        <div className="break-words text-xs text-secondary">
                          {cell.warehouseName}
                        </div>
                      </td>
                      <td className="px-3 py-3 text-right align-top">
                        <span className="tabular">
                          {formatQty(cell.qtyOnHand)}
                        </span>{" "}
                        <span className="text-xs text-secondary">
                          {cell.uom}
                        </span>
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
            />
          )}
        </>
      )}
    </AppShell>
  );
}

/**
 * Products below their reorder point, at the top of the screen where stock is
 * being looked at.
 *
 * Silent when nothing is low: a banner that is always present stops being read,
 * and this one exists to be noticed.
 */
function LowStockBanner() {
  const { state } = useAsync("low-stock", () =>
    listLowStock({ pageSize: 5, sort: "-shortfall" }),
  );

  if (state.status !== "ready" || state.data.totalItems === 0) return null;

  return (
    <section className="mb-6 rounded-lg border border-warning/40 bg-surface p-4">
      <h2 className="text-sm font-semibold text-warning">
        {state.data.totalItems === 1
          ? "1 product is below its reorder point"
          : `${state.data.totalItems} products are below their reorder point`}
      </h2>
      <ul className="mt-3 space-y-1.5 text-sm">
        {state.data.data.map((row) => (
          <li key={row.productId} className="flex flex-wrap gap-x-2">
            <Link
              to={`/inventory/products/${row.productId}`}
              className="tabular underline decoration-hairline underline-offset-2"
            >
              {row.sku}
            </Link>
            <span className="text-secondary">{row.name}</span>
            <span className="tabular ml-auto text-secondary">
              {formatQty(row.qtyOnHand)} of {formatQty(row.reorderPoint)} —{" "}
              <span className="text-warning">
                short {formatQty(row.shortfall)} {row.uom}
              </span>
            </span>
          </li>
        ))}
      </ul>
      {state.data.totalItems > state.data.data.length && (
        <p className="mt-2 text-xs text-secondary">
          Showing the {state.data.data.length} largest shortfalls.
        </p>
      )}
    </section>
  );
}
