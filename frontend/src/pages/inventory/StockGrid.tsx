import { useState } from "react";
import { Link } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { FilterBar, FilterDropdown, SearchInput } from "../../components/Filters";
import {
  EmptyState,
  ErrorNotice,
  frozenCell,
  Pagination,
  ScrollableTable,
  SkeletonRows,
  TableHead,
  tableRow,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { usePagination } from "../../hooks/usePagination";
import { listLowStock, listStock, listWarehouses } from "../../lib/api";
import { formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";

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
  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [search, setSearch] = useState("");
  const [warehouseId, setWarehouseId] = useState("");

  const warehouses = useAsync("stock-warehouses", () =>
    listWarehouses({ pageSize: 100, sort: "code" }),
  );

  const { state, reload } = useAsync(
    `stock:${key}:${search}:${warehouseId}`,
    () =>
      listStock({
        page,
        pageSize,
        q: search,
        warehouseId: warehouseId || undefined,
        sort: "sku",
      }),
  );

  const filtered = search !== "" || warehouseId !== "";

  return (
    <AppShell title="Stock on hand">
      <LowStockBanner />

      <FilterBar>
        <SearchInput
          label="Search stock"
          value={search}
          onChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          placeholder="SKU, product, or warehouse"
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
      </FilterBar>

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
                      className={`group ${tableRow}`}
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
              onPageSize={setPageSize}
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
 *
 * THE SHORTCUT MOVED HERE FROM THE DASHBOARD. It used to hang off the dashboard's
 * Low stock panel, which was removed as a duplicate of the "Below reorder point"
 * tile — but the shortcut is not a duplicate of anything, and it is the reason
 * the panel was worth having. A count of low products tells somebody there is a
 * problem; "Create requisition" is the thing they were going to do about it, with
 * the products and their shortfalls already on it. This is where the tile now
 * points, and where somebody looking at low stock already is, so it is arguably
 * where it always belonged.
 */
function LowStockBanner() {
  const me = useMe();
  const { state } = useAsync("low-stock", () =>
    listLowStock({ pageSize: 5, sort: "-shortfall" }),
  );

  if (state.status !== "ready" || state.data.totalItems === 0) return null;

  // Cross-module and cosmetic (I12): seeing that stock is low is Inventory,
  // asking for more of it is Procurement, and plenty of people can do one and
  // not the other — Budi in the seed is `viewer` in Inventory and `approver` in
  // Procurement, and Dewi is the other way round.
  const canRaise = holds(me.moduleRoles, "procurement", "user");

  // `<id>:<qty>` per product. The quantity is the shortfall the server computed,
  // so the shortcut produces a requisition that would actually clear the reorder
  // point rather than one for a single unit of something short by forty.
  // `String()` rather than formatQty: this lands in a numeric input, and a
  // thousands separator would make it unparseable.
  const prefill = state.data.data
    .map((row) => `${row.productId}:${String(row.shortfall)}`)
    .join(",");

  return (
    <section className="mb-6 rounded-xl border border-warning/40 bg-surface p-4">
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

      {canRaise && (
        <Link
          to={`/procurement/requisitions/new?products=${prefill}`}
          className="mt-4 inline-flex min-h-11 items-center rounded-lg border border-hairline px-4 text-sm transition-colors hover:bg-subtle"
        >
          Create requisition
        </Link>
      )}
    </section>
  );
}
