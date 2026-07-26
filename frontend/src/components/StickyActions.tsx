import type { ReactNode } from "react";

/**
 * The sticky bottom action bar of §10.7.5: "below `md`, forms are single-column
 * with full-width inputs and a sticky bottom action bar holding the primary
 * action, so 'Post receipt' is always reachable without scrolling to the end of
 * a long line list."
 *
 * That sentence names the screen this exists for. A goods receipt against an
 * eight-line order is two phone screens of quantity boxes with the button
 * underneath, and the person filling it in is standing at a loading dock holding
 * the phone one-handed (§10.7.1). Scrolling back down to commit is the friction
 * worth removing.
 *
 * `sticky`, not `fixed`. A sticky element still occupies its place in the flow,
 * so nothing needs a spacer underneath it and nothing can end up hidden behind
 * it — the two bugs a fixed bar produces. It pins to the viewport only while
 * there is more form below it, and settles into place at the end.
 *
 * `bottom-14` clears the bottom tab bar, which is `fixed` and 3.5rem tall
 * (§10.7.3). Above `md` the bar is `static` and the tab bar is gone, so the
 * actions sit in the flow exactly as they did before this component existed.
 *
 * The negative margins let the bar's background reach the edges of the screen
 * while its contents stay aligned with the form, which is what stops it reading
 * as a floating card rather than as part of the page.
 */
export function StickyActions({
  children,
  hint,
}: {
  children: ReactNode;
  /** A line under the buttons — the receipt's "safe to retry", say. Inside the
   *  bar rather than above it, so it travels with the action it qualifies. */
  hint?: ReactNode;
}) {
  return (
    <div
      className={
        "sticky bottom-14 z-20 -mx-4 border-t border-hairline bg-surface px-4 py-3 " +
        "sm:-mx-6 sm:px-6 " +
        "md:static md:mx-0 md:border-0 md:bg-transparent md:px-0 md:py-0"
      }
    >
      {children}
      {hint && <p className="mt-2 text-xs text-secondary">{hint}</p>}
    </div>
  );
}
