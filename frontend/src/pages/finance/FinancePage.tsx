import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import {
  EmptyState,
  ErrorNotice,
  frozenCell,
  Pagination,
  ScrollableTable,
  SkeletonRows,
  SourceFilterNotice,
  TableHead,
  tableRow,
} from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { usePagination } from "../../hooks/usePagination";
import { listAccounts, listJournalEntries } from "../../lib/api";
import { formatDateTime, formatMoney } from "../../lib/format";

const COLUMNS = 5;

/**
 * `/finance` — the whole Finance module (§10.5).
 *
 * The header says "coming soon" and means it: there is no reporting here, no
 * trial balance, no period close, and no way to post an entry by hand. What
 * there is, is the journal itself, live.
 *
 * That is the point of the screen rather than a consolation prize. An empty
 * placeholder says "unfinished"; a list of real postings under a "coming soon"
 * header says "the cross-module write works, and this module is where you will
 * eventually read it properly". Every row below was written inside the same
 * transaction as a goods receipt — if one is here, the stock movement and the
 * receipt document are too, because none of the three could have committed
 * without the others.
 */
export function FinancePage() {
  const me = useMe();
  const timezone = me.tenant?.timezone ?? "UTC";

  const { page, pageSize, setPage, setPageSize, key } = usePagination();
  const [accountId, setAccountId] = useState("");

  // `?sourceId=` narrows the journal to the entry one document posted. It comes
  // from the URL rather than from a control, because nobody would type it — it
  // is what "journal entry JE-… posted" links to on the goods receipt
  // confirmation panel, and it has to keep working when that link is pasted to a
  // colleague.
  const [params] = useSearchParams();
  const sourceId = params.get("sourceId") ?? "";

  const accounts = useAsync("finance-accounts", () =>
    listAccounts({ pageSize: 100, sort: "code" }),
  );

  const { state, reload } = useAsync(
    `journal:${key}:${accountId}:${sourceId}`,
    () =>
      listJournalEntries({
        page,
        pageSize,
        accountId: accountId || undefined,
        sourceId: sourceId || undefined,
        sort: "-postedAt",
      }),
  );

  const filtered = accountId !== "" || sourceId !== "";

  return (
    <AppShell title="Finance — coming soon">
      <p className="mb-6 max-w-2xl text-sm text-secondary">
        Postings from other modules are already flowing in. Reporting and period
        close are not built yet.
      </p>

      {/* The chart of accounts. Seeded when the workspace was created and not
          editable here — an editable chart needs rules about accounts that
          already carry postings, and those belong with the real Finance module
          (§9.6). Each row filters the journal below it, which is the only thing
          you can currently do with an account. */}
      <section className="mb-6" aria-labelledby="chart-heading">
        <h2 id="chart-heading" className="mb-2 text-sm font-medium">
          Chart of accounts
        </h2>
        <div className="flex flex-wrap gap-2">
          {/* The same skeleton-not-spinner rule as every list (§10.7.6): two
              placeholders the size of the two seeded accounts, so the journal
              below does not jump down when they arrive. */}
          {accounts.state.status === "loading" &&
            [0, 1].map((n) => (
              <div
                key={n}
                className="h-11 w-48 animate-pulse rounded-md border border-hairline bg-subtle"
              />
            ))}
          {accounts.state.status === "ready" &&
            accounts.state.data.data.map((account) => {
              const active = accountId === account.id;
              return (
                <button
                  key={account.id}
                  type="button"
                  aria-pressed={active}
                  onClick={() => {
                    setAccountId(active ? "" : account.id);
                    setPage(1);
                  }}
                  className={`min-h-11 rounded-lg border px-3 text-left text-sm transition-colors ${
                    active
                      ? "border-accent bg-subtle"
                      : "border-hairline hover:bg-subtle"
                  }`}
                >
                  <span className="tabular font-medium">{account.code}</span>{" "}
                  {account.name}
                  <span className="ml-2 text-xs text-secondary">
                    {account.type}
                  </span>
                </button>
              );
            })}
          {accounts.state.status === "error" && (
            <p className="text-sm text-secondary">
              The chart of accounts could not be loaded.
            </p>
          )}
        </div>
      </section>

      <SourceFilterNotice
        showing="the posting from one document"
        clearLabel="Show all postings"
        sourceNumber={
          (state.status === "ready" && state.data.data[0]?.sourceNumber) || null
        }
        onCleared={() => setPage(1)}
      />

      <h2 className="mb-2 text-sm font-medium">Journal</h2>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          {/* Phase 7.5's finding 9. This is the widest table in the application at
              `min-w-[52rem]`, so at 360px the Amount column is well off-screen —
              and scrolling to it used to take the entry number with it, leaving a
              column of money belonging to nothing. `frozenCell` had gone to the
              stock grid and the ledger and not here.

              The Entry column also had to become the *first* one for that to work:
              only the first column can sensibly be frozen, and a timestamp is the
              wrong thing to pin anyway. §10.0.2 — "document numbers are identity,
              not metadata; treat it as the primary label rather than hiding it as
              grey small print" — says the reorder is the right way round
              regardless of the scrolling. */}
          <ScrollableTable>
            <table className="w-full min-w-[52rem] text-left text-sm">
              <TableHead
                sticky
                columns={[
                  { label: "Entry", sticky: true },
                  { label: "Posted" },
                  { label: "Posting" },
                  { label: "Source" },
                  { label: "Amount", align: "right" },
                ]}
              />

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={filtered}
                  firstRun="Nothing has been posted yet. Receive goods against a purchase order and the entry appears here."
                  noResults="No postings match that filter."
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((entry) => (
                    // `group` so the frozen cell follows the row's hover; without
                    // it the pinned column stays surface-coloured while the rest
                    // of the row lifts.
                    <tr
                      key={entry.id}
                      className={`group align-top ${tableRow}`}
                    >
                      <td className={`px-3 py-3 ${frozenCell}`}>
                        <span className="tabular font-medium">
                          {entry.entryNumber}
                        </span>
                        <div className="text-xs text-secondary">
                          {entry.description}
                        </div>
                      </td>
                      <td className="px-3 py-3 text-secondary">
                        {formatDateTime(entry.postedAt, timezone)}
                      </td>
                      {/* The double entry, written the way a journal is read:
                          the debit side above the credit side, each naming its
                          account. The server orders the lines, so this cell does
                          not decide which is which. */}
                      <td className="px-3 py-3">
                        <ul className="space-y-1">
                          {entry.lines.map((line) => (
                            <li key={line.id} className="flex gap-2">
                              <span className="tabular w-6 text-secondary">
                                {line.debit > 0 ? "Dr" : "Cr"}
                              </span>
                              <span className="tabular w-12">
                                {line.accountCode}
                              </span>
                              <span className="min-w-0 grow">
                                {line.accountName}
                              </span>
                              <span className="tabular whitespace-nowrap">
                                {formatMoney(
                                  line.debit > 0 ? line.debit : line.credit,
                                )}
                              </span>
                            </li>
                          ))}
                        </ul>
                      </td>
                      <td className="px-3 py-3">
                        {/* A goods receipt names its document and links to the
                            order it arrived against — the counterpart of the
                            stock ledger's own source column (§10.4). */}
                        {entry.sourceNumber && entry.sourcePoId ? (
                          <Link
                            to={`/procurement/orders/${entry.sourcePoId}`}
                            className="tabular underline decoration-hairline underline-offset-2"
                          >
                            {entry.sourceNumber}
                          </Link>
                        ) : (
                          <span className="text-secondary">
                            {entry.sourceType}
                          </span>
                        )}
                        <div className="text-xs text-secondary">
                          {entry.createdByName}
                        </div>
                      </td>
                      <td className="tabular px-3 py-3 text-right font-medium">
                        {formatMoney(entry.amount)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              )}
            </table>
          </ScrollableTable>

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
