// The purchase requisition lifecycle (§8.2, §9.4, §10.3), and the purchase order
// generation that approval performs (§8.3).
//
// FOUR THINGS TO KNOW BEFORE EDITING THIS FILE.
//
//  1. Every state transition locks the requisition with `SELECT … FOR UPDATE`
//     BEFORE reading its status. Read-then-check without the lock is a race, and
//     the race is not theoretical: two managers opening the same pending
//     requisition and both tapping Approve would each see `submitted`, and one
//     requisition would generate two purchase orders (§8.6.2, H4).
//
//  2. Self-approval is a RECORD rule, not a role rule. `decided_by ==
//     requested_by` is refused for everybody including a tenant admin, and it
//     cannot live in the middleware because the middleware has no row to look at
//     (C2).
//
//  3. Approval generates the purchase order in the SAME transaction as the
//     status change and the number allocation. A requisition that is approved
//     without a PO, or a PO whose requisition is still `submitted`, is a state
//     no reader could explain.
//
//  4. Money and quantities are summed by PostgreSQL, never in Go. `total_amount`
//     is `SUM(qty * est_unit_cost)` evaluated where both operands are still
//     NUMERIC (I8) — httpx.Numeric has no arithmetic on purpose.
package api

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/docnum"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// The requisition statuses, from the §3 naming contract and the pr_status_valid
// CHECK. Ordered as the lifecycle runs, which is the order the filter chips
// render in.
var requisitionStatuses = []string{"draft", "submitted", "approved", "rejected", "cancelled"}

var requisitionSortable = map[string]string{
	"prNumber":    "r.pr_number",
	"status":      "r.status",
	"createdAt":   "r.created_at",
	"submittedAt": "r.submitted_at",
	"decidedAt":   "r.decided_at",
	"supplier":    "s.name",
	"warehouse":   "w.code",
	"total":       "COALESCE(l.estimated_total, 0)",
}

type requisitionRow struct {
	ID       uuid.UUID `json:"id"`
	PRNumber string    `json:"prNumber"`
	Status   string    `json:"status"`

	WarehouseID   uuid.UUID `json:"warehouseId"`
	WarehouseCode string    `json:"warehouseCode"`
	WarehouseName string    `json:"warehouseName"`

	// Nullable: a requisition may be raised before anyone has decided who to buy
	// from. Approval is where a supplier becomes mandatory (§8.3).
	SupplierID   *uuid.UUID `json:"supplierId"`
	SupplierCode *string    `json:"supplierCode"`
	SupplierName *string    `json:"supplierName"`

	Notes *string `json:"notes"`

	// The timeline (§10.3). Every actor is an id plus a name, because the id is
	// what the screen links to and the name is what it shows — and a deactivated
	// user still names their own history (G10).
	RequestedByID   uuid.UUID  `json:"requestedById"`
	RequestedByName string     `json:"requestedByName"`
	SubmittedAt     *time.Time `json:"submittedAt"`
	DecidedByID     *uuid.UUID `json:"decidedById"`
	DecidedByName   *string    `json:"decidedByName"`
	DecidedAt       *time.Time `json:"decidedAt"`
	RejectReason    *string    `json:"rejectReason"`
	CancelledByID   *uuid.UUID `json:"cancelledById"`
	CancelledByName *string    `json:"cancelledByName"`
	CancelledAt     *time.Time `json:"cancelledAt"`
	CancelReason    *string    `json:"cancelReason"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	LineCount int `json:"lineCount"`
	// EstimatedTotal is SUM(qty × est_unit_cost), computed by PostgreSQL. It is
	// an estimate until approval copies it onto the PO as `total_amount`.
	EstimatedTotal httpx.Numeric `json:"estimatedTotal"`

	// The order approval generated, so the detail screen can link to it rather
	// than making the reader search the PO list for a matching number.
	PurchaseOrderID     *uuid.UUID `json:"purchaseOrderId"`
	PurchaseOrderNumber *string    `json:"purchaseOrderNumber"`
}

type requisitionLine struct {
	ID          uuid.UUID `json:"id"`
	LineNo      int       `json:"lineNo"`
	ProductID   uuid.UUID `json:"productId"`
	SKU         string    `json:"sku"`
	ProductName string    `json:"productName"`
	UOM         string    `json:"uom"`
	// ProductDeleted marks a line whose product has since been deleted. The line
	// stays and still resolves the name — a requisition is a historical record
	// (§6.9.1, G6).
	ProductDeleted bool          `json:"productDeleted"`
	Qty            httpx.Numeric `json:"qty"`
	EstUnitCost    httpx.Numeric `json:"estUnitCost"`
	LineTotal      httpx.Numeric `json:"lineTotal"`
}

type requisitionDetail struct {
	requisitionRow
	Lines []requisitionLine `json:"lines"`
}

// --------------------------------------------------------------------------
// Reads.
// --------------------------------------------------------------------------

// listRequisitions is GET /api/procurement/requisitions — the §10.3 list, with
// the status filter the chips drive.
func (s *server) listRequisitions(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, requisitionSortable, "-createdAt")
	if err != nil {
		return malformed(c, "%s", err)
	}

	status := trimmed(c.Query("status"))
	if status != "" && !contains(requisitionStatuses, status) {
		return malformed(c, "status must be one of %s.",
			strings.Join(requisitionStatuses, ", "))
	}
	supplierID, ok := optionalUUID(c, "supplierId")
	if !ok {
		return malformed(c, "supplierId is not a valid id.")
	}

	// `r.supplier_id` is NULLABLE here, unlike the purchase order's: a draft
	// requisition need not name a supplier yet, and only approval makes one
	// mandatory (§8.3). So filtering by supplier necessarily excludes the drafts
	// that have not chosen one — which is the honest answer to "show me what we
	// are buying from Acme", not a bug to paper over with a COALESCE.
	where := `
		WHERE (? = '' OR r.status = ?)
		  AND (?::uuid IS NULL OR r.supplier_id = ?)
		  AND (r.pr_number ILIKE ? OR COALESCE(s.name, '') ILIKE ?
		       OR COALESCE(r.notes, '') ILIKE ?)`
	args := []any{
		status, status,
		supplierID, supplierID,
		params.Like(), params.Like(), params.Like(),
	}

	var total int64
	if err := tx.Raw(`
		SELECT count(*)
		FROM purchase_requisitions r
		LEFT JOIN suppliers s ON s.id = r.supplier_id`+where, args...).
		Scan(&total).Error; err != nil {
		return err
	}

	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	rows, err := s.requisitionRows(tx, where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("r.id")), page...)
	if err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// requisitionRows runs the requisition projection with a caller-supplied tail.
//
// Warehouses, suppliers, and products are joined WITHOUT a deleted filter. A
// requisition is a historical document: if the supplier was tidied away last
// month, the requisition still says who it was for (§6.9.1, Trap 3). Adding
// `AND deleted_at IS NULL` to any of these joins deletes history from the screen.
//
// `users` carries no RLS (Trap 2), but these joins need no tenant filter of
// their own: RLS scopes the requisition rows, and the only user rows reachable
// are the ones this tenant's own requisitions point at.
func (s *server) requisitionRows(tx *gorm.DB, tail string, args ...any) ([]requisitionRow, error) {
	var rows []requisitionRow
	err := tx.Raw(`
		SELECT r.id, r.pr_number, r.status,
		       r.warehouse_id, w.code AS warehouse_code, w.name AS warehouse_name,
		       r.supplier_id, s.code AS supplier_code, s.name AS supplier_name,
		       r.notes,
		       r.requested_by AS requested_by_id, ru.full_name AS requested_by_name,
		       r.submitted_at,
		       r.decided_by AS decided_by_id, du.full_name AS decided_by_name,
		       r.decided_at, r.reject_reason,
		       r.cancelled_by AS cancelled_by_id, cu.full_name AS cancelled_by_name,
		       r.cancelled_at, r.cancel_reason,
		       r.created_at, r.updated_at,
		       COALESCE(l.line_count, 0)          AS line_count,
		       COALESCE(l.estimated_total, 0)::text AS estimated_total,
		       po.id AS purchase_order_id, po.po_number AS purchase_order_number
		FROM purchase_requisitions r
		JOIN warehouses w ON w.id = r.warehouse_id
		LEFT JOIN suppliers s ON s.id = r.supplier_id
		JOIN users ru ON ru.id = r.requested_by
		LEFT JOIN users du ON du.id = r.decided_by
		LEFT JOIN users cu ON cu.id = r.cancelled_by
		LEFT JOIN (
		  SELECT requisition_id,
		         count(*)                                AS line_count,
		         SUM(qty * est_unit_cost)::numeric(18,2)  AS estimated_total
		  FROM purchase_requisition_lines
		  GROUP BY requisition_id
		) l ON l.requisition_id = r.id
		LEFT JOIN purchase_orders po ON po.requisition_id = r.id
		`+tail, args...).Scan(&rows).Error
	return rows, err
}

// getRequisition is GET /api/procurement/requisitions/:id.
func (s *server) getRequisition(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That requisition")
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That requisition")
	}
	return c.JSON(detail)
}

// requisitionDetail reads one requisition with its lines, or nil if there is no
// such requisition in this tenant. RLS is the tenant filter.
func (s *server) requisitionDetail(tx *gorm.DB, id uuid.UUID) (*requisitionDetail, error) {
	rows, err := s.requisitionRows(tx, `WHERE r.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	var lines []requisitionLine
	if err := tx.Raw(`
		SELECT rl.id, rl.line_no, rl.product_id, p.sku, p.name AS product_name, p.uom,
		       (p.deleted_at IS NOT NULL)             AS product_deleted,
		       rl.qty::text                           AS qty,
		       rl.est_unit_cost::text                 AS est_unit_cost,
		       (rl.qty * rl.est_unit_cost)::numeric(18,2)::text AS line_total
		FROM purchase_requisition_lines rl
		JOIN products p ON p.id = rl.product_id
		WHERE rl.requisition_id = ?
		ORDER BY rl.line_no`, id).Scan(&lines).Error; err != nil {
		return nil, err
	}
	if lines == nil {
		lines = []requisitionLine{}
	}

	return &requisitionDetail{requisitionRow: rows[0], Lines: lines}, nil
}

// --------------------------------------------------------------------------
// Line input, shared by create and edit.
// --------------------------------------------------------------------------

type requisitionLineInput struct {
	ProductID   string         `json:"productId"`
	Qty         httpx.Numeric  `json:"qty"`
	EstUnitCost *httpx.Numeric `json:"estUnitCost"`
}

// resolvedLine is one validated line, ready to insert.
type resolvedLine struct {
	ProductID   uuid.UUID
	Qty         httpx.Numeric
	EstUnitCost httpx.Numeric
	LineNo      int
}

// resolveLines validates the submitted lines against the product catalogue.
//
// Pure in the sense that matters: it returns a real error and never writes a
// response. A helper that signalled failure by returning what httpx.Fail
// returned would be signalling with nil, and the caller's `if err != nil` would
// never fire — see parseMatrix, and the 403-with-a-success-body B8 caught.
//
// `errLineProduct` is the one failure the caller renders as a 404 rather than a
// 400, because "no such product" and "another tenant's product" have to be
// indistinguishable and RLS already makes them so.
var errLineProduct = errors.New("line names a product that does not exist")

func resolveLines(tx *gorm.DB, inputs []requisitionLineInput) ([]resolvedLine, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	resolved := make([]resolvedLine, 0, len(inputs))
	seen := map[uuid.UUID]int{}

	for i, in := range inputs {
		lineNo := i + 1

		productID, err := uuid.Parse(trimmed(in.ProductID))
		if err != nil {
			return nil, fmt.Errorf("line %d: productId is required and must be an id", lineNo)
		}
		// One line per product, matching pol_one_line_per_product on the order
		// this requisition will become. Refusing it here means the user finds out
		// while editing rather than at approval, when the constraint fires and
		// the requisition is already locked.
		if first, dup := seen[productID]; dup {
			return nil, fmt.Errorf("line %d repeats the product on line %d; "+
				"put the whole quantity on one line", lineNo, first)
		}
		seen[productID] = lineNo

		qty, err := httpx.ParseNumeric(in.Qty.String())
		if err != nil {
			return nil, fmt.Errorf("line %d: qty: %w", lineNo, err)
		}
		if qty.IsZero() || qty.IsNegative() {
			return nil, fmt.Errorf("line %d: qty must be greater than zero", lineNo)
		}

		// The picker case, so the deleted filter is ON: a new requisition may not
		// name a product in the recycle bin. Discontinued (`is_active = false`) is
		// allowed — the two columns are two questions (§6.9.1), and a product
		// being wound down is still a product you may order the last of.
		var product []struct {
			StandardCost httpx.Numeric
		}
		if err := tx.Raw(`
			SELECT standard_cost::text AS standard_cost
			FROM products WHERE id = ? AND deleted_at IS NULL`, productID).
			Scan(&product).Error; err != nil {
			return nil, err
		}
		if len(product) == 0 {
			return nil, fmt.Errorf("line %d: %w", lineNo, errLineProduct)
		}

		// The estimate defaults to the product's standard cost rather than to
		// zero. The column defaults to 0 and nothing would complain — but that
		// zero is copied to the PO line as `unit_cost`, and from there to the
		// goods receipt's journal entry, which would then post Dr 0 / Cr 0. A
		// balanced entry for nothing is worse than an error: it looks fine.
		estUnitCost := product[0].StandardCost
		if in.EstUnitCost != nil {
			estUnitCost, err = nonNegative(in.EstUnitCost, fmt.Sprintf("line %d: estUnitCost", lineNo))
			if err != nil {
				return nil, err
			}
		}

		resolved = append(resolved, resolvedLine{
			ProductID:   productID,
			Qty:         qty,
			EstUnitCost: estUnitCost,
			LineNo:      lineNo,
		})
	}
	return resolved, nil
}

// insertLines writes the resolved lines for a requisition.
func insertLines(tx *gorm.DB, tenantID, requisitionID uuid.UUID, lines []resolvedLine) error {
	for _, line := range lines {
		if err := tx.Exec(`
			INSERT INTO purchase_requisition_lines
			  (id, tenant_id, requisition_id, product_id, qty, est_unit_cost, line_no)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.New(), tenantID, requisitionID, line.ProductID,
			line.Qty, line.EstUnitCost, line.LineNo).Error; err != nil {
			return err
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Create and edit.
// --------------------------------------------------------------------------

type requisitionWriteRequest struct {
	WarehouseID *string `json:"warehouseId"`
	// SupplierID is a pointer to a pointer of meaning: absent leaves it alone,
	// and an explicit null clears it. A requisition may legitimately not name a
	// supplier yet.
	SupplierID *string                 `json:"supplierId"`
	Notes      *string                 `json:"notes"`
	Lines      *[]requisitionLineInput `json:"lines"`
}

// createRequisition is POST /api/procurement/requisitions — level `user`.
//
// It always creates a `draft`. Submitting is a separate request, so a client
// that crashes between the two leaves a draft the user can find and finish
// rather than a document in a state nobody chose.
func (s *server) createRequisition(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}

	var req requisitionWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	warehouseID, err := uuid.Parse(trimmed(derefString(req.WarehouseID)))
	if err != nil {
		return malformed(c, "warehouseId is required and must be an id.")
	}
	live, err := liveWarehouse(tx, warehouseID)
	if err != nil {
		return err
	}
	if !live {
		return notFound(c, "That warehouse")
	}

	supplierID, ok, err := resolveOptionalSupplier(tx, req.SupplierID)
	if err != nil {
		return err
	}
	if !ok {
		return notFound(c, "That supplier")
	}

	var inputs []requisitionLineInput
	if req.Lines != nil {
		inputs = *req.Lines
	}
	lines, err := resolveLines(tx, inputs)
	if err != nil {
		if errors.Is(err, errLineProduct) {
			return notFound(c, "A product on one of those lines")
		}
		return malformed(c, "%s", err)
	}

	// The number is allocated in this transaction, so a rollback below does not
	// consume it (§8.1, E4).
	number, err := docnum.Allocate(tx, caller.TenantID, docnum.PR)
	if err != nil {
		return err
	}

	id := uuid.New()
	if err := tx.Exec(`
		INSERT INTO purchase_requisitions
		  (id, tenant_id, pr_number, warehouse_id, supplier_id, status, notes, requested_by)
		VALUES (?, ?, ?, ?, ?, 'draft', ?, ?)`,
		id, caller.TenantID, number, warehouseID, supplierID,
		nullIfEmpty(trimmed(derefString(req.Notes))), caller.UserID).Error; err != nil {
		return err
	}
	if err := insertLines(tx, caller.TenantID, id, lines); err != nil {
		return err
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(detail)
}

// patchRequisition is PATCH /api/procurement/requisitions/:id — level `user`,
// creator only, draft only (§9.4).
//
// `lines`, when present, REPLACES the whole set. That is the one DELETE in this
// module and it is deliberate: a draft is a form the user has not committed yet,
// its lines are referenced by nothing, and the alternative — a line that can
// only be added, never removed — forces the user to cancel and re-key the
// requisition, consuming a document number to fix a typo. Once submitted, the
// lines are frozen, which is what the status check below enforces.
func (s *server) patchRequisition(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That requisition")
	}

	var req requisitionWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	// Locked before its status is read, like every other transition here. An edit
	// racing an approval must not slip its change in underneath the approver.
	current, err := lockRequisition(tx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return notFound(c, "That requisition")
	}
	if current.RequestedBy != caller.UserID {
		return forbidden(c, "Only the person who raised this requisition can change it.")
	}
	if current.Status != "draft" {
		return stateConflict(c, "This requisition", current.Status, "draft")
	}

	sets := map[string]any{}
	if req.WarehouseID != nil {
		warehouseID, err := uuid.Parse(trimmed(*req.WarehouseID))
		if err != nil {
			return malformed(c, "warehouseId must be an id.")
		}
		live, err := liveWarehouse(tx, warehouseID)
		if err != nil {
			return err
		}
		if !live {
			return notFound(c, "That warehouse")
		}
		sets["warehouse_id"] = warehouseID
	}
	if req.SupplierID != nil {
		supplierID, ok, err := resolveOptionalSupplier(tx, req.SupplierID)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(c, "That supplier")
		}
		sets["supplier_id"] = supplierID
	}
	if req.Notes != nil {
		sets["notes"] = nullIfEmpty(trimmed(*req.Notes))
	}

	var lines []resolvedLine
	if req.Lines != nil {
		lines, err = resolveLines(tx, *req.Lines)
		if err != nil {
			if errors.Is(err, errLineProduct) {
				return notFound(c, "A product on one of those lines")
			}
			return malformed(c, "%s", err)
		}
	}
	if len(sets) == 0 && req.Lines == nil {
		return malformed(c, "Nothing to change.")
	}

	if len(sets) > 0 {
		if err := tx.Table("purchase_requisitions").
			Where("id = ?", id).Updates(sets).Error; err != nil {
			return err
		}
	}
	if req.Lines != nil {
		if err := tx.Exec(`
			DELETE FROM purchase_requisition_lines WHERE requisition_id = ?`, id).Error; err != nil {
			return err
		}
		if err := insertLines(tx, caller.TenantID, id, lines); err != nil {
			return err
		}
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

// --------------------------------------------------------------------------
// Transitions.
// --------------------------------------------------------------------------

// lockedRequisition is the minimum a transition needs to decide: who owns it,
// where it is, and how many lines it has.
type lockedRequisition struct {
	ID          uuid.UUID
	Status      string
	RequestedBy uuid.UUID
	SupplierID  *uuid.UUID
	LineCount   int
}

// lockRequisition takes the row lock of §8.6.2, then reads the row. Returns nil
// when there is no such requisition in this tenant.
//
// The lock is the whole point and it has to come first: `SELECT … FOR UPDATE`
// makes check-and-act atomic, so the second of two concurrent approvals blocks
// here, then re-reads `status = 'approved'` and refuses cleanly (H4). A plain
// SELECT would have both pass the status check and generate a purchase order
// each.
//
// The line count is a separate statement because FOR UPDATE cannot be used with
// an aggregate. Counting after the lock is still safe: a line can only be added
// to a `draft`, and every caller of this either requires `draft` and is the
// creator, or requires `submitted`, which nothing can add lines to.
func lockRequisition(tx *gorm.DB, id uuid.UUID) (*lockedRequisition, error) {
	var rows []lockedRequisition
	if err := tx.Raw(`
		SELECT id, status, requested_by, supplier_id
		FROM purchase_requisitions
		WHERE id = ?
		FOR UPDATE`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	var count int
	if err := tx.Raw(`
		SELECT count(*) FROM purchase_requisition_lines WHERE requisition_id = ?`, id).
		Scan(&count).Error; err != nil {
		return nil, err
	}
	rows[0].LineCount = count
	return &rows[0], nil
}

// submitRequisition is POST /api/procurement/requisitions/:id/submit — level
// `user`, creator only.
func (s *server) submitRequisition(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That requisition")
	}

	current, err := lockRequisition(tx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return notFound(c, "That requisition")
	}
	if current.RequestedBy != caller.UserID {
		return forbidden(c, "Only the person who raised this requisition can submit it.")
	}
	if current.Status != "draft" {
		return stateConflict(c, "This requisition", current.Status, "draft")
	}
	// C1. A requisition with nothing on it asks for nothing, and approving it
	// would generate a purchase order with no lines and a total of zero.
	if current.LineCount == 0 {
		return unprocessable(c, "empty_requisition",
			"Add at least one line before submitting this requisition.")
	}

	if err := tx.Exec(`
		UPDATE purchase_requisitions
		SET status = 'submitted', submitted_at = now()
		WHERE id = ?`, id).Error; err != nil {
		return err
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

type approveRequest struct {
	// SupplierID is required only when the requisition does not already name one
	// (§8.3). Sending it for a requisition that does overrides it, which is the
	// approver's call to make.
	SupplierID *string `json:"supplierId"`
}

// approveRequisition is POST /api/procurement/requisitions/:id/approve — level
// `approver`.
//
// One transaction: the status change, the PO number allocation, the purchase
// order, and its lines. The whole demonstration of §8.3 is that there is no
// window in which a requisition is approved and its order does not exist.
func (s *server) approveRequisition(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That requisition")
	}

	var req approveRequest
	// An empty body is normal here — most requisitions already name a supplier —
	// so a parse failure is only worth reporting when something was actually
	// sent.
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return malformed(c, "The request body is not valid JSON.")
		}
	}

	current, err := lockRequisition(tx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return notFound(c, "That requisition")
	}
	// Status before segregation of duties: a requisition that has already been
	// decided is not waiting for anybody, and telling its author "you may not
	// approve your own" when the answer is "it was approved yesterday" sends
	// them to argue with the wrong person (C4).
	if current.Status != "submitted" {
		return stateConflict(c, "This requisition", current.Status, "submitted")
	}
	// C2 — segregation of duties. A record rule: it applies to tenant admins
	// too, and it is why this cannot live in RequireModule.
	if current.RequestedBy == caller.UserID {
		return httpx.Fail(c, fiber.StatusForbidden, "self_approval_forbidden",
			"You raised this requisition, so somebody else has to approve it.")
	}
	if current.LineCount == 0 {
		// Unreachable through submit, which refuses an empty requisition. Kept
		// because approval is what generates the order, and an order with no
		// lines is worse than a refusal.
		return unprocessable(c, "empty_requisition",
			"This requisition has no lines, so there is nothing to order.")
	}

	// The supplier: the requisition's own, or one the approver names now.
	supplierID := current.SupplierID
	if req.SupplierID != nil {
		chosen, ok, err := resolveOptionalSupplier(tx, req.SupplierID)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(c, "That supplier")
		}
		supplierID = chosen
	}
	if supplierID == nil {
		return unprocessable(c, "supplier_required",
			"Choose a supplier before approving: a purchase order has to be addressed to somebody.")
	}

	if err := tx.Exec(`
		UPDATE purchase_requisitions
		SET status = 'approved', decided_by = ?, decided_at = now(), supplier_id = ?
		WHERE id = ?`, caller.UserID, supplierID, id).Error; err != nil {
		return err
	}

	if err := s.generatePurchaseOrder(tx, caller, id, *supplierID); err != nil {
		if errors.Is(err, errDuplicateProductLine) {
			return stateConflict(c,
				"This requisition lists one product twice, so it cannot become an order",
				current.Status, "submitted")
		}
		return err
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

// errDuplicateProductLine is pol_one_line_per_product firing during PO
// generation. Unreachable through the API — resolveLines refuses a repeated
// product when the requisition is written — so it is mapped rather than
// swallowed, and reaching it means a write path bypassed that validation.
var errDuplicateProductLine = errors.New("purchase order lines repeat a product")

// generatePurchaseOrder is §8.3, steps 2-5, inside the approving transaction.
//
// Every number in it is computed by PostgreSQL. `total_amount` is
// `SUM(qty * est_unit_cost)` over the requisition's lines and `expected_at` is
// the supplier's lead time added to today's date *in the tenant's timezone* —
// business dates belong to the tenant, not the server (§2.5.3).
func (s *server) generatePurchaseOrder(tx *gorm.DB, caller *identity.Identity, requisitionID, supplierID uuid.UUID) error {
	var plan []struct {
		WarehouseID uuid.UUID
		ExpectedAt  string
		TotalAmount httpx.Numeric
	}
	if err := tx.Raw(`
		SELECT r.warehouse_id,
		       ((now() AT TIME ZONE t.timezone)::date + s.lead_time_days)::text AS expected_at,
		       COALESCE((SELECT SUM(l.qty * l.est_unit_cost)
		                 FROM purchase_requisition_lines l
		                 WHERE l.requisition_id = r.id), 0)::numeric(18,2)::text AS total_amount
		FROM purchase_requisitions r
		JOIN suppliers s ON s.id = ?
		JOIN tenants   t ON t.id = ?
		WHERE r.id = ?`, supplierID, caller.TenantID, requisitionID).Scan(&plan).Error; err != nil {
		return err
	}
	if len(plan) == 0 {
		return fmt.Errorf("api: requisition %s vanished between locking and ordering", requisitionID)
	}

	number, err := docnum.Allocate(tx, caller.TenantID, docnum.PO)
	if err != nil {
		return err
	}

	poID := uuid.New()
	if err := tx.Exec(`
		INSERT INTO purchase_orders
		  (id, tenant_id, po_number, requisition_id, supplier_id, warehouse_id,
		   status, total_amount, expected_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?::date, ?)`,
		poID, caller.TenantID, number, requisitionID, supplierID, plan[0].WarehouseID,
		plan[0].TotalAmount, plan[0].ExpectedAt, caller.UserID).Error; err != nil {
		return err
	}

	// `line_no` is preserved rather than renumbered, so a conversation about
	// "line 3" means the same line on the requisition and on the order. Nothing
	// is initialised for received quantity — it is derived from the receipt lines
	// through po_line_status (I6).
	if err := tx.Exec(`
		INSERT INTO purchase_order_lines
		  (id, tenant_id, po_id, product_id, qty_ordered, unit_cost, line_no)
		SELECT gen_random_uuid(), l.tenant_id, ?, l.product_id,
		       l.qty, l.est_unit_cost, l.line_no
		FROM purchase_requisition_lines l
		WHERE l.requisition_id = ?
		ORDER BY l.line_no`, poID, requisitionID).Error; err != nil {
		if db.IsUniqueViolation(err) && db.ConstraintName(err) == "pol_one_line_per_product" {
			return errDuplicateProductLine
		}
		return err
	}
	return nil
}

type rejectRequest struct {
	Reason *string `json:"reason"`
}

// rejectRequisition is POST /api/procurement/requisitions/:id/reject — level
// `approver`.
//
// A reason is mandatory (C3). The `pr_reject_needs_reason` CHECK says the same
// thing at the database level (G13), which is the difference between a promise
// and a guarantee — but the handler is what turns it into a sentence.
func (s *server) rejectRequisition(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That requisition")
	}

	var req rejectRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return malformed(c, "The request body is not valid JSON.")
		}
	}
	reason := trimmed(derefString(req.Reason))

	current, err := lockRequisition(tx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return notFound(c, "That requisition")
	}
	if current.Status != "submitted" {
		return stateConflict(c, "This requisition", current.Status, "submitted")
	}
	if reason == "" {
		return unprocessable(c, "reason_required",
			"Say why this requisition is being rejected — the person who raised it "+
				"has to know what to change.")
	}

	if err := tx.Exec(`
		UPDATE purchase_requisitions
		SET status = 'rejected', decided_by = ?, decided_at = now(), reject_reason = ?
		WHERE id = ?`, caller.UserID, reason, id).Error; err != nil {
		return err
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

type cancelRequest struct {
	Reason *string `json:"reason"`
}

// cancelRequisition is POST /api/procurement/requisitions/:id/cancel.
//
// Who may cancel depends on where the requisition is (§6.9.2):
//
//	draft      — its creator. Nobody else has any business in someone's draft.
//	submitted  — its creator, who has changed their mind, or an `approver`.
//	approved   — nobody: cancel the resulting purchase order instead (G8).
//
// So the route is gated at `user`, the lowest level a creator can hold, and the
// rest is a record rule here. Cancelling records who, when, and why, the same
// shape as approve and reject.
func (s *server) cancelRequisition(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That requisition")
	}

	var req cancelRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return malformed(c, "The request body is not valid JSON.")
		}
	}
	reason := trimmed(derefString(req.Reason))

	current, err := lockRequisition(tx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return notFound(c, "That requisition")
	}
	if current.Status != "draft" && current.Status != "submitted" {
		// G8. An approved requisition has produced a purchase order, and
		// cancelling the paperwork behind an order the supplier has already been
		// sent would leave the order with nothing explaining it.
		return stateConflict(c, "This requisition", current.Status, "draft", "submitted")
	}

	isCreator := current.RequestedBy == caller.UserID
	isApprover := caller.LevelFor(ModuleProcurement) >= identity.RoleApprover
	if current.Status == "draft" && !isCreator {
		return forbidden(c, "Only the person who raised this draft can cancel it.")
	}
	if !isCreator && !isApprover {
		return forbidden(c, "Only the person who raised this requisition, "+
			"or an approver, can cancel it.")
	}
	if reason == "" {
		return unprocessable(c, "reason_required",
			"Say why this requisition is being cancelled — it stays on the record.")
	}

	if err := tx.Exec(`
		UPDATE purchase_requisitions
		SET status = 'cancelled', cancelled_by = ?, cancelled_at = now(), cancel_reason = ?
		WHERE id = ?`, caller.UserID, reason, id).Error; err != nil {
		return err
	}

	detail, err := s.requisitionDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

// --------------------------------------------------------------------------
// Shared lookups. Pure: they return errors, never responses.
// --------------------------------------------------------------------------

// liveWarehouse reports whether this warehouse exists and is not deleted.
//
// Absent, deleted, and another tenant's all come back false, and the caller
// renders all three as one 404: distinguishing them is a cross-tenant existence
// oracle. The error is kept separate from the answer so a database failure
// cannot arrive at the client dressed as "no such warehouse".
func liveWarehouse(tx *gorm.DB, id uuid.UUID) (bool, error) {
	var found []int
	if err := tx.Raw(`
		SELECT 1 FROM warehouses WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&found).Error; err != nil {
		return false, err
	}
	return len(found) > 0, nil
}

// resolveOptionalSupplier reads a supplier field that may be absent, explicitly
// null, or a UUID.
//
// It reports `ok = false` for a supplier that does not exist or is deleted —
// this is the picker case, so the deleted filter is on — and (nil, true, nil)
// for "no supplier", which is a legitimate state for a requisition to be in.
func resolveOptionalSupplier(tx *gorm.DB, raw *string) (*uuid.UUID, bool, error) {
	if raw == nil || trimmed(*raw) == "" {
		return nil, true, nil
	}
	id, err := uuid.Parse(trimmed(*raw))
	if err != nil {
		return nil, false, nil
	}
	var found []int
	if err := tx.Raw(`
		SELECT 1 FROM suppliers WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&found).Error; err != nil {
		return nil, false, err
	}
	if len(found) == 0 {
		return nil, false, nil
	}
	return &id, true, nil
}
