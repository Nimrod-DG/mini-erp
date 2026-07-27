/**
 * The pure half of pagination: the numbers, with no React and no markup.
 *
 * Separate from `hooks/usePagination` (which holds the state) and from
 * `components/ListStates` (which draws it) so that the window arithmetic — the
 * only part with edge cases — can be tested directly rather than through a
 * rendered table.
 */

/**
 * The page sizes offered in the picker.
 *
 * 100 is `httpx.MaxPageSize` — asking for more is clamped by the server, so
 * offering more would be offering a lie. The rest exist because the useful size
 * is a property of the screen, not of the application: a phone wants five rows
 * and the stock grid on a monitor wants fifty.
 */
export const PAGE_SIZE_OPTIONS = [5, 10, 25, 50, 100];

/**
 * What a list asks for before anybody chooses.
 *
 * **Five, not `httpx.DefaultPageSize`'s 25.** The two numbers are allowed to
 * differ: the server's default is what an API client gets when it says nothing,
 * and this is a presentation choice about a screen. At 25 the seeded workspaces
 * — nine products, six orders — were a single page, so the page controls had
 * nothing to show and the feature looked missing. A default that fits on a phone
 * without scrolling, and that makes the paging visible on real data, is the more
 * useful one; anybody who wants the whole list in one go has the picker.
 */
export const DEFAULT_PAGE_SIZE = 5;

/**
 * Which page numbers to draw, given where you are and how many there are.
 *
 * Always the first and the last, always the three around the current one, and an
 * ellipsis wherever that skips something — so the row is at most seven controls
 * wide and fits at 360px, and the two ends stay reachable in one press from
 * anywhere. Near either end the window slides inward instead of overlapping the
 * end it is already touching, which is what keeps the count of buttons from
 * changing as you page through and the buttons from moving under the finger.
 */
export function pageWindow(
  page: number,
  totalPages: number,
): (number | "gap")[] {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1);
  }

  const wanted = new Set([1, totalPages, page, page - 1, page + 1]);
  if (page <= 3) [2, 3, 4].forEach((p) => wanted.add(p));
  if (page >= totalPages - 2) {
    [totalPages - 3, totalPages - 2, totalPages - 1].forEach((p) =>
      wanted.add(p),
    );
  }

  const shown = [...wanted]
    .filter((p) => p >= 1 && p <= totalPages)
    .sort((a, b) => a - b);

  const out: (number | "gap")[] = [];
  shown.forEach((p, i) => {
    if (i > 0 && p - shown[i - 1] > 1) out.push("gap");
    out.push(p);
  });
  return out;
}
