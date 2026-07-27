import { useCallback, useState } from "react";

import { DEFAULT_PAGE_SIZE } from "../lib/pagination";

/**
 * Page number and page size for one list screen.
 *
 * Both are here rather than in each screen because they are one piece of state:
 * changing the size has to reset to page 1, and every screen that got that wrong
 * would strand its reader on a page that no longer exists. `key` is the fragment
 * a caller splices into its `useAsync` cache key, which is the other half of the
 * same bug — a screen that changes the size without re-keying keeps showing the
 * old page.
 *
 * Filters still reset the page themselves, with `setPage(1)`: only the screen
 * knows what its filters are.
 */
export function usePagination(initialSize: number = DEFAULT_PAGE_SIZE) {
  const [page, setPage] = useState(1);
  const [pageSize, setSize] = useState(initialSize);

  const setPageSize = useCallback((size: number) => {
    setSize(size);
    setPage(1);
  }, []);

  return { page, pageSize, setPage, setPageSize, key: `${page}:${pageSize}` };
}
