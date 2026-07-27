import type { ReactNode } from "react";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { getDashboardSummary, type DashboardSummary } from "../../lib/api";
import { formatMoney, formatQty } from "../../lib/format";
import { ActivityTable } from "./ActivityTable";
import { StatTile } from "./StatTile";

/**
 * `/` — the dashboard of §10.2, in one request.
 *
 * WHICH WIDGETS APPEAR IS THE SERVER'S DECISION, NOT THIS FILE'S. The response
 * omits a widget the caller cannot read, and this component renders exactly what
 * it is given. That is deliberate and it is not the usual `holds(...)` pattern:
 * every other screen hides controls cosmetically and lets the server refuse the
 * request behind them, but here there is no request behind a widget to refuse —
 * the data would already be in the payload. Filtering server-side is what makes
 * the omission real rather than decorative.
 *
 * ------------------------------------------------------------------------
 * THE LAYOUT: THREE NUMBERS AND ONE TABLE. NOTHING ELSE.
 *
 * It was four cards in two columns, each leading with its own figure. Then it was
 * a stat strip over an approval queue over two side-by-side panels. This is the
 * third arrangement and the first that is not fighting itself, because it stopped
 * saying anything twice:
 *
 *   - **The approval queue was the "Awaiting approval" tile again**, with a list
 *     under it. Both went to the same place; one of them was a whole panel.
 *   - **The Low stock panel was the "Below reorder point" tile again**, with the
 *     same rows the tile's destination shows.
 *   - **The activity feed was a narrow column several times taller than anything
 *     beside it**, which is what made every version of this page look lopsided.
 *
 * So: the tiles are the summary, each tile is the way in to its own screen, and
 * the one thing that is genuinely *only* here — the last fifteen movements — gets
 * the full width and becomes a table like every other list in the application.
 *
 *   [ three tiles ]      the summary, one row, one baseline
 *   [ recent activity ]  full width, with the standard filter row
 *
 * WHAT THIS GAVE UP, RECORDED HONESTLY. §10.7.1 asks for requisition approval as
 * "a two-button decision between meetings", and the queue panel was that. It is
 * gone, so approving now means opening the requisition. That was the owner's call
 * against the duplication, and it is the one thing to reverse first if the
 * two-button decision turns out to matter more than the tidiness — see
 * `PROGRESS.md`.
 *
 * The low-stock panel's "Create requisition" shortcut was *not* given up: it moved
 * to `/inventory/stock`, which is where the tile now points and where somebody
 * looking at low stock already is.
 *
 * THE TILE ORDER IS DELIBERATE AND FIXED: what needs a decision, then what is at
 * risk, then what is in flight. Urgency descending, and stable no matter which
 * subset of tiles a given identity gets — a strip whose order depends on your
 * entitlements is a strip you have to read rather than recognise.
 */
export function DashboardPage() {
  const me = useMe();
  const { state, reload } = useAsync("dashboard", getDashboardSummary);

  if (state.status === "error") {
    return (
      <AppShell title="Dashboard">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }
  if (state.status === "loading") {
    return (
      <AppShell title="Dashboard">
        <DashboardSkeleton />
      </AppShell>
    );
  }

  const summary = state.data;
  const nothing =
    !summary.openOrders &&
    !summary.pendingApprovals &&
    !summary.lowStock &&
    !summary.recentActivity;

  return (
    <AppShell title={`Good to see you, ${me.user.fullName.split(" ")[0]}`}>
      {nothing ? (
        <EmptyDashboard superadmin={me.user.tenantRole === "superadmin"} />
      ) : (
        <div className="space-y-8">
          <StatStrip summary={summary} />

          {summary.recentActivity && me.tenant && (
            <ActivityTable
              widget={summary.recentActivity}
              timezone={me.tenant.timezone}
            />
          )}
        </div>
      )}
    </AppShell>
  );
}

/** The headline numbers, in fixed urgency order. */
function StatStrip({ summary }: { summary: DashboardSummary }) {
  const tiles: ReactNode[] = [];

  if (summary.pendingApprovals) {
    const { count, canApprove } = summary.pendingApprovals;
    tiles.push(
      <StatTile
        key="approvals"
        label="Awaiting approval"
        value={count}
        // Three different sentences for three different situations, because
        // "3 requisitions waiting" tells an approver nothing they did not
        // already know from the number.
        detail={
          count === 0
            ? "Nothing waiting on a decision"
            : canApprove
              ? count === 1
                ? "1 requisition needs you"
                : `${count} requisitions need you`
              : "An approver decides these"
        }
        href="/procurement/requisitions?status=submitted"
        attention={count > 0 && canApprove}
      />,
    );
  }

  if (summary.lowStock) {
    const { count, products } = summary.lowStock;
    const worst = products[0];
    tiles.push(
      <StatTile
        key="low-stock"
        label="Below reorder point"
        value={count}
        // The worst one by name. "3 products are low" is the number again in
        // words; "PKG-BOX-S is short 60 pcs" is the thing to do something about.
        detail={
          worst
            ? `${worst.sku} short ${formatQty(worst.shortfall)} ${worst.uom}`
            : "Everything is above its reorder point"
        }
        href="/inventory/stock"
        attention={count > 0}
      />,
    );
  }

  if (summary.openOrders) {
    const { count, totalValue } = summary.openOrders;
    tiles.push(
      <StatTile
        key="open-orders"
        label="Open purchase orders"
        value={count}
        detail={
          count === 0
            ? "Nothing is on order"
            : `Worth ${formatMoney(totalValue)}`
        }
        href="/procurement/orders"
      />,
    );
  }

  if (tiles.length === 0) return null;
  return (
    <ul
      aria-label="Summary"
      className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
    >
      {tiles}
    </ul>
  );
}

/** Skeletons in the shape of the real thing, so nothing lurches when the data
 *  lands (§10.7.6) — a strip of tiles over one wide table. */
function DashboardSkeleton() {
  return (
    <div className="space-y-8">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="h-28 animate-pulse rounded-xl border border-hairline bg-surface"
          />
        ))}
      </div>
      <div className="h-96 animate-pulse rounded-xl border border-hairline bg-surface" />
    </div>
  );
}

/**
 * The dashboard with no widgets at all, which happens to exactly two kinds of
 * caller — and they need different sentences.
 *
 * A superadmin has no tenant and so no business data by design (§5.5); telling
 * them to ask their administrator would be telling them to ask themselves. An
 * employee with no modules has a real problem somebody else has to fix.
 */
function EmptyDashboard({ superadmin }: { superadmin: boolean }) {
  return (
    <div className="max-w-xl rounded-xl border border-hairline bg-surface p-6">
      <p className="text-sm text-secondary">
        {superadmin
          ? "You administer workspaces rather than working inside one, so there is nothing here to count. Open Workspaces to manage tenants and their modules."
          : "You have not been given access to any module yet. Your workspace administrator assigns these, and this page fills in as soon as they do."}
      </p>
    </div>
  );
}
