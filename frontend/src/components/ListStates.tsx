import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

import { ApiError } from "../lib/api";

/**
 * Skeleton rows rather than a spinner (§10.7.6): they hold the layout still, so
 * nothing lurches when the data arrives. The row height has to match the real
 * one or the point is lost.
 */
export function SkeletonRows({
  rows = 5,
  cols,
}: {
  rows?: number;
  cols: number;
}) {
  return (
    <tbody>
      {Array.from({ length: rows }, (_, row) => (
        <tr key={row} className="border-t border-hairline">
          {Array.from({ length: cols }, (_, col) => (
            <td key={col} className="px-3 py-3">
              <div
                className="h-4 animate-pulse rounded bg-raised"
                style={{ width: col === 0 ? "60%" : "40%" }}
              />
            </td>
          ))}
        </tr>
      ))}
    </tbody>
  );
}

/**
 * One column heading. `align: "right"` is the only alignment variant, because the
 * only columns that are not left-aligned are numbers, and every number in this
 * application is right-aligned.
 */
export type Column = {
  label: string;
  align?: "right";
  /** An actions column has no visible heading, but a table still needs one —
   *  a screen reader announcing "column 6" is not a heading. */
  hidden?: boolean;
  /** Freeze this column while the table scrolls sideways (§10.7.4). Only the
   *  first column of a table may sensibly carry it, and only the two dense
   *  grids do: a stock or ledger row whose product has scrolled out of sight is
   *  a row of numbers about nothing. Pair it with `frozenCell` on the matching
   *  `<td>`. */
  sticky?: boolean;
};

/**
 * The classes a frozen first column needs, on the `<td>` side.
 *
 * Two things are load-bearing and neither is obvious:
 *
 *   - The background must be OPAQUE. A sticky cell is painted over the cells
 *     scrolling underneath it, and a transparent one shows both at once.
 *   - Which means it also has to follow the row's hover, or the frozen column
 *     stays surface-coloured while the rest of the row lifts. `group-hover`
 *     does that, and the `<tr>` carries `group`.
 *
 * The z-index is below the header's, which is the collision §10.7.4 warns
 * about: where a sticky header crosses a frozen column, one of them has to win,
 * and it should be the header — it is the thing that says what the frozen
 * column *is*.
 */
export const frozenCell =
  "sticky left-0 z-10 bg-surface group-hover:bg-raised";

/**
 * The scroll box the two dense grids live in.
 *
 * `overflow-auto` rather than `overflow-x-auto`, deliberately: a container with
 * `overflow-x: auto` computes `overflow-y` to `auto` as well, so it is already a
 * scroll container and `sticky top-0` inside it stops tracking the page. Making
 * both axes explicit and capping the height means the sticky header and the
 * frozen column are both relative to this box, which is the only way the two can
 * work at once.
 *
 * The cap only bites when there are more rows than fit; a short table is
 * unchanged.
 */
export function ScrollableTable({ children }: { children: ReactNode }) {
  return (
    <div className="max-h-[calc(100vh-18rem)] overflow-auto rounded-lg border border-hairline bg-surface">
      {children}
    </div>
  );
}

/**
 * The heading row every data table in this application shares.
 *
 * Extracted in Phase 5B, at the point §12A.4 sets: the goods receipt screens made
 * this the *fourth* copy of the same nineteen lines, and Phase 4 had already
 * noted that catching a duplicated component after two copies is a ten-minute fix
 * and after five it is an afternoon.
 *
 * Only the heading row is shared, deliberately. The cells stay with their screen,
 * because a column's *content* is where the interesting decisions live — a link
 * target, a deleted-product marker, a signed delta — and a config object rich
 * enough to express those would grow a case per column type and be worse than the
 * duplication it replaced.
 */
export function TableHead({
  columns,
  sticky,
}: {
  columns: Column[];
  /** Keep the heading row visible while the body scrolls (§10.7.4). Only
   *  meaningful inside ScrollableTable, which is what the row is sticky to. */
  sticky?: boolean;
}) {
  return (
    <thead
      className={`text-xs uppercase tracking-wide text-secondary${
        sticky ? " sticky top-0 z-20 bg-surface" : ""
      }`}
    >
      <tr>
        {columns.map((column, index) => (
          <th
            key={index}
            scope="col"
            className={`px-3 py-2.5 font-medium${
              column.align === "right" ? " text-right" : ""
            }${
              // z-30 so the corner cell sits above both the sticky header row
              // and the frozen column it crosses.
              column.sticky ? " sticky left-0 z-30 bg-surface" : ""
            }`}
          >
            {column.hidden ? (
              <span className="sr-only">{column.label}</span>
            ) : (
              column.label
            )}
          </th>
        ))}
      </tr>
    </thead>
  );
}

/** ErrorNotice renders a failed load with the option to try again. */
export function ErrorNotice({
  error,
  onRetry,
}: {
  error: Error;
  onRetry?: () => void;
}) {
  // The envelope's `message` is written for people; `error` is the code the app
  // branches on. Showing the sentence and not the code is the right way round.
  const message =
    error instanceof ApiError
      ? error.message
      : "Could not reach the server. Try again in a moment.";

  return (
    <div
      role="alert"
      className="rounded-lg border border-danger/40 bg-surface p-4 text-sm"
    >
      <p className="text-danger">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 min-h-11 rounded-md border border-hairline px-3 text-sm"
        >
          Try again
        </button>
      )}
    </div>
  );
}

/**
 * EmptyState distinguishes the two empty states §10.7.6 insists are different:
 * nothing exists yet (explain, and offer the action that creates one) versus
 * nothing matches the filters (offer to clear them). One message for both is
 * how you get "no results" on a screen with no create button.
 */
export function EmptyState({
  colSpan,
  filtered,
  firstRun,
  noResults,
  action,
}: {
  colSpan: number;
  filtered: boolean;
  firstRun: string;
  noResults: string;
  action?: ReactNode;
}) {
  return (
    <tbody>
      <tr className="border-t border-hairline">
        <td colSpan={colSpan} className="px-3 py-10 text-center">
          <EmptyMessage
            filtered={filtered}
            firstRun={firstRun}
            noResults={noResults}
            action={action}
          />
        </td>
      </tr>
    </tbody>
  );
}

/** The words and the action, without the table cell around them — so the card
 *  view below `md` says exactly what the table above it would. */
export function EmptyMessage({
  filtered,
  firstRun,
  noResults,
  action,
}: {
  filtered: boolean;
  firstRun: string;
  noResults: string;
  action?: ReactNode;
}) {
  return (
    <>
      <p className="text-sm text-secondary">{filtered ? noResults : firstRun}</p>
      {!filtered && action && <div className="mt-4">{action}</div>}
    </>
  );
}

/**
 * The banner over a list narrowed to one source document by `?sourceId=`.
 *
 * Both halves of the goods receipt confirmation panel link to a list this way —
 * the stock ledger's rows and the journal's entry (§10.3) — and the reader
 * arriving there has to be told why they are looking at two rows out of
 * thousands, and given the way back. Shared because it is the same banner twice,
 * with the noun changed.
 *
 * It owns the parameter as well as the banner: clearing the filter is removing
 * `sourceId` from the URL, and a version where the caller did that itself would
 * be a component that renders a button whose behaviour lives somewhere else.
 * `onCleared` is for the page's own state — the page number, which has to go
 * back to one.
 */
export function SourceFilterNotice({
  sourceNumber,
  showing,
  clearLabel,
  onCleared,
}: {
  /** The document's number, once the first row has resolved it. Null until
   *  then: the banner appears immediately and names the document when it can,
   *  rather than waiting and shifting the table down a moment later. */
  sourceNumber: string | null;
  showing: string;
  clearLabel: string;
  onCleared: () => void;
}) {
  const [params, setParams] = useSearchParams();
  if (!params.get("sourceId")) return null;

  return (
    <div className="mb-4 flex flex-wrap items-baseline justify-between gap-3 rounded-lg border border-hairline bg-raised px-4 py-3 text-sm">
      <span>
        Showing only {showing}
        {sourceNumber && (
          <>
            {" — "}
            <span className="tabular">{sourceNumber}</span>
          </>
        )}
        .
      </span>
      <button
        type="button"
        onClick={() => {
          const next = new URLSearchParams(params);
          next.delete("sourceId");
          setParams(next, { replace: true });
          onCleared();
        }}
        // A text button still has to be a 44px target (§10.7.5). `-my-2` keeps
        // the extra height from pushing the banner taller than its one line.
        className="-my-2 inline-flex min-h-11 items-center text-accent underline decoration-hairline underline-offset-2"
      >
        {clearLabel}
      </button>
    </div>
  );
}

/**
 * Pagination always shows the total count. "Page 3 of ?" strands people
 * (§10.7.4), which is why `totalItems` is mandatory in the §9.0 envelope rather
 * than optional.
 */
export function Pagination({
  page,
  pageSize,
  totalItems,
  totalPages,
  onPage,
}: {
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
  onPage: (page: number) => void;
}) {
  if (totalItems === 0) return null;

  const first = (page - 1) * pageSize + 1;
  const last = Math.min(page * pageSize, totalItems);

  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm">
      <p className="text-secondary">
        <span className="tabular">
          {first}–{last}
        </span>{" "}
        of <span className="tabular">{totalItems}</span>
      </p>
      {totalPages > 1 && (
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => onPage(page - 1)}
            disabled={page <= 1}
            className="min-h-11 rounded-md border border-hairline px-3 disabled:opacity-40"
          >
            Previous
          </button>
          <span className="text-secondary">
            Page <span className="tabular">{page}</span> of{" "}
            <span className="tabular">{totalPages}</span>
          </span>
          <button
            type="button"
            onClick={() => onPage(page + 1)}
            disabled={page >= totalPages}
            className="min-h-11 rounded-md border border-hairline px-3 disabled:opacity-40"
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
