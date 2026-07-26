/**
 * Measuring a Tailwind class list, because jsdom has no layout engine.
 *
 * READ THIS BEFORE TRUSTING FE9 OR FE13. `getBoundingClientRect` returns zeroes
 * in jsdom, and the stylesheet is Tailwind source text that is never compiled
 * during a unit test — so nothing here measures pixels that a browser produced.
 * What it measures is the size a component *declares*, by reading the utilities
 * that set it.
 *
 * That is a weaker claim than "this control is 44px on a phone", and it is the
 * claim these two tests make. It is still the useful one: the Phase 7 touch-target
 * audit found four controls under 44px, and every one of them was a control that
 * had never declared a minimum — `px-3 py-1` around a glyph, a bare text link. A
 * declared floor is what stops that class of regression, and it is exactly what
 * this file checks. The real pixels are the browser walk's job, and the 360px walk
 * at the MVP gate is where they were checked.
 *
 * Tailwind's spacing unit is 0.25rem, and this codebase sets the root font size
 * nowhere, so 1 unit is 4px.
 */

const UNIT_PX = 4;
const REM_PX = 16;

/** `text-sm` is 0.875rem with Tailwind's paired 1.25rem line height, which is the
 *  content height of a single-line table cell throughout this application. */
export const TEXT_SM_LINE_PX = 20;

/** Widths that fill whatever contains them. Treated as satisfying the floor: the
 *  containers in question are a form column and a table cell, both far wider than
 *  44px at 360px, and asserting otherwise would mean modelling the page's grid. */
const FILLS_CONTAINER = new Set(["w-full", "flex-1", "grow", "w-auto", "flex-auto"]);

function pxFromToken(token: string, prefixes: readonly string[]): number | null {
  for (const prefix of prefixes) {
    if (!token.startsWith(`${prefix}-`)) continue;
    const value = token.slice(prefix.length + 1);

    // `min-h-11` — a spacing-scale multiple.
    if (/^\d+(\.\d+)?$/.test(value)) return Number.parseFloat(value) * UNIT_PX;

    // `min-h-[3rem]` or `min-h-[44px]` — an arbitrary value.
    const arbitrary = /^\[([\d.]+)(rem|px)\]$/.exec(value);
    if (arbitrary) {
      const size = Number.parseFloat(arbitrary[1]);
      return arbitrary[2] === "rem" ? size * REM_PX : size;
    }
  }
  return null;
}

/** The largest height this element declares, or null if it declares none.
 *  `size-*` sets both axes, so it counts for either. */
export function declaredHeightPx(element: Element): number | null {
  const tokens = element.className.split(/\s+/).filter(Boolean);
  const found = tokens
    .map((token) => pxFromToken(token, ["min-h", "h", "size"]))
    .filter((px): px is number => px !== null);
  return found.length > 0 ? Math.max(...found) : null;
}

/** The largest width this element declares. `Infinity` for one that fills its
 *  container — see FILLS_CONTAINER above. */
export function declaredWidthPx(element: Element): number | null {
  const tokens = element.className.split(/\s+/).filter(Boolean);
  if (tokens.some((token) => FILLS_CONTAINER.has(token))) return Infinity;

  const found = tokens
    .map((token) => pxFromToken(token, ["min-w", "w", "size"]))
    .filter((px): px is number => px !== null);
  return found.length > 0 ? Math.max(...found) : null;
}

/** The utilities that set an element's box padding, in the order written. Two
 *  cells with the same set are the same height for the same content. */
export function paddingTokens(element: Element): string[] {
  return element.className
    .split(/\s+/)
    .filter((token) => /^p[xytrbl]?-/.test(token))
    .sort();
}

/** A short description for a failure message. An element identified only as
 *  `<button>` in a form of eleven of them is not a finding anybody can act on. */
export function describe(element: Element): string {
  const label =
    element.getAttribute("aria-label") ??
    element.textContent?.trim().slice(0, 40) ??
    "";
  return `<${element.tagName.toLowerCase()}${label ? ` "${label}"` : ""}> class="${element.className}"`;
}
