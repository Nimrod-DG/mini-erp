import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { StickyActions } from "../../components/StickyActions";
import { useToast } from "../../components/Toasts";
import { createRequisition, submitRequisition } from "../../lib/api";
import {
  emptyLine,
  prefillLines,
  usableLines,
  useRequisitionPickers,
  type RequisitionFormValues,
} from "../../lib/requisitionForm";
import { RequisitionFields } from "./RequisitionFields";

/**
 * `/procurement/requisitions/new` — warehouse, supplier, product lines, and the
 * choice of saving a draft or submitting (§10.3).
 *
 * **Submitting is a second request, deliberately.** The API creates drafts only,
 * so "Submit for approval" is create-then-submit. If the second call fails the
 * user has a draft they can find and finish, rather than a document in a state
 * nobody chose — and the failure is reported rather than swallowed.
 */
export function RequisitionNew() {
  const navigate = useNavigate();
  const toast = useToast();

  // `?products=<id>:<qty>,…` is the dashboard's low-stock shortcut (§10.2). Read
  // once, into the initial state, rather than watched: this pre-fills a form the
  // user then edits, and re-reading the URL would undo their edits every render.
  const [params] = useSearchParams();
  const [values, setValues] = useState<RequisitionFormValues>(() => {
    const prefilled = prefillLines(params.get("products"));
    return {
      warehouseId: "",
      supplierId: "",
      notes: "",
      // One blank line when nothing was pre-filled: an empty form still needs a
      // row to type into.
      lines: prefilled.length > 0 ? prefilled : [{ ...emptyLine }],
    };
  });
  const [saving, setSaving] = useState(false);

  const { state, reload } = useRequisitionPickers("requisition-form");

  async function save(thenSubmit: boolean) {
    setSaving(true);
    try {
      const created = await createRequisition({
        warehouseId: values.warehouseId,
        supplierId: values.supplierId,
        notes: values.notes.trim(),
        lines: usableLines(values.lines),
      });

      if (!thenSubmit) {
        toast.success(`${created.prNumber} saved as a draft.`);
        navigate(`/procurement/requisitions/${created.id}`, { replace: true });
        return;
      }

      // Two calls, and the second one's failure is the user's to see: they are
      // left on the draft, which is a state they can act on.
      try {
        await submitRequisition(created.id);
        toast.success(`${created.prNumber} submitted for approval.`);
      } catch (caught) {
        toast.failure(caught);
      }
      navigate(`/procurement/requisitions/${created.id}`, { replace: true });
    } catch (caught) {
      toast.failure(caught);
    } finally {
      setSaving(false);
    }
  }

  if (state.status === "error") {
    return (
      <AppShell title="New requisition">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (state.status === "loading") {
    return (
      <AppShell title="New requisition">
        <div className="h-64 animate-pulse rounded-lg border border-hairline bg-surface" />
      </AppShell>
    );
  }

  const incomplete = values.warehouseId === "";
  const nothingToOrder = usableLines(values.lines).length === 0;

  return (
    <AppShell title="New requisition">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void save(false);
        }}
        className="max-w-3xl space-y-6"
      >
        <RequisitionFields
          pickers={state.data}
          values={values}
          onChange={setValues}
        />

        {/* §10.7.5's sticky bar. A requisition with six lines is longer than a
            phone screen, and the three choices at the end of it are the point of
            the form. */}
        <StickyActions>
          <div className="flex flex-wrap gap-3">
            {/* Disabled rather than refused when there is nothing to order: an
                empty requisition is a form that is not finished, not a mistake to
                report. The server refuses it too, with `empty_requisition` (C1) —
                this is the cosmetic half (I12). */}
            <button
              type="button"
              disabled={saving || incomplete || nothingToOrder}
              onClick={() => void save(true)}
              className="min-h-11 grow rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50 sm:grow-0"
            >
              {saving ? "Saving…" : "Submit for approval"}
            </button>
            <button
              type="submit"
              disabled={saving || incomplete}
              className="min-h-11 grow rounded-md border border-hairline px-4 text-sm disabled:opacity-50 sm:grow-0"
            >
              Save as draft
            </button>
            <button
              type="button"
              onClick={() => navigate("/procurement/requisitions")}
              className="min-h-11 rounded-md border border-hairline px-4 text-sm"
            >
              Cancel
            </button>
          </div>
        </StickyActions>
      </form>
    </AppShell>
  );
}
