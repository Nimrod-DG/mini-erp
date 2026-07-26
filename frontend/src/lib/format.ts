/**
 * Display formatting. Nothing here decides anything — it renders numbers the
 * server already computed.
 *
 * Quantities are NUMERIC(18,4) and money NUMERIC(18,2) in the database, and
 * neither is ever a float on the server (I8). They arrive here as JSON numbers
 * for display only: no total, no comparison, and no reorder decision is made in
 * this file, because a second implementation of a rule is one that can disagree
 * with the one that counts.
 */

/** Quantities carry four decimals in the database but reading `12.0000` in a
 *  table is worse than reading `12`. Trailing zeros go; significant digits stay. */
export function formatQty(value: number): string {
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 4,
  }).format(value);
}

/** A signed quantity, for ledger deltas where the sign is the whole point. */
export function formatDelta(value: number): string {
  return value > 0 ? `+${formatQty(value)}` : formatQty(value);
}

export function formatMoney(value: number): string {
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

/**
 * Timestamps are stored UTC and rendered in the **tenant's** business timezone,
 * never the browser's (I7, FE15).
 *
 * A receipt posted at 23:30 in Jakarta is on that day's books; a colleague
 * opening the same screen in London must see the same date, or two people
 * reading one ledger disagree about which month a movement fell in.
 */
export function formatDateTime(iso: string, timeZone: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone,
  }).format(new Date(iso));
}
