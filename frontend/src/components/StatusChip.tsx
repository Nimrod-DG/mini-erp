import type { DocumentStatus } from "../lib/format";
import { statusLabel } from "../lib/format";

/**
 * The status vocabulary of the two procurement documents, rendered the same way
 * everywhere (§10.3).
 *
 * Colour is never the only signal: every chip carries its word. A reader who
 * cannot distinguish the amber from the green still reads "submitted" and
 * "received", which is the accessibility floor §10.7.5 sets and also the honest
 * design — "the green one" is not a status.
 *
 * Written out as whole class strings rather than composed from a template,
 * because Tailwind scans source text: a class it cannot see literally is a class
 * it does not generate.
 */
const CHIP: Record<DocumentStatus, string> = {
  // Not started: nothing has happened yet, so nothing is coloured.
  draft: "border-hairline text-secondary",
  // Waiting on somebody. The one status that is a call to action.
  submitted: "border-warning/40 text-warning",
  approved: "border-success/40 text-success",
  rejected: "border-danger/40 text-danger",
  // Cancelled is not a failure, it is a decision — so it recedes rather than
  // alarming.
  cancelled: "border-hairline text-secondary",
  open: "border-accent/40 text-accent",
  partially_received: "border-warning/40 text-warning",
  received: "border-success/40 text-success",
};

export function StatusChip({ status }: { status: DocumentStatus }) {
  return (
    <span
      className={`inline-block whitespace-nowrap rounded-full border px-2 py-0.5 text-xs ${
        CHIP[status] ?? "border-hairline text-secondary"
      }`}
    >
      {statusLabel(status)}
    </span>
  );
}

// `StatusFilter` — the row of chips §10.3 originally asked for — was removed
// when the filter row became a search box and dropdowns on one line. Five chips
// plus a search box could not share a line, and the chips were the half that
// scaled worst: the requisition list has five statuses, and a sixth would have
// wrapped. The rule the chips existed to enforce is unchanged and now lives in
// the dropdown's `onChange`: the status is a *server* parameter, never a
// `.filter()` over the fetched page. Filtering the page would report "3 drafts"
// for a tenant with forty, because only one page arrived — the count in the
// pagination line and the rows in the table have to answer the same question.
