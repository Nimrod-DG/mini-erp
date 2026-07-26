import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice, TableHead } from "../../components/ListStates";
import { StatusChip } from "../../components/StatusChip";
import { useToast } from "../../components/Toasts";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import {
  approveRequisition,
  cancelRequisition,
  getRequisition,
  listSuppliers,
  patchRequisition,
  rejectRequisition,
  submitRequisition,
  type RequisitionDetail as Detail,
} from "../../lib/api";
import { formatDateTime, formatMoney, formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";
import {
  emptyLine,
  usableLines,
  useRequisitionPickers,
  type RequisitionFormValues,
} from "../../lib/requisitionForm";
import { RequisitionFields } from "./RequisitionFields";

const field =
  "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";

/**
 * `/procurement/requisitions/:id` — lines, the status timeline, and the actions
 * this reader is actually allowed to take (§10.3).
 *
 * WHICH ACTIONS SHOW IS COSMETIC (I12). Every one of them is independently
 * enforced: `approve` and `reject` are `approver` routes, editing and submitting
 * are the author's alone, and the self-approval rule is checked against the row on
 * the server whatever this screen renders. Hiding the Approve button on your own
 * requisition is a courtesy, not the rule — the rule is C2.
 */
export function RequisitionDetailPage() {
  const { id = "" } = useParams();
  const me = useMe();
  const toast = useToast();
  const [nonce, setNonce] = useState(0);
  const [busy, setBusy] = useState(false);
  // A draft opens as something you read, with an explicit Edit — not as a live
  // form. Phase 4's product detail did the opposite and the walkthrough found it
  // confusing: there was no point at which you had committed to changing
  // something.
  const [editing, setEditing] = useState(false);

  const { state, reload } = useAsync(`requisition:${id}:${nonce}`, () =>
    getRequisition(id),
  );

  // Only loaded when it is needed: a requisition with no supplier that this user
  // could approve. Every other reader is spared the request.
  const { state: supplierState } = useAsync(
    `approval-suppliers:${holds(me.moduleRoles, "procurement", "approver")}`,
    async () =>
      holds(me.moduleRoles, "procurement", "approver")
        ? (await listSuppliers({ pageSize: 100, sort: "code" })).data
        : [],
  );

  async function run(action: () => Promise<unknown>, confirmation: string) {
    setBusy(true);
    try {
      await action();
      toast.success(confirmation);
      setNonce((n) => n + 1);
    } catch (caught) {
      // Every refusal this screen can raise is one the server owns: a
      // state_conflict because somebody else decided first, a
      // self_approval_forbidden, a reason_required. The envelope's sentence is
      // written to be shown.
      toast.failure(caught);
    } finally {
      setBusy(false);
    }
  }

  if (state.status === "error") {
    return (
      <AppShell title="Requisition">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (state.status === "loading") {
    return (
      <AppShell title="Requisition">
        <div className="h-64 animate-pulse rounded-lg border border-hairline bg-surface" />
      </AppShell>
    );
  }

  const pr = state.data;
  const timezone = me.tenant?.timezone ?? "UTC";
  const suppliers = supplierState.status === "ready" ? supplierState.data : [];

  // The §6.9.2 and §8.2 rules, as this screen reads them. Cosmetic — the server
  // decides each of them again, against the row (I12).
  const isAuthor = pr.requestedById === me.user.id;
  const isApprover = holds(me.moduleRoles, "procurement", "approver");
  const canEdit = isAuthor && pr.status === "draft";
  const canSubmit = canEdit;
  const canDecide = isApprover && pr.status === "submitted" && !isAuthor;
  const canCancel =
    (pr.status === "draft" && isAuthor) ||
    (pr.status === "submitted" && (isAuthor || isApprover));

  if (editing && canEdit) {
    return (
      <EditDraft
        pr={pr}
        onDone={(changed) => {
          setEditing(false);
          if (changed) setNonce((n) => n + 1);
        }}
      />
    );
  }

  return (
    <AppShell
      title={pr.prNumber}
      actions={
        <div className="flex flex-wrap items-center gap-3">
          <StatusChip status={pr.status} />
          {canEdit && (
            <button
              type="button"
              onClick={() => setEditing(true)}
              className="min-h-11 rounded-md border border-hairline px-4 text-sm"
            >
              Edit
            </button>
          )}
          {pr.purchaseOrderNumber && (
            <Link
              to={`/procurement/orders/${pr.purchaseOrderId}`}
              className="inline-flex min-h-11 items-center rounded-md bg-accent px-4 text-sm font-medium text-white"
            >
              View {pr.purchaseOrderNumber}
            </Link>
          )}
        </div>
      }
    >
      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="min-w-0 space-y-6">
          <Summary pr={pr} />
          <LinesTable pr={pr} />
        </div>

        <div className="min-w-0 space-y-6">
          <Timeline pr={pr} timezone={timezone} />
          <ActionsPanel
            pr={pr}
            suppliers={suppliers}
            busy={busy}
            run={run}
            rights={{ isAuthor, canSubmit, canDecide, canCancel, isApprover }}
          />

          {pr.status === "approved" && (
            <p className="rounded-lg border border-hairline bg-surface p-4 text-sm text-secondary">
              An approved requisition cannot be cancelled. Cancel the purchase
              order instead — the goods, not the paperwork, are what is
              committed.
            </p>
          )}
        </div>
      </div>
    </AppShell>
  );
}

/** What this reader may do to this document, decided once and passed down rather
 *  than re-derived per button. */
type Rights = {
  isAuthor: boolean;
  isApprover: boolean;
  canSubmit: boolean;
  canDecide: boolean;
  canCancel: boolean;
};

/**
 * The actions panel.
 *
 * Every branch here is cosmetic (I12): the server independently refuses a submit
 * from a non-author, a decision from below `approver`, a self-approval (C2), and a
 * cancel of anything past `submitted` (G8). What this component decides is what to
 * *show* — and, in one case, what to say instead.
 */
function ActionsPanel({
  pr,
  suppliers,
  busy,
  run,
  rights,
}: {
  pr: Detail;
  suppliers: { id: string; code: string; name: string }[];
  busy: boolean;
  run: (action: () => Promise<unknown>, confirmation: string) => void;
  rights: Rights;
}) {
  const { isAuthor, isApprover, canSubmit, canDecide, canCancel } = rights;
  if (!canSubmit && !canDecide && !canCancel) return null;

  return (
    <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-sm font-medium">Actions</h2>

      {canSubmit && (
        <>
          <button
            type="button"
            disabled={busy || pr.lineCount === 0}
            onClick={() =>
              run(
                () => submitRequisition(pr.id),
                `${pr.prNumber} submitted for approval.`,
              )
            }
            className="min-h-11 w-full rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            Submit for approval
          </button>
          <p className="text-xs text-secondary">
            Once submitted it cannot be edited — an approver has to be looking
            at the same document you sent.
          </p>
        </>
      )}

      {canDecide && (
        <ApproveAndReject
          pr={pr}
          suppliers={suppliers}
          busy={busy}
          onApprove={(supplierId) =>
            run(
              () => approveRequisition(pr.id, supplierId),
              `${pr.prNumber} approved. A purchase order has been created.`,
            )
          }
          onReject={(reason) =>
            run(
              () => rejectRequisition(pr.id, reason),
              `${pr.prNumber} rejected.`,
            )
          }
        />
      )}

      {/* An approver looking at their own requisition: the buttons are absent,
          and the reason is said out loud rather than left as an apparently
          broken screen. */}
      {isAuthor && isApprover && pr.status === "submitted" && (
        <p className="text-sm text-secondary">
          You raised this requisition, so somebody else has to approve it.
        </p>
      )}

      {canCancel && (
        <ReasonAction
          actionLabel="Cancel requisition"
          placeholder="Why it is being cancelled"
          danger
          busy={busy}
          onSubmit={(reason) =>
            run(
              () => cancelRequisition(pr.id, reason),
              `${pr.prNumber} cancelled.`,
            )
          }
        />
      )}
    </section>
  );
}

/** Who it is for, where it is going, and what it comes to. */
function Summary({ pr }: { pr: Detail }) {
  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <dl className="grid gap-4 sm:grid-cols-2">
        <div>
          <dt className="text-sm text-secondary">Supplier</dt>
          <dd>
            {pr.supplierName ?? (
              <span className="text-secondary">not chosen yet</span>
            )}
          </dd>
        </div>
        <div>
          <dt className="text-sm text-secondary">Deliver to</dt>
          <dd>
            {pr.warehouseCode} — {pr.warehouseName}
          </dd>
        </div>
        <div>
          <dt className="text-sm text-secondary">Raised by</dt>
          <dd>{pr.requestedByName}</dd>
        </div>
        <div>
          <dt className="text-sm text-secondary">Estimated total</dt>
          <dd className="tabular">{formatMoney(pr.estimatedTotal)}</dd>
        </div>
        {pr.notes && (
          <div className="sm:col-span-2">
            <dt className="text-sm text-secondary">Notes</dt>
            <dd className="whitespace-pre-wrap">{pr.notes}</dd>
          </div>
        )}
      </dl>
    </section>
  );
}

/** What was asked for. Every total on it was computed by PostgreSQL (I8). */
function LinesTable({ pr }: { pr: Detail }) {
  return (
    <section className="overflow-x-auto rounded-lg border border-hairline bg-surface">
      <table className="w-full min-w-[36rem] text-left text-sm">
        <caption className="px-3 pt-3 text-left text-sm font-medium">
          Lines
        </caption>
        <TableHead
          columns={[
            { label: "#" },
            { label: "Product" },
            { label: "Quantity", align: "right" },
            { label: "Unit cost", align: "right" },
            { label: "Line total", align: "right" },
          ]}
        />
        <tbody>
          {pr.lines.length === 0 && (
            <tr className="border-t border-hairline">
              <td
                colSpan={5}
                className="px-3 py-8 text-center text-sm text-secondary"
              >
                No lines yet. A requisition needs at least one to be submitted.
              </td>
            </tr>
          )}
          {pr.lines.map((row) => (
            <tr key={row.id} className="border-t border-hairline">
              <td className="px-3 py-3 tabular text-secondary">{row.lineNo}</td>
              <td className="px-3 py-3">
                <Link
                  to={`/inventory/products/${row.productId}`}
                  className="text-accent"
                >
                  {row.sku}
                </Link>
                <div className="text-xs text-secondary">
                  {row.productName}
                  {/* The product has since been deleted. The line stays —
                      saying why is the point of the flag (§6.9.1). */}
                  {row.productDeleted && " · deleted from the catalogue"}
                </div>
              </td>
              <td className="px-3 py-3 text-right tabular">
                {formatQty(row.qty)}{" "}
                <span className="text-secondary">{row.uom}</span>
              </td>
              <td className="px-3 py-3 text-right tabular">
                {formatMoney(row.estUnitCost)}
              </td>
              <td className="px-3 py-3 text-right tabular">
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
 * Editing a draft in place, with the same form `/new` uses.
 *
 * `lines` is sent whole and replaces the set (the API's own contract), which is
 * why the editor starts from the lines the document actually has rather than from
 * a blank row: sending a partial set would delete the rest.
 *
 * Only drafts reach here, and only their author — both cosmetically, by `canEdit`,
 * and for real, on the server, which refuses anything else with `state_conflict`
 * or `forbidden` (C5).
 */
function EditDraft({
  pr,
  onDone,
}: {
  pr: Detail;
  onDone: (changed: boolean) => void;
}) {
  const toast = useToast();
  const [saving, setSaving] = useState(false);
  const [values, setValues] = useState<RequisitionFormValues>({
    warehouseId: pr.warehouseId,
    supplierId: pr.supplierId ?? "",
    notes: pr.notes ?? "",
    lines:
      pr.lines.length > 0
        ? pr.lines.map((row) => ({
            productId: row.productId,
            qty: String(row.qty),
            estUnitCost: String(row.estUnitCost),
          }))
        : [{ ...emptyLine }],
  });

  const { state, reload } = useRequisitionPickers(`requisition-edit:${pr.id}`);

  if (state.status === "error") {
    return (
      <AppShell title={`Edit ${pr.prNumber}`}>
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (state.status === "loading") {
    return (
      <AppShell title={`Edit ${pr.prNumber}`}>
        <div className="h-64 animate-pulse rounded-lg border border-hairline bg-surface" />
      </AppShell>
    );
  }

  return (
    <AppShell title={`Edit ${pr.prNumber}`}>
      <form
        className="max-w-3xl space-y-6"
        onSubmit={(event) => {
          event.preventDefault();
          setSaving(true);
          patchRequisition(pr.id, {
            warehouseId: values.warehouseId,
            supplierId: values.supplierId,
            notes: values.notes.trim(),
            lines: usableLines(values.lines),
          })
            .then(() => {
              toast.success("Changes saved.");
              onDone(true);
            })
            .catch((caught: unknown) => {
              toast.failure(caught);
            })
            .finally(() => setSaving(false));
        }}
      >
        <RequisitionFields
          pickers={state.data}
          values={values}
          onChange={setValues}
        />

        <div className="flex flex-wrap gap-3">
          <button
            type="submit"
            disabled={saving || values.warehouseId === ""}
            className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            {saving ? "Saving…" : "Save changes"}
          </button>
          <button
            type="button"
            onClick={() => onDone(false)}
            className="min-h-11 rounded-md border border-hairline px-4 text-sm"
          >
            Cancel
          </button>
        </div>
      </form>
    </AppShell>
  );
}

/**
 * The status timeline (§10.3): what happened, who did it, and when — in the
 * tenant's timezone, never the browser's (I7).
 *
 * Only steps that actually happened are shown. A greyed-out "Approved" on a
 * rejected requisition would read as a step still to come.
 */
function Timeline({ pr, timezone }: { pr: Detail; timezone: string }) {
  const steps: {
    label: string;
    who?: string;
    when: string;
    detail?: string;
  }[] = [{ label: "Raised", who: pr.requestedByName, when: pr.createdAt }];
  if (pr.submittedAt) {
    steps.push({
      label: "Submitted",
      who: pr.requestedByName,
      when: pr.submittedAt,
    });
  }
  if (pr.decidedAt) {
    steps.push({
      label: pr.status === "rejected" ? "Rejected" : "Approved",
      who: pr.decidedByName ?? undefined,
      when: pr.decidedAt,
      detail: pr.rejectReason ?? undefined,
    });
  }
  if (pr.cancelledAt) {
    steps.push({
      label: "Cancelled",
      who: pr.cancelledByName ?? undefined,
      when: pr.cancelledAt,
      detail: pr.cancelReason ?? undefined,
    });
  }

  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="mb-4 text-sm font-medium">History</h2>
      <ol className="space-y-4">
        {steps.map((step) => (
          <li key={step.label} className="border-l-2 border-hairline pl-3">
            <p className="text-sm font-medium">{step.label}</p>
            <p className="text-xs text-secondary">
              {step.who && <>{step.who} · </>}
              {formatDateTime(step.when, timezone)}
            </p>
            {step.detail && <p className="mt-1 text-sm">“{step.detail}”</p>}
          </li>
        ))}
      </ol>
    </section>
  );
}

/** Approve and reject, side by side, because they are the same decision with two
 *  answers — and rejecting needs a reason (C3) while approving may need a
 *  supplier (§8.3). */
function ApproveAndReject({
  pr,
  suppliers,
  busy,
  onApprove,
  onReject,
}: {
  pr: Detail;
  suppliers: { id: string; code: string; name: string }[];
  busy: boolean;
  onApprove: (supplierId?: string) => void;
  onReject: (reason: string) => void;
}) {
  const [supplierId, setSupplierId] = useState("");

  return (
    <div className="space-y-4">
      {pr.supplierId === null && (
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">
            Supplier for this order
          </span>
          <select
            value={supplierId}
            onChange={(event) => setSupplierId(event.target.value)}
            className={field}
          >
            <option value="">Choose a supplier…</option>
            {suppliers.map((supplier) => (
              <option key={supplier.id} value={supplier.id}>
                {supplier.code} — {supplier.name}
              </option>
            ))}
          </select>
          <span className="mt-1 block text-xs text-secondary">
            This requisition names none, and a purchase order has to be
            addressed to somebody.
          </span>
        </label>
      )}

      <button
        type="button"
        disabled={busy || (pr.supplierId === null && supplierId === "")}
        onClick={() => onApprove(supplierId || undefined)}
        className="min-h-11 w-full rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
      >
        Approve and create order
      </button>

      <ReasonAction
        actionLabel="Reject"
        placeholder="Why it is being rejected"
        danger
        busy={busy}
        onSubmit={onReject}
      />
    </div>
  );
}

/**
 * An action that needs a reason before it will fire.
 *
 * The reason box is disclosed by pressing the button, not always on screen: a
 * page with three empty explain-yourself fields reads as a form, and none of
 * these is the ordinary thing to do next. The second press is the confirmation
 * step, which is also why a destructive action here needs no separate dialog.
 */
function ReasonAction({
  actionLabel,
  placeholder,
  danger,
  busy,
  onSubmit,
}: {
  actionLabel: string;
  placeholder: string;
  danger?: boolean;
  busy: boolean;
  onSubmit: (reason: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");

  const outline = danger
    ? "border-danger/40 text-danger"
    : "border-hairline text-primary";

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={`min-h-11 w-full rounded-md border px-4 text-sm ${outline}`}
      >
        {actionLabel}
      </button>
    );
  }

  return (
    <div className="space-y-3">
      <label className="block">
        <span className="mb-1 block text-sm text-secondary">Reason</span>
        <textarea
          autoFocus
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          rows={2}
          placeholder={placeholder}
          className="w-full rounded-md border border-hairline bg-surface px-3 py-2 text-sm"
        />
      </label>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={busy || reason.trim() === ""}
          onClick={() => onSubmit(reason.trim())}
          className={`min-h-11 rounded-md border px-4 text-sm disabled:opacity-50 ${outline}`}
        >
          {actionLabel}
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
  );
}
