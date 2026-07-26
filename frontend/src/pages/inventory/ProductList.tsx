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
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { listProducts, type Product } from "../../lib/api";
import { formatMoney, formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";

const COLUMNS = 5;

/**
 * `/inventory/products` — the product list with current stock and the reorder
 * flag (§10.4).
 *
 * `qtyOnHand` is SUM(qty_delta) from the ledger, computed by the database on
 * this request (I6). There is no cached total anywhere between that sum and this
 * cell, which is why the number here can always be explained by the rows on
 * `/inventory/ledger`.
 */
export function ProductList() {
  const me = useMe();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [showDeleted, setShowDeleted] = useState(false);

  // The recycle bin is `admin` only (§9.0), so the toggle is hidden rather than
  // shown-and-refused. The server enforces it independently (I12).
  const canManage = holds(me.moduleRoles, "inventory", "admin");

  const { state, reload } = useAsync(
    `products:${page}:${search}:${showDeleted}`,
    () =>
      listProducts({
        page,
        q: search,
        sort: "sku",
        includeDeleted: canManage && showDeleted,
      }),
  );

  const newProduct = canManage ? (
    <Link
      to="/inventory/products/new"
      className="flex min-h-11 items-center rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      Add product
    </Link>
  ) : null;

  return (
    <AppShell title="Products" actions={newProduct}>
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
            placeholder="SKU or name"
            className="min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm"
          />
        </label>

        {canManage && (
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={showDeleted}
              onChange={(event) => {
                setShowDeleted(event.target.checked);
                setPage(1);
              }}
              className="size-4 accent-accent"
            />
            Show deleted
          </label>
        )}
      </div>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[42rem] text-left text-sm">
              <TableHead
                columns={[
                  { label: "SKU" },
                  { label: "Name" },
                  { label: "On hand", align: "right" },
                  { label: "Reorder point", align: "right" },
                  { label: "Standard cost", align: "right" },
                ]}
              />

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={search !== ""}
                  firstRun="No products yet. Add the things you buy and hold, and their stock will build itself from the ledger."
                  noResults="No products match that search."
                  action={newProduct}
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((product) => (
                    <ProductRow key={product.id} product={product} />
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

/**
 * One row. The two flags it can carry are deliberately different words, because
 * they are different facts (§6.9.1): "discontinued" means it cannot be used in
 * new documents but still counts and still shows; "deleted" means it is in the
 * recycle bin and only visible because Show deleted is on.
 */
function ProductRow({ product }: { product: Product }) {
  return (
    <tr className="border-t border-hairline hover:bg-raised">
      <td className="px-3 py-3">
        <Link
          to={`/inventory/products/${product.id}`}
          className="tabular font-medium underline decoration-hairline underline-offset-2"
        >
          {product.sku}
        </Link>
      </td>
      <td className="px-3 py-3">
        {product.name}
        <div className="flex flex-wrap gap-1.5">
          {product.deletedAt && (
            <span className="text-xs text-secondary">deleted</span>
          )}
          {!product.isActive && !product.deletedAt && (
            <span className="text-xs text-secondary">discontinued</span>
          )}
        </div>
      </td>
      <td className="px-3 py-3 text-right">
        <span className="tabular">{formatQty(product.qtyOnHand)}</span>{" "}
        <span className="text-xs text-secondary">{product.uom}</span>
        {product.belowReorderPoint && (
          <div>
            {/* whitespace-nowrap: at 360px the three words wrapped to two lines
                inside the badge's border, which read as two badges. Phase 7.5's
                finding 6. */}
            <span className="whitespace-nowrap rounded border border-warning/40 px-1.5 py-0.5 text-xs text-warning">
              below reorder point
            </span>
          </div>
        )}
      </td>
      <td className="tabular px-3 py-3 text-right">
        {formatQty(product.reorderPoint)}
      </td>
      <td className="tabular px-3 py-3 text-right">
        {formatMoney(product.standardCost)}
      </td>
    </tr>
  );
}
