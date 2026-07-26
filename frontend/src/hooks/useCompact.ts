import { useCallback, useSyncExternalStore } from "react";

/**
 * The `md` breakpoint, in JavaScript.
 *
 * WHY THIS EXISTS RATHER THAN A `md:hidden` CLASS. §10.7.4 is explicit: "when
 * transforming to cards, render a genuinely different component below the
 * breakpoint rather than CSS-hiding cells — a `<td>` styled to look like a card
 * still announces as a table cell." The mirror image is just as bad: rendering
 * both a table and a list and hiding one with CSS puts every row in the
 * accessibility tree twice, and a screen reader reads the invisible one.
 *
 * So the card lists ask this and render one thing or the other. Everything that
 * is merely a layout change — a grid going from two columns to one, a button
 * going full width — stays in Tailwind, where it belongs.
 *
 * 767.98px rather than 767px: Tailwind's `md` is `min-width: 768px`, and a
 * viewport can be a fractional width on a zoomed or scaled display. The
 * fractional bound is what stops both queries being false in the gap.
 */
const COMPACT = "(max-width: 767.98px)";

/**
 * useSyncExternalStore rather than useState + useEffect, so the very first
 * render already knows the answer. With an effect, every phone would paint the
 * desktop table for one frame and then swap it for cards — a visible lurch on
 * exactly the device §10.7.6 says to hold the layout still for.
 */
export function useCompact(): boolean {
  const subscribe = useCallback((onChange: () => void) => {
    const query = window.matchMedia(COMPACT);
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);

  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(COMPACT).matches,
    // Server snapshot. Nothing renders this app on a server today, but the
    // hook has to answer, and "not compact" is the safer guess: a table is
    // usable on a phone, whereas cards on a desktop throw away the comparison
    // the wide screen was for.
    () => false,
  );
}
