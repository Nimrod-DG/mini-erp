import { useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { FilterBar, FilterDropdown, SearchInput } from "../../components/Filters";
import {
  Pagination,
  TableFrame,
  TableHead,
  tableRow,
} from "../../components/ListStates";
import { usePagination } from "../../hooks/usePagination";
import type { LedgerEntry, RecentActivityWidget } from "../../lib/api";
import { formatDateTime, formatDelta } from "../../lib/format";

const MOVEMENTS: Record<string, string> = {
  receipt: "Receipts",
  issue: "Issues",
  adjustment: "Adjustments",
};

/**
 * §10.2 widget 4 — the last fifteen stock movements, as the dashboard's one
 * table.
 *
 * WHAT THIS REPLACED. It was a narrow feed in the right-hand column, several
 * times taller than the card beside it, which is what made the old dashboard
 * look lopsided. Across the full width the same fifteen rows are a table with
 * columns that line up, and the page ends where the table ends.
 *
 * ------------------------------------------------------------------------
 * THE SEARCH AND THE FILTERS NARROW *THESE FIFTEEN ROWS*, IN THE BROWSER.
 *
 * Every other search box in this application is a server parameter, and the
 * distinction is load-bearing rather than pedantic: filtering a fetched page
 * elsewhere would report "3 drafts" for a tenant with forty. Here there is no
 * page to disagree with — `/api/dashboard/summary` returns exactly fifteen rows
 * with no `q` and no paging, and the widget is *defined* as the last fifteen
 * movements. Narrowing them client-side is the honest reading of "recent
 * activity, filtered"; sending a query to the ledger endpoint instead would
 * quietly turn the dashboard into a second copy of `/inventory/ledger`.
 *
 * So the footer says which set is being narrowed, in words, on every render.
 * A search box that looks like the others and silently searches less is the one
 * way this could mislead, and saying "of the last 15 movements" is the fix.
 *
 * THE FILTER OPTIONS ARE DERIVED FROM THE ROWS, for the same reason: over a
 * fixed set of fifteen, every option that exists should match something. A
 * warehouse dropdown listing all six when four of them have not moved anything
 * this week is five dead ends and one answer.
 */
export function ActivityTable({
  widget,
  timezone,
}: {
  widget: RecentActivityWidget;
  /** The tenant's business timezone. Every timestamp renders in it, never the
   *  browser's (I7, FE15): two colleagues in different countries reading one
   *  ledger must agree about which day a movement fell on. */
  timezone: string;
}) {
  const { page, pageSize, setPage, setPageSize } = usePagination();
  const [search, setSearch] = useState("");
  const [movement, setMovement] = useState("");
  const [warehouseId, setWarehouseId] = useState("");

  const all = widget.entries;

  const warehouses = useMemo(() => {
    const seen = new Map<string, string>();
    for (const entry of all) seen.set(entry.warehouseId, entry.warehouseCode);
    return [...seen].map(([value, label]) => ({ value, label }));
  }, [all]);

  const movements = useMemo(() => {
    const seen = new Set(all.map((entry) => entry.entryType));
    return Object.entries(MOVEMENTS)
      .filter(([value]) => seen.has(value as LedgerEntry["entryType"]))
      .map(([value, label]) => ({ value, label }));
  }, [all]);

  const rows = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return all.filter((entry) => {
      if (movement !== "" && entry.entryType !== movement) return false;
      if (warehouseId !== "" && entry.warehouseId !== warehouseId) return false;
      if (needle === "") return true;
      // The same fields the ledger's own `q` searches, plus the document
      // number — on this screen the row a reader is hunting for is usually the
      // receipt they just posted.
      return [
        entry.sku,
        entry.productName,
        entry.note,
        entry.sourceNumber,
        entry.warehouseCode,
      ].some((field) => field?.toLowerCase().includes(needle));
    });
  }, [all, search, movement, warehouseId]);

  // Paged in the browser too, for the same reason as the filtering. `Pagination`
  // is the same component every server-paged list uses, so the control reads
  // identically wherever it appears — what differs is only where the slicing
  // happens, and the sentence under the heading is what says so.
  //
  // `current` is clamped rather than trusted. Every filter resets the page, so
  // it cannot go out of range through the UI today; a slice past the end renders
  // an empty tbody with no explanation — a silent blank table — and one
  // `Math.min` is cheaper than the next person having to know that.
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const current = Math.min(page, totalPages);
  const shown = rows.slice((current - 1) * pageSize, current * pageSize);

  return (
    <section aria-labelledby="activity-heading">
      <div className="mb-1 flex items-baseline justify-between gap-3">
        <h2 id="activity-heading" className="text-base font-semibold">
          Recent activity
        </h2>
        <Link
          to="/inventory/ledger"
          className="shrink-0 text-sm text-accent underline decoration-hairline underline-offset-2"
        >
          Full ledger
        </Link>
      </div>

      {/* The scope, said once and always, under the heading rather than in the
          pagination line — because the pagination line is about the *filtered*
          set and this is about the window the whole table sits in. It is the
          mitigation for a search box that looks like five server-side ones and
          searches fifteen rows. */}
      <p className="mb-4 text-sm text-secondary">
        The last <span className="tabular">{all.length}</span>{" "}
        {all.length === 1 ? "stock movement" : "stock movements"} — the full
        ledger goes back further.
      </p>

      {all.length === 0 ? (
        <div className="rounded-xl border border-hairline bg-surface px-4 py-10 text-center">
          <p className="text-sm text-secondary">
            No stock has moved yet. Receiving a purchase order or posting an
            adjustment writes the first entry.
          </p>
        </div>
      ) : (
        <>
          <FilterBar>
            <SearchInput
              label="Search recent activity"
              value={search}
              onChange={(next) => {
                setSearch(next);
                setPage(1);
              }}
              placeholder="SKU, product, note, or document"
            />
            <FilterDropdown
              label="Movement"
              value={movement}
              options={movements}
              allLabel="All movements"
              onChange={(next) => {
                setMovement(next);
                setPage(1);
              }}
            />
            <FilterDropdown
              label="Warehouse"
              value={warehouseId}
              options={warehouses}
              allLabel="All warehouses"
              onChange={(next) => {
                setWarehouseId(next);
                setPage(1);
              }}
            />
          </FilterBar>

          <TableFrame>
            <table className="w-full min-w-[44rem] text-left text-sm">
              <TableHead
                columns={[
                  { label: "When" },
                  { label: "Product" },
                  { label: "Warehouse" },
                  { label: "Change", align: "right" },
                  { label: "Source" },
                ]}
              />
              {rows.length === 0 ? (
                <tbody>
                  <tr className="border-t border-hairline">
                    <td colSpan={5} className="px-3 py-10 text-center">
                      <p className="text-sm text-secondary">
                        Nothing in the last {all.length} movements matches those
                        filters.
                      </p>
                      <Link
                        to="/inventory/ledger"
                        className="mt-2 inline-block text-sm text-accent underline decoration-hairline underline-offset-2"
                      >
                        Search the full ledger
                      </Link>
                    </td>
                  </tr>
                </tbody>
              ) : (
                <tbody>
                  {shown.map((entry) => (
                    <tr key={entry.id} className={tableRow}>
                      <td className="tabular px-3 py-3 text-secondary">
                        {formatDateTime(entry.occurredAt, timezone)}
                      </td>
                      <td className="px-3 py-3">
                        <Link
                          to={`/inventory/products/${entry.productId}`}
                          className="text-accent"
                        >
                          {entry.productName}
                        </Link>
                        <div className="tabular text-xs text-secondary">
                          {entry.sku}
                          {entry.productDeleted && (
                            // The product was tidied away; the movement still
                            // happened (§6.9.1). Saying so beats leaving the
                            // reader to wonder why it is not in the product list.
                            <span className="ml-2">deleted</span>
                          )}
                        </div>
                      </td>
                      <td className="tabular px-3 py-3">
                        {entry.warehouseCode}
                      </td>
                      <td
                        className={`tabular px-3 py-3 text-right font-medium ${
                          entry.qtyDelta < 0 ? "text-danger" : "text-success"
                        }`}
                      >
                        {formatDelta(entry.qtyDelta)}
                      </td>
                      <td className="px-3 py-3">
                        <SourceLink entry={entry} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              )}
            </table>
          </TableFrame>

          <Pagination
            page={current}
            pageSize={pageSize}
            totalItems={rows.length}
            totalPages={totalPages}
            onPage={setPage}
            onPageSize={setPageSize}
          />
        </>
      )}
    </section>
  );
}

/** The source document, or the person, depending on which one there is.
 *
 *  A manual adjustment has no document behind it — the person is the source
 *  (§6.3) — so its row names who made it instead of pretending to link
 *  somewhere. */
function SourceLink({ entry }: { entry: LedgerEntry }) {
  if (entry.sourceType === "goods_receipt" && entry.sourcePoId) {
    return (
      <Link
        to={`/procurement/orders/${entry.sourcePoId}`}
        className="tabular text-accent"
      >
        {entry.sourceNumber ?? "Goods receipt"}
      </Link>
    );
  }
  return (
    <span className="text-secondary">Adjustment by {entry.createdByName}</span>
  );
}
