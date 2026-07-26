import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { EmptyMessage } from "./ListStates";

/**
 * The card transformation of §10.7.4, for the two document lists a manager
 * scans on the move: requisitions and purchase orders.
 *
 * WHY THESE TWO AND NOT EVERY TABLE. §10.7.4 says to choose per screen and be
 * able to say why, and the choice here is the one that section describes:
 *
 *   - Requisition and PO lists become **cards**. They are read one row at a
 *     time — "what is this, who raised it, how much" — the counts are modest,
 *     and they are what §10.7.1 puts on a phone.
 *   - The stock grid and the ledger keep their **table** and scroll sideways
 *     with a frozen first column. Their power is horizontal comparison across
 *     many columns, and a stack of cards throws exactly that away.
 *
 * A card is a genuinely different component, rendered instead of the table
 * rather than beside it with CSS hiding one — see useCompact for why that
 * distinction is an accessibility one rather than a stylistic one.
 */

/** One label/value pair inside a card. */
export type CardField = {
  label: string;
  value: ReactNode;
  /** Numbers are right-aligned in the table, and stay right-aligned here so a
   *  reader moving between the two is reading the same shape. */
  align?: "right";
};

/**
 * One document as a card.
 *
 * The card is NOT one big link, tempting as that is on a touch screen: several
 * of these rows carry a second link — a requisition's resulting order, an
 * order's originating requisition — and a link inside a link is invalid markup
 * that browsers resolve by guessing. The number is the primary target instead,
 * with a 44px hit area of its own (§10.7.5).
 */
export function DocumentCard({
  to,
  number,
  caption,
  chip,
  fields,
  footer,
}: {
  to: string;
  number: string;
  caption?: ReactNode;
  chip?: ReactNode;
  fields: CardField[];
  footer?: ReactNode;
}) {
  return (
    <li className="rounded-lg border border-hairline bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Link
            to={to}
            className="tabular inline-flex min-h-11 items-center font-medium text-accent"
          >
            {number}
          </Link>
          {caption && (
            <div className="text-xs text-secondary">{caption}</div>
          )}
        </div>
        {chip && <div className="shrink-0 pt-2.5">{chip}</div>}
      </div>

      <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        {fields.map((field) => (
          <div key={field.label} className={field.align === "right" ? "text-right" : ""}>
            <dt className="text-xs uppercase tracking-wide text-secondary">
              {field.label}
            </dt>
            <dd className={field.align === "right" ? "tabular" : ""}>
              {field.value}
            </dd>
          </div>
        ))}
      </dl>

      {footer && <div className="mt-3 text-xs">{footer}</div>}
    </li>
  );
}

/** The list the cards sit in. */
export function CardList({ children }: { children: ReactNode }) {
  return <ul className="space-y-3">{children}</ul>;
}

/**
 * Skeleton cards, at roughly the height of a real one. Same reasoning as
 * SkeletonRows (§10.7.6): a skeleton that is the wrong height lurches when the
 * data arrives, which is the thing it exists to prevent.
 */
export function SkeletonCards({ count = 4 }: { count?: number }) {
  return (
    <CardList>
      {Array.from({ length: count }, (_, i) => (
        <li
          key={i}
          className="h-32 animate-pulse rounded-lg border border-hairline bg-surface"
        />
      ))}
    </CardList>
  );
}

/** The card view's empty state, saying exactly what the table's would. */
export function EmptyCards({
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
    <div className="rounded-lg border border-hairline bg-surface px-4 py-10 text-center">
      <EmptyMessage
        filtered={filtered}
        firstRun={firstRun}
        noResults={noResults}
        action={action}
      />
    </div>
  );
}
