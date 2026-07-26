package main

// The procurement and inventory history, as a plan.
//
// ONE RECIPE, BOTH TENANTS. Every reference below is an *index* into the
// tenant's own products, suppliers, and warehouses, so the two workspaces get
// the same shape of history over completely different data. That is what makes
// them comparable when a reviewer opens both, and it is why there is no second
// copy of this table with Bahari's names in it.
//
// The volumes are §15's, exactly, and `verifyDocumentCounts` checks them after
// the fact — a table like this is easy to edit into disagreeing with the
// specification it transcribes.
//
//	requisitions   2 draft · 3 submitted · 1 rejected · 1 cancelled · 6 approved
//	orders         2 open · 1 partially_received · 2 received · 1 cancelled
//
// The six approved requisitions are what produce the six orders (§8.3): there is
// no such thing here as an order without a requisition behind it, because there
// is no endpoint that creates one.

// actor names which of the tenant's users did something. The seed cannot use a
// user's index — Nusantara has four and Bahari three — so it uses the role they
// play, and each tenant maps the role to one of its own people.
type actor int

const (
	// actorRaiser holds `user` in procurement: the person who writes
	// requisitions. Sari at Nusantara, Rudi at Bahari.
	actorRaiser actor = iota
	// actorApprover holds `approver`: decides requisitions, and receives goods.
	// Budi at Nusantara, Intan at Bahari.
	//
	// Deliberately never the same person as actorRaiser, which is C2 —
	// segregation of duties. The seed writes these rows directly and so is not
	// held to it by the handler, and a demo whose own history breaks the rule it
	// is demonstrating would be the first thing a reader noticed.
	actorApprover
)

// linePlan is one line: which product, and how much of it.
type linePlan struct {
	Product int
	Qty     string
}

// receiptPlan is one delivery against the order a requisition became.
type receiptPlan struct {
	DaysAgo int
	// Lines maps a position in the requisition's Lines to the quantity that
	// arrived. A line absent from the map did not arrive at all, which is what
	// makes an order `partially_received`.
	Lines map[int]string
	Note  string
}

// requisitionPlan is one requisition and everything that happened to it.
type requisitionPlan struct {
	// Seq is this requisition's stable identity inside its tenant. It is what
	// the deterministic UUID is derived from, so re-running the seed writes to
	// the same rows rather than to new ones — renumbering these is renumbering
	// the demo database.
	Seq int

	Status  string
	DaysAgo int

	Warehouse int
	// Supplier is an index, or -1 for a requisition that does not name one yet.
	// A draft legitimately may not: choosing who to buy from can wait until
	// approval, which is where it becomes mandatory (§8.3).
	Supplier int
	Raiser   actor
	Notes    string
	Lines    []linePlan

	// Reason is the rejection or cancellation reason. Mandatory for `rejected`
	// in the database (`pr_reject_needs_reason`, G13) and mandatory for
	// `cancelled` in the handler.
	Reason string

	// Order describes the purchase order an `approved` requisition produced.
	// Empty for every other status.
	Order *orderPlan
}

// orderPlan is the purchase order behind an approved requisition.
type orderPlan struct {
	// ExpectedInDays is relative to today, so `open` orders come out as a mix of
	// overdue and not — §15 wants "overdue" visually distinguishable, and an
	// order whose expected date is always in the future can never show it.
	ExpectedInDays int
	// Cancelled makes this the one cancelled order. It is set at insert rather
	// than by a later UPDATE, because `po_terminal_immutable` refuses to modify
	// a row that is already terminal and there is no transition worth faking.
	Cancelled bool
	Reason    string
	// Receipts move the order to `partially_received` or `received`. The seed
	// does NOT set those statuses: PostGoodsReceipt does, in step 4, from
	// po_line_status. An order the seed declared `received` with no receipt
	// behind it would be a lie the whole demo rests on.
	Receipts []receiptPlan
}

// The thirteen requisitions, oldest last so the reader can follow the story
// downwards: the recent activity is at the top, the completed history at the
// bottom.
var requisitionPlans = []requisitionPlan{
	{
		Seq: 1, Status: "draft", DaysAgo: 5, Warehouse: 1, Supplier: -1, Raiser: actorRaiser,
		Notes: "Waiting on a quote before submitting.",
		Lines: []linePlan{{Product: 6, Qty: "50"}, {Product: 7, Qty: "20"}},
	},
	{
		Seq: 2, Status: "draft", DaysAgo: 12, Warehouse: 0, Supplier: 0, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 1, Qty: "100"}},
	},
	{
		Seq: 3, Status: "submitted", DaysAgo: 3, Warehouse: 0, Supplier: 2, Raiser: actorRaiser,
		Notes: "Running low ahead of the month end.",
		Lines: []linePlan{{Product: 7, Qty: "25"}},
	},
	{
		Seq: 4, Status: "submitted", DaysAgo: 6, Warehouse: 1, Supplier: 0, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 1, Qty: "200"}, {Product: 6, Qty: "40"}},
	},
	{
		// Submitted with no supplier chosen, so the inline approve button on the
		// dashboard has the case it must refuse: approval generates an order
		// addressed to somebody, and there is nowhere in a queue row to pick one.
		Seq: 5, Status: "submitted", DaysAgo: 9, Warehouse: 0, Supplier: -1, Raiser: actorRaiser,
		Notes: "Supplier to be decided — two quotes still outstanding.",
		Lines: []linePlan{{Product: 3, Qty: "30"}},
	},
	{
		Seq: 6, Status: "rejected", DaysAgo: 20, Warehouse: 0, Supplier: 3, Raiser: actorRaiser,
		Lines:  []linePlan{{Product: 6, Qty: "500"}},
		Reason: "Far more than the branch can store. Re-raise for a quarter of this.",
	},
	{
		Seq: 7, Status: "cancelled", DaysAgo: 16, Warehouse: 1, Supplier: 0, Raiser: actorRaiser,
		Lines:  []linePlan{{Product: 3, Qty: "10"}},
		Reason: "Raised against the wrong warehouse.",
	},
	{
		Seq: 8, Status: "approved", DaysAgo: 11, Warehouse: 0, Supplier: 0, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 1, Qty: "150"}, {Product: 7, Qty: "40"}},
		Order: &orderPlan{ExpectedInDays: 4},
	},
	{
		// Overdue: expected three days ago and nothing has arrived.
		Seq: 9, Status: "approved", DaysAgo: 24, Warehouse: 1, Supplier: 2, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 3, Qty: "60"}},
		Order: &orderPlan{ExpectedInDays: -3},
	},
	{
		Seq: 10, Status: "approved", DaysAgo: 33, Warehouse: 0, Supplier: 1, Raiser: actorRaiser,
		Notes: "Bulk order, split delivery agreed with the supplier.",
		Lines: []linePlan{{Product: 6, Qty: "120"}, {Product: 1, Qty: "80"}},
		Order: &orderPlan{
			ExpectedInDays: 2,
			// One of two lines, and only part of it: the order comes out
			// `partially_received` because po_line_status says something is still
			// outstanding, not because the seed said so.
			Receipts: []receiptPlan{
				{DaysAgo: 4, Lines: map[int]string{0: "70"}, Note: "First of two deliveries."},
			},
		},
	},
	{
		Seq: 11, Status: "approved", DaysAgo: 45, Warehouse: 0, Supplier: 0, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 1, Qty: "120"}, {Product: 3, Qty: "25"}},
		Order: &orderPlan{
			ExpectedInDays: -38,
			// Everything, in two deliveries — so the order detail screen has a
			// receipt history with more than one row in it.
			Receipts: []receiptPlan{
				{DaysAgo: 40, Lines: map[int]string{0: "120"}},
				{DaysAgo: 38, Lines: map[int]string{1: "25"}, Note: "Balance of the order."},
			},
		},
	},
	{
		Seq: 12, Status: "approved", DaysAgo: 52, Warehouse: 1, Supplier: 2, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 7, Qty: "60"}},
		Order: &orderPlan{
			ExpectedInDays: -45,
			Receipts: []receiptPlan{
				{DaysAgo: 46, Lines: map[int]string{0: "60"}, Note: "Complete on the first delivery."},
			},
		},
	},
	{
		Seq: 13, Status: "approved", DaysAgo: 28, Warehouse: 1, Supplier: 3, Raiser: actorRaiser,
		Lines: []linePlan{{Product: 6, Qty: "90"}},
		Order: &orderPlan{
			ExpectedInDays: -14,
			Cancelled:      true,
			Reason:         "Supplier could not meet the delivery window.",
		},
	},
}

// adjustmentPlan is one manual stock adjustment: the §6.3 case where the person
// is the source and there is no document behind the movement.
type adjustmentPlan struct {
	Seq       int
	DaysAgo   int
	Product   int
	Warehouse int
	// Qty is signed decimal text. At least one is negative, per §15 — a stock
	// count that found less than the books said is the ordinary reason this
	// endpoint exists.
	Qty  string
	Note string
	By   actor
}

// Eight adjustments spread across the window, on top of the ten opening ones.
// With the receipts above that is around two dozen ledger rows per tenant, which
// is §15's "15–25 further entries" once the openings are excluded.
//
// None of these touches products 2 or 5, and the one that touches product 0
// pushes it further below rather than rescuing it — the three deliberately-low
// products stay low. `verifyLowStock` is what actually checks that.
var adjustmentPlans = []adjustmentPlan{
	{Seq: 1, DaysAgo: 51, Product: 1, Warehouse: 0, Qty: "-20", By: actorApprover,
		Note: "Stock count correction — twenty fewer than the books said."},
	{Seq: 2, DaysAgo: 44, Product: 3, Warehouse: 1, Qty: "-5", By: actorApprover,
		Note: "Damaged during handling."},
	{Seq: 3, DaysAgo: 37, Product: 6, Warehouse: 0, Qty: "30", By: actorApprover,
		Note: "Found in the back store during the count."},
	{Seq: 4, DaysAgo: 29, Product: 4, Warehouse: 0, Qty: "-2", By: actorApprover,
		Note: "Written off — damaged beyond use."},
	{Seq: 5, DaysAgo: 21, Product: 7, Warehouse: 1, Qty: "-5", By: actorApprover,
		Note: "Sent to a customer as a sample."},
	{Seq: 6, DaysAgo: 14, Product: 1, Warehouse: 1, Qty: "40", By: actorApprover,
		Note: "Returned from the branch."},
	// Against a DISCONTINUED product, deliberately. Writing off the last of
	// something being wound down is the ordinary reason to reach for this
	// endpoint, and Phase 4 decided it is allowed — `is_active` and `deleted_at`
	// are two different questions (§6.9.1).
	{Seq: 7, DaysAgo: 8, Product: 8, Warehouse: 0, Qty: "-3", By: actorApprover,
		Note: "Written off — discontinued line, no longer sellable."},
	{Seq: 8, DaysAgo: 2, Product: 0, Warehouse: 1, Qty: "-10", By: actorApprover,
		Note: "Stock count correction."},
}
