// Purchase orders — the read side and cancellation (§9.4, §10.3).
//
// RECEIVED QUANTITY IS DERIVED, exactly like stock on hand. There is no
// `qty_received` column on `purchase_order_lines` and there is never going to be
// one (I6): every line's received and outstanding quantities are read through the
// `po_line_status` view, which sums that line's goods receipt lines. A stored
// counter can drift from the receipts it claims to count, and when it does,
// nothing in the system can say which number is the truth.
//
// The receipt endpoint that writes those lines is Session B. This file is what
// reads them back.
package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/httpx"
)

// The purchase order statuses, from the §3 naming contract and po_status_valid.
var purchaseOrderStatuses = []string{"open", "partially_received", "received", "cancelled"}

var purchaseOrderSortable = map[string]string{
	"poNumber":   "po.po_number",
	"status":     "po.status",
	"orderedAt":  "po.ordered_at",
	"expectedAt": "po.expected_at",
	"supplier":   "s.name",
	"warehouse":  "w.code",
	"total":      "po.total_amount",
}

type purchaseOrderRow struct {
	ID       uuid.UUID `json:"id"`
	PONumber string    `json:"poNumber"`
	Status   string    `json:"status"`

	SupplierID   uuid.UUID `json:"supplierId"`
	SupplierCode string    `json:"supplierCode"`
	SupplierName string    `json:"supplierName"`

	WarehouseID   uuid.UUID `json:"warehouseId"`
	WarehouseCode string    `json:"warehouseCode"`
	WarehouseName string    `json:"warehouseName"`

	// The requisition this order came from. Nullable in the schema because a
	// direct order is imaginable; every order the MVP creates has one.
	RequisitionID     *uuid.UUID `json:"requisitionId"`
	RequisitionNumber *string    `json:"requisitionNumber"`

	TotalAmount httpx.Numeric `json:"totalAmount"`
	OrderedAt   time.Time     `json:"orderedAt"`
	// ExpectedAt is a DATE, carried as `YYYY-MM-DD` text rather than as an
	// instant. A business date has no time and no zone (§2.5.3); sending it as a
	// timestamp would invite the browser to render it a day early.
	ExpectedAt *string `json:"expectedAt"`

	CreatedByID   uuid.UUID `json:"createdById"`
	CreatedByName string    `json:"createdByName"`

	CancelledByID   *uuid.UUID `json:"cancelledById"`
	CancelledByName *string    `json:"cancelledByName"`
	CancelledAt     *time.Time `json:"cancelledAt"`
	CancelReason    *string    `json:"cancelReason"`

	UpdatedAt time.Time `json:"updatedAt"`

	LineCount int `json:"lineCount"`
	// The three quantities the list needs to show progress, all summed from
	// po_line_status by PostgreSQL.
	QtyOrdered     httpx.Numeric `json:"qtyOrdered"`
	QtyReceived    httpx.Numeric `json:"qtyReceived"`
	QtyOutstanding httpx.Numeric `json:"qtyOutstanding"`
}

type purchaseOrderLine struct {
	ID          uuid.UUID `json:"id"`
	LineNo      int       `json:"lineNo"`
	ProductID   uuid.UUID `json:"productId"`
	SKU         string    `json:"sku"`
	ProductName string    `json:"productName"`
	UOM         string    `json:"uom"`
	// ProductDeleted marks a line whose product has since been deleted. The line
	// still resolves the name — that is G6, and it is the reason this join carries
	// no deleted filter.
	ProductDeleted bool `json:"productDeleted"`

	QtyOrdered httpx.Numeric `json:"qtyOrdered"`
	UnitCost   httpx.Numeric `json:"unitCost"`
	LineTotal  httpx.Numeric `json:"lineTotal"`
	// Derived from the receipt lines, never stored (I6, G11).
	QtyReceived    httpx.Numeric `json:"qtyReceived"`
	QtyOutstanding httpx.Numeric `json:"qtyOutstanding"`
}

type purchaseOrderDetail struct {
	purchaseOrderRow
	Lines []purchaseOrderLine `json:"lines"`
}

// --------------------------------------------------------------------------
// Reads.
// --------------------------------------------------------------------------

// listPurchaseOrders is GET /api/procurement/purchase-orders — with the status
// and supplier filters §10.3 asks for.
func (s *server) listPurchaseOrders(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, purchaseOrderSortable, "-orderedAt")
	if err != nil {
		return malformed(c, "%s", err)
	}

	status := trimmed(c.Query("status"))
	if status != "" && !contains(purchaseOrderStatuses, status) {
		return malformed(c, "status must be one of %s.",
			strings.Join(purchaseOrderStatuses, ", "))
	}
	supplierID, ok := optionalUUID(c, "supplierId")
	if !ok {
		return malformed(c, "supplierId is not a valid id.")
	}

	where := `
		WHERE (? = '' OR po.status = ?)
		  AND (?::uuid IS NULL OR po.supplier_id = ?)
		  AND (po.po_number ILIKE ? OR s.name ILIKE ?)`
	args := []any{
		status, status,
		supplierID, supplierID,
		params.Like(), params.Like(),
	}

	var total int64
	if err := tx.Raw(`
		SELECT count(*)
		FROM purchase_orders po
		JOIN suppliers s ON s.id = po.supplier_id`+where, args...).
		Scan(&total).Error; err != nil {
		return err
	}

	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	rows, err := s.purchaseOrderRows(tx, where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("po.id")), page...)
	if err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// purchaseOrderRows runs the order projection with a caller-supplied tail.
//
// The progress columns come from po_line_status, aggregated per order. Reading
// them from the view rather than from a column is the whole of I6 in this module:
// the view is a GROUP BY over the receipt lines, so it cannot disagree with them.
func (s *server) purchaseOrderRows(tx *gorm.DB, tail string, args ...any) ([]purchaseOrderRow, error) {
	var rows []purchaseOrderRow
	err := tx.Raw(`
		SELECT po.id, po.po_number, po.status,
		       po.supplier_id, s.code AS supplier_code, s.name AS supplier_name,
		       po.warehouse_id, w.code AS warehouse_code, w.name AS warehouse_name,
		       po.requisition_id, r.pr_number AS requisition_number,
		       po.total_amount::text AS total_amount,
		       po.ordered_at, po.expected_at::text AS expected_at,
		       po.created_by AS created_by_id, cu.full_name AS created_by_name,
		       po.cancelled_by AS cancelled_by_id, xu.full_name AS cancelled_by_name,
		       po.cancelled_at, po.cancel_reason,
		       po.updated_at,
		       COALESCE(ls.line_count, 0)           AS line_count,
		       COALESCE(ls.qty_ordered, 0)::text     AS qty_ordered,
		       COALESCE(ls.qty_received, 0)::text    AS qty_received,
		       COALESCE(ls.qty_outstanding, 0)::text AS qty_outstanding
		FROM purchase_orders po
		JOIN suppliers  s ON s.id = po.supplier_id
		JOIN warehouses w ON w.id = po.warehouse_id
		JOIN users     cu ON cu.id = po.created_by
		LEFT JOIN users xu ON xu.id = po.cancelled_by
		LEFT JOIN purchase_requisitions r ON r.id = po.requisition_id
		LEFT JOIN (
		  SELECT po_id,
		         count(*)             AS line_count,
		         SUM(qty_ordered)     AS qty_ordered,
		         SUM(qty_received)    AS qty_received,
		         SUM(qty_outstanding) AS qty_outstanding
		  FROM po_line_status
		  GROUP BY po_id
		) ls ON ls.po_id = po.id
		`+tail, args...).Scan(&rows).Error
	return rows, err
}

// getPurchaseOrder is GET /api/procurement/purchase-orders/:id — the detail
// screen's "ordered vs received per line".
func (s *server) getPurchaseOrder(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That purchase order")
	}

	detail, err := s.purchaseOrderDetail(tx, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That purchase order")
	}
	return c.JSON(detail)
}

func (s *server) purchaseOrderDetail(tx *gorm.DB, id uuid.UUID) (*purchaseOrderDetail, error) {
	rows, err := s.purchaseOrderRows(tx, `WHERE po.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	lines, err := s.purchaseOrderLines(tx, id)
	if err != nil {
		return nil, err
	}
	return &purchaseOrderDetail{purchaseOrderRow: rows[0], Lines: lines}, nil
}

// purchaseOrderLines reads one order's lines with their derived receipt
// progress. The products join carries no deleted filter, deliberately (G6).
func (s *server) purchaseOrderLines(tx *gorm.DB, poID uuid.UUID) ([]purchaseOrderLine, error) {
	var lines []purchaseOrderLine
	if err := tx.Raw(`
		SELECT pol.id, pol.line_no, pol.product_id,
		       p.sku, p.name AS product_name, p.uom,
		       (p.deleted_at IS NOT NULL) AS product_deleted,
		       pol.qty_ordered::text      AS qty_ordered,
		       pol.unit_cost::text        AS unit_cost,
		       (pol.qty_ordered * pol.unit_cost)::numeric(18,2)::text AS line_total,
		       st.qty_received::text      AS qty_received,
		       st.qty_outstanding::text   AS qty_outstanding
		FROM purchase_order_lines pol
		JOIN products p ON p.id = pol.product_id
		JOIN po_line_status st ON st.po_line_id = pol.id
		WHERE pol.po_id = ?
		ORDER BY pol.line_no`, poID).Scan(&lines).Error; err != nil {
		return nil, err
	}
	if lines == nil {
		lines = []purchaseOrderLine{}
	}
	return lines, nil
}

// --------------------------------------------------------------------------
// Cancellation.
// --------------------------------------------------------------------------

// cancelPurchaseOrder is POST /api/procurement/purchase-orders/:id/cancel —
// level `approver`, `open` orders only (§6.9.2, G7).
//
// A `partially_received` or `received` order cannot be cancelled, and the reason
// is worth saying out loud: goods have physically arrived and the stock ledger
// has already recorded them. Cancellation is constrained by what has happened in
// the real world, not by what the UI would find convenient.
//
// The `po_terminal_immutable` trigger refuses an UPDATE to a `received` or
// `cancelled` order independently (§6.10.8). The lock and the check here are what
// turn that guarantee into a sentence a user can read.
func (s *server) cancelPurchaseOrder(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That purchase order")
	}

	var req cancelRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return malformed(c, "The request body is not valid JSON.")
		}
	}
	reason := trimmed(derefString(req.Reason))

	// Locked before its status is read, for the same reason every transition in
	// this module is: two approvers cancelling at once, or a cancel racing a
	// receipt, must resolve to one winner and one clean refusal.
	var rows []struct {
		Status string
	}
	if err := tx.Raw(`
		SELECT status FROM purchase_orders WHERE id = ? FOR UPDATE`, id).
		Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return notFound(c, "That purchase order")
	}
	if rows[0].Status != "open" {
		return stateConflict(c, "This purchase order", rows[0].Status, "open")
	}
	if reason == "" {
		return unprocessable(c, "reason_required",
			"Say why this order is being cancelled — the supplier has already been sent it.")
	}

	if err := tx.Exec(`
		UPDATE purchase_orders
		SET status = 'cancelled', cancelled_by = ?, cancelled_at = now(), cancel_reason = ?
		WHERE id = ?`, caller.UserID, reason, id).Error; err != nil {
		return err
	}

	detail, err := s.purchaseOrderDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}
