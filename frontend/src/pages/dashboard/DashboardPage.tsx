import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import { getDashboardSummary } from "../../lib/api";
import { formatMoney } from "../../lib/format";
import { holds } from "../../lib/levels";
import { ApprovalQueue } from "./ApprovalQueue";
import { LowStockCard } from "./LowStockCard";
import { RecentActivityCard } from "./RecentActivityCard";
import { WidgetCard, WidgetEmpty, WidgetFigure } from "./WidgetCard";

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
 * The one thing decided here is the *shortcut* on the low-stock widget, which
 * crosses modules: seeing that stock is low is Inventory, asking for more of it
 * is Procurement, and plenty of people can do one and not the other.
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
        {/* Skeletons at the real card height, so nothing lurches when the data
            lands (§10.7.6). */}
        <div className="grid gap-4 lg:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <div
              key={i}
              className="h-48 animate-pulse rounded-lg border border-hairline bg-surface"
            />
          ))}
        </div>
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
        // One column on a phone, two from `lg`. Not three: the approval queue and
        // the activity feed are both tall, and a third column would make every
        // card narrow enough that the queue's two buttons stack.
        <div className="grid items-start gap-4 lg:grid-cols-2">
          {summary.openOrders && (
            <WidgetCard title="Open purchase orders" href="/procurement/orders">
              <WidgetFigure
                value={String(summary.openOrders.count)}
                caption={
                  summary.openOrders.count === 1
                    ? "order still expecting goods"
                    : "orders still expecting goods"
                }
              />
              {summary.openOrders.count === 0 ? (
                <WidgetEmpty>Nothing is on order.</WidgetEmpty>
              ) : (
                <p className="mt-4 text-sm text-secondary">
                  Worth{" "}
                  <span className="tabular text-primary">
                    {formatMoney(summary.openOrders.totalValue)}
                  </span>{" "}
                  in total.
                </p>
              )}
            </WidgetCard>
          )}

          {summary.pendingApprovals && (
            <ApprovalQueue
              widget={summary.pendingApprovals}
              meId={me.user.id}
              onDecided={reload}
            />
          )}

          {summary.lowStock && (
            <LowStockCard
              widget={summary.lowStock}
              canRaise={holds(me.moduleRoles, "procurement", "user")}
            />
          )}

          {summary.recentActivity && me.tenant && (
            <RecentActivityCard
              widget={summary.recentActivity}
              timezone={me.tenant.timezone}
            />
          )}
        </div>
      )}
    </AppShell>
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
    <div className="max-w-xl rounded-lg border border-hairline bg-surface p-6">
      <p className="text-sm text-secondary">
        {superadmin
          ? "You administer workspaces rather than working inside one, so there is nothing here to count. Open Workspaces to manage tenants and their modules."
          : "You have not been given access to any module yet. Your workspace administrator assigns these, and this page fills in as soon as they do."}
      </p>
    </div>
  );
}
