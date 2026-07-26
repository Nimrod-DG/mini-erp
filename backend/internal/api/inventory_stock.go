// Stock — the balances grid, the low-stock query, the ledger, and the one
// endpoint that writes to it (§9.5, §10.4).
//
// THE DESIGN DECISION THIS FILE EXISTS TO PROTECT.
//
// Stock on hand is SUM(qty_delta) over stock_ledger, read through the
// stock_balances view, on every single read (I6). There is no products.current_stock
// and there is never going to be one. A mutable counter can be wrong, and when
// it is wrong nothing in the system can say why; a ledger can always answer "why
// is stock 47?" by showing the 47. If a query here gets slow, the fix is an
// index — not a cached total.
//
// The ledger itself is append-only (§6.9.3, Tier 3). There is no UPDATE and no
// DELETE below, and there could not be one if someone wrote it: erp_app has the
// grant REVOKEd, so a bug on a future code path fails loudly at the database
// rather than quietly rewriting history (G9).
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

var stockSortable = map[string]string{
	"sku":           "p.sku",
	"productName":   "p.name",
	"warehouseCode": "w.code",
	"qtyOnHand":     "sb.qty_on_hand",
}

var lowStockSortable = map[string]string{
	"sku":          "p.sku",
	"name":         "p.name",
	"qtyOnHand":    "COALESCE(b.qty_on_hand, 0)",
	"reorderPoint": "p.reorder_point",
	"shortfall":    "p.reorder_point - COALESCE(b.qty_on_hand, 0)",
}

var ledgerSortable = map[string]string{
	"occurredAt": "l.occurred_at",
	"entryType":  "l.entry_type",
	"sku":        "p.sku",
	"qtyDelta":   "l.qty_delta",
}

type stockRow struct {
	ProductID   uuid.UUID `json:"productId"`
	SKU         string    `json:"sku"`
	ProductName string    `json:"productName"`
	UOM         string    `json:"uom"`
	// ProductDeleted marks a balance whose product is in the recycle bin. The
	// goods are still on the shelf, so the row is shown — see listStock.
	ProductDeleted bool          `json:"productDeleted"`
	WarehouseID    uuid.UUID     `json:"warehouseId"`
	WarehouseCode  string        `json:"warehouseCode"`
	WarehouseName  string        `json:"warehouseName"`
	QtyOnHand      httpx.Numeric `json:"qtyOnHand"`
}

type lowStockRow struct {
	ProductID    uuid.UUID     `json:"productId"`
	SKU          string        `json:"sku"`
	Name         string        `json:"name"`
	UOM          string        `json:"uom"`
	QtyOnHand    httpx.Numeric `json:"qtyOnHand"`
	ReorderPoint httpx.Numeric `json:"reorderPoint"`
	// Shortfall is how much would have to arrive to clear the reorder point.
	// Computed in SQL, where both operands are still NUMERIC.
	Shortfall httpx.Numeric `json:"shortfall"`
}

type ledgerRow struct {
	ID         uuid.UUID     `json:"id"`
	OccurredAt time.Time     `json:"occurredAt"`
	EntryType  string        `json:"entryType"`
	QtyDelta   httpx.Numeric `json:"qtyDelta"`
	UnitCost   httpx.Numeric `json:"unitCost"`
	SourceType string        `json:"sourceType"`
	SourceID   *uuid.UUID    `json:"sourceId"`
	// SourceNumber and SourcePOID resolve a `goods_receipt` row's document, so
	// §10.4's "rows linked to source documents" is a link rather than a UUID the
	// reader has to go and look up. Null for a manual adjustment, which has no
	// document behind it — the person is the source (§6.3).
	SourceNumber *string    `json:"sourceNumber"`
	SourcePOID   *uuid.UUID `json:"sourcePoId"`
	Note         *string    `json:"note"`
	ProductID    uuid.UUID  `json:"productId"`
	SKU          string     `json:"sku"`
	ProductName  string     `json:"productName"`
	// ProductDeleted lets the screen mark a row whose product has since been
	// deleted, rather than leaving the reader wondering why it is not in the
	// product list.
	ProductDeleted bool      `json:"productDeleted"`
	WarehouseID    uuid.UUID `json:"warehouseId"`
	WarehouseCode  string    `json:"warehouseCode"`
	CreatedByID    uuid.UUID `json:"createdById"`
	CreatedByName  string    `json:"createdByName"`
}

// --------------------------------------------------------------------------
// Reads.
// --------------------------------------------------------------------------

// listStock is GET /api/inventory/stock — the product × warehouse grid (§10.4).
//
// Rows come from stock_balances, so a product that has never moved has no row
// and a product whose movements cancel out has a row of zero. Both are shown:
// "we hold none of this here" is an answer, and hiding it makes the grid
// disagree with the ledger below it.
//
// A DELETED PRODUCT'S BALANCE IS SHOWN, marked. This is not the recycle bin —
// a balance is not the product record, it is a quantity of goods in a place, and
// the goods are still on the shelf. Three things have to agree or the screen
// lies to somebody:
//
//	the ledger        — already shows a deleted product's movements
//	this grid         — shows their sum
//	the warehouse row — counts it, and refuses the delete because of it (G5)
//
// Hiding it here was the first version, and it produced exactly the failure the
// agreement is meant to prevent: the warehouse list said "1 product, 30 on hand"
// and refused deletion over stock the grid showed nowhere.
//
// Deleted *warehouses* are still hidden, because a warehouse can only be deleted
// once it is empty — so nothing is being hidden. The `qty_on_hand <> 0` escape
// hatch is there in case that ever stops being true: stranded stock must surface
// somewhere.
func (s *server) listStock(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, stockSortable, "sku")
	if err != nil {
		return malformed(c, "%s", err)
	}

	productID, ok := optionalUUID(c, "productId")
	if !ok {
		return malformed(c, "productId is not a valid id.")
	}
	warehouseID, ok := optionalUUID(c, "warehouseId")
	if !ok {
		return malformed(c, "warehouseId is not a valid id.")
	}

	// One filter expression, used by the count and the page, so the two cannot
	// drift apart and report a total that does not match the rows.
	where := `
		WHERE (w.deleted_at IS NULL OR sb.qty_on_hand <> 0)
		  AND (?::uuid IS NULL OR sb.product_id = ?)
		  AND (?::uuid IS NULL OR sb.warehouse_id = ?)
		  AND (p.sku ILIKE ? OR p.name ILIKE ? OR w.code ILIKE ?)`
	args := []any{
		productID, productID,
		warehouseID, warehouseID,
		params.Like(), params.Like(), params.Like(),
	}
	from := `
		FROM stock_balances sb
		JOIN products   p ON p.id = sb.product_id
		JOIN warehouses w ON w.id = sb.warehouse_id`

	var total int64
	if err := tx.Raw(`SELECT count(*) `+from+where, args...).Scan(&total).Error; err != nil {
		return err
	}

	var rows []stockRow
	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	if err := tx.Raw(`
		SELECT sb.product_id, p.sku, p.name AS product_name, p.uom,
		       (p.deleted_at IS NOT NULL) AS product_deleted,
		       sb.warehouse_id, w.code AS warehouse_code, w.name AS warehouse_name,
		       sb.qty_on_hand::text AS qty_on_hand
		`+from+where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("sb.product_id")), page...).
		Scan(&rows).Error; err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// listLowStock is GET /api/inventory/stock/low — products below their reorder
// point (F2), and the source of the dashboard widget in §10.2.
//
// A product with no ledger rows at all counts: COALESCE makes its balance zero,
// and zero is below any positive reorder point. That is the case that matters —
// something has been set up and never received.
//
// `reorder_point > 0` excludes products nobody has set a level for. Without it
// every product with zero stock and the default reorder point of zero would be
// "low", and the widget would be noise on day one.
func (s *server) listLowStock(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, lowStockSortable, "-shortfall")
	if err != nil {
		return malformed(c, "%s", err)
	}

	where := lowStockFrom + `
		  AND (p.sku ILIKE ? OR p.name ILIKE ?)`
	args := []any{params.Like(), params.Like()}

	var total int64
	if err := tx.Raw(`SELECT count(*) `+where, args...).Scan(&total).Error; err != nil {
		return err
	}

	var rows []lowStockRow
	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	if err := tx.Raw(lowStockSelect+where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("p.id")), page...).Scan(&rows).Error; err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// lowStockFrom and lowStockSelect are the definition of "below reorder point",
// shared by this list and by the §10.2 dashboard widget.
//
// Extracted because there are now two callers, which is the bar §4 sets. The
// point is not the saved typing: the widget says "4 products are low" and the
// list it links to has to show four rows. Two copies of this expression is how
// they come to disagree — one of them gains `AND p.is_active` and the count
// stops matching the page underneath it.
//
// Both halves of the rule are load-bearing and neither is obvious:
// `reorder_point > 0` excludes products nobody has set a level for (every
// product defaults to zero, so without it the widget is noise on day one), and
// the comparison is strict, so a product sitting exactly at its point is not yet
// below it.
const lowStockFrom = `
		FROM products p
		LEFT JOIN (
		  SELECT product_id, SUM(qty_on_hand) AS qty_on_hand
		  FROM stock_balances GROUP BY product_id
		) b ON b.product_id = p.id
		WHERE p.deleted_at IS NULL
		  AND p.reorder_point > 0
		  AND COALESCE(b.qty_on_hand, 0) < p.reorder_point`

const lowStockSelect = `
		SELECT p.id AS product_id, p.sku, p.name, p.uom,
		       COALESCE(b.qty_on_hand, 0)::text                     AS qty_on_hand,
		       p.reorder_point::text                                AS reorder_point,
		       (p.reorder_point - COALESCE(b.qty_on_hand, 0))::text AS shortfall`

// listLedger is GET /api/inventory/ledger — the full, filterable ledger (§10.4).
//
// Products and warehouses are joined WITHOUT a deleted filter, deliberately.
// This is the historical-reference case of §6.9.1: a movement of a product
// someone deleted last week is still a movement that happened, and a ledger that
// drops rows when master data is tidied up is not a ledger. The `deletedAt` flag
// on the row is how the screen says so.
func (s *server) listLedger(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, ledgerSortable, "-occurredAt")
	if err != nil {
		return malformed(c, "%s", err)
	}

	productID, ok := optionalUUID(c, "productId")
	if !ok {
		return malformed(c, "productId is not a valid id.")
	}
	warehouseID, ok := optionalUUID(c, "warehouseId")
	if !ok {
		return malformed(c, "warehouseId is not a valid id.")
	}
	entryType := trimmed(c.Query("entryType"))
	if entryType != "" && !contains(entryTypes, entryType) {
		return malformed(c, "entryType must be one of %s.", strings.Join(entryTypes, ", "))
	}
	sourceType := trimmed(c.Query("sourceType"))
	if sourceType != "" && !contains(sourceTypes, sourceType) {
		return malformed(c, "sourceType must be one of %s.", strings.Join(sourceTypes, ", "))
	}
	// The rows one document wrote. This is what the goods receipt confirmation
	// panel links to: "2 stock ledger entries created" has to be followed by the
	// two entries themselves, or it is a claim the reader cannot check.
	sourceID, ok := optionalUUID(c, "sourceId")
	if !ok {
		return malformed(c, "sourceId is not a valid id.")
	}
	from, ok := optionalTime(c, "from")
	if !ok {
		return malformed(c, "from must be an RFC 3339 timestamp.")
	}
	to, ok := optionalTime(c, "to")
	if !ok {
		return malformed(c, "to must be an RFC 3339 timestamp.")
	}

	where := ledgerFrom + `
		WHERE (?::uuid IS NULL OR l.product_id = ?)
		  AND (?::uuid IS NULL OR l.warehouse_id = ?)
		  AND (? = '' OR l.entry_type = ?)
		  AND (? = '' OR l.source_type = ?)
		  AND (?::uuid IS NULL OR l.source_id = ?)
		  AND (?::timestamptz IS NULL OR l.occurred_at >= ?)
		  AND (?::timestamptz IS NULL OR l.occurred_at <= ?)
		  AND (p.sku ILIKE ? OR p.name ILIKE ? OR COALESCE(l.note, '') ILIKE ?)`
	args := []any{
		productID, productID,
		warehouseID, warehouseID,
		entryType, entryType,
		sourceType, sourceType,
		sourceID, sourceID,
		from, from,
		to, to,
		params.Like(), params.Like(), params.Like(),
	}

	var total int64
	if err := tx.Raw(`SELECT count(*) `+where, args...).Scan(&total).Error; err != nil {
		return err
	}

	var rows []ledgerRow
	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	if err := tx.Raw(ledgerSelect+where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("l.id")), page...).Scan(&rows).Error; err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// --------------------------------------------------------------------------
// The one write.
// --------------------------------------------------------------------------

// entryTypes and sourceTypes are the naming contract, and the same strings the
// CHECK constraints in 003 enforce. `reversal` is post-MVP.
var (
	entryTypes  = []string{"receipt", "issue", "adjustment"}
	sourceTypes = []string{"goods_receipt", "manual_adjustment"}
)

// ledgerSourceJoin resolves the document a `goods_receipt` row came from. It is
// shared by the list and the single-row read so the two cannot disagree about
// what a ledger row says its source is.
//
// A LEFT JOIN, because a manual adjustment legitimately has no document — and
// the `source_type` guard is on the join rather than in the WHERE, so a future
// source type with a UUID that happens to match a receipt cannot pick it up.
const ledgerSourceJoin = `
		LEFT JOIN goods_receipts gr
		  ON gr.id = l.source_id AND l.source_type = 'goods_receipt'`

// ledgerSourceColumns is the projection half of the same thing.
const ledgerSourceColumns = `
		       gr.gr_number AS source_number, gr.po_id AS source_po_id,`

// ledgerSelect and ledgerFrom are the ledger projection, in one place.
//
// Three callers read a ledger row now — the list, the single-row read after an
// adjustment, and the §10.2 recent-activity widget — and all three must produce
// the same shape, because all three fill the same `ledgerRow` struct and the
// same table renders it. Two of them were already exact copies of each other
// before the widget existed, which is the usual way a column gets added to one
// and not the other.
//
// The product and warehouse joins carry NO deleted filter, deliberately. A
// movement of a product someone deleted last week is still a movement that
// happened, and adding `AND p.deleted_at IS NULL` here by reflex would delete
// last quarter's history from three screens at once (§6.9.1, Trap 3). The
// `productDeleted` flag is how a row says so instead.
const ledgerSelect = `
		SELECT l.id, l.occurred_at, l.entry_type,
		       l.qty_delta::text AS qty_delta,
		       l.unit_cost::text AS unit_cost,
		       l.source_type, l.source_id, l.note,` + ledgerSourceColumns + `
		       l.product_id, p.sku, p.name AS product_name,
		       (p.deleted_at IS NOT NULL) AS product_deleted,
		       l.warehouse_id, w.code AS warehouse_code,
		       l.created_by AS created_by_id, u.full_name AS created_by_name`

const ledgerFrom = `
		FROM stock_ledger l
		JOIN products   p ON p.id = l.product_id
		JOIN warehouses w ON w.id = l.warehouse_id
		JOIN users      u ON u.id = l.created_by
		` + ledgerSourceJoin

type adjustmentRequest struct {
	ProductID   string         `json:"productId"`
	WarehouseID string         `json:"warehouseId"`
	QtyDelta    httpx.Numeric  `json:"qtyDelta"`
	UnitCost    *httpx.Numeric `json:"unitCost"`
	Note        string         `json:"note"`
}

type adjustmentResponse struct {
	Entry ledgerRow `json:"entry"`
	// Balance is the product/warehouse balance *after* the entry, read back
	// through the view inside the same transaction. The screen shows the number
	// move, and it is the view that says so rather than the handler doing the
	// arithmetic it just wrote.
	Balance httpx.Numeric `json:"qtyOnHand"`
}

// createAdjustment is POST /api/inventory/adjustments — level `approver`.
//
// It appends exactly one row: entry_type `adjustment`, source_type
// `manual_adjustment`, source_id NULL because there is no document behind it —
// the person is the source, and created_by records which person (§6.3).
//
// Deleted master data is refused; discontinued master data is not. Writing off
// the last of a product that has been discontinued is the ordinary reason to
// reach for this endpoint, and refusing it would leave that stock permanently
// on the books (§6.9.1 — is_active and deleted_at are different questions).
func (s *server) createAdjustment(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}

	var req adjustmentRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	productID, err := uuid.Parse(trimmed(req.ProductID))
	if err != nil {
		return malformed(c, "productId is required and must be an id.")
	}
	warehouseID, err := uuid.Parse(trimmed(req.WarehouseID))
	if err != nil {
		return malformed(c, "warehouseId is required and must be an id.")
	}

	// Zero is refused here as well as by ledger_qty_nonzero, so the caller gets
	// a sentence rather than a constraint name. The constraint stays because it
	// is what makes the rule true for every future code path (I10).
	qty, err := httpx.ParseNumeric(req.QtyDelta.String())
	if err != nil {
		return malformed(c, "qtyDelta: %s.", err)
	}
	if qty.IsZero() {
		return malformed(c, "qtyDelta cannot be zero — an adjustment of nothing is not a movement.")
	}

	// Both looked up with the deleted filter ON: this is a picker, not a
	// historical reference. A 404 rather than a 422, and the same 404 for "no
	// such product" and "another tenant's product" — RLS makes the second
	// indistinguishable from the first anyway.
	var product []struct {
		StandardCost httpx.Numeric
	}
	if err := tx.Raw(`
		SELECT standard_cost::text AS standard_cost
		FROM products WHERE id = ? AND deleted_at IS NULL`, productID).
		Scan(&product).Error; err != nil {
		return err
	}
	if len(product) == 0 {
		return notFound(c, "That product")
	}

	var warehouse []int
	if err := tx.Raw(`
		SELECT 1 FROM warehouses WHERE id = ? AND deleted_at IS NULL`, warehouseID).
		Scan(&warehouse).Error; err != nil {
		return err
	}
	if len(warehouse) == 0 {
		return notFound(c, "That warehouse")
	}

	// Unit cost defaults to the product's standard cost: an adjustment that
	// values stock at zero would understate inventory, and the person counting
	// a shelf has no cost to hand.
	unitCost := product[0].StandardCost
	if req.UnitCost != nil {
		unitCost, err = nonNegative(req.UnitCost, "unitCost")
		if err != nil {
			return malformed(c, "%s", err)
		}
	}

	id := uuid.New()
	if err := tx.Exec(`
		INSERT INTO stock_ledger
		  (id, tenant_id, product_id, warehouse_id, entry_type, qty_delta,
		   unit_cost, source_type, source_id, note, created_by)
		VALUES (?, ?, ?, ?, 'adjustment', ?, ?, 'manual_adjustment', NULL, ?, ?)`,
		id, caller.TenantID, productID, warehouseID, qty, unitCost,
		nullIfEmpty(trimmed(req.Note)), caller.UserID).Error; err != nil {
		return err
	}

	entry, err := s.ledgerEntry(tx, id)
	if err != nil {
		return err
	}
	if entry == nil {
		return fiber.NewError(fiber.StatusInternalServerError,
			"the adjustment was written but could not be read back")
	}

	var balance []struct {
		QtyOnHand httpx.Numeric
	}
	if err := tx.Raw(`
		SELECT COALESCE(SUM(qty_delta), 0)::text AS qty_on_hand
		FROM stock_ledger WHERE product_id = ? AND warehouse_id = ?`,
		productID, warehouseID).Scan(&balance).Error; err != nil {
		return err
	}
	after := httpx.Zero
	if len(balance) > 0 {
		after = balance[0].QtyOnHand
	}

	return c.Status(fiber.StatusCreated).
		JSON(adjustmentResponse{Entry: *entry, Balance: after})
}

// ledgerEntry reads one ledger row back with its joins resolved.
func (s *server) ledgerEntry(tx *gorm.DB, id uuid.UUID) (*ledgerRow, error) {
	var rows []ledgerRow
	if err := tx.Raw(ledgerSelect+ledgerFrom+`
		WHERE l.id = ?`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// --------------------------------------------------------------------------
// Query-parameter helpers. All pure — none of them writes a response.
// --------------------------------------------------------------------------

// optionalUUID reads a filter that may be absent. It reports validity rather
// than silently dropping an unparseable value: a filter that quietly stops
// filtering shows the caller more rows than they asked for, and in an ERP that
// reads as data appearing from nowhere.
func optionalUUID(c *fiber.Ctx, name string) (*uuid.UUID, bool) {
	raw := trimmed(c.Query(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false
	}
	return &id, true
}

// optionalTime reads an RFC 3339 filter bound. Stored timestamps are UTC (I7);
// a bound carrying an offset is converted, never reinterpreted.
func optionalTime(c *fiber.Ctx, name string) (*time.Time, bool) {
	raw := trimmed(c.Query(name))
	if raw == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, false
	}
	utc := parsed.UTC()
	return &utc, true
}

// nullIfEmpty stores an omitted note as NULL rather than as ”. "Nobody wrote a
// note" and "somebody wrote an empty one" are the same event, and only one of
// them should be representable.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
