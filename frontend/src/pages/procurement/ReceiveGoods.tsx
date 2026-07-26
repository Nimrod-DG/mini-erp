import { useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice, TableHead } from "../../components/ListStates";
import { StickyActions } from "../../components/StickyActions";
import { useToast } from "../../components/Toasts";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import {
  getPurchaseOrder,
  postGoodsReceipt,
  type PurchaseOrderLine,
  type ReceiptResult,
} from "../../lib/api";
import { formatMoney, formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";

/**
 * `/procurement/orders/:id/receive` — the goods receipt form (§10.3, §8.4).
 *
 * THE IDEMPOTENCY KEY IS GENERATED WHEN THIS SCREEN MOUNTS, not when the button
 * is pressed (§8.6.1). That is the whole mechanism: a receipt is posted from a
 * phone at a loading dock, and a request that times out client-side but succeeded
 * server-side is an ordinary Tuesday there. If the key were minted per submit,
 * every retry would be a new receipt — stock credited twice, with a second
 * journal entry to match, and nothing in the schema to flag it.
 *
 * Quantities are held and sent as **strings**. A quantity that passes through a
 * JavaScript number on the way out has already lost whatever it is going to lose
 * before the server's NUMERIC ever sees it (I8).
 *
 * The client-side outstanding check is a courtesy, not the rule: the server
 * refuses over-receipt with `422 over_receipt` computed against `po_line_status`
 * under a row lock, and the database refuses it again in a trigger (§6.10.6). Two
 * receipts posted at once can each pass this check and still jointly over-receive,
 * which is exactly why the real check is not here (I12).
 */
export function ReceiveGoodsPage() {
  const { id = "" } = useParams();
  const me = useMe();
  const toast = useToast();

  // One key per mounted form, held in a ref so a re-render cannot change it. This
  // is what makes the retry safe; see the note above.
  const idempotencyKey = useRef(crypto.randomUUID()).current;

  const [quantities, setQuantities] = useState<Record<string, string>>({});
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [posted, setPosted] = useState<ReceiptResult | null>(null);

  const { state, reload } = useAsync(`receive:${id}`, () =>
    getPurchaseOrder(id),
  );

  if (state.status === "error") {
    return (
      <AppShell title="Receive goods">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (state.status === "loading") {
    return (
      <AppShell title="Receive goods">
        <div className="h-64 animate-pulse rounded-lg border border-hairline bg-surface" />
      </AppShell>
    );
  }

  const po = state.data;

  // The confirmation panel replaces the form once the receipt is posted. It is
  // the one screen in this application where boldness is budgeted (§10.3).
  if (posted) {
    return (
      <AppShell title="Receive goods">
        <ReceiptConfirmation result={posted} />
      </AppShell>
    );
  }

  const receivable = po.status === "open" || po.status === "partially_received";
  const canReceive = holds(me.moduleRoles, "procurement", "approver");

  if (!receivable || !canReceive) {
    return (
      <AppShell title="Receive goods">
        <div className="rounded-lg border border-hairline bg-surface p-5">
          <p className="text-sm">
            {receivable
              ? "Receiving goods needs the approver level in procurement."
              : `${po.poNumber} is ${po.status.replace("_", " ")}, so nothing more can be received against it.`}
          </p>
          <Link
            to={`/procurement/orders/${po.id}`}
            className="mt-4 inline-block text-sm text-accent"
          >
            Back to {po.poNumber}
          </Link>
        </div>
      </AppShell>
    );
  }

  return (
    <ReceiveForm
      po={po}
      note={note}
      busy={busy}
      quantities={quantities}
      onNote={setNote}
      onQuantity={(lineId, value) =>
        setQuantities((current) => ({ ...current, [lineId]: value }))
      }
      onSubmit={(lines) => {
        setBusy(true);
        postGoodsReceipt(po.id, idempotencyKey, lines, note.trim() || undefined)
          .then((result) => {
            setPosted(result);
            // A replay is a success the user should still be told about: their
            // first tap worked, and nothing was written twice.
            toast.success(
              result.replayed
                ? `${result.receipt.grNumber} had already been posted.`
                : `${result.receipt.grNumber} posted.`,
            );
          })
          .catch((caught: unknown) => toast.failure(caught))
          .finally(() => setBusy(false));
      }}
    />
  );
}

type ReceiveFormProps = {
  po: Awaited<ReturnType<typeof getPurchaseOrder>>;
  quantities: Record<string, string>;
  note: string;
  busy: boolean;
  onQuantity: (lineId: string, value: string) => void;
  onNote: (value: string) => void;
  onSubmit: (lines: { poLineId: string; qtyReceived: string }[]) => void;
};

/**
 * The form itself. Each line defaults to its outstanding quantity, which is the
 * overwhelmingly common case — a full delivery — while leaving every box editable
 * for a partial one. A blank or zero box means "none of this arrived", and that
 * line is simply not sent.
 */
function ReceiveForm({
  po,
  quantities,
  note,
  busy,
  onQuantity,
  onNote,
  onSubmit,
}: ReceiveFormProps) {
  const outstanding = useMemo(
    () => po.lines.filter((line) => line.qtyOutstanding > 0),
    [po.lines],
  );

  // Read once per render from the boxes, so the button's disabled state and what
  // gets posted are decided by the same expression.
  const entered = outstanding
    .map((line) => ({
      line,
      raw: (quantities[line.id] ?? String(line.qtyOutstanding)).trim(),
    }))
    .filter((row) => row.raw !== "" && Number(row.raw) > 0);

  const overEntered = entered.filter(
    (row) => Number(row.raw) > row.line.qtyOutstanding,
  );
  const unparseable = entered.filter(
    (row) => !Number.isFinite(Number(row.raw)),
  );

  return (
    <AppShell
      title={`Receive against ${po.poNumber}`}
      actions={
        <Link
          to={`/procurement/orders/${po.id}`}
          className="text-sm text-secondary"
        >
          Back to the order
        </Link>
      }
    >
      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="min-w-0 space-y-6">
          <section className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[40rem] text-left text-sm">
              <caption className="px-3 pt-3 text-left text-sm font-medium">
                What arrived
              </caption>
              <TableHead
                columns={[
                  { label: "#" },
                  { label: "Product" },
                  { label: "Outstanding", align: "right" },
                  { label: "Receiving", align: "right" },
                ]}
              />
              <tbody>
                {outstanding.map((line) => (
                  <ReceiveRow
                    key={line.id}
                    line={line}
                    value={quantities[line.id] ?? String(line.qtyOutstanding)}
                    onChange={(value) => onQuantity(line.id, value)}
                  />
                ))}
              </tbody>
            </table>
          </section>

          <label className="block">
            <span className="mb-1 block text-sm text-secondary">
              Note (optional)
            </span>
            <textarea
              value={note}
              onChange={(event) => onNote(event.target.value)}
              rows={2}
              placeholder="Delivered by supplier truck, 2 boxes"
              className="w-full rounded-md border border-hairline bg-surface px-3 py-2 text-sm"
            />
          </label>
        </div>

        <div className="min-w-0 space-y-6">
          <section className="rounded-lg border border-hairline bg-surface p-5">
            <h2 className="mb-3 text-sm font-medium">This will</h2>
            <ul className="space-y-2 text-sm text-secondary">
              <li>
                record{" "}
                <span className="text-primary">
                  {entered.length === 1 ? "1 line" : `${entered.length} lines`}
                </span>{" "}
                against {po.poNumber}
              </li>
              <li>
                add stock to{" "}
                <span className="tabular text-primary">{po.warehouseCode}</span>
              </li>
              <li>post one balanced journal entry in finance</li>
            </ul>
            <p className="mt-3 text-xs text-secondary">
              All of it in one transaction: if any part fails, none of it
              happened.
            </p>

            {overEntered.length > 0 && (
              <p className="mt-4 text-sm text-warning">
                {overEntered.length === 1 ? "One line has" : "Some lines have"}{" "}
                more entered than is outstanding. The server will refuse it —
                raise a second order for the excess.
              </p>
            )}

          </section>

          {/* §10.7.5's sticky bar, and this is the screen that clause names.
              On a phone the line table is two screens tall and this button was
              underneath all of it — on the one flow §10.7.1 calls genuinely
              mobile, performed one-handed at a loading dock. */}
          <StickyActions
            hint="Safe to retry: this form carries one key, so posting twice cannot receive the goods twice."
          >
            <button
              type="button"
              disabled={
                busy ||
                entered.length === 0 ||
                overEntered.length > 0 ||
                unparseable.length > 0
              }
              onClick={() =>
                onSubmit(
                  entered.map((row) => ({
                    poLineId: row.line.id,
                    qtyReceived: row.raw,
                  })),
                )
              }
              className="min-h-12 w-full rounded-md bg-accent px-4 text-sm font-medium text-canvas disabled:opacity-50"
            >
              {busy ? "Posting…" : "Post receipt"}
            </button>
          </StickyActions>
        </div>
      </div>
    </AppShell>
  );
}

function ReceiveRow({
  line,
  value,
  onChange,
}: {
  line: PurchaseOrderLine;
  value: string;
  onChange: (value: string) => void;
}) {
  const over = Number(value) > line.qtyOutstanding;

  return (
    <tr className="border-t border-hairline">
      <td className="tabular px-3 py-3 text-secondary">{line.lineNo}</td>
      <td className="px-3 py-3">
        <span className="tabular">{line.sku}</span>
        <div className="text-xs text-secondary">
          {line.productName}
          {line.productDeleted && " · deleted from the catalogue"}
        </div>
      </td>
      <td className="tabular px-3 py-3 text-right">
        {formatQty(line.qtyOutstanding)}{" "}
        <span className="text-secondary">{line.uom}</span>
      </td>
      <td className="px-3 py-3 text-right">
        <input
          // `inputMode` rather than `type="number"`: a number input on a phone
          // brings the right keyboard but also lets the browser reformat the
          // value, and this string goes to a NUMERIC(18,4) unchanged (I8).
          inputMode="decimal"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          aria-label={`Quantity received for line ${line.lineNo}`}
          aria-invalid={over}
          className={`tabular min-h-11 w-28 rounded-md border bg-surface px-3 text-right text-sm ${
            over ? "border-warning" : "border-hairline"
          }`}
        />
      </td>
    </tr>
  );
}

/**
 * THE CONFIRMATION PANEL (§10.3). This is the screenshot the project exists for:
 * one business event, named in the words of all three modules it touched.
 *
 * Every number and identifier here comes from the server's response, which was
 * itself rebuilt from the committed rows — so the panel cannot claim something the
 * database does not say happened.
 *
 * Both lines link, as §10.3 asks: the inventory line to the ledger rows this
 * receipt wrote, the finance line to the journal entry it posted. Each lands on
 * a list filtered to this one document, so every claim the panel makes is one
 * click from the rows that back it.
 */
function ReceiptConfirmation({ result }: { result: ReceiptResult }) {
  const { receipt, purchaseOrder, inventory, finance } = result;

  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-success/40 bg-surface p-6">
        <p className="text-lg font-medium">
          Goods receipt <span className="tabular">{receipt.grNumber}</span>{" "}
          {result.replayed ? "was already posted." : "posted."}
        </p>
        <p className="mt-1 text-sm text-secondary">
          {purchaseOrder.poNumber} is now{" "}
          {purchaseOrder.status === "received"
            ? "fully received"
            : "partly received"}
          .
        </p>

        <ul className="mt-5 space-y-3 text-sm">
          <li className="flex flex-wrap items-baseline gap-2">
            <span className="text-secondary">→ Inventory:</span>
            <Link
              to={`/inventory/ledger?sourceId=${receipt.id}`}
              className="text-accent underline decoration-hairline underline-offset-2"
            >
              {inventory.entryCount === 1
                ? "1 stock ledger entry"
                : `${inventory.entryCount} stock ledger entries`}{" "}
              created
            </Link>
            <span className="text-secondary">in {receipt.warehouseCode}</span>
          </li>
          <li className="flex flex-wrap items-baseline gap-2">
            <span className="text-secondary">→ Finance:</span>
            {/* The counterpart of the inventory line above, closed in Phase 6
                when `/finance` came to exist (§10.5). Both lines of §10.3's
                panel now link, and both land on a filtered list showing exactly
                what this receipt wrote — the claim and the evidence are one
                click apart on each side. */}
            <Link
              to={`/finance?sourceId=${receipt.id}`}
              className="text-accent underline decoration-hairline underline-offset-2"
            >
              journal entry{" "}
              <span className="tabular">{finance.entryNumber}</span> posted
            </Link>
            <span className="tabular text-secondary">
              (Dr {finance.debitAccountName} {formatMoney(finance.amount)} / Cr{" "}
              {finance.creditAccountName} {formatMoney(finance.amount)})
            </span>
          </li>
        </ul>

        {result.replayed && (
          <p className="mt-4 text-xs text-secondary">
            This form had already been posted, so nothing was written a second
            time — the numbers above are the original receipt's.
          </p>
        )}
      </section>

      <section className="overflow-x-auto rounded-lg border border-hairline bg-surface">
        <table className="w-full min-w-[36rem] text-left text-sm">
          <caption className="px-3 pt-3 text-left text-sm font-medium">
            Received
          </caption>
          <TableHead
            columns={[
              { label: "Product" },
              { label: "Quantity", align: "right" },
              { label: "At", align: "right" },
              { label: "Value", align: "right" },
            ]}
          />
          <tbody>
            {receipt.lines.map((row) => (
              <tr key={row.id} className="border-t border-hairline">
                <td className="px-3 py-3">
                  <Link
                    to={`/inventory/products/${row.productId}`}
                    className="tabular text-accent"
                  >
                    {row.sku}
                  </Link>
                  <div className="text-xs text-secondary">
                    {row.productName}
                  </div>
                </td>
                <td className="tabular px-3 py-3 text-right">
                  {formatQty(row.qtyReceived)}{" "}
                  <span className="text-secondary">{row.uom}</span>
                </td>
                <td className="tabular px-3 py-3 text-right">
                  {formatMoney(row.unitCost)}
                </td>
                <td className="tabular px-3 py-3 text-right">
                  {formatMoney(row.lineTotal)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <div className="flex flex-wrap gap-3">
        <Link
          to={`/procurement/orders/${purchaseOrder.id}`}
          className="min-h-11 rounded-md border border-hairline px-4 py-2.5 text-sm"
        >
          Back to {purchaseOrder.poNumber}
        </Link>
        <Link
          to="/inventory/stock"
          className="min-h-11 rounded-md border border-hairline px-4 py-2.5 text-sm"
        >
          See stock on hand
        </Link>
      </div>
    </div>
  );
}
