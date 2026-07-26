import { Link } from "react-router-dom";

import type { LedgerEntry, RecentActivityWidget } from "../../lib/api";
import { formatDateTime, formatDelta } from "../../lib/format";
import { WidgetCard, WidgetEmpty } from "./WidgetCard";

/**
 * §10.2 widget 4 — the last fifteen stock movements, each linking to its source.
 *
 * "Each linking to its source document" is the requirement, and it is the reason
 * the server resolves `sourceNumber` and `sourcePoId` on every ledger row: a
 * link to `stock_ledger.source_id` alone would be a UUID the reader has to go and
 * look up, which is not a link, it is a puzzle.
 *
 * A manual adjustment has no document behind it — the person is the source
 * (§6.3) — so its row names who made it instead of pretending to link somewhere.
 */
export function RecentActivityCard({
  widget,
  timezone,
}: {
  widget: RecentActivityWidget;
  /** The tenant's business timezone. Every timestamp on this screen renders in
   *  it, never the browser's (I7, FE15): two colleagues in different countries
   *  reading one ledger must agree about which day a movement fell on. */
  timezone: string;
}) {
  return (
    <WidgetCard
      title="Recent activity"
      href="/inventory/ledger"
      hrefLabel="Full ledger"
    >
      {widget.entries.length === 0 ? (
        <WidgetEmpty>
          No stock has moved yet. Receiving a purchase order or posting an
          adjustment writes the first entry.
        </WidgetEmpty>
      ) : (
        <ul className="divide-y divide-hairline">
          {widget.entries.map((entry) => (
            <li key={entry.id} className="flex gap-3 py-2.5 text-sm">
              {/* The delta leads, because the question a movement feed answers
                  is "what changed", and the sign is the whole point. */}
              <span
                className={`tabular w-20 shrink-0 text-right ${
                  entry.qtyDelta < 0 ? "text-danger" : "text-success"
                }`}
              >
                {formatDelta(entry.qtyDelta)}
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate">
                  {entry.productName}
                  {entry.productDeleted && (
                    // The product was tidied away; the movement still happened
                    // (§6.9.1). Saying so is better than leaving the reader to
                    // wonder why it is not in the product list.
                    <span className="ml-2 text-xs text-secondary">deleted</span>
                  )}
                </p>
                <p className="text-xs text-secondary">
                  <span className="tabular">{entry.warehouseCode}</span> ·{" "}
                  {formatDateTime(entry.occurredAt, timezone)} ·{" "}
                  <SourceLink entry={entry} />
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </WidgetCard>
  );
}

/** The source document, or the person, depending on which one there is. */
function SourceLink({ entry }: { entry: LedgerEntry }) {
  if (entry.sourceType === "goods_receipt" && entry.sourcePoId) {
    return (
      <Link
        to={`/procurement/orders/${entry.sourcePoId}`}
        className="tabular text-accent underline decoration-hairline underline-offset-2"
      >
        {entry.sourceNumber ?? "Goods receipt"}
      </Link>
    );
  }
  return <>Adjustment by {entry.createdByName}</>;
}
