// Tenant user management — the tenant plane of §5.7. It answers "who inside
// this company may do what?" and has no say in which modules the company has.
//
// ONE THING TO KNOW BEFORE EDITING THIS FILE.
//
// `users` and `user_module_roles` are platform tables and carry NO row-level
// security (§6.8). They must not: identity resolution reads `users` before any
// tenant context exists. So RLS is not protecting anything here, and every
// single query below carries an explicit `tenant_id = <the caller's>` — for
// child rows, via a join back to `users`. A query in this file without that
// filter is a cross-tenant read or write that the database will happily perform.
//
// That is why the ID-not-in-this-tenant tests exist, and why every miss is a
// 404 rather than a 403: an admin probing another workspace's user ID must not
// be able to tell an ID that exists elsewhere from one that never existed.
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
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

var userSortable = map[string]string{
	"fullName":   "u.full_name",
	"email":      "u.email",
	"tenantRole": "u.tenant_role",
	"isActive":   "u.is_active",
	"createdAt":  "u.created_at",
}

type userRow struct {
	ID         uuid.UUID `json:"id"`
	Email      string    `json:"email"`
	FullName   string    `json:"fullName"`
	TenantRole string    `json:"tenantRole"`
	IsActive   bool      `json:"isActive"`
	CreatedAt  time.Time `json:"createdAt"`

	// ModuleRoles is what is *stored* — the rows in user_module_roles. For an
	// admin it is usually empty, and may hold levels from a previous staff
	// period that are deliberately kept so demotion restores them (§5.4).
	ModuleRoles map[string]string `json:"moduleRoles" gorm:"-"`
	// EffectiveRoles is what LevelFor actually resolves, entitlement ceiling
	// and implicit-admin rule applied. The two differ for every admin, and the
	// screens must show this one or they misrepresent the model.
	EffectiveRoles map[string]string `json:"effectiveRoles" gorm:"-"`
}

type userModuleRow struct {
	Code string `json:"code"`
	Name string `json:"name"`
	// RoleLevel is the stored level, "none" when there is no row.
	RoleLevel string `json:"roleLevel"`
	// EffectiveLevel is what this user actually has in the module right now.
	EffectiveLevel string `json:"effectiveLevel"`
}

type userDetail struct {
	userRow
	Modules []userModuleRow `json:"modules"`
}

// effectiveRoles is what the target user actually holds in each entitled module.
//
// It builds a throwaway Identity and calls LevelFor rather than reimplementing
// the rules, because there must be exactly one implementation of §5.4. A second
// copy here — or, worse, in TypeScript — is a copy that will disagree with the
// enforcing one, and the disagreement will be in the direction of showing
// people controls that then 403.
//
// `enabled` is the *caller's* entitlement set, which is sound only because the
// target is in the caller's tenant; every path into this function has already
// established that.
func effectiveRoles(enabled map[string]bool, tenantRole string, stored map[string]string) map[string]string {
	view := &identity.Identity{
		TenantRole:     tenantRole,
		EnabledModules: enabled,
		ModuleRoles:    stored,
	}
	out := make(map[string]string, len(enabled))
	for module := range enabled {
		if level := view.LevelFor(module); level != identity.RoleNone {
			out[module] = level.String()
		}
	}
	return out
}

// listUsers is GET /api/tenant/users.
func (s *server) listUsers(c *fiber.Ctx) error {
	id, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, userSortable, "fullName")
	if err != nil {
		return malformed(c, "%s", err)
	}

	var total int64
	if err := tx.Raw(`
		SELECT count(*) FROM users u
		WHERE u.tenant_id = ? AND (u.full_name ILIKE ? OR u.email ILIKE ?)`,
		id.TenantID, params.Like(), params.Like()).Scan(&total).Error; err != nil {
		return err
	}

	var rows []userRow
	if err := tx.Raw(fmt.Sprintf(`
		SELECT u.id, u.email, u.full_name, u.tenant_role, u.is_active, u.created_at
		FROM users u
		WHERE u.tenant_id = ? AND (u.full_name ILIKE ? OR u.email ILIKE ?)
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("u.id")),
		id.TenantID, params.Like(), params.Like(), params.PageSize, params.Offset()).
		Scan(&rows).Error; err != nil {
		return err
	}

	stored, err := s.storedRoles(tx, id.TenantID, userIDs(rows))
	if err != nil {
		return err
	}
	for i := range rows {
		rows[i].ModuleRoles = stored[rows[i].ID]
		if rows[i].ModuleRoles == nil {
			rows[i].ModuleRoles = map[string]string{}
		}
		rows[i].EffectiveRoles = effectiveRoles(
			id.EnabledModules, rows[i].TenantRole, rows[i].ModuleRoles)
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// getUser is GET /api/tenant/users/:id.
func (s *server) getUser(c *fiber.Ctx) error {
	id, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	userID, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That user")
	}

	detail, err := s.userDetail(c, tx, id, userID)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That user")
	}
	return c.JSON(detail)
}

// userDetail reads one user of the caller's tenant with the full role matrix, or
// nil if there is no such user *in this tenant*.
func (s *server) userDetail(c *fiber.Ctx, tx *gorm.DB, caller *identity.Identity, userID uuid.UUID) (*userDetail, error) {
	var rows []userRow
	if err := tx.Raw(`
		SELECT u.id, u.email, u.full_name, u.tenant_role, u.is_active, u.created_at
		FROM users u
		WHERE u.id = ? AND u.tenant_id = ?`, userID, caller.TenantID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]

	stored, err := s.storedRoles(tx, caller.TenantID, []uuid.UUID{userID})
	if err != nil {
		return nil, err
	}
	row.ModuleRoles = stored[userID]
	if row.ModuleRoles == nil {
		row.ModuleRoles = map[string]string{}
	}
	row.EffectiveRoles = effectiveRoles(caller.EnabledModules, row.TenantRole, row.ModuleRoles)

	// A row per *entitled* module, including the ones this user has no level in
	// — the matrix screen needs a dropdown for each, and "none" is a selectable
	// value there even though it is never a stored one (§5.3).
	var catalogue []struct {
		Code string
		Name string
	}
	if err := tx.Raw(`
		SELECT m.code, m.name
		FROM tenant_modules tm
		JOIN modules m ON m.code = tm.module_code AND m.is_available = true
		WHERE tm.tenant_id = ? AND tm.enabled = true
		ORDER BY m.sort_order`, caller.TenantID).Scan(&catalogue).Error; err != nil {
		return nil, err
	}

	modules := make([]userModuleRow, 0, len(catalogue))
	for _, m := range catalogue {
		level := row.ModuleRoles[m.Code]
		if level == "" {
			level = identity.RoleNone.String()
		}
		effective := row.EffectiveRoles[m.Code]
		if effective == "" {
			effective = identity.RoleNone.String()
		}
		modules = append(modules, userModuleRow{
			Code: m.Code, Name: m.Name,
			RoleLevel: level, EffectiveLevel: effective,
		})
	}

	return &userDetail{userRow: row, Modules: modules}, nil
}

// storedRoles reads user_module_roles for a set of users, joined back through
// `users` so a user ID from another tenant returns nothing even if it is real.
func (s *server) storedRoles(tx *gorm.DB, tenantID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID]map[string]string, error) {
	out := map[uuid.UUID]map[string]string{}
	if len(userIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		UserID     uuid.UUID
		ModuleCode string
		RoleLevel  string
	}
	if err := tx.Raw(`
		SELECT umr.user_id, umr.module_code, umr.role_level
		FROM user_module_roles umr
		JOIN users u ON u.id = umr.user_id AND u.tenant_id = ?
		WHERE umr.user_id IN ?`, tenantID, userIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if out[r.UserID] == nil {
			out[r.UserID] = map[string]string{}
		}
		out[r.UserID][r.ModuleCode] = r.RoleLevel
	}
	return out, nil
}

func userIDs(rows []userRow) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	return ids
}

// --------------------------------------------------------------------------
// Writes.
// --------------------------------------------------------------------------

type createUserRequest struct {
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Password string `json:"password"`
	// TenantRole defaults to staff. `superadmin` is refused: that role belongs
	// to the platform plane, and a tenant admin who could mint one would have
	// escalated out of their own workspace (§5.7).
	TenantRole  string            `json:"tenantRole"`
	ModuleRoles map[string]string `json:"moduleRoles"`
}

// createUser is POST /api/tenant/users, following §3.3.
//
// The provider account is created first and deleted again if the database
// refuses the row. The order is not interchangeable: this way a failure leaves
// nothing behind, whereas writing the row first would leave a user who can
// never sign in.
//
// Everything checkable is checked *before* the provider call, so the only
// remaining insert failure is losing a uniqueness race — which is caught below
// and compensated. The residual window is a COMMIT that fails after a
// successful INSERT; there are no deferred constraints on these two tables, so
// that needs the connection to drop between the two, and it is logged loudly
// enough to find.
func (s *server) createUser(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}

	var req createUserRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}
	req.Email = normalizeEmail(req.Email)
	req.FullName = trimmed(req.FullName)
	if req.TenantRole == "" {
		req.TenantRole = identity.TenantStaff
	}

	if req.FullName == "" {
		return malformed(c, "fullName is required.")
	}
	if err := validateEmail(req.Email); err != nil {
		return malformed(c, "%s", err)
	}
	if err := validatePassword(req.Password); err != nil {
		return malformed(c, "%s", err)
	}
	if req.TenantRole != identity.TenantStaff && req.TenantRole != identity.TenantAdmin {
		return malformed(c, "tenantRole must be staff or admin.")
	}

	levels, err := parseMatrix(caller, req.ModuleRoles)
	if err != nil {
		return failMatrix(c, err)
	}

	// Globally unique, because Firebase Auth has one user pool per project: the
	// same address cannot belong to two workspaces (§3.5.1). Checked across all
	// tenants deliberately — but reported without confirming *where* it exists.
	var existing []int
	if err := tx.Raw(`SELECT 1 FROM users WHERE email = ? LIMIT 1`, req.Email).
		Scan(&existing).Error; err != nil {
		return err
	}
	if len(existing) > 0 {
		return httpx.Fail(c, fiber.StatusConflict, "in_use",
			fmt.Sprintf("%s already has an account.", req.Email))
	}

	uid, err := s.users.CreateUser(c.UserContext(), req.Email, req.Password, req.FullName)
	if err != nil {
		if errors.Is(err, auth.ErrEmailExists) {
			return httpx.Fail(c, fiber.StatusConflict, "in_use",
				fmt.Sprintf("%s already has an account.", req.Email))
		}
		return err
	}

	newID := uuid.New()
	insertErr := tx.Exec(`
		INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role)
		VALUES (?, ?, ?, ?, ?, ?)`,
		newID, caller.TenantID, uid, req.Email, req.FullName, req.TenantRole).Error

	if insertErr == nil {
		// Stored even for an admin, who resolves to `admin` implicitly and so
		// gains nothing from them now. §9.3: they take effect on demotion.
		for module, level := range levels {
			if level == identity.RoleNone {
				continue // `none` is the absence of a row (§5.3)
			}
			if err := tx.Exec(`
				INSERT INTO user_module_roles (user_id, module_code, role_level)
				VALUES (?, ?, ?)`, newID, module, level.String()).Error; err != nil {
				insertErr = err
				break
			}
		}
	}

	if insertErr != nil {
		// §3.3 step 4.
		if delErr := s.users.DeleteUser(c.UserContext(), uid); delErr != nil {
			log.Printf("api: ORPHANED firebase account %s for %s — the users insert failed (%v) "+
				"and the compensating delete also failed: %v", uid, req.Email, insertErr, delErr)
		}
		if db.IsUniqueViolation(insertErr) {
			return httpx.Fail(c, fiber.StatusConflict, "in_use",
				fmt.Sprintf("%s already has an account.", req.Email))
		}
		return insertErr
	}

	detail, err := s.userDetail(c, tx, caller, newID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(detail)
}

type patchUserRequest struct {
	FullName   *string `json:"fullName"`
	IsActive   *bool   `json:"isActive"`
	TenantRole *string `json:"tenantRole"`
}

// patchUser is PATCH /api/tenant/users/:id — rename, activate/deactivate, and
// promote/demote between admin and staff.
//
// It carries the last-admin rule (§5.4): a tenant must always have at least one
// active admin, or it becomes unmanageable with no in-app recovery path. The
// check is a `SELECT … FOR UPDATE` in the same transaction as the write, which
// is what makes it hold under concurrency rather than merely usually (B10).
func (s *server) patchUser(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	userID, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That user")
	}

	var req patchUserRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	sets := map[string]any{}
	if req.FullName != nil {
		name := trimmed(*req.FullName)
		if name == "" {
			return malformed(c, "fullName cannot be empty.")
		}
		sets["full_name"] = name
	}
	if req.TenantRole != nil {
		if *req.TenantRole != identity.TenantStaff && *req.TenantRole != identity.TenantAdmin {
			// Including `superadmin`, which is the escalation this refuses.
			return malformed(c, "tenantRole must be staff or admin.")
		}
		sets["tenant_role"] = *req.TenantRole
	}
	if req.IsActive != nil {
		sets["is_active"] = *req.IsActive
	}
	if len(sets) == 0 {
		return malformed(c, "Nothing to change.")
	}

	// Lock the tenant's active admins FIRST, and always in the same order.
	// Ordering is what keeps two concurrent demotions from deadlocking instead
	// of one of them losing cleanly.
	//
	// Under READ COMMITTED, a row this blocks on is re-checked against the WHERE
	// clause once the lock is released, so an admin another transaction has just
	// demoted drops out of this set rather than being counted. That is precisely
	// what makes the second of two concurrent demotions see one admin left.
	var activeAdmins []uuid.UUID
	if err := tx.Raw(`
		SELECT u.id FROM users u
		WHERE u.tenant_id = ? AND u.tenant_role = 'admin' AND u.is_active = true
		ORDER BY u.id
		FOR UPDATE`, caller.TenantID).Scan(&activeAdmins).Error; err != nil {
		return err
	}

	var target []struct {
		FirebaseUID string
		TenantRole  string
		IsActive    bool
	}
	if err := tx.Raw(`
		SELECT u.firebase_uid, u.tenant_role, u.is_active FROM users u
		WHERE u.id = ? AND u.tenant_id = ?
		FOR UPDATE`, userID, caller.TenantID).Scan(&target).Error; err != nil {
		return err
	}
	if len(target) == 0 {
		return notFound(c, "That user")
	}
	current := target[0]

	demoting := req.TenantRole != nil && *req.TenantRole == identity.TenantStaff &&
		current.TenantRole == identity.TenantAdmin
	deactivating := req.IsActive != nil && !*req.IsActive && current.IsActive

	if (demoting || deactivating) &&
		current.TenantRole == identity.TenantAdmin && current.IsActive &&
		len(activeAdmins) == 1 && activeAdmins[0] == userID {
		return httpx.FailWith(c, fiber.StatusConflict, "last_admin",
			"This workspace needs at least one active administrator. "+
				"Promote someone else first.",
			map[string]any{"userId": userID.String()})
	}

	result := tx.Table("users").
		Where("id = ? AND tenant_id = ?", userID, caller.TenantID).
		Updates(sets)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notFound(c, "That user")
	}

	// Mirror the flag onto the provider account, best effort and after the row
	// is written. The database is the authority on access — identity resolution
	// already turns a deactivated user away on their next request (I9) — so a
	// provider that is briefly unreachable must not fail the deactivation. It
	// is defence in depth, and a failure is a log line, not a 500.
	if req.IsActive != nil && *req.IsActive != current.IsActive {
		if err := s.users.SetDisabled(c.UserContext(), current.FirebaseUID, !*req.IsActive); err != nil {
			log.Printf("api: users.is_active=%t committed for %s but the provider account "+
				"was not updated: %v", *req.IsActive, current.FirebaseUID, err)
		}
	}

	detail, err := s.userDetail(c, tx, caller, userID)
	if err != nil {
		return err
	}
	if detail == nil {
		// Unreachable: the update above already 404s on RowsAffected == 0. Said
		// anyway because the alternative is `c.JSON(nil)`, which is a 200 with a
		// body of `null` — a success the frontend would try to render.
		return notFound(c, "That user")
	}
	return c.JSON(detail)
}

// setUserModule is PUT /api/tenant/users/:id/modules/:code.
func (s *server) setUserModule(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	userID, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That user")
	}

	var req struct {
		RoleLevel string `json:"roleLevel"`
	}
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	levels, err := parseMatrix(caller, map[string]string{c.Params("code"): req.RoleLevel})
	if err != nil {
		return failMatrix(c, err)
	}
	return s.applyMatrix(c, tx, caller, userID, levels)
}

// setUserModules is PUT /api/tenant/users/:id/modules — the bulk endpoint.
//
// It exists because the UI is a matrix (§10.6): saving six dropdowns should be
// one request and one transaction, not six that can half-fail and leave a user
// with three of the six levels the admin chose.
//
// Modules the payload does not name are left alone. The alternative — treating
// an absent module as `none` — makes a client that drops a field silently
// revoke access, and this endpoint's own screen always sends every module
// anyway, so nothing is gained by the destructive reading.
func (s *server) setUserModules(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	userID, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That user")
	}

	var req struct {
		ModuleRoles map[string]string `json:"moduleRoles"`
	}
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}
	if len(req.ModuleRoles) == 0 {
		return malformed(c, "moduleRoles is required.")
	}

	levels, err := parseMatrix(caller, req.ModuleRoles)
	if err != nil {
		return failMatrix(c, err)
	}
	return s.applyMatrix(c, tx, caller, userID, levels)
}

// notEntitledError says the payload named a module the caller's tenant does not
// have. A distinct type rather than a message, because it maps to a different
// status than the other validation failures here.
type notEntitledError struct{ module string }

func (e notEntitledError) Error() string {
	return fmt.Sprintf("the %s module is not enabled for this workspace", e.module)
}

// parseMatrix validates a {module: level} payload against the caller's own
// entitlements and the level vocabulary.
//
// It is pure: no database, and — importantly — no response writing. A helper
// here cannot signal failure by returning what httpx.Fail returned, because
// Fail deliberately returns nil so Fiber does not write a second body over the
// one it just wrote. A helper that tried would have its refusal silently
// overwritten by the success path's own JSON, keeping the status code and
// losing the reason.
//
// Entitlement is checked on the write path and not only on the read path: a
// tenant admin can never grant a level in a module their tenant has not bought,
// because there is nothing to allocate (§5.7).
func parseMatrix(caller *identity.Identity, raw map[string]string) (map[string]identity.RoleLevel, error) {
	levels := make(map[string]identity.RoleLevel, len(raw))
	for module, name := range raw {
		if !caller.TenantEntitled(module) {
			return nil, notEntitledError{module: module}
		}
		level, ok := identity.ParseRoleLevel(name)
		if !ok {
			return nil, fmt.Errorf(
				"%q is not a role level. Use none, viewer, user, approver, or admin", name)
		}
		levels[module] = level
	}
	return levels, nil
}

// failMatrix renders parseMatrix's error. The refusal reuses
// module_not_enabled so the console can say which of the two planes has to act.
func failMatrix(c *fiber.Ctx, err error) error {
	var notEntitled notEntitledError
	if errors.As(err, &notEntitled) {
		return httpx.FailWith(c, fiber.StatusForbidden, "module_not_enabled",
			fmt.Sprintf("This workspace does not have the %s module enabled.", notEntitled.module),
			map[string]any{"module": notEntitled.module})
	}
	return malformed(c, "%s.", err)
}

// applyMatrix writes the validated levels and returns the user's new detail.
func (s *server) applyMatrix(c *fiber.Ctx, tx *gorm.DB, caller *identity.Identity, userID uuid.UUID, levels map[string]identity.RoleLevel) error {
	// The target must be in the caller's tenant. `user_module_roles` has no
	// tenant_id of its own — it hangs off `users` — so without this check the
	// inserts below would happily grant roles inside another workspace.
	var found []int
	if err := tx.Raw(`SELECT 1 FROM users WHERE id = ? AND tenant_id = ? LIMIT 1`,
		userID, caller.TenantID).Scan(&found).Error; err != nil {
		return err
	}
	if len(found) == 0 {
		return notFound(c, "That user")
	}

	for module, level := range levels {
		if level == identity.RoleNone {
			// `none` DELETEs the row rather than storing itself (§5.3), and the
			// CHECK on role_level would refuse it anyway. The one DELETE in this
			// codebase, and the one grant erp_app holds for it: a present-tense
			// grant has no history worth keeping, unlike every document and
			// ledger row I5 protects.
			if err := tx.Exec(`
				DELETE FROM user_module_roles
				WHERE user_id = ? AND module_code = ?`, userID, module).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Exec(`
			INSERT INTO user_module_roles (user_id, module_code, role_level)
			VALUES (?, ?, ?)
			ON CONFLICT (user_id, module_code)
			DO UPDATE SET role_level = EXCLUDED.role_level`,
			userID, module, level.String()).Error; err != nil {
			return err
		}
	}

	detail, err := s.userDetail(c, tx, caller, userID)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That user")
	}
	return c.JSON(detail)
}
