// The dashboard — §9.7's one endpoint and §10.2's four widgets.
//
// THE THING THAT MAKES THIS ENDPOINT DIFFERENT FROM EVERY OTHER ONE.
//
// It is the only route below /api that spans modules, so it cannot be gated by
// RequireModule: the four widgets answer to two different modules, and a caller
// entitled to one of them must get that one rather than a 403 for the other.
// The gate is therefore inside, per widget, through the same LevelFor every
// other permission check goes through — and a widget the caller cannot read is
// **absent from the response**, not present and empty.
//
// Absent rather than empty matters. `{"lowStock": {"count": 0}}` says "nothing
// is low"; the widget's absence says "you cannot see Inventory". The frontend
// renders exactly the widgets it is given, so the two cannot be confused, and a
// user with no Inventory access never sees a stock panel reporting zero.
//
// This is still cosmetic in the sense of I12: every screen the widgets link to
// is independently gated, and each widget's own query would be refused on its
// real endpoint too. What the filtering buys is a dashboard that does not lie.
package api

import (
	"github.com/gofiber/fiber/v2"

	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/internal/middleware"
)

// approvalQueueSize and lowStockPreview cap the two widgets that carry rows.
//
// The counts beside them are the real totals, computed over everything — a
// widget that says "5 awaiting approval" and lists five when there are eleven is
// worse than one that says eleven and lists five, because only the second sends
// the reader to the full list.
const (
	approvalQueueSize   = 5
	lowStockPreview     = 5
	recentActivityLimit = 15 // §10.2, in as many words
)

// openOrdersWidget is §10.2 widget 1.
type openOrdersWidget struct {
	Count int `json:"count"`
	// TotalValue is SUM(qty_ordered × unit_cost) over the outstanding orders,
	// computed by PostgreSQL where both operands are still NUMERIC (I8).
	TotalValue httpx.Numeric `json:"totalValue"`
}

// pendingApprovalsWidget is §10.2 widget 2.
type pendingApprovalsWidget struct {
	Count int `json:"count"`
	// CanApprove is whether this caller holds `approver` in procurement, and so
	// whether Queue is populated at all. Sent explicitly rather than left to be
	// inferred from an empty Queue, which would be indistinguishable from "there
	// is nothing waiting".
	CanApprove bool `json:"canApprove"`
	// Queue is the inline approve/reject queue §10.2 gives to `approver`+, and is
	// empty for everybody else. Rows carry the full requisition shape because the
	// queue has to say what is being approved — a PR number alone is not a
	// decision anybody should make.
	Queue []requisitionRow `json:"queue"`
}

// lowStockWidget is §10.2 widget 3.
type lowStockWidget struct {
	Count int `json:"count"`
	// Products is the worst few by shortfall — the same ordering the low-stock
	// list defaults to, so the widget is the top of that page rather than a
	// different selection from it.
	Products []lowStockRow `json:"products"`
}

// recentActivityWidget is §10.2 widget 4.
type recentActivityWidget struct {
	Entries []ledgerRow `json:"entries"`
}

// dashboardSummary is the §9.7 response. Every field is a pointer so that a
// widget the caller cannot read is omitted entirely rather than sent as a zero
// value — see the file comment.
type dashboardSummary struct {
	OpenOrders       *openOrdersWidget       `json:"openOrders,omitempty"`
	PendingApprovals *pendingApprovalsWidget `json:"pendingApprovals,omitempty"`
	LowStock         *lowStockWidget         `json:"lowStock,omitempty"`
	RecentActivity   *recentActivityWidget   `json:"recentActivity,omitempty"`
}

// getDashboardSummary is GET /api/dashboard/summary (§9.7).
//
// No RequireModule on the route, so this handler cannot use tenantScope: that
// helper's contract is "a miss here is a wiring bug", which is true only of
// routes a superadmin cannot reach, and a superadmin can reach this one. The
// tenantless caller gets an empty summary rather than a 500 — they have no
// tenant, so there is genuinely nothing to count, and §5.5 says a superadmin
// reads no tenant business data anyway.
func (s *server) getDashboardSummary(c *fiber.Ctx) error {
	caller := middleware.IdentityFrom(c)
	if caller == nil {
		return httpx.Unauthenticated(c)
	}

	summary := dashboardSummary{}

	tx := middleware.TxFrom(c)
	if tx == nil {
		// A superadmin. Every widget stays absent, which is the same answer the
		// nav gives them: they administer workspaces, not stock.
		return c.JSON(summary)
	}

	if caller.LevelFor(ModuleProcurement) >= identity.RoleViewer {
		orders, err := s.readOpenOrders(c)
		if err != nil {
			return err
		}
		summary.OpenOrders = orders

		approvals, err := s.readPendingApprovals(c, caller)
		if err != nil {
			return err
		}
		summary.PendingApprovals = approvals
	}

	if caller.LevelFor(ModuleInventory) >= identity.RoleViewer {
		low, err := s.readLowStock(c)
		if err != nil {
			return err
		}
		summary.LowStock = low

		activity, err := s.readRecentActivity(c)
		if err != nil {
			return err
		}
		summary.RecentActivity = activity
	}

	return c.JSON(summary)
}

// readOpenOrders counts the orders still expecting goods, and what they are
// worth.
//
// "Open" here means outstanding — `open` **and** `partially_received` — not the
// single status of that name. An order half of which has arrived is emphatically
// still an order somebody is waiting on, and a widget that dropped it the moment
// the first box landed would count down to zero while goods were still in
// transit. The PO list this links to is where the two are told apart.
//
// The value is the whole ordered value, not the outstanding remainder. That is
// the commitment the company has made, which is the question a dashboard number
// is answering; the remainder is a per-line quantity the order screen shows.
func (s *server) readOpenOrders(c *fiber.Ctx) (*openOrdersWidget, error) {
	tx := middleware.TxFrom(c)

	var rows []openOrdersWidget
	if err := tx.Raw(`
		SELECT count(*) AS count,
		       COALESCE(SUM(v.value), 0)::numeric(18,2)::text AS total_value
		FROM purchase_orders po
		LEFT JOIN (
		  SELECT po_id, SUM(qty_ordered * unit_cost) AS value
		  FROM purchase_order_lines GROUP BY po_id
		) v ON v.po_id = po.id
		WHERE po.status IN ('open', 'partially_received')`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		// count(*) over no rows is still one row of zero, so this is unreachable.
		return &openOrdersWidget{TotalValue: httpx.Zero}, nil
	}
	return &rows[0], nil
}

// readPendingApprovals counts what is waiting, and for an approver lists the
// oldest few so the decision can be made without leaving the dashboard.
//
// The queue is NOT filtered to requisitions this caller may actually approve.
// Their own submissions stay in it, because C2 refuses self-approval for
// everybody and hiding those rows would make the count disagree with the list
// directly underneath it — "3 awaiting approval" above two rows is a bug report
// waiting to be filed. The row carries `requestedById`, and the screen compares
// it with `me.user.id` to disable its own buttons and say why. The server
// refuses regardless (I12).
func (s *server) readPendingApprovals(c *fiber.Ctx, caller *identity.Identity) (*pendingApprovalsWidget, error) {
	tx := middleware.TxFrom(c)

	widget := &pendingApprovalsWidget{
		CanApprove: caller.LevelFor(ModuleProcurement) >= identity.RoleApprover,
		Queue:      []requisitionRow{},
	}

	if err := tx.Raw(`
		SELECT count(*) FROM purchase_requisitions WHERE status = 'submitted'`).
		Scan(&widget.Count).Error; err != nil {
		return nil, err
	}
	if !widget.CanApprove || widget.Count == 0 {
		return widget, nil
	}

	// Oldest first: the queue is a backlog, and the thing that has been waiting
	// longest is the thing to look at. Through requisitionRows, so the queue
	// cannot render a requisition differently from the list screen.
	rows, err := s.requisitionRows(tx, `
		WHERE r.status = 'submitted'
		ORDER BY r.submitted_at ASC, r.id ASC
		LIMIT ?`, approvalQueueSize)
	if err != nil {
		return nil, err
	}
	if rows != nil {
		widget.Queue = rows
	}
	return widget, nil
}

// readLowStock is the top of the low-stock list, and its full count.
//
// Both halves come from lowStockFrom, which is also what GET
// /inventory/stock/low reads — so the number here and the number of rows there
// are the same question asked once.
func (s *server) readLowStock(c *fiber.Ctx) (*lowStockWidget, error) {
	tx := middleware.TxFrom(c)

	widget := &lowStockWidget{Products: []lowStockRow{}}
	if err := tx.Raw(`SELECT count(*) ` + lowStockFrom).Scan(&widget.Count).Error; err != nil {
		return nil, err
	}
	if widget.Count == 0 {
		return widget, nil
	}

	var rows []lowStockRow
	if err := tx.Raw(lowStockSelect+lowStockFrom+`
		ORDER BY (p.reorder_point - COALESCE(b.qty_on_hand, 0)) DESC, p.id ASC
		LIMIT ?`, lowStockPreview).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if rows != nil {
		widget.Products = rows
	}
	return widget, nil
}

// readRecentActivity is the last fifteen stock movements (§10.2).
//
// Through ledgerSelect, so each row already resolves its source document's
// number — which is what makes "each linking to its source" a link rather than a
// UUID the reader has to go and look up.
func (s *server) readRecentActivity(c *fiber.Ctx) (*recentActivityWidget, error) {
	tx := middleware.TxFrom(c)

	widget := &recentActivityWidget{Entries: []ledgerRow{}}

	// `l.id` breaks the tie, because the seed and a bulk receipt both write
	// several rows at one instant and an unstable order would shuffle the widget
	// between two reloads that saw identical data.
	var rows []ledgerRow
	if err := tx.Raw(ledgerSelect+ledgerFrom+`
		ORDER BY l.occurred_at DESC, l.id DESC
		LIMIT ?`, recentActivityLimit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if rows != nil {
		widget.Entries = rows
	}
	return widget, nil
}
