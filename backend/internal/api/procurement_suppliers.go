// Procurement master data — suppliers (§9.4, §10.3).
//
// The same two-column model as products and warehouses (§6.9.1): `is_active` is
// discontinued and stays everywhere except the pickers, `deleted_at` is deleted
// and is hidden from lists while still resolving by foreign key. Resolving a
// supplier BY ID is deliberately unscoped, so a purchase order from March still
// renders the name of a supplier somebody has since tidied away.
//
// The one rule here that is not shared with inventory is the in-use check: a
// supplier with an open or partially-received purchase order cannot be deleted
// (G4). A *received* order does not block — that is precisely what soft delete
// is for.
package api

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
)

// ModuleProcurement is the module code from the naming contract.
const ModuleProcurement = "procurement"

var supplierSortable = map[string]string{
	"code":         "s.code",
	"name":         "s.name",
	"isActive":     "s.is_active",
	"leadTimeDays": "s.lead_time_days",
	"createdAt":    "s.created_at",
	"deletedAt":    "s.deleted_at",
	"openOrders":   "COALESCE(o.open_orders, 0)",
}

type supplierRow struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	ContactEmail *string   `json:"contactEmail"`
	ContactPhone *string   `json:"contactPhone"`
	LeadTimeDays int       `json:"leadTimeDays"`
	PaymentTerms string    `json:"paymentTerms"`
	IsActive     bool      `json:"isActive"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	// DeletedAt is null for a live supplier. The screens use it to mark a row in
	// the "show deleted" view and to offer Restore instead of Delete.
	DeletedAt *time.Time `json:"deletedAt"`

	// OpenOrders counts the orders that would refuse a delete. Shown in the list
	// so the refusal is not the first the user hears of it (G4).
	OpenOrders int `json:"openOrders"`
}

// --------------------------------------------------------------------------
// Reads.
// --------------------------------------------------------------------------

// listSuppliers is GET /api/procurement/suppliers.
func (s *server) listSuppliers(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, supplierSortable, "code")
	if err != nil {
		return malformed(c, "%s", err)
	}
	withDeleted, allowed := wantsDeleted(c, caller, ModuleProcurement)
	if withDeleted && !allowed {
		return refuseDeletedView(c, caller, ModuleProcurement)
	}

	var total int64
	if err := tx.Raw(`
		SELECT count(*) FROM suppliers s
		WHERE (?::bool OR s.deleted_at IS NULL)
		  AND (s.code ILIKE ? OR s.name ILIKE ?)`,
		withDeleted, params.Like(), params.Like()).Scan(&total).Error; err != nil {
		return err
	}

	rows, err := s.supplierRows(tx, fmt.Sprintf(`
		WHERE (?::bool OR s.deleted_at IS NULL)
		  AND (s.code ILIKE ? OR s.name ILIKE ?)
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("s.id")),
		withDeleted, params.Like(), params.Like(), params.PageSize, params.Offset())
	if err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// supplierRows runs the supplier projection with a caller-supplied tail.
//
// The tail is a constant in every caller — never a request value — and the only
// interpolation in it is params.OrderBy, whose column came out of the sortable
// allowlist. A request cannot introduce SQL here.
func (s *server) supplierRows(tx *gorm.DB, tail string, args ...any) ([]supplierRow, error) {
	var rows []supplierRow
	err := tx.Raw(`
		SELECT s.id, s.code, s.name, s.contact_email, s.contact_phone,
		       s.lead_time_days, s.payment_terms, s.is_active,
		       s.created_at, s.updated_at, s.deleted_at,
		       COALESCE(o.open_orders, 0) AS open_orders
		FROM suppliers s
		LEFT JOIN (
		  SELECT supplier_id, count(*) AS open_orders
		  FROM purchase_orders
		  WHERE status IN ('open', 'partially_received')
		  GROUP BY supplier_id
		) o ON o.supplier_id = s.id
		`+tail, args...).Scan(&rows).Error
	return rows, err
}

// getSupplier is GET /api/procurement/suppliers/:id. Unscoped by design: a
// deleted supplier resolves, because every purchase order links to one and a 404
// there would make last quarter's orders unreadable (§6.9.1).
func (s *server) getSupplier(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That supplier")
	}

	row, err := s.supplierByID(tx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return notFound(c, "That supplier")
	}
	return c.JSON(row)
}

func (s *server) supplierByID(tx *gorm.DB, id uuid.UUID) (*supplierRow, error) {
	rows, err := s.supplierRows(tx, `WHERE s.id = ?`, id)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// --------------------------------------------------------------------------
// Writes.
// --------------------------------------------------------------------------

type supplierWriteRequest struct {
	Code         *string `json:"code"`
	Name         *string `json:"name"`
	ContactEmail *string `json:"contactEmail"`
	ContactPhone *string `json:"contactPhone"`
	LeadTimeDays *int    `json:"leadTimeDays"`
	PaymentTerms *string `json:"paymentTerms"`
	IsActive     *bool   `json:"isActive"`
}

// defaultPaymentTerms and defaultLeadTimeDays mirror the column defaults in
// §6.4. Written here as well so a create request that omits them produces the
// same row whether it arrives as JSON `null` or as an absent key.
const (
	defaultPaymentTerms = "NET30"
	defaultLeadTimeDays = 7
)

// createSupplier is POST /api/procurement/suppliers.
func (s *server) createSupplier(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}

	var req supplierWriteRequest
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
	if email := trimmed(derefString(req.ContactEmail)); email != "" {
		if err := validateEmail(email); err != nil {
			return malformed(c, "contactEmail: %s", err)
		}
	}
	leadTime := defaultLeadTimeDays
	if req.LeadTimeDays != nil {
		if *req.LeadTimeDays < 0 {
			return malformed(c, "leadTimeDays cannot be negative.")
		}
		leadTime = *req.LeadTimeDays
	}
	terms := trimmed(derefString(req.PaymentTerms))
	if terms == "" {
		terms = defaultPaymentTerms
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	id := uuid.New()
	if err := tx.Exec(`
		INSERT INTO suppliers
		  (id, tenant_id, code, name, contact_email, contact_phone,
		   lead_time_days, payment_terms, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, caller.TenantID, code, name,
		nullIfEmpty(trimmed(derefString(req.ContactEmail))),
		nullIfEmpty(trimmed(derefString(req.ContactPhone))),
		leadTime, terms, isActive).Error; err != nil {
		if db.IsUniqueViolation(err) {
			return duplicateSupplierCode(c, code)
		}
		return err
	}

	row, err := s.supplierByID(tx, id)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

// patchSupplier is PATCH /api/procurement/suppliers/:id.
//
// A deleted supplier cannot be edited — restore it first. Editing something in
// the recycle bin is how two rows end up fighting over one code without anyone
// having chosen that.
func (s *server) patchSupplier(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That supplier")
	}

	var req supplierWriteRequest
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
	// The two contact fields are nullable, so an empty string means "clear it"
	// rather than "leave it alone" — absence is what means leave it alone.
	if req.ContactEmail != nil {
		email := trimmed(*req.ContactEmail)
		if email != "" {
			if err := validateEmail(email); err != nil {
				return malformed(c, "contactEmail: %s", err)
			}
		}
		sets["contact_email"] = nullIfEmpty(email)
	}
	if req.ContactPhone != nil {
		sets["contact_phone"] = nullIfEmpty(trimmed(*req.ContactPhone))
	}
	if req.LeadTimeDays != nil {
		if *req.LeadTimeDays < 0 {
			return malformed(c, "leadTimeDays cannot be negative.")
		}
		sets["lead_time_days"] = *req.LeadTimeDays
	}
	if req.PaymentTerms != nil {
		terms := trimmed(*req.PaymentTerms)
		if terms == "" {
			return malformed(c, "paymentTerms cannot be empty.")
		}
		sets["payment_terms"] = terms
	}
	if req.IsActive != nil {
		sets["is_active"] = *req.IsActive
	}
	if len(sets) == 0 {
		return malformed(c, "Nothing to change.")
	}

	result := tx.Table("suppliers").
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(sets)
	if result.Error != nil {
		if db.IsUniqueViolation(result.Error) {
			return duplicateSupplierCode(c, fmt.Sprint(sets["code"]))
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That supplier")
	}

	row, err := s.supplierByID(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(row)
}

// deleteSupplier is DELETE /api/procurement/suppliers/:id — a soft delete
// (§6.9.1), refused while an order is still in flight (G4).
//
// `open` and `partially_received` block; `received` and `cancelled` do not. The
// question is whether anything still *depends* on this supplier, not whether
// anything ever referenced it — a supplier who filled an order in March is
// exactly the case soft delete exists to keep resolvable.
func (s *server) deleteSupplier(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That supplier")
	}

	// The check and the write share one transaction, so an order created between
	// them cannot slip past: serialization is the database's problem rather than
	// a lock this handler has to hold.
	var open []struct {
		Orders  int64
		Numbers string
	}
	if err := tx.Raw(`
		SELECT count(*) AS orders,
		       COALESCE(string_agg(po_number, ', ' ORDER BY po_number), '') AS numbers
		FROM purchase_orders
		WHERE supplier_id = ? AND status IN ('open', 'partially_received')`,
		id).Scan(&open).Error; err != nil {
		return err
	}
	if len(open) > 0 && open[0].Orders > 0 {
		return httpx.FailWith(c, fiber.StatusConflict, "in_use",
			fmt.Sprintf("This supplier is on %s (%s). Receive or cancel them first.",
				plural(open[0].Orders, "open purchase order"), open[0].Numbers),
			map[string]any{
				"openPurchaseOrders":       open[0].Orders,
				"openPurchaseOrderNumbers": open[0].Numbers,
			})
	}

	result := tx.Exec(`
		UPDATE suppliers SET deleted_at = now(), deleted_by = ?
		WHERE id = ? AND deleted_at IS NULL`, caller.UserID, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That supplier")
	}

	row, err := s.supplierByID(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(row)
}

// restoreSupplier is POST /api/procurement/suppliers/:id/restore.
//
// Restoring something that was never deleted is a 200 no-op: the caller wanted
// it live and it is live, and two admins clicking Restore on the same row should
// not produce a failure for the slower one.
func (s *server) restoreSupplier(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That supplier")
	}

	var rows []struct {
		Code    string
		Deleted bool
	}
	if err := tx.Raw(`
		SELECT code, (deleted_at IS NOT NULL) AS deleted
		FROM suppliers WHERE id = ?`, id).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return notFound(c, "That supplier")
	}

	if rows[0].Deleted {
		// suppliers_code_active is a partial unique index over live rows, so a
		// replacement supplier with this code was legal to create and this one is
		// now the intruder. The database says so.
		if err := tx.Exec(`
			UPDATE suppliers SET deleted_at = NULL, deleted_by = NULL
			WHERE id = ? AND deleted_at IS NOT NULL`, id).Error; err != nil {
			if db.IsUniqueViolation(err) {
				return httpx.FailWith(c, fiber.StatusConflict, "in_use",
					fmt.Sprintf("Another supplier now uses code %s. "+
						"Change that one's code, or this one's, before restoring it.",
						rows[0].Code),
					map[string]any{"code": rows[0].Code})
			}
			return err
		}
	}

	row, err := s.supplierByID(tx, id)
	if err != nil {
		return err
	}
	return c.JSON(row)
}

func duplicateSupplierCode(c *fiber.Ctx, code string) error {
	return httpx.FailWith(c, fiber.StatusConflict, "in_use",
		fmt.Sprintf("Supplier code %s is already in use.", code),
		map[string]any{"code": code})
}
