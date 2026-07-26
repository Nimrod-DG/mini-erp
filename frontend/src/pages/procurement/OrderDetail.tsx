import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { StatusChip } from "../../components/StatusChip";
import { useToast } from "../../components/Toasts";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { cancelPurchaseOrder, getPurchaseOrder } from "../../lib/api";
import { formatDateTime, formatMoney, formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";

/**
 * `/procurement/orders/:id` — ordered against received, per line (§10.3).
 *
 * Every quantity in the right-hand columns is derived: `po_line_status` sums the
 * goods receipt lines for each order line on every read (I6, G11). Nothing on this
 * screen is a stored total, which is why "ordered 40, received 25" can always be
 * traced to the receipts that say so.
 *
 * The receipt history table and the "Receive goods" action belong here and are
 * Session B of this phase, not post-MVP: the endpoint that writes those receipts
 * does not exist yet.
 */
export function OrderDetailPage() {
  const { id = "" } = useParams();
  const me = useMe();
  const toast = useToast();
  const [nonce, setNonce] = useState(0);
  const [busy, setBusy] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [reason, setReason] = useState("");

  const { state, reload } = useAsync(`order:${id}:${nonce}`, () =>
    getPurchaseOrder(id),
  );

  if (state.status === "error") {
    return (
      <AppShell title="Purchase order">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (state.status === "loading") {
    return (
      <AppShell title="Purchase order">
        <div className="h-64 animate-pulse rounded-lg border border-hairline bg-surface" />
      </AppShell>
    );
  }

  const po = state.data;
  const timezone = me.tenant?.timezone ?? "UTC";
  // `open` only, and the server says the same (G7): a partially received order has
  // goods on the shelf and a ledger that already recorded them.
  const canCancel =
    holds(me.moduleRoles, "procurement", "approver") && po.status === "open";

  function cancel() {
    setBusy(true);
    cancelPurchaseOrder(po.id, reason.trim())
      .then(() => {
        toast.success(`${po.poNumber} cancelled.`);
        setCancelling(false);
        setReason("");
        setNonce((n) => n + 1);
      })
      .catch((caught: unknown) => {
        toast.failure(caught);
      })
      .finally(() => setBusy(false));
  }

  return (
    <AppShell
      title={po.poNumber}
      actions={
        <div className="flex flex-wrap items-center gap-3">
          <StatusChip status={po.status} />
        </div>
      }
    >
      <div className="grid gap-6 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-6">
          <section className="rounded-lg border border-hairline bg-surface p-5">
            <dl className="grid gap-4 sm:grid-cols-2">
              <div>
                <dt className="text-sm text-secondary">Supplier</dt>
                <dd>
                  {po.supplierCode} — {po.supplierName}
                </dd>
              </div>
              <div>
                <dt className="text-sm text-secondary">Deliver to</dt>
                <dd>
                  {po.warehouseCode} — {po.warehouseName}
                </dd>
              </div>
              <div>
                <dt className="text-sm text-secondary">Ordered</dt>
                <dd>{formatDateTime(po.orderedAt, timezone)}</dd>
              </div>
              <div>
                <dt className="text-sm text-secondary">Expected</dt>
                <dd className="tabular">{po.expectedAt ?? "—"}</dd>
              </div>
              <div>
                <dt className="text-sm text-secondary">Raised from</dt>
                <dd>
                  {po.requisitionNumber ? (
                    <Link
                      to={`/procurement/requisitions/${po.requisitionId}`}
                      className="tabular text-accent"
                    >
                      {po.requisitionNumber}
                    </Link>
                  ) : (
                    <span className="text-secondary">no requisition</span>
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-sm text-secondary">Total</dt>
                <dd className="tabular">{formatMoney(po.totalAmount)}</dd>
              </div>
            </dl>
          </section>

          <section className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[44rem] text-left text-sm">
              <caption className="px-3 pt-3 text-left text-sm font-medium">
                Lines
              </caption>
              <thead className="text-xs uppercase tracking-wide text-secondary">
                <tr>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    #
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Product
                  </th>
                  <th scope="col" className="px-3 py-2.5 text-right font-medium">
                    Ordered
                  </th>
                  <th scope="col" className="px-3 py-2.5 text-right font-medium">
                    Received
                  </th>
                  <th scope="col" className="px-3 py-2.5 text-right font-medium">
                    Outstanding
                  </th>
                  <th scope="col" className="px-3 py-2.5 text-right font-medium">
                    Line total
                  </th>
                </tr>
              </thead>
              <tbody>
                {po.lines.map((row) => (
                  <tr key={row.id} className="border-t border-hairline">
                    <td className="px-3 py-3 tabular text-secondary">
                      {row.lineNo}
                    </td>
                    <td className="px-3 py-3">
                      <Link
                        to={`/inventory/products/${row.productId}`}
                        className="text-accent"
                      >
                        {row.sku}
                      </Link>
                      <div className="text-xs text-secondary">
                        {row.productName}
                        {row.productDeleted && " · deleted from the catalogue"}
                      </div>
                    </td>
                    <td className="px-3 py-3 text-right tabular">
                      {formatQty(row.qtyOrdered)}{" "}
                      <span className="text-secondary">{row.uom}</span>
                    </td>
                    <td className="px-3 py-3 text-right tabular">
                      {formatQty(row.qtyReceived)}
                    </td>
                    <td
                      className={`px-3 py-3 text-right tabular ${
                        row.qtyOutstanding > 0 ? "" : "text-secondary"
                      }`}
                    >
                      {formatQty(row.qtyOutstanding)}
                    </td>
                    <td className="px-3 py-3 text-right tabular">
                      {formatMoney(row.lineTotal)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        </div>

        <div className="space-y-6">
          <section className="rounded-lg border border-hairline bg-surface p-5">
            <h2 className="mb-4 text-sm font-medium">Progress</h2>
            <p className="text-sm text-secondary">
              <span className="tabular text-primary">
                {formatQty(po.qtyReceived)}
              </span>{" "}
              of <span className="tabular">{formatQty(po.qtyOrdered)}</span>{" "}
              received across {po.lineCount === 1 ? "1 line" : `${po.lineCount} lines`}.
            </p>
            <p className="mt-2 text-xs text-secondary">
              Counted from the goods receipts themselves, not from a running total
              on the order.
            </p>

            {po.cancelledAt && (
              <div className="mt-4 border-t border-hairline pt-4">
                <p className="text-sm font-medium">Cancelled</p>
                <p className="text-xs text-secondary">
                  {po.cancelledByName} · {formatDateTime(po.cancelledAt, timezone)}
                </p>
                {po.cancelReason && (
                  <p className="mt-1 text-sm">“{po.cancelReason}”</p>
                )}
              </div>
            )}
          </section>

          {canCancel && (
            <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
              <h2 className="text-sm font-medium">Actions</h2>

              {cancelling ? (
                <div className="space-y-3">
                  <label className="block">
                    <span className="mb-1 block text-sm text-secondary">
                      Reason
                    </span>
                    <textarea
                      autoFocus
                      value={reason}
                      onChange={(event) => setReason(event.target.value)}
                      rows={2}
                      placeholder="Why the order is being cancelled"
                      className="w-full rounded-md border border-hairline bg-surface px-3 py-2 text-sm"
                    />
                  </label>
                  <div className="flex flex-wrap gap-2">
                    <button
                      type="button"
                      disabled={busy || reason.trim() === ""}
                      onClick={cancel}
                      className="min-h-11 rounded-md border border-danger/40 px-4 text-sm text-danger disabled:opacity-50"
                    >
                      Cancel order
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setReason("");
                        setCancelling(false);
                      }}
                      className="min-h-11 rounded-md border border-hairline px-4 text-sm"
                    >
                      Keep it
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <button
                    type="button"
                    onClick={() => setCancelling(true)}
                    className="min-h-11 w-full rounded-md border border-danger/40 px-4 text-sm text-danger"
                  >
                    Cancel order
                  </button>
                  <p className="text-xs text-secondary">
                    Only while nothing has arrived. Once any of it has been
                    received the order cannot be cancelled — the goods are on the
                    shelf and the ledger already says so.
                  </p>
                </>
              )}
            </section>
          )}
        </div>
      </div>
    </AppShell>
  );
}
