import type { ReactNode } from "react";

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
};

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
export function TableHead({ columns }: { columns: Column[] }) {
  return (
    <thead className="text-xs uppercase tracking-wide text-secondary">
      <tr>
        {columns.map((column, index) => (
          <th
            key={index}
            scope="col"
            className={`px-3 py-2.5 font-medium${
              column.align === "right" ? " text-right" : ""
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
          <p className="text-sm text-secondary">
            {filtered ? noResults : firstRun}
          </p>
          {!filtered && action && <div className="mt-4">{action}</div>}
        </td>
      </tr>
    </tbody>
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
