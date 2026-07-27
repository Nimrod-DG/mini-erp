import { useState, type ReactNode } from "react";

import { AppShell } from "./AppShell";
import { FilterBar, SearchInput } from "./Filters";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
  TableFrame,
  TableHead,
  tableRow,
  type Column,
} from "./ListStates";
import { useAsync } from "../hooks/useAsync";
import { usePagination } from "../hooks/usePagination";
import type { ListResponse, MasterDataQuery } from "../lib/api";

/**
 * The shape every master-data list has: a search box, a "show deleted" toggle for
 * whoever may see the recycle bin, a table with the four states of §10.7.6, and
 * pagination that names the total.
 *
 * **Extracted at the second copy, deliberately.** Warehouses and suppliers are the
 * same screen with different columns, and §12A.4 is explicit about the economics:
 * "catching a duplicated form component after two copies is a ten-minute fix;
 * after five it is an afternoon." What is shared is the scaffolding — the fields
 * are not, so they stay with their entity as a `row` render prop rather than being
 * described in a config object that would grow a case per column type.
 *
 * `includeDeleted` is module `admin` only (§9.0). The caller decides that, because
 * the level is per module; this component only knows whether to offer the toggle.
 */
export function MasterDataList<T extends { id: string }>({
  title,
  cacheKey,
  canManage,
  columns,
  minWidthClass,
  searchPlaceholder,
  firstRun,
  noResults,
  addButton,
  form,
  load,
  row,
}: {
  title: string;
  /** Distinguishes this list's cache entries from another's. */
  cacheKey: string;
  /** Whether this caller may create, edit, delete — and see deleted rows. */
  canManage: boolean;
  columns: Column[];
  /** A Tailwind `min-w-[…]` literal. Passed as a whole class string because
   *  Tailwind scans source text: a class assembled at runtime is not generated. */
  minWidthClass: string;
  searchPlaceholder: string;
  /** Nothing exists yet — explain what this is for and offer the action that
   *  creates one. Distinct from `noResults`, which is a filter problem (§10.7.6). */
  firstRun: string;
  noResults: string;
  addButton?: ReactNode;
  /** The create form, disclosed by the add button. */
  form?: (onCreated: () => void) => ReactNode;
  load: (query: MasterDataQuery) => Promise<ListResponse<T>>;
  row: (item: T, onChanged: () => void) => ReactNode;
}) {
  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [search, setSearch] = useState("");
  const [showDeleted, setShowDeleted] = useState(false);
  const [adding, setAdding] = useState(false);
  const [nonce, setNonce] = useState(0);

  const { state, reload } = useAsync(
    `${cacheKey}:${key}:${search}:${showDeleted}:${nonce}`,
    () =>
      load({
        page,
        pageSize,
        q: search,
        includeDeleted: canManage && showDeleted,
      }),
  );

  const refresh = () => setNonce((n) => n + 1);

  const toggle = addButton ? (
    <button
      type="button"
      onClick={() => setAdding((open) => !open)}
      className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      {adding ? "Cancel" : addButton}
    </button>
  ) : null;

  return (
    <AppShell title={title} actions={toggle}>
      {adding &&
        form?.(() => {
          setAdding(false);
          refresh();
        })}

      <FilterBar>
        <SearchInput
          label={`Search ${title.toLowerCase()}`}
          value={search}
          onChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          placeholder={searchPlaceholder}
        />

        {canManage && (
          <label className="flex min-h-11 shrink-0 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={showDeleted}
              onChange={(event) => {
                setShowDeleted(event.target.checked);
                setPage(1);
              }}
              className="size-4 accent-accent"
            />
            Show deleted
          </label>
        )}
      </FilterBar>

      {state.status === "error" ? (
        // A failed *load* stays inline, where the data would have been. A toast
        // there fades and leaves an empty screen with no explanation.
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          <TableFrame>
            <table className={`w-full ${minWidthClass} text-left text-sm`}>
              <TableHead columns={columns} />

              {state.status === "loading" && (
                <SkeletonRows cols={columns.length} />
              )}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={columns.length}
                  filtered={search !== ""}
                  firstRun={firstRun}
                  noResults={noResults}
                  action={toggle}
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((item) => (
                    <tr key={item.id} className={tableRow}>
                      {row(item, refresh)}
                    </tr>
                  ))}
                </tbody>
              )}
            </table>
          </TableFrame>

          {state.status === "ready" && (
            <Pagination
              page={state.data.page}
              pageSize={state.data.pageSize}
              totalItems={state.data.totalItems}
              totalPages={state.data.totalPages}
              onPage={setPage}
              onPageSize={setPageSize}
            />
          )}
        </>
      )}
    </AppShell>
  );
}
