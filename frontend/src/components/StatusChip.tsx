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

/**
 * The filter chips §10.3 asks for, driven by the server's `?status=` parameter
 * rather than by filtering a page in the browser.
 *
 * That distinction matters: filtering the current page would silently show "3
 * drafts" when the tenant has forty, because only 25 rows were fetched. The
 * count in the pagination line and the rows in the table have to be answers to
 * the same question.
 */
export function StatusFilter<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T | "";
  options: readonly T[];
  onChange: (next: T | "") => void;
}) {
  const chip = (active: boolean) =>
    `min-h-11 rounded-full border px-3 text-sm ${
      active
        ? "border-accent bg-accent/10 text-accent"
        : "border-hairline text-secondary hover:text-primary"
    }`;

  return (
    <div className="flex flex-wrap gap-2" role="group" aria-label="Filter by status">
      <button
        type="button"
        aria-pressed={value === ""}
        onClick={() => onChange("")}
        className={chip(value === "")}
      >
        All
      </button>
      {options.map((option) => (
        <button
          key={option}
          type="button"
          aria-pressed={value === option}
          onClick={() => onChange(option)}
          className={chip(value === option)}
        >
          {statusLabel(option as DocumentStatus)}
        </button>
      ))}
    </div>
  );
}
