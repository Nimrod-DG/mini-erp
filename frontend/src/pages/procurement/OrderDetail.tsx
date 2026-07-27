import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice, TableHead } from "../../components/ListStates";
import { StatusChip } from "../../components/StatusChip";
import { useToast } from "../../components/Toasts";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import {
  cancelPurchaseOrder,
  getPurchaseOrder,
  listGoodsReceipts,
  type GoodsReceipt,
  type PurchaseOrderDetail,
  type PurchaseOrderLine,
} from "../../lib/api";
import { formatDateTime, formatMoney, formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";

/**
 * `/procurement/orders/:id` — ordered against received, per line, with the
 * receipt history and the way in to receiving more (§10.3).
 *
 * Every quantity in the right-hand columns is derived: `po_line_status` sums the
 * goods receipt lines for each order line on every read (I6, G11). Nothing on this
 * screen is a stored total, which is why "ordered 40, received 25" can always be
 * traced to the receipts listed below it — and the history is what makes that
 * traceable rather than merely true.
 */
export function OrderDetailPage() {
  const { id = "" } = useParams();
  const me = useMe();
  const [nonce, setNonce] = useState(0);

  const { state, reload } = useAsync(`order:${id}:${nonce}`, () =>
    getPurchaseOrder(id),
  );
  // The receipt history, keyed on the same nonce so cancelling or returning from
  // a receipt refreshes both halves of the screen together.
  const receipts = useAsync(`order:${id}:${nonce}:receipts`, () =>
    listGoodsReceipts({ poId: id, pageSize: 100, sort: "receivedAt" }),
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
  const isApprover = holds(me.moduleRoles, "procurement", "approver");
  // `open` only, and the server says the same (G7): a partially received order has
  // goods on the shelf and a ledger that already recorded them.
  const canCancel = isApprover && po.status === "open";
  // Receiving stays available while anything is outstanding, which is what
  // `partially_received` means. Cosmetic, like every gate in the browser: the
  // endpoint independently requires `approver` and refuses a closed order (I12).
  const canReceive =
    isApprover &&
    (po.status === "open" || po.status === "partially_received") &&
    po.qtyOutstanding > 0;

  return (
    <AppShell
      title={po.poNumber}
      actions={
        <div className="flex flex-wrap items-center gap-3">
          <StatusChip status={po.status} />
          {canReceive && (
            <Link
              to={`/procurement/orders/${po.id}/receive`}
              className="min-h-11 rounded-md bg-accent px-4 py-2.5 text-sm font-medium text-canvas"
            >
              Receive goods
            </Link>
          )}
        </div>
      }
    >
      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="min-w-0 space-y-6">
          <OrderSummary po={po} timezone={timezone} />
          <OrderLines lines={po.lines} />

          <ReceiptHistory
            receipts={
              receipts.state.status === "ready" ? receipts.state.data.data : []
            }
            loading={receipts.state.status === "loading"}
            timezone={timezone}
          />
        </div>

        <div className="min-w-0 space-y-6">
          <OrderProgress po={po} timezone={timezone} />

          {canReceive && (
            <section className="rounded-lg border border-hairline bg-surface p-5">
              <h2 className="mb-3 text-sm font-medium">Goods arriving</h2>
              <Link
                to={`/procurement/orders/${po.id}/receive`}
                className="block min-h-11 rounded-md bg-accent px-4 py-2.5 text-center text-sm font-medium text-canvas"
              >
                Receive goods
              </Link>
              <p className="mt-2 text-xs text-secondary">
                One step: the receipt, the stock, and the journal entry are
                written together or not at all.
              </p>
            </section>
          )}

          {canCancel && (
            <CancelOrderPanel
              po={po}
              onCancelled={() => setNonce((n) => n + 1)}
            />
          )}
        </div>
      </div>
    </AppShell>
  );
}

/**
 * How much of the order has arrived — every number of it derived from the receipts
 * rather than kept on the order (I6) — and the cancellation record, if there is
 * one.
 */
function OrderProgress({
  po,
  timezone,
}: {
  po: PurchaseOrderDetail;
  timezone: string;
}) {
  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="mb-4 text-sm font-medium">Progress</h2>
      <p className="text-sm text-secondary">
        <span className="tabular text-primary">
          {formatQty(po.qtyReceived)}
        </span>{" "}
        of <span className="tabular">{formatQty(po.qtyOrdered)}</span> received
        across {po.lineCount === 1 ? "1 line" : `${po.lineCount} lines`}.
      </p>
      <p className="mt-2 text-xs text-secondary">
        Counted from the goods receipts themselves, not from a running total on
        the order.
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
  );
}

/**
 * Cancelling an order, which needs a reason: the supplier has already been sent
 * it, and §6.9.2 wants who, when, and why on the record.
 *
 * It owns its own state — whether the reason box is open, what is in it, and
 * whether a request is in flight — because none of that is anything the rest of
 * the screen needs to know. The page only wants telling that something changed.
 */
function CancelOrderPanel({
  po,
  onCancelled,
}: {
  po: PurchaseOrderDetail;
  onCancelled: () => void;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);

  function cancel() {
    setBusy(true);
    cancelPurchaseOrder(po.id, reason.trim())
      .then(() => {
        toast.success(`${po.poNumber} cancelled.`);
        setOpen(false);
        setReason("");
        onCancelled();
      })
      .catch((caught: unknown) => {
        toast.failure(caught);
      })
      .finally(() => setBusy(false));
  }

  return (
    <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-sm font-medium">Actions</h2>

      {open ? (
        <div className="space-y-3">
          <label className="block">
            <span className="mb-1 block text-sm text-secondary">Reason</span>
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
                setOpen(false);
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
            onClick={() => setOpen(true)}
            className="min-h-11 w-full rounded-md border border-danger/40 px-4 text-sm text-danger"
          >
            Cancel order
          </button>
          <p className="text-xs text-secondary">
            Only while nothing has arrived. Once any of it has been received the
            order cannot be cancelled — the goods are on the shelf and the
            ledger already says so.
          </p>
        </>
      )}
    </section>
  );
}

/** Who the order is with, where it is going, and what it is worth. */
function OrderSummary({
  po,
  timezone,
}: {
  po: PurchaseOrderDetail;
  timezone: string;
}) {
  return (
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
          {/* A business date, rendered as it arrived: `YYYY-MM-DD` text, so no
              browser can move it a day (§2.5.3). */}
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
  );
}

/**
 * Ordered against received, per line. The last three columns are all derived from
 * the receipts through `po_line_status` — nothing here is a stored total (I6).
 */
function OrderLines({ lines }: { lines: PurchaseOrderLine[] }) {
  return (
    <section className="overflow-x-auto rounded-xl border border-hairline bg-surface">
      <table className="w-full min-w-[44rem] text-left text-sm">
        <caption className="px-3 pt-3 text-left text-sm font-medium">
          Lines
        </caption>
        <TableHead
          columns={[
            { label: "#" },
            { label: "Product" },
            { label: "Ordered", align: "right" },
            { label: "Received", align: "right" },
            { label: "Outstanding", align: "right" },
            { label: "Line total", align: "right" },
          ]}
        />
        <tbody>
          {lines.map((row) => (
            <tr key={row.id} className="border-t border-hairline">
              <td className="tabular px-3 py-3 text-secondary">{row.lineNo}</td>
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
              <td className="tabular px-3 py-3 text-right">
                {formatQty(row.qtyOrdered)}{" "}
                <span className="text-secondary">{row.uom}</span>
              </td>
              <td className="tabular px-3 py-3 text-right">
                {formatQty(row.qtyReceived)}
              </td>
              <td
                className={`tabular px-3 py-3 text-right ${
                  row.qtyOutstanding > 0 ? "" : "text-secondary"
                }`}
              >
                {formatQty(row.qtyOutstanding)}
              </td>
              <td className="tabular px-3 py-3 text-right">
                {formatMoney(row.lineTotal)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

/**
 * The receipt history (§10.3). This is where "received 25 of 40" stops being a
 * number on a screen and becomes a list of deliveries somebody can check: each
 * row is one goods receipt, with the value the journal entry was posted for.
 *
 * Empty is the ordinary state for a new order, and it says so rather than showing
 * an empty table with headings.
 */
function ReceiptHistory({
  receipts,
  loading,
  timezone,
}: {
  receipts: GoodsReceipt[];
  loading: boolean;
  timezone: string;
}) {
  if (loading) {
    return (
      <div className="h-28 animate-pulse rounded-lg border border-hairline bg-surface" />
    );
  }
  if (receipts.length === 0) {
    return (
      <section className="rounded-lg border border-hairline bg-surface p-5">
        <h2 className="text-sm font-medium">Receipts</h2>
        <p className="mt-1 text-sm text-secondary">
          Nothing has arrived against this order yet.
        </p>
      </section>
    );
  }

  return (
    <section className="overflow-x-auto rounded-xl border border-hairline bg-surface">
      <table className="w-full min-w-[36rem] text-left text-sm">
        <caption className="px-3 pt-3 text-left text-sm font-medium">
          Receipts
        </caption>
        <TableHead
          columns={[
            { label: "Receipt" },
            { label: "When" },
            { label: "Received by" },
            { label: "Lines", align: "right" },
            { label: "Quantity", align: "right" },
            { label: "Value", align: "right" },
          ]}
        />
        <tbody>
          {receipts.map((receipt) => (
            <tr key={receipt.id} className="border-t border-hairline">
              <td className="px-3 py-3">
                {/* The ledger rows this receipt wrote — the same link the
                    confirmation panel offers, reachable again later. */}
                <Link
                  to={`/inventory/ledger?sourceId=${receipt.id}`}
                  className="tabular text-accent"
                >
                  {receipt.grNumber}
                </Link>
                {receipt.note && (
                  <div className="text-xs text-secondary">“{receipt.note}”</div>
                )}
              </td>
              <td className="px-3 py-3 text-secondary">
                {formatDateTime(receipt.receivedAt, timezone)}
              </td>
              <td className="px-3 py-3">{receipt.receivedByName}</td>
              <td className="tabular px-3 py-3 text-right">
                {receipt.lineCount}
              </td>
              <td className="tabular px-3 py-3 text-right">
                {formatQty(receipt.qtyReceived)}
              </td>
              <td className="tabular px-3 py-3 text-right">
                {formatMoney(receipt.totalValue)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
