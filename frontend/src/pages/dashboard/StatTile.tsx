import { Link } from "react-router-dom";

/**
 * One headline number, in the strip across the top of the dashboard.
 *
 * WHY THESE ARE A ROW AND NOT FOUR CARDS. Every widget used to lead with its own
 * big figure, inside a card sized by whatever it carried underneath. So the three
 * numbers a reader actually scans for sat at three different heights, in three
 * different columns, separated by a table and a decision queue — and comparing
 * them meant three saccades and a scroll. A handful of headline numbers is a
 * *row of stat tiles*: same size, same baseline, one glance.
 *
 * The tile's contract is label / value / one supporting fact. The supporting
 * fact is not optional in practice — a naked "3" is a number the reader has to
 * open something to understand, and the whole point of the strip is that they do
 * not have to.
 *
 * PROPORTIONAL FIGURES, NOT `tabular`. This is the one place in the application
 * where digits should *not* be tabular: `tabular-nums` gives every digit the
 * width of a zero, which is what makes a column of numbers line up and what makes
 * a standalone number at 36px look gappy. Columns get `tabular`; display figures
 * get the font's own spacing.
 */
export function StatTile({
  label,
  value,
  detail,
  href,
  attention,
}: {
  /** Sentence case, no trailing colon. */
  label: string;
  value: number;
  /** The one fact that makes the number mean something. */
  detail: string;
  href: string;
  /** Whether this number is a problem — something below its reorder point,
   *  something waiting on a decision.
   *
   *  It is carried by a MARK, not by the value's colour. The number stays in
   *  primary ink whatever it says, because ink is for text and a status hue on a
   *  36px numeral turns the whole tile into an alarm. The dot beside the label
   *  does the signalling, and the label and `detail` say it in words as well —
   *  colour is never the only channel (§10.8.4). */
  attention?: boolean;
}) {
  return (
    // An `<li>`: the strip is a list of figures, and saying so gives a screen
    // reader the count before it starts reading them. `StatStrip` owns the `<ul>`.
    //
    // The `<li>` is the grid cell and the link fills it, rather than the `<li>`
    // being `display: contents` with the link as the cell. `display: contents`
    // is the tidier CSS and it has a history of dropping the element from the
    // accessibility tree — which would silently undo the reason for the list.
    <li className="min-w-0">
      <Link
        to={href}
        className="flex h-full flex-col gap-1 rounded-xl border border-hairline bg-surface p-4 transition-colors hover:bg-subtle"
      >
        <span className="flex items-center gap-2 text-sm text-secondary">
          {attention && (
            <span
              aria-hidden="true"
              className="size-1.5 shrink-0 rounded-full bg-warning"
            />
          )}
          {label}
        </span>
        <span className="text-4xl font-semibold leading-none tracking-tight">
          {value}
        </span>
        <span className="truncate text-sm text-secondary">{detail}</span>
      </Link>
    </li>
  );
}
