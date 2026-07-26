import { Link } from "react-router-dom";

import type { LowStockWidget } from "../../lib/api";
import { formatQty } from "../../lib/format";
import { WidgetCard, WidgetEmpty, WidgetFigure } from "./WidgetCard";

/**
 * §10.2 widget 3 — products below their reorder point, and the shortcut that
 * turns the list into a requisition.
 *
 * THE SHORTCUT IS THE WIDGET'S REASON TO EXIST. A count of low products tells
 * somebody there is a problem; "Create requisition" is the thing they were going
 * to do about it, with the products already on it. The products ride in the URL
 * (`?products=<id>,<id>`) rather than in navigation state, so the link can be
 * copied, bookmarked, and reloaded — and so the create screen has one way of
 * being pre-filled rather than two.
 *
 * The count and the rows come from the same expression as
 * `/inventory/stock/low`, server-side, so "4 products are low" and the four rows
 * on that page cannot disagree.
 */
export function LowStockCard({
  widget,
  canRaise,
}: {
  widget: LowStockWidget;
  /** Whether this caller holds `user` in **procurement**. The widget itself is
   *  an Inventory one, and plenty of people can see that stock is low without
   *  being able to ask for more of it — Budi in the seed is `viewer` in
   *  Inventory and `approver` in Procurement, and Dewi is the other way round.
   *  Cosmetic: POST /procurement/requisitions refuses regardless (I12). */
  canRaise: boolean;
}) {
  // `<id>:<qty>` per product — see prefillLines. The quantity is the shortfall
  // the server computed, so the shortcut produces a requisition that would
  // actually clear the reorder point rather than one for a single unit of
  // something short by forty. `String()` rather than formatQty: this lands in a
  // numeric input, and a thousands separator would make it unparseable.
  const prefill = widget.products
    .map((row) => `${row.productId}:${String(row.shortfall)}`)
    .join(",");

  return (
    <WidgetCard title="Low stock" href="/inventory/stock">
      <WidgetFigure
        value={String(widget.count)}
        caption={
          widget.count === 1
            ? "product below its reorder point"
            : "products below their reorder point"
        }
      />

      {widget.count === 0 ? (
        <WidgetEmpty>Everything is above its reorder point.</WidgetEmpty>
      ) : (
        <>
          {/* A real table rather than a list of divs: it has columns and a
              header, and native table semantics give a screen reader the
              row/column relationship for free (§10.7.4). */}
          <table className="mt-4 w-full text-sm">
            <thead className="text-xs uppercase tracking-wide text-secondary">
              <tr>
                <th scope="col" className="py-1.5 pr-3 text-left font-medium">
                  Product
                </th>
                <th scope="col" className="py-1.5 pl-3 text-right font-medium">
                  Short by
                </th>
              </tr>
            </thead>
            <tbody>
              {widget.products.map((row) => (
                <tr key={row.productId} className="border-t border-hairline">
                  <td className="py-2 pr-3">
                    <Link
                      to={`/inventory/products/${row.productId}`}
                      className="text-accent underline decoration-hairline underline-offset-2"
                    >
                      {row.name}
                    </Link>
                    <span className="tabular ml-2 text-xs text-secondary">
                      {row.sku}
                    </span>
                  </td>
                  <td className="tabular py-2 pl-3 text-right text-warning">
                    {formatQty(row.shortfall)} {row.uom}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {widget.count > widget.products.length && (
            <p className="mt-3 text-sm text-secondary">
              and {widget.count - widget.products.length} more.
            </p>
          )}

          {canRaise && (
            <Link
              to={`/procurement/requisitions/new?products=${prefill}`}
              className="mt-4 inline-flex min-h-11 items-center rounded-md border border-hairline px-4 text-sm"
            >
              Create requisition
            </Link>
          )}
        </>
      )}
    </WidgetCard>
  );
}
