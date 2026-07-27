import type { ReactNode } from "react";

import type { AsyncState } from "../hooks/useAsync";
import { useCompact } from "../hooks/useCompact";
import type { ListResponse } from "../lib/api";
import { CardList, EmptyCards, SkeletonCards } from "./CardList";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
  TableFrame,
  TableHead,
  type Column,
} from "./ListStates";

/**
 * A §9.0 list rendered as a table on a wide screen and as cards on a phone,
 * with all four §10.7.6 states in both.
 *
 * WHY THIS EXISTS. The Phase 6 log named the target: "the
 * pagination-and-empty-state block is still the largest known clone family and
 * still wants a `DataTable` wrapper rather than another header-sized
 * extraction." Phase 7's card transformation is what made it worth building —
 * giving the requisition and PO lists a second view doubled the scaffolding
 * around them, and `fallow audit` reported the two screens as a 28-line and a
 * 22-line clone the moment the second copy landed. §12A.4: catching a duplicated
 * component after two copies is a ten-minute fix; after five it is an afternoon.
 *
 * WHAT IT DOES *NOT* OWN, deliberately: the cells and the card fields. A
 * column's content is where the decisions live — a link target, a status chip, a
 * deleted-product marker, a signed delta — and a config object rich enough to
 * express those would grow a case per column type and be worse than the
 * duplication it replaced. `row` and `card` are render props for exactly that
 * reason, and they are the same reason `TableHead` shares only the heading row.
 *
 * The filters above the list stay with the screen too. They are the part that
 * differs most and the part a reader most needs to see next to the query it
 * drives.
 */
export function ResponsiveList<T extends { id: string }>({
  state,
  onRetry,
  onPage,
  onPageSize,
  columns,
  minWidth,
  filtered,
  firstRun,
  noResults,
  action,
  row,
  card,
}: {
  state: AsyncState<ListResponse<T>>;
  onRetry: () => void;
  onPage: (page: number) => void;
  /** Omit to render the list without a page-size picker. */
  onPageSize?: (pageSize: number) => void;

  columns: Column[];
  /** The table's minimum width, as a literal Tailwind class — the width below
   *  which it scrolls sideways rather than crushing its columns. */
  minWidth: string;

  /** Whether any filter is applied, which is what tells the two §10.7.6 empty
   *  states apart: "nothing exists yet" needs the action that creates one,
   *  "nothing matches" needs the filters cleared. */
  filtered: boolean;
  firstRun: string;
  noResults: string;
  action?: ReactNode;

  /** One `<tr>`. */
  row: (item: T) => ReactNode;
  /** One card, for below `md`. */
  card: (item: T) => ReactNode;
}) {
  const compact = useCompact();

  // A failed *load* stays inline, where the data would have been. A toast there
  // would fade and leave an empty screen with no explanation (Phase 4's split
  // between load failures and action outcomes).
  if (state.status === "error") {
    return <ErrorNotice error={state.error} onRetry={onRetry} />;
  }

  const rows = state.status === "ready" ? state.data.data : [];
  const empty = state.status === "ready" && rows.length === 0;

  return (
    <>
      {compact ? (
        <>
          {state.status === "loading" && <SkeletonCards />}
          {empty && (
            <EmptyCards
              filtered={filtered}
              firstRun={firstRun}
              noResults={noResults}
              action={action}
            />
          )}
          {rows.length > 0 && <CardList>{rows.map(card)}</CardList>}
        </>
      ) : (
        <TableFrame>
          <table className={`w-full ${minWidth} text-left text-sm`}>
            <TableHead columns={columns} />
            {state.status === "loading" && <SkeletonRows cols={columns.length} />}
            {empty && (
              <EmptyState
                colSpan={columns.length}
                filtered={filtered}
                firstRun={firstRun}
                noResults={noResults}
                action={action}
              />
            )}
            {rows.length > 0 && <tbody>{rows.map(row)}</tbody>}
          </table>
        </TableFrame>
      )}

      {state.status === "ready" && (
        <Pagination
          page={state.data.page}
          pageSize={state.data.pageSize}
          totalItems={state.data.totalItems}
          totalPages={state.data.totalPages}
          onPage={onPage}
          onPageSize={onPageSize}
        />
      )}
    </>
  );
}
