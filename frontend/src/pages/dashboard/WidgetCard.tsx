import type { ReactNode } from "react";
import { Link } from "react-router-dom";

/**
 * The frame the four §10.2 widgets share: a heading, an optional link out, and
 * a body.
 *
 * Extracted because there are four of them and they must line up in a grid — a
 * card that sets its own padding is the one that ends up a few pixels taller
 * than its neighbours. What is *not* shared is anything about the content: each
 * widget renders a different shape of answer, and a config object rich enough
 * for a currency total, a decision queue, a shortfall table, and a movement feed
 * would be four components wearing a trenchcoat.
 *
 * `href` is the widget's "see all". §10.2 asks for it on the open-orders widget
 * by name, and it is the same affordance on the other three: a dashboard number
 * that cannot be opened is a number the reader has to take on trust.
 */
export function WidgetCard({
  title,
  href,
  hrefLabel = "See all",
  children,
}: {
  title: string;
  href?: string;
  hrefLabel?: string;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col rounded-lg border border-hairline bg-surface">
      <div className="flex items-baseline justify-between gap-3 border-b border-hairline px-4 py-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {href && (
          <Link
            to={href}
            className="shrink-0 text-sm text-accent underline decoration-hairline underline-offset-2"
          >
            {hrefLabel}
          </Link>
        )}
      </div>
      <div className="flex-1 p-4">{children}</div>
    </section>
  );
}

/**
 * The one big number a widget leads with, plus a caption under it.
 *
 * `tabular` throughout: these are figures, and proportional digits make a column
 * of them jitter. Kept out of WidgetCard because two of the four widgets lead
 * with a list rather than a total.
 */
export function WidgetFigure({
  value,
  caption,
}: {
  value: string;
  caption: string;
}) {
  return (
    <div>
      <p className="tabular text-3xl font-semibold leading-none">{value}</p>
      <p className="mt-1.5 text-sm text-secondary">{caption}</p>
    </div>
  );
}

/**
 * What a widget shows when its number is zero.
 *
 * A separate component rather than a conditional inside each card, because
 * §10.7.6's rule — "a blank panel reads as broken" — applies to a widget just as
 * much as to a table, and the four would otherwise each invent their own way of
 * saying nothing.
 */
export function WidgetEmpty({ children }: { children: ReactNode }) {
  return <p className="py-2 text-sm text-secondary">{children}</p>;
}
