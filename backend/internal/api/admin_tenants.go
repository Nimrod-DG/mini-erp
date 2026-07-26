// Superadmin endpoints — the platform plane of §5.7. They answer "which modules
// does this company have?" and nothing about who inside it may do what.
//
// Every handler here runs on the erp_admin pool, which is REVOKED from all
// fourteen tenant business tables (§4.2). That revoke, not the code below, is
// what makes §5.5's "superadmins have no access to tenant business data" a
// property: a bug, a bad join, or a compromised handler still cannot read a
// tenant's purchase orders, because the database refuses (A11).
package api

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/auth"
	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
)

// tenantSortable is the §9.0 sort allowlist: API field → SQL column.
var tenantSortable = map[string]string{
	"name":      "t.name",
	"slug":      "t.slug",
	"status":    "t.status",
	"createdAt": "t.created_at",
}

type tenantListRow struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`
	Status   string    `json:"status"`
	Timezone string    `json:"timezone"`
	// UserCount counts active users only. A workspace whose people have all
	// been deactivated is empty in the sense the console is asking about.
	UserCount    int       `json:"userCount"`
	ModuleCount  int       `json:"moduleCount"`
	AdminCount   int       `json:"adminCount"`
	CreatedAt    time.Time `json:"createdAt"`
	EnabledCodes []string  `json:"enabledModules" gorm:"-"`
}

// listTenants is GET /api/admin/tenants.
func (s *server) listTenants(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, tenantSortable, "name")
	if err != nil {
		return malformed(c, "%s", err)
	}

	var total int64
	if err := g.WithContext(c.UserContext()).Raw(`
		SELECT count(*) FROM tenants t
		WHERE t.name ILIKE ? OR t.slug ILIKE ?`,
		params.Like(), params.Like()).Scan(&total).Error; err != nil {
		return err
	}

	// The counts are correlated subqueries rather than joins with GROUP BY:
	// three aggregates over three different tables would otherwise multiply
	// each other's rows, and the classic symptom is a user count that grows
	// every time a module is enabled.
	var rows []tenantListRow
	if err := g.WithContext(c.UserContext()).Raw(fmt.Sprintf(`
		SELECT t.id, t.name, t.slug, t.status, t.timezone, t.created_at,
		       (SELECT count(*) FROM users u
		         WHERE u.tenant_id = t.id AND u.is_active) AS user_count,
		       (SELECT count(*) FROM users u
		         WHERE u.tenant_id = t.id AND u.is_active
		           AND u.tenant_role = 'admin')            AS admin_count,
		       (SELECT count(*) FROM tenant_modules tm
		         WHERE tm.tenant_id = t.id AND tm.enabled)  AS module_count
		FROM tenants t
		WHERE t.name ILIKE ? OR t.slug ILIKE ?
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("t.id")),
		params.Like(), params.Like(), params.PageSize, params.Offset()).
		Scan(&rows).Error; err != nil {
		return err
	}

	// The pills the list screen renders (§10.6) need the codes, not just a
	// count. One query for the whole page rather than one per row.
	if err := s.attachEnabledModules(c, g, rows); err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// attachEnabledModules fills EnabledCodes for a page of tenants.
func (s *server) attachEnabledModules(c *fiber.Ctx, g *gorm.DB, rows []tenantListRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
		rows[i].EnabledCodes = []string{}
	}

	var pairs []struct {
		TenantID   uuid.UUID
		ModuleCode string
	}
	if err := g.WithContext(c.UserContext()).Raw(`
		SELECT tm.tenant_id, tm.module_code
		FROM tenant_modules tm
		JOIN modules m ON m.code = tm.module_code AND m.is_available = true
		WHERE tm.tenant_id IN ? AND tm.enabled = true
		ORDER BY m.sort_order`, ids).Scan(&pairs).Error; err != nil {
		return err
	}

	byID := make(map[uuid.UUID]*tenantListRow, len(rows))
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	for _, p := range pairs {
		if row, ok := byID[p.TenantID]; ok {
			row.EnabledCodes = append(row.EnabledCodes, p.ModuleCode)
		}
	}
	return nil
}

type tenantModuleRow struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type tenantDetail struct {
	tenantListRow
	Modules []tenantModuleRow `json:"modules"`
}

// getTenant is GET /api/admin/tenants/:id.
func (s *server) getTenant(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That workspace")
	}

	detail, err := s.tenantDetail(c, g, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That workspace")
	}
	return c.JSON(detail)
}

// tenantDetail reads one tenant with its entitlement matrix, or nil if there is
// no such tenant.
func (s *server) tenantDetail(c *fiber.Ctx, g *gorm.DB, id uuid.UUID) (*tenantDetail, error) {
	var rows []tenantListRow
	if err := g.WithContext(c.UserContext()).Raw(`
		SELECT t.id, t.name, t.slug, t.status, t.timezone, t.created_at,
		       (SELECT count(*) FROM users u
		         WHERE u.tenant_id = t.id AND u.is_active) AS user_count,
		       (SELECT count(*) FROM users u
		         WHERE u.tenant_id = t.id AND u.is_active
		           AND u.tenant_role = 'admin')            AS admin_count,
		       (SELECT count(*) FROM tenant_modules tm
		         WHERE tm.tenant_id = t.id AND tm.enabled)  AS module_count
		FROM tenants t WHERE t.id = ?`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// LEFT JOIN from `modules`, so the matrix has a row per module in the
	// catalogue whether or not this tenant has ever been given one. A screen
	// built from tenant_modules alone cannot offer a toggle for a module the
	// tenant has never held.
	var matrix []tenantModuleRow
	if err := g.WithContext(c.UserContext()).Raw(`
		SELECT m.code, m.name, m.description,
		       COALESCE(tm.enabled, false) AS enabled
		FROM modules m
		LEFT JOIN tenant_modules tm ON tm.module_code = m.code AND tm.tenant_id = ?
		WHERE m.is_available = true
		ORDER BY m.sort_order`, id).Scan(&matrix).Error; err != nil {
		return nil, err
	}

	detail := &tenantDetail{tenantListRow: rows[0], Modules: matrix}
	detail.EnabledCodes = []string{}
	for _, m := range matrix {
		if m.Enabled {
			detail.EnabledCodes = append(detail.EnabledCodes, m.Code)
		}
	}
	return detail, nil
}

type createTenantRequest struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Timezone string `json:"timezone"`
	// Modules is the initial entitlement set. Absent means all available
	// modules — a workspace created with nothing enabled looks broken to the
	// admin who signs into it first, and the create screen always sends this
	// explicitly anyway.
	Modules *[]string `json:"modules"`
	Admin   struct {
		Email    string `json:"email"`
		FullName string `json:"fullName"`
		Password string `json:"password"`
	} `json:"admin"`
}

// createTenant is POST /api/admin/tenants.
//
// This is the **only** operation where a superadmin writes a row scoped to a
// tenant, and it exists because of a bootstrap problem rather than a permission
// one: a brand-new workspace has no users, so nobody inside it can create the
// first one (§5.7). After this call, user management belongs entirely to the
// tenant.
//
// The order is §3.3's, and it is not interchangeable: the provider account is
// created first, then the database rows, and the provider account is deleted if
// the database rejects them. The reverse order would leave a `users` row whose
// firebase_uid names nothing, which is a user who cannot ever sign in; this
// order's failure leaves nothing at all.
func (s *server) createTenant(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}

	var req createTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	req.Name = trimmed(req.Name)
	req.Slug = trimmed(req.Slug)
	req.Timezone = trimmed(req.Timezone)
	req.Admin.Email = normalizeEmail(req.Admin.Email)
	req.Admin.FullName = trimmed(req.Admin.FullName)

	if req.Timezone == "" {
		req.Timezone = "Asia/Jakarta" // the tenants.timezone column default
	}
	switch {
	case req.Name == "":
		return malformed(c, "name is required.")
	case !slugPattern.MatchString(req.Slug):
		return malformed(c, "slug must be lowercase letters, digits, and single hyphens.")
	case req.Admin.FullName == "":
		return malformed(c, "admin.fullName is required.")
	}
	if err := validateTimezone(req.Timezone); err != nil {
		return malformed(c, "%s", err)
	}
	if err := validateEmail(req.Admin.Email); err != nil {
		return malformed(c, "%s", err)
	}
	if err := validatePassword(req.Admin.Password); err != nil {
		return malformed(c, "%s", err)
	}

	available, err := s.availableModules(c, g)
	if err != nil {
		return err
	}
	modules, err := resolveModules(available, req.Modules)
	if err != nil {
		return malformed(c, "%s.", err)
	}

	// Cheap pre-checks. The unique indexes below are the real guard — this is a
	// race, and losing it is handled — but they spare the common mistake a
	// provider account that has to be created and then deleted again.
	if taken, err := s.slugTaken(c, g, req.Slug); err != nil {
		return err
	} else if taken {
		return httpx.Fail(c, fiber.StatusConflict, "in_use",
			fmt.Sprintf("The slug %q is already taken.", req.Slug))
	}
	if taken, err := s.emailTaken(c, g, req.Admin.Email); err != nil {
		return err
	} else if taken {
		return httpx.Fail(c, fiber.StatusConflict, "in_use",
			fmt.Sprintf("%s already has an account.", req.Admin.Email))
	}

	uid, err := s.users.CreateUser(c.UserContext(),
		req.Admin.Email, req.Admin.Password, req.Admin.FullName)
	if err != nil {
		if errors.Is(err, auth.ErrEmailExists) {
			return httpx.Fail(c, fiber.StatusConflict, "in_use",
				fmt.Sprintf("%s already has an account.", req.Admin.Email))
		}
		return err
	}

	tenantID, adminID := uuid.New(), uuid.New()
	txErr := g.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO tenants (id, name, slug, timezone)
			VALUES (?, ?, ?, ?)`, tenantID, req.Name, req.Slug, req.Timezone).Error; err != nil {
			return err
		}

		// A row per available module, enabled or not, so the entitlement matrix
		// is complete from the start and PUT .../modules/:code is an update
		// rather than a first insert.
		for _, m := range modules {
			if err := tx.Exec(`
				INSERT INTO tenant_modules (tenant_id, module_code, enabled)
				VALUES (?, ?, ?)`, tenantID, m.code, m.enabled).Error; err != nil {
				return err
			}
		}

		// Through the SECURITY DEFINER function of §4.2.1: erp_admin has no
		// grant on `accounts` and must not be given one — that is the surface
		// A11 exists to keep closed.
		if err := tx.Exec(`SELECT seed_tenant_accounts(?)`, tenantID).Error; err != nil {
			return err
		}

		// The first admin. No user_module_roles rows: a tenant admin holds
		// `admin` implicitly in every entitled module, and seeding rows would
		// make demotion restore levels they never actually chose (§5.4).
		return tx.Exec(`
			INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role)
			VALUES (?, ?, ?, ?, ?, 'admin')`,
			adminID, tenantID, uid, req.Admin.Email, req.Admin.FullName).Error
	})

	if txErr != nil {
		// §3.3 step 4. An orphaned provider account authenticates successfully
		// and then resolves to no `users` row — a state the middleware treats
		// as 401 rather than a crash, but still an account nobody can use and
		// whose address is now unavailable to the retry.
		if delErr := s.users.DeleteUser(c.UserContext(), uid); delErr != nil {
			log.Printf("api: ORPHANED firebase account %s for %s — tenant insert failed (%v) "+
				"and the compensating delete also failed: %v", uid, req.Admin.Email, txErr, delErr)
		}
		if db.IsUniqueViolation(txErr) {
			return httpx.Fail(c, fiber.StatusConflict, "in_use",
				"That slug or email was taken while this workspace was being created.")
		}
		return txErr
	}

	detail, err := s.tenantDetail(c, g, tenantID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"tenant": detail,
		"admin": fiber.Map{
			"id":       adminID,
			"email":    req.Admin.Email,
			"fullName": req.Admin.FullName,
		},
	})
}

// moduleToggle is one row destined for tenant_modules.
type moduleToggle struct {
	code    string
	enabled bool
}

// availableModules is the catalogue's enabled-for-sale codes, in display order.
func (s *server) availableModules(c *fiber.Ctx, g *gorm.DB) ([]string, error) {
	var available []string
	err := g.WithContext(c.UserContext()).Raw(`
		SELECT code FROM modules WHERE is_available = true ORDER BY sort_order`).
		Scan(&available).Error
	return available, err
}

// resolveModules turns the requested code list into one toggle per available
// module. A nil requested list means "everything available".
//
// Pure, and it returns a plain error rather than writing a response — see the
// comment on parseMatrix for why a helper here must never signal failure with
// httpx.Fail's return value. An unknown code is an error rather than a silent
// drop: that is the superadmin's typo, and quietly ignoring it would create a
// workspace missing a module they believe they granted.
func resolveModules(available []string, requested *[]string) ([]moduleToggle, error) {
	if requested == nil {
		toggles := make([]moduleToggle, 0, len(available))
		for _, code := range available {
			toggles = append(toggles, moduleToggle{code: code, enabled: true})
		}
		return toggles, nil
	}

	wanted := make(map[string]bool, len(*requested))
	for _, code := range *requested {
		wanted[code] = true
	}
	for code := range wanted {
		if !contains(available, code) {
			return nil, fmt.Errorf("%q is not a module", code)
		}
	}

	toggles := make([]moduleToggle, 0, len(available))
	for _, code := range available {
		toggles = append(toggles, moduleToggle{code: code, enabled: wanted[code]})
	}
	return toggles, nil
}

type patchTenantRequest struct {
	// Pointers, so "field absent" and "field set to empty" are different
	// requests. A PATCH that cannot tell them apart either blanks fields the
	// caller never mentioned or refuses to clear one deliberately.
	Name     *string `json:"name"`
	Timezone *string `json:"timezone"`
	Status   *string `json:"status"`
}

// patchTenant is PATCH /api/admin/tenants/:id — rename, retime, suspend,
// reactivate. There is deliberately no DELETE: tenants are suspended, never
// deleted (§6.9.4, I5).
func (s *server) patchTenant(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That workspace")
	}

	var req patchTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	sets := map[string]any{}
	if req.Name != nil {
		name := trimmed(*req.Name)
		if name == "" {
			return malformed(c, "name cannot be empty.")
		}
		sets["name"] = name
	}
	if req.Timezone != nil {
		tz := trimmed(*req.Timezone)
		if err := validateTimezone(tz); err != nil {
			return malformed(c, "%s", err)
		}
		sets["timezone"] = tz
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "suspended" {
			return malformed(c, "status must be active or suspended.")
		}
		sets["status"] = *req.Status
	}
	if len(sets) == 0 {
		return malformed(c, "Nothing to change.")
	}

	// GORM builds the SET list from the map's keys, which are literals in this
	// file. updated_at is left to the tenants_touch_updated_at trigger.
	result := g.WithContext(c.UserContext()).Table("tenants").Where("id = ?", id).Updates(sets)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That workspace")
	}

	detail, err := s.tenantDetail(c, g, id)
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

// listModules is GET /api/admin/modules — the catalogue.
func (s *server) listModules(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}
	var rows []struct {
		Code        string `json:"code"`
		Name        string `json:"name"`
		Description string `json:"description"`
		SortOrder   int    `json:"sortOrder"`
	}
	if err := g.WithContext(c.UserContext()).Raw(`
		SELECT code, name, description, sort_order
		FROM modules WHERE is_available = true ORDER BY sort_order`).
		Scan(&rows).Error; err != nil {
		return err
	}
	return c.JSON(rows)
}

// listTenantModules is GET /api/admin/tenants/:id/modules.
func (s *server) listTenantModules(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That workspace")
	}
	detail, err := s.tenantDetail(c, g, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That workspace")
	}
	return c.JSON(detail.Modules)
}

// setTenantModule is PUT /api/admin/tenants/:id/modules/:code.
//
// It takes effect on the affected tenant's very next request, with no restart
// and no cache to invalidate, because entitlement is read from the database
// during identity resolution on every request (I9, B5). That is the entire
// reason there is no entitlement cache to be clever about.
func (s *server) setTenantModule(c *fiber.Ctx) error {
	g, err := s.pools.Admin()
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That workspace")
	}
	code := c.Params("code")

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}
	if req.Enabled == nil {
		return malformed(c, "enabled is required.")
	}

	// Checked here rather than left to the foreign keys, because both would
	// surface as the same 23503 and the console needs to know which of the two
	// names in the URL was wrong.
	if exists, err := s.tenantExists(c, g, id); err != nil {
		return err
	} else if !exists {
		return notFound(c, "That workspace")
	}
	if exists, err := s.moduleExists(c, g, code); err != nil {
		return err
	} else if !exists {
		return notFound(c, "That module")
	}

	// updated_at is the trigger's job on the conflict path.
	if err := g.WithContext(c.UserContext()).Exec(`
		INSERT INTO tenant_modules (tenant_id, module_code, enabled)
		VALUES (?, ?, ?)
		ON CONFLICT (tenant_id, module_code)
		DO UPDATE SET enabled = EXCLUDED.enabled`, id, code, *req.Enabled).Error; err != nil {
		return err
	}

	detail, err := s.tenantDetail(c, g, id)
	if err != nil {
		return err
	}
	return c.JSON(detail.Modules)
}

// --------------------------------------------------------------------------
// Small reads, shared by the handlers above.
// --------------------------------------------------------------------------

func (s *server) slugTaken(c *fiber.Ctx, g *gorm.DB, slug string) (bool, error) {
	return s.exists(c, g, `SELECT 1 FROM tenants WHERE slug = ?`, slug)
}

// emailTaken looks across every tenant, not within one: `users.email` is
// globally unique because Firebase Auth has one user pool per project, so the
// same address cannot belong to two workspaces (§3.5.1).
func (s *server) emailTaken(c *fiber.Ctx, g *gorm.DB, email string) (bool, error) {
	return s.exists(c, g, `SELECT 1 FROM users WHERE email = ?`, email)
}

func (s *server) tenantExists(c *fiber.Ctx, g *gorm.DB, id uuid.UUID) (bool, error) {
	return s.exists(c, g, `SELECT 1 FROM tenants WHERE id = ?`, id)
}

func (s *server) moduleExists(c *fiber.Ctx, g *gorm.DB, code string) (bool, error) {
	return s.exists(c, g, `SELECT 1 FROM modules WHERE code = ? AND is_available = true`, code)
}

func (s *server) exists(c *fiber.Ctx, g *gorm.DB, query string, args ...any) (bool, error) {
	var found []int
	if err := g.WithContext(c.UserContext()).Raw(query+` LIMIT 1`, args...).Scan(&found).Error; err != nil {
		return false, err
	}
	return len(found) > 0, nil
}
