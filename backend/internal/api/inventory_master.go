// Inventory master data — products and warehouses (§9.5, §10.4).
//
// TWO THINGS TO KNOW BEFORE EDITING THIS FILE.
//
//  1. `is_active` and `deleted_at` answer two different questions (§6.9.1).
//     `is_active = false` is discontinued: it stays in lists, reports, and the
//     ledger, and only disappears from the pickers that start new documents.
//     `deleted_at` is deleted: hidden from lists everywhere, still resolvable by
//     foreign key, and restorable. Conflating them loses either the history or
//     the recycle bin.
//
//  2. Resolving a row BY ID is deliberately unscoped. A ledger line from last
//     quarter must still render its product's name, so getProduct and every join
//     in inventory_stock.go read deleted rows too. It is the *lists* that filter,
//     and the writes that refuse. This is the `.Unscoped()` of §6.9.1, made
//     explicit because these queries are raw SQL rather than a GORM model — there
//     is no implicit `WHERE deleted_at IS NULL` to opt out of, which means every
//     filter here has to be written down and every omission is deliberate.
package api

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// ModuleInventory is the module code from the naming contract.
const ModuleInventory = "inventory"

var productSortable = map[string]string{
	"sku":          "p.sku",
	"name":         "p.name",
	"isActive":     "p.is_active",
	"reorderPoint": "p.reorder_point",
	"standardCost": "p.standard_cost",
	"qtyOnHand":    "COALESCE(b.qty_on_hand, 0)",
	"createdAt":    "p.created_at",
	"deletedAt":    "p.deleted_at",
}

var warehouseSortable = map[string]string{
	"code":      "w.code",
	"name":      "w.name",
	"isActive":  "w.is_active",
	"createdAt": "w.created_at",
	"deletedAt": "w.deleted_at",
}

type productRow struct {
	ID           uuid.UUID     `json:"id"`
	SKU          string        `json:"sku"`
	Name         string        `json:"name"`
	UOM          string        `json:"uom"`
	ReorderPoint httpx.Numeric `json:"reorderPoint"`
	StandardCost httpx.Numeric `json:"standardCost"`
	IsActive     bool          `json:"isActive"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
	// DeletedAt is null for a live product. The screens use it to mark a row as
	// deleted in the "show deleted" view and to offer Restore instead of Delete.
	DeletedAt *time.Time `json:"deletedAt"`

	// QtyOnHand is SUM(qty_delta) over the whole ledger for this product, read
	// through stock_balances. It is derived on every read and stored nowhere
	// (I6): the question "why is stock 47?" has to stay answerable.
	QtyOnHand httpx.Numeric `json:"qtyOnHand"`
	// BelowReorderPoint is decided by PostgreSQL, comparing two NUMERICs. Doing
	// it in Go would mean parsing both into floats first.
	BelowReorderPoint bool `json:"belowReorderPoint"`
}

// productBalanceRow is one warehouse's holding of one product.
type productBalanceRow struct {
	WarehouseID   uuid.UUID     `json:"warehouseId"`
	WarehouseCode string        `json:"warehouseCode"`
	WarehouseName string        `json:"warehouseName"`
	QtyOnHand     httpx.Numeric `json:"qtyOnHand"`
}

type productDetail struct {
	productRow
	Balances []productBalanceRow `json:"balances"`
}

type warehouseRow struct {
	ID        uuid.UUID  `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	IsActive  bool       `json:"isActive"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt"`

	// QtyOnHand is every product's balance in this warehouse, summed. It is what
	// the delete refusal is about, so the list shows it before anyone tries.
	QtyOnHand httpx.Numeric `json:"qtyOnHand"`
	// ProductCount counts products with a non-zero balance here.
	ProductCount int `json:"productCount"`
}

// --------------------------------------------------------------------------
// Soft-delete visibility.
// --------------------------------------------------------------------------

// wantsDeleted reads `?includeDeleted=true` (§9.0) and reports separately
// whether the caller asked and whether they may.
//
// Two return values rather than one refusal, because this must not write a
// response: a helper that signalled failure by returning what httpx.Fail
// returned would be signalling with nil, and the caller's `if err != nil` would
// never fire (see parseMatrix). The caller writes the 403.
//
// `admin` is the bar because §9.0 puts the restore workflow there — the recycle
// bin is an administrative view, and a viewer seeing deleted rows in an ordinary
// list would have no way to tell them apart from live ones.
func wantsDeleted(c *fiber.Ctx, caller *identity.Identity) (want, allowed bool) {
	want = c.QueryBool("includeDeleted", false)
	return want, caller.LevelFor(ModuleInventory) >= identity.RoleAdmin
}

// refuseDeletedView is the 403 for a viewer asking to see the recycle bin. It
// reuses insufficient_module_role with the level that would have worked, so the
// console can name the dropdown to change (§7).
func refuseDeletedView(c *fiber.Ctx, caller *identity.Identity) error {
	return httpx.FailWith(c, fiber.StatusForbidden, "insufficient_module_role",
		"Only an inventory administrator can see deleted records.",
		map[string]any{
			"module":   ModuleInventory,
			"required": identity.RoleAdmin.String(),
			"actual":   caller.LevelFor(ModuleInventory).String(),
		})
}

// --------------------------------------------------------------------------
// Products.
// --------------------------------------------------------------------------

// listProducts is GET /api/inventory/products — the §10.4 list, with current
// stock and the reorder flag.
func (s *server) listProducts(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, productSortable, "sku")
	if err != nil {
		return malformed(c, "%s", err)
	}
	withDeleted, allowed := wantsDeleted(c, caller)
	if withDeleted && !allowed {
		return refuseDeletedView(c, caller)
	}

	var total int64
	if err := tx.Raw(`
		SELECT count(*) FROM products p
		WHERE (?::bool OR p.deleted_at IS NULL)
		  AND (p.sku ILIKE ? OR p.name ILIKE ?)`,
		withDeleted, params.Like(), params.Like()).Scan(&total).Error; err != nil {
		return err
	}

	rows, err := s.productRows(tx, fmt.Sprintf(`
		WHERE (?::bool OR p.deleted_at IS NULL)
		  AND (p.sku ILIKE ? OR p.name ILIKE ?)
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("p.id")),
		withDeleted, params.Like(), params.Like(), params.PageSize, params.Offset())
	if err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// productRows runs the product projection with a caller-supplied tail.
//
// The tail is a constant in every caller — never a request value — and the only
// interpolation in it is params.OrderBy, whose column came out of the sortable
// allowlist. A request cannot introduce SQL here.
//
// The balance is a LEFT JOIN against a grouped stock_balances rather than a
// correlated subquery per row: the list needs it for a whole page at a time.
func (s *server) productRows(tx *gorm.DB, tail string, args ...any) ([]productRow, error) {
	var rows []productRow
	err := tx.Raw(`
		SELECT p.id, p.sku, p.name, p.uom,
		       p.reorder_point::text  AS reorder_point,
		       p.standard_cost::text  AS standard_cost,
		       p.is_active, p.created_at, p.updated_at, p.deleted_at,
		       COALESCE(b.qty_on_hand, 0)::text AS qty_on_hand,
		       COALESCE(b.qty_on_hand, 0) < p.reorder_point AS below_reorder_point
		FROM products p
		LEFT JOIN (
		  SELECT product_id, SUM(qty_on_hand) AS qty_on_hand
		  FROM stock_balances GROUP BY product_id
		) b ON b.product_id = p.id
		`+tail, args...).Scan(&rows).Error
	return rows, err
}

// getProduct is GET /api/inventory/products/:id.
//
// Unscoped by design: a deleted product resolves. The ledger screen links every
// historical row to its product, and a 404 there would make last quarter's stock
// movements unreadable — which is the exact failure soft delete exists to
// prevent (§6.9.1). `deletedAt` in the payload is how the screen knows to say so.
func (s *server) getProduct(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That product")
	}

	detail, err := s.productDetail(tx, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That product")
	}
	return c.JSON(detail)
}

// productDetail reads one product with its per-warehouse balances, or nil if
// there is no such product in this tenant. RLS is the tenant filter — products
// is one of the fourteen RLS-forced tables, unlike `users` next door.
func (s *server) productDetail(tx *gorm.DB, id uuid.UUID) (*productDetail, error) {
	rows, err := s.productRows(tx, `WHERE p.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Warehouses joined without a deleted filter, for the same reason as above:
	// this is a historical reference being resolved, not a picker being filled.
	var balances []productBalanceRow
	if err := tx.Raw(`
		SELECT sb.warehouse_id, w.code AS warehouse_code, w.name AS warehouse_name,
		       sb.qty_on_hand::text AS qty_on_hand
		FROM stock_balances sb
		JOIN warehouses w ON w.id = sb.warehouse_id
		WHERE sb.product_id = ? AND sb.qty_on_hand <> 0
		ORDER BY w.code`, id).Scan(&balances).Error; err != nil {
		return nil, err
	}
	if balances == nil {
		balances = []productBalanceRow{}
	}

	return &productDetail{productRow: rows[0], Balances: balances}, nil
}

type productWriteRequest struct {
	SKU          *string        `json:"sku"`
	Name         *string        `json:"name"`
	UOM          *string        `json:"uom"`
	ReorderPoint *httpx.Numeric `json:"reorderPoint"`
	StandardCost *httpx.Numeric `json:"standardCost"`
	IsActive     *bool          `json:"isActive"`
}

// createProduct is POST /api/inventory/products.
func (s *server) createProduct(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}

	var req productWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	sku := trimmed(derefString(req.SKU))
	name := trimmed(derefString(req.Name))
	uom := trimmed(derefString(req.UOM))
	if uom == "" {
		uom = "pcs"
	}
	if sku == "" {
		return malformed(c, "sku is required.")
	}
	if name == "" {
		return malformed(c, "name is required.")
	}

	reorderPoint, err := nonNegative(req.ReorderPoint, "reorderPoint")
	if err != nil {
		return malformed(c, "%s", err)
	}
	standardCost, err := nonNegative(req.StandardCost, "standardCost")
	if err != nil {
		return malformed(c, "%s", err)
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	id := uuid.New()
	if err := tx.Exec(`
		INSERT INTO products
		  (id, tenant_id, sku, name, uom, reorder_point, standard_cost, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, caller.TenantID, sku, name, uom, reorderPoint, standardCost, isActive).Error; err != nil {
		if db.IsUniqueViolation(err) {
			return duplicateSKU(c, sku)
		}
		return err
	}

	detail, err := s.productDetail(tx, id)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(detail)
}

// patchProduct is PATCH /api/inventory/products/:id.
//
// A deleted product cannot be edited — restore it first. Editing something in
// the recycle bin is how two rows end up fighting over one SKU without anyone
// having chosen that.
func (s *server) patchProduct(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That product")
	}

	var req productWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	sets := map[string]any{}
	if req.SKU != nil {
		sku := trimmed(*req.SKU)
		if sku == "" {
			return malformed(c, "sku cannot be empty.")
		}
		sets["sku"] = sku
	}
	if req.Name != nil {
		name := trimmed(*req.Name)
		if name == "" {
			return malformed(c, "name cannot be empty.")
		}
		sets["name"] = name
	}
	if req.UOM != nil {
		uom := trimmed(*req.UOM)
		if uom == "" {
			return malformed(c, "uom cannot be empty.")
		}
		sets["uom"] = uom
	}
	if req.ReorderPoint != nil {
		value, err := nonNegative(req.ReorderPoint, "reorderPoint")
		if err != nil {
			return malformed(c, "%s", err)
		}
		sets["reorder_point"] = value
	}
	if req.StandardCost != nil {
		value, err := nonNegative(req.StandardCost, "standardCost")
		if err != nil {
			return malformed(c, "%s", err)
		}
		sets["standard_cost"] = value
	}
	if req.IsActive != nil {
		sets["is_active"] = *req.IsActive
	}
	if len(sets) == 0 {
		return malformed(c, "Nothing to change.")
	}

	result := tx.Table("products").
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(sets)
	if result.Error != nil {
		if db.IsUniqueViolation(result.Error) {
			return duplicateSKU(c, fmt.Sprint(sets["sku"]))
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That product")
	}

	detail, err := s.productDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

// deleteProduct is DELETE /api/inventory/products/:id — a soft delete (§6.9.1).
//
// There is no DELETE statement here and there is not going to be one (I5). The
// row keeps its identity so every ledger entry and purchase order line that
// points at it still resolves; it simply stops appearing.
//
// It is refused while something in flight still depends on it. Historical
// references do NOT block — a product on a received PO from March is exactly
// what soft delete is for. Only *open* commitments do.
func (s *server) deleteProduct(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That product")
	}

	// The in-use check and the write are in one transaction, so a PO line
	// created between them cannot slip past: TenantTx opened it, and the
	// serialization is the database's problem rather than a lock we hold.
	var openLines int64
	if err := tx.Raw(`
		SELECT count(*)
		FROM purchase_order_lines pol
		JOIN purchase_orders po ON po.id = pol.po_id
		WHERE pol.product_id = ? AND po.status IN ('open', 'partially_received')`,
		id).Scan(&openLines).Error; err != nil {
		return err
	}
	if openLines > 0 {
		return httpx.FailWith(c, fiber.StatusConflict, "in_use",
			fmt.Sprintf("This product is on %s. Cancel or receive the order first.",
				plural(openLines, "open purchase order line")),
			map[string]any{"openPurchaseOrderLines": openLines})
	}

	result := tx.Exec(`
		UPDATE products SET deleted_at = now(), deleted_by = ?
		WHERE id = ? AND deleted_at IS NULL`, caller.UserID, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That product")
	}

	detail, err := s.productDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

// restoreProduct is POST /api/inventory/products/:id/restore.
//
// Any user who can delete can restore (§6.9.1) — the same `admin` level, not a
// higher one. Restoring is refused when the SKU has been taken in the meantime:
// products_sku_active is a partial unique index over live rows, so the second
// product was legal to create and this one is now the intruder. The database
// says so, and G3 is the test that the index is actually there.
func (s *server) restoreProduct(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That product")
	}

	var rows []struct {
		SKU     string
		Deleted bool
	}
	if err := tx.Raw(`
		SELECT sku, (deleted_at IS NOT NULL) AS deleted
		FROM products WHERE id = ?`, id).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return notFound(c, "That product")
	}

	// Restoring something that was never deleted is a no-op rather than an
	// error: the caller wanted it live and it is live. Two admins clicking
	// Restore on the same row should not produce a failure for the slower one.
	if rows[0].Deleted {
		if err := tx.Exec(`
			UPDATE products SET deleted_at = NULL, deleted_by = NULL
			WHERE id = ? AND deleted_at IS NOT NULL`, id).Error; err != nil {
			if db.IsUniqueViolation(err) {
				return httpx.FailWith(c, fiber.StatusConflict, "in_use",
					fmt.Sprintf("Another product now uses SKU %s. "+
						"Change that one's SKU, or this product's, before restoring it.",
						rows[0].SKU),
					map[string]any{"sku": rows[0].SKU})
			}
			return err
		}
	}

	detail, err := s.productDetail(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

func duplicateSKU(c *fiber.Ctx, sku string) error {
	return httpx.FailWith(c, fiber.StatusConflict, "in_use",
		fmt.Sprintf("SKU %s is already in use.", sku),
		map[string]any{"sku": sku})
}

// --------------------------------------------------------------------------
// Warehouses.
// --------------------------------------------------------------------------

// listWarehouses is GET /api/inventory/warehouses.
func (s *server) listWarehouses(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, warehouseSortable, "code")
	if err != nil {
		return malformed(c, "%s", err)
	}
	withDeleted, allowed := wantsDeleted(c, caller)
	if withDeleted && !allowed {
		return refuseDeletedView(c, caller)
	}

	var total int64
	if err := tx.Raw(`
		SELECT count(*) FROM warehouses w
		WHERE (?::bool OR w.deleted_at IS NULL)
		  AND (w.code ILIKE ? OR w.name ILIKE ?)`,
		withDeleted, params.Like(), params.Like()).Scan(&total).Error; err != nil {
		return err
	}

	rows, err := s.warehouseRows(tx, fmt.Sprintf(`
		WHERE (?::bool OR w.deleted_at IS NULL)
		  AND (w.code ILIKE ? OR w.name ILIKE ?)
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("w.id")),
		withDeleted, params.Like(), params.Like(), params.PageSize, params.Offset())
	if err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// warehouseRows runs the warehouse projection with a caller-supplied tail.
//
// product_count counts products with a NON-ZERO balance, not rows in the view:
// a product received and then fully issued leaves a stock_balances row of 0,
// and counting it would tell the user a warehouse holds two products when it
// holds none.
func (s *server) warehouseRows(tx *gorm.DB, tail string, args ...any) ([]warehouseRow, error) {
	var rows []warehouseRow
	err := tx.Raw(`
		SELECT w.id, w.code, w.name, w.is_active,
		       w.created_at, w.updated_at, w.deleted_at,
		       COALESCE(b.qty_on_hand, 0)::text AS qty_on_hand,
		       COALESCE(b.product_count, 0)     AS product_count
		FROM warehouses w
		LEFT JOIN (
		  SELECT warehouse_id,
		         SUM(qty_on_hand)                            AS qty_on_hand,
		         count(*) FILTER (WHERE qty_on_hand <> 0)    AS product_count
		  FROM stock_balances GROUP BY warehouse_id
		) b ON b.warehouse_id = w.id
		`+tail, args...).Scan(&rows).Error
	return rows, err
}

// getWarehouse is GET /api/inventory/warehouses/:id. Unscoped, for the reason
// given at the top of this file.
func (s *server) getWarehouse(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That warehouse")
	}

	row, err := s.warehouseByID(tx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return notFound(c, "That warehouse")
	}
	return c.JSON(row)
}

func (s *server) warehouseByID(tx *gorm.DB, id uuid.UUID) (*warehouseRow, error) {
	rows, err := s.warehouseRows(tx, `WHERE w.id = ?`, id)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

type warehouseWriteRequest struct {
	Code     *string `json:"code"`
	Name     *string `json:"name"`
	IsActive *bool   `json:"isActive"`
}

// createWarehouse is POST /api/inventory/warehouses.
func (s *server) createWarehouse(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}

	var req warehouseWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}
	code := trimmed(derefString(req.Code))
	name := trimmed(derefString(req.Name))
	if code == "" {
		return malformed(c, "code is required.")
	}
	if name == "" {
		return malformed(c, "name is required.")
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	id := uuid.New()
	if err := tx.Exec(`
		INSERT INTO warehouses (id, tenant_id, code, name, is_active)
		VALUES (?, ?, ?, ?, ?)`,
		id, caller.TenantID, code, name, isActive).Error; err != nil {
		if db.IsUniqueViolation(err) {
			return duplicateWarehouseCode(c, code)
		}
		return err
	}

	row, err := s.warehouseByID(tx, id)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

// patchWarehouse is PATCH /api/inventory/warehouses/:id.
func (s *server) patchWarehouse(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That warehouse")
	}

	var req warehouseWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	sets := map[string]any{}
	if req.Code != nil {
		code := trimmed(*req.Code)
		if code == "" {
			return malformed(c, "code cannot be empty.")
		}
		sets["code"] = code
	}
	if req.Name != nil {
		name := trimmed(*req.Name)
		if name == "" {
			return malformed(c, "name cannot be empty.")
		}
		sets["name"] = name
	}
	if req.IsActive != nil {
		sets["is_active"] = *req.IsActive
	}
	if len(sets) == 0 {
		return malformed(c, "Nothing to change.")
	}

	result := tx.Table("warehouses").
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(sets)
	if result.Error != nil {
		if db.IsUniqueViolation(result.Error) {
			return duplicateWarehouseCode(c, fmt.Sprint(sets["code"]))
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That warehouse")
	}

	row, err := s.warehouseByID(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(row)
}

// deleteWarehouse is DELETE /api/inventory/warehouses/:id — soft delete, blocked
// while stock is on hand (§9.5, G5).
//
// "Non-zero stock" is asked per product, never as one total. A warehouse holding
// +5 of one product and -5 of another sums to zero and is emphatically not
// empty; deleting it would strand both balances somewhere nobody can see.
func (s *server) deleteWarehouse(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That warehouse")
	}

	var holdings []struct {
		Products int64
		Qty      httpx.Numeric
	}
	if err := tx.Raw(`
		SELECT count(*)                        AS products,
		       COALESCE(SUM(qty_on_hand), 0)::text AS qty
		FROM stock_balances
		WHERE warehouse_id = ? AND qty_on_hand <> 0`, id).Scan(&holdings).Error; err != nil {
		return err
	}
	if len(holdings) > 0 && holdings[0].Products > 0 {
		return httpx.FailWith(c, fiber.StatusConflict, "in_use",
			fmt.Sprintf("This warehouse still holds stock of %s. "+
				"Move or write off the stock first.",
				plural(holdings[0].Products, "product")),
			map[string]any{
				"productsWithStock": holdings[0].Products,
				"qtyOnHand":         holdings[0].Qty,
			})
	}

	result := tx.Exec(`
		UPDATE warehouses SET deleted_at = now(), deleted_by = ?
		WHERE id = ? AND deleted_at IS NULL`, caller.UserID, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That warehouse")
	}

	row, err := s.warehouseByID(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(row)
}

// restoreWarehouse is POST /api/inventory/warehouses/:id/restore.
func (s *server) restoreWarehouse(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That warehouse")
	}

	var rows []struct {
		Code    string
		Deleted bool
	}
	if err := tx.Raw(`
		SELECT code, (deleted_at IS NOT NULL) AS deleted
		FROM warehouses WHERE id = ?`, id).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return notFound(c, "That warehouse")
	}

	if rows[0].Deleted {
		if err := tx.Exec(`
			UPDATE warehouses SET deleted_at = NULL, deleted_by = NULL
			WHERE id = ? AND deleted_at IS NOT NULL`, id).Error; err != nil {
			if db.IsUniqueViolation(err) {
				return httpx.FailWith(c, fiber.StatusConflict, "in_use",
					fmt.Sprintf("Another warehouse now uses code %s. "+
						"Change that one's code, or this one's, before restoring it.",
						rows[0].Code),
					map[string]any{"code": rows[0].Code})
			}
			return err
		}
	}

	row, err := s.warehouseByID(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(row)
}

func duplicateWarehouseCode(c *fiber.Ctx, code string) error {
	return httpx.FailWith(c, fiber.StatusConflict, "in_use",
		fmt.Sprintf("Warehouse code %s is already in use.", code),
		map[string]any{"code": code})
}
