import { useState } from "react";
import { Link } from "react-router-dom";

import { useToast } from "../../components/Toasts";
import {
  approveRequisition,
  rejectRequisition,
  type PendingApprovalsWidget,
  type Requisition,
} from "../../lib/api";
import { formatMoney } from "../../lib/format";
import { WidgetCard, WidgetEmpty, WidgetFigure } from "./WidgetCard";

/**
 * §10.2 widget 2 — what is waiting, and for an `approver` the decision itself.
 *
 * THE POINT OF THIS WIDGET IS THAT THE DECISION HAPPENS HERE. §10.7.1 puts
 * requisition approval on a phone, between meetings: "a two-button decision".
 * Making the manager open a detail screen to press one of two buttons is what
 * this is instead of.
 *
 * Three refusals are possible and all three are the server's (I12):
 *
 *   - the caller raised it themselves       — `self_approval_forbidden` (C2)
 *   - somebody else decided it a moment ago — `state_conflict`
 *   - it names no supplier yet              — `supplier_required`
 *
 * Two of them are visible before the tap and are pre-empted below, because a
 * button that is going to fail should say so rather than fail. The third is a
 * race and can only be reported. All three still come back from the server, and
 * the toast shows whichever arrives.
 */
export function ApprovalQueue({
  widget,
  meId,
  onDecided,
}: {
  widget: PendingApprovalsWidget;
  /** The signed-in user, for the self-approval check. Compared by id rather
   *  than by name: two people can share a name and neither can share an id. */
  meId: string;
  onDecided: () => void;
}) {
  return (
    <WidgetCard
      title="Awaiting approval"
      href="/procurement/requisitions?status=submitted"
    >
      <WidgetFigure
        value={String(widget.count)}
        caption={
          widget.count === 1 ? "requisition waiting" : "requisitions waiting"
        }
      />

      {widget.count === 0 ? (
        <WidgetEmpty>Nothing is waiting on a decision.</WidgetEmpty>
      ) : !widget.canApprove ? (
        // Deliberately still showing the count. A viewer who cannot decide can
        // still see that a backlog exists, which is the difference between an
        // informative dashboard and one that hides the parts of the business a
        // reader is not personally responsible for.
        <p className="mt-4 text-sm text-secondary">
          An approver has to make these decisions.
        </p>
      ) : (
        <ul className="mt-4 space-y-3">
          {widget.queue.map((row) => (
            <QueueRow
              key={row.id}
              row={row}
              isOwn={row.requestedById === meId}
              onDecided={onDecided}
            />
          ))}
          {widget.count > widget.queue.length && (
            <li className="pt-1 text-sm text-secondary">
              and {widget.count - widget.queue.length} more.
            </li>
          )}
        </ul>
      )}
    </WidgetCard>
  );
}

/**
 * One requisition in the queue, with its two buttons.
 *
 * Rejection needs a reason (C3), so Reject opens a box rather than firing
 * immediately — the reason is mandatory in the handler *and* in the database
 * (G13), and a button that produced a 422 every time would be a worse way to
 * discover that.
 */
function QueueRow({
  row,
  isOwn,
  onDecided,
}: {
  row: Requisition;
  isOwn: boolean;
  onDecided: () => void;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");

  // Approval generates a purchase order addressed to somebody, so a requisition
  // with no supplier cannot be approved from here — there is nowhere to pick one.
  // The detail screen has the picker, so that is where the link goes.
  const needsSupplier = row.supplierId === null;
  const blocked = isOwn || needsSupplier;

  async function decide(action: "approve" | "reject") {
    setBusy(true);
    try {
      if (action === "approve") {
        const decided = await approveRequisition(row.id);
        toast.success(
          decided.purchaseOrderNumber
            ? `${row.prNumber} approved — ${decided.purchaseOrderNumber} raised.`
            : `${row.prNumber} approved.`,
        );
      } else {
        await rejectRequisition(row.id, reason.trim());
        toast.success(`${row.prNumber} rejected.`);
      }
      setRejecting(false);
      setReason("");
      onDecided();
    } catch (caught) {
      // Includes the two refusals the buttons below try to pre-empt. If one of
      // them arrives anyway, the state changed underneath this render and the
      // reload the toast does not do is the user's next tap.
      toast.failure(caught);
    } finally {
      setBusy(false);
    }
  }

  return (
    <li className="rounded-md border border-hairline p-3">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <Link
          to={`/procurement/requisitions/${row.id}`}
          className="tabular text-sm font-medium text-accent underline decoration-hairline underline-offset-2"
        >
          {row.prNumber}
        </Link>
        <span className="tabular text-sm">
          {formatMoney(row.estimatedTotal)}
        </span>
      </div>
      <p className="mt-1 text-sm text-secondary">
        {row.requestedByName} · {row.lineCount}{" "}
        {row.lineCount === 1 ? "line" : "lines"}
        {row.supplierName && ` · ${row.supplierName}`}
      </p>

      {blocked ? (
        <p className="mt-3 text-sm text-secondary">
          {isOwn
            ? "You raised this, so somebody else has to approve it."
            : "No supplier chosen yet — open it to pick one."}
        </p>
      ) : rejecting ? (
        <div className="mt-3 space-y-2">
          <label
            htmlFor={`reason-${row.id}`}
            className="block text-sm text-secondary"
          >
            Why is this being rejected?
          </label>
          <textarea
            id={`reason-${row.id}`}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            rows={2}
            className="w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm"
          />
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              disabled={busy || reason.trim() === ""}
              onClick={() => void decide("reject")}
              className="min-h-11 rounded-md bg-danger px-4 text-sm font-medium text-white disabled:opacity-50"
            >
              {busy ? "Rejecting…" : "Confirm rejection"}
            </button>
            <button
              type="button"
              onClick={() => setRejecting(false)}
              className="min-h-11 rounded-md border border-hairline px-4 text-sm"
            >
              Back
            </button>
          </div>
        </div>
      ) : (
        // Full-width buttons below `sm`: this is the phone case §10.7.1 names,
        // and both targets clear 44px in either direction.
        <div className="mt-3 flex flex-col gap-2 sm:flex-row">
          <button
            type="button"
            disabled={busy}
            onClick={() => void decide("approve")}
            className="min-h-11 flex-1 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            {busy ? "Working…" : "Approve"}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => setRejecting(true)}
            className="min-h-11 flex-1 rounded-md border border-hairline px-4 text-sm disabled:opacity-50"
          >
            Reject
          </button>
        </div>
      )}
    </li>
  );
}
