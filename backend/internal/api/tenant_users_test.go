package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// userDetailBody is the shape /api/tenant/users/:id returns.
type userDetailBody struct {
	ID             string            `json:"id"`
	Email          string            `json:"email"`
	FullName       string            `json:"fullName"`
	TenantRole     string            `json:"tenantRole"`
	IsActive       bool              `json:"isActive"`
	ModuleRoles    map[string]string `json:"moduleRoles"`
	EffectiveRoles map[string]string `json:"effectiveRoles"`
	Modules        []struct {
		Code           string `json:"code"`
		RoleLevel      string `json:"roleLevel"`
		EffectiveLevel string `json:"effectiveLevel"`
	} `json:"modules"`
}

type userListBody struct {
	Data       []userDetailBody `json:"data"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	TotalItems int64            `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

func userPath(id fmt.Stringer) string { return "/api/tenant/users/" + id.String() }

// --------------------------------------------------------------------------
// Who may manage users at all.
// --------------------------------------------------------------------------

// The gate is the *tenant* role. A staff user holding `admin` in every module
// still administers no people: "who inside this company may do what" is not any
// one module's business (§5.7).
func TestOnlyTenantAdminsManageUsers(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Gatekeeping Ltd")
	admin := tenant.NewAdmin(t)

	// tenant.User is staff with `admin` in all three modules.
	staff := tenant.User

	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/tenant/users", staff.FirebaseUID), http.StatusForbidden, "forbidden")
	testsupport.AssertErrorCode(t,
		h.Patch(t, userPath(staff.ID), staff.FirebaseUID, map[string]any{"fullName": "Self Promoted"}),
		http.StatusForbidden, "forbidden")

	testsupport.AssertStatus(t, h.Get(t, "/api/tenant/users", admin.FirebaseUID), http.StatusOK)
}

// `users` carries no RLS — it cannot, because identity resolution reads it
// before tenant context exists (§6.8). So the tenant filter is application-side,
// and this is the test that it is actually there.
func TestUserManagementCannotCrossTenants(t *testing.T) {
	h := testsupport.NewHarness(t)
	mine := h.DB.NewTenant(t, "Mine Ltd")
	theirs := h.DB.NewTenant(t, "Theirs Ltd")

	admin := mine.NewAdmin(t)
	victim := theirs.NewUser(t, map[string]string{"procurement": "viewer"})

	// A real user ID, and it must be indistinguishable from a made-up one: the
	// difference would be a cross-tenant existence oracle.
	for _, tc := range []struct {
		name string
		resp *http.Response
	}{
		{"read", h.Get(t, userPath(victim.ID), admin.FirebaseUID)},
		{"rename", h.Patch(t, userPath(victim.ID), admin.FirebaseUID,
			map[string]any{"fullName": "Renamed By An Outsider"})},
		{"deactivate", h.Patch(t, userPath(victim.ID), admin.FirebaseUID,
			map[string]any{"isActive": false})},
		{"grant one module", h.Put(t, userPath(victim.ID)+"/modules/procurement",
			admin.FirebaseUID, map[string]any{"roleLevel": "admin"})},
		{"grant the matrix", h.Put(t, userPath(victim.ID)+"/modules",
			admin.FirebaseUID, map[string]any{"moduleRoles": map[string]string{"procurement": "admin"}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testsupport.AssertStatus(t, tc.resp, http.StatusNotFound)
		})
	}

	// Nothing was written to the other tenant's user.
	var got struct {
		FullName  string
		IsActive  bool
		RoleLevel string
	}
	if err := h.DB.Owner.Raw(`
		SELECT u.full_name, u.is_active,
		       (SELECT umr.role_level FROM user_module_roles umr
		         WHERE umr.user_id = u.id AND umr.module_code = 'procurement') AS role_level
		FROM users u WHERE u.id = ?`, victim.ID).Scan(&got).Error; err != nil {
		t.Fatalf("re-read victim: %v", err)
	}
	if got.FullName == "Renamed By An Outsider" || !got.IsActive || got.RoleLevel != "viewer" {
		t.Fatalf("another tenant's admin changed %+v", got)
	}

	// And the list shows only the caller's own workspace.
	body := testsupport.Decode[userListBody](t, h.Get(t, "/api/tenant/users", admin.FirebaseUID))
	for _, row := range body.Data {
		if row.ID == victim.ID.String() {
			t.Fatal("the user list leaked a user from another tenant")
		}
	}
}

// --------------------------------------------------------------------------
// B9 — the last-admin rule (§5.4).
// --------------------------------------------------------------------------

// A tenant must always have at least one active admin, or it becomes
// unmanageable with no in-app recovery path.
func TestB9LastAdminCannotBeDemotedOrDeactivated(t *testing.T) {
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"demote to staff", map[string]any{"tenantRole": "staff"}},
		{"deactivate", map[string]any{"isActive": false}},
		// Both at once is still one admin short.
		{"demote and deactivate", map[string]any{"tenantRole": "staff", "isActive": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testsupport.NewHarness(t)
			tenant := h.DB.NewTenant(t, "Sole Admin Ltd")
			admin := tenant.NewAdmin(t)
			// Staff users do not count: the rule is about *admins*.
			tenant.NewUser(t, map[string]string{"procurement": "admin"})

			testsupport.AssertErrorCode(t,
				h.Patch(t, userPath(admin.ID), admin.FirebaseUID, tc.body),
				http.StatusConflict, "last_admin")

			// The refusal has to be a refusal, not a partial write.
			var after struct {
				TenantRole string
				IsActive   bool
			}
			if err := h.DB.Owner.Raw(`
				SELECT tenant_role, is_active FROM users WHERE id = ?`, admin.ID).
				Scan(&after).Error; err != nil {
				t.Fatalf("re-read admin: %v", err)
			}
			if after.TenantRole != "admin" || !after.IsActive {
				t.Fatalf("the last admin is now %+v — the 409 did not roll back", after)
			}

			// And they can still administer, which is the point of the rule.
			testsupport.AssertStatus(t,
				h.Get(t, "/api/tenant/users", admin.FirebaseUID), http.StatusOK)
		})
	}
}

// There is no cap on admins, and no rule against demoting one while another
// remains. "Exactly one admin" would be a single point of failure (§5.4).
func TestB9DemotingIsAllowedWhileAnotherAdminRemains(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Two Admins Ltd")
	first := tenant.NewAdmin(t)
	second := tenant.NewAdmin(t)

	testsupport.AssertStatus(t,
		h.Patch(t, userPath(second.ID), first.FirebaseUID, map[string]any{"tenantRole": "staff"}),
		http.StatusOK)

	// And now the survivor is the last one, so the rule engages for them.
	testsupport.AssertErrorCode(t,
		h.Patch(t, userPath(first.ID), first.FirebaseUID, map[string]any{"tenantRole": "staff"}),
		http.StatusConflict, "last_admin")
}

// A deactivated admin does not count as an active one, so the *other* admin
// becomes the last — even though two admin rows exist.
func TestB9DeactivatedAdminsDoNotCount(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Half Staffed Ltd")
	active := tenant.NewAdmin(t)
	dormant := tenant.NewAdmin(t)
	h.DB.Deactivate(t, dormant.ID)

	testsupport.AssertErrorCode(t,
		h.Patch(t, userPath(active.ID), active.FirebaseUID, map[string]any{"isActive": false}),
		http.StatusConflict, "last_admin")
}

// §9.3: there is no DELETE /tenant/users/:id. Users are deactivated, never
// deleted (§6.9.4, I5) — and the route not existing is the enforcement, so a
// route added later for symmetry would fail this.
func TestNoDeleteRouteForUsersOrTenants(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "No Deletes Ltd")
	admin := tenant.NewAdmin(t)
	super := h.DB.NewSuperadmin(t)

	for _, tc := range []struct{ path, token string }{
		{userPath(admin.ID), admin.FirebaseUID},
		{"/api/admin/tenants/" + tenant.ID.String(), super.FirebaseUID},
	} {
		resp := h.Request(t, http.MethodDelete, tc.path, tc.token, nil)
		if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE %s → %d; there must be no delete route", tc.path, resp.StatusCode)
		}
	}
}

// --------------------------------------------------------------------------
// B10 — the rule holds under concurrency, which is why it is FOR UPDATE.
// --------------------------------------------------------------------------

// Two admins, two simultaneous requests, each demoting the other. Without the
// row lock both read "there are two admins", both proceed, and the tenant is
// left with none — the classic check-then-act race, and the reason §5.4 says the
// count must be taken under SELECT … FOR UPDATE in the same transaction as the
// write.
func TestB10ConcurrentDemotionsOfTheLastTwoAdmins(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Race Ltd")
	first := tenant.NewAdmin(t)
	second := tenant.NewAdmin(t)

	type outcome struct {
		status int
		code   string
		err    error
	}
	results := make(chan outcome, 2)

	// Each request demotes a *different* admin, so neither is a duplicate of the
	// other and both would succeed if the lock were missing.
	for _, target := range []*testsupport.UserFixture{first, second} {
		// Deliberately not using the harness helpers: they call t.Fatalf and
		// t.Cleanup, neither of which is safe from a non-test goroutine.
		go func(targetID, actorUID string) {
			body := strings.NewReader(`{"tenantRole":"staff"}`)
			req := httptest.NewRequest(http.MethodPatch, "/api/tenant/users/"+targetID, body)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+actorUID)

			resp, err := h.App.Test(req, -1)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

			var envelope testsupport.ErrorBody
			_ = json.NewDecoder(resp.Body).Decode(&envelope)
			results <- outcome{status: resp.StatusCode, code: envelope.Code}
		}(target.ID.String(), target.FirebaseUID)
	}

	var succeeded, refused int
	for range 2 {
		got := <-results
		switch {
		case got.err != nil:
			t.Fatalf("request failed: %v", got.err)
		case got.status == http.StatusOK:
			succeeded++
		case got.status == http.StatusConflict && got.code == "last_admin":
			refused++
		default:
			t.Fatalf("unexpected outcome: %d %q", got.status, got.code)
		}
	}

	if succeeded != 1 || refused != 1 {
		t.Fatalf("%d succeeded and %d were refused; want exactly one of each",
			succeeded, refused)
	}

	// The invariant, checked against the database rather than the responses:
	// one active admin survives.
	var remaining int
	if err := h.DB.Owner.Raw(`
		SELECT count(*) FROM users
		WHERE tenant_id = ? AND tenant_role = 'admin' AND is_active = true`, tenant.ID).
		Scan(&remaining).Error; err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("%d active admins left; the tenant must never reach zero", remaining)
	}
}

// --------------------------------------------------------------------------
// Provisioning (§3.3).
// --------------------------------------------------------------------------

func TestCreateUserProvisionsProviderAccountThenRow(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Hiring Ltd")
	admin := tenant.NewAdmin(t)

	resp := h.Post(t, "/api/tenant/users", admin.FirebaseUID, map[string]any{
		"email":    "Budi@Example.Test", // mixed case on purpose
		"fullName": "Budi Santoso",
		"password": "correct horse battery",
		"moduleRoles": map[string]string{
			"procurement": "approver",
			"inventory":   "viewer",
			"finance":     "none", // must not be stored (§5.3)
		},
	})
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	created := testsupport.Decode[userDetailBody](t, resp)

	if created.Email != "budi@example.test" {
		t.Errorf("email = %q, want it normalised to lower case", created.Email)
	}
	if created.TenantRole != "staff" {
		t.Errorf("tenantRole = %q, want staff by default", created.TenantRole)
	}

	account, ok := h.Users.LastCreated()
	if !ok {
		t.Fatal("no provider account was created")
	}
	if account.Email != "budi@example.test" || account.DisplayName != "Budi Santoso" {
		t.Errorf("provider account = %+v", account)
	}

	// The row carries the UID the provider returned, which is the whole point of
	// creating the account first.
	var storedUID string
	if err := h.DB.Owner.Raw(`SELECT firebase_uid FROM users WHERE id = ?`, created.ID).
		Row().Scan(&storedUID); err != nil {
		t.Fatalf("read firebase_uid: %v", err)
	}
	if storedUID != account.UID {
		t.Errorf("users.firebase_uid = %q, want the provider's %q", storedUID, account.UID)
	}

	if created.ModuleRoles["procurement"] != "approver" ||
		created.ModuleRoles["inventory"] != "viewer" {
		t.Errorf("moduleRoles = %v", created.ModuleRoles)
	}
	if level, present := created.ModuleRoles["finance"]; present {
		t.Errorf("finance stored as %q; `none` is the absence of a row (§5.3)", level)
	}

	// And the new hire can sign in and reach exactly what they were given.
	testsupport.AssertStatus(t,
		h.Get(t, testsupport.ProbePath("procurement", identity.RoleApprover), account.UID),
		http.StatusOK)
}

// §3.3 step 4. An orphaned provider account authenticates successfully and then
// resolves to no `users` row — reachable, and a real bug rather than a cosmetic
// one. The compensating delete is invisible in the response, so this is the only
// place it can be checked.
func TestCreateUserDeletesTheProviderAccountWhenTheInsertFails(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Rollback Ltd")
	other := h.DB.NewTenant(t, "Squatter Ltd")
	admin := tenant.NewAdmin(t)

	// Park the UID the provider is about to return on a row in a *different*
	// tenant, so the users insert violates users_firebase_uid_key. That is the
	// realistic shape of this failure: the provider succeeded and the database
	// refused.
	squatterUID := "squatter-" + other.ID.String()
	if err := h.DB.Owner.Exec(`
		INSERT INTO users (tenant_id, firebase_uid, email, full_name, tenant_role)
		VALUES (?, ?, ?, 'Squatter', 'staff')`,
		other.ID, squatterUID, "squatter-"+other.ID.String()+"@example.test").Error; err != nil {
		t.Fatalf("seed squatter: %v", err)
	}
	h.Users.ForceUID = squatterUID

	resp := h.Post(t, "/api/tenant/users", admin.FirebaseUID, map[string]any{
		"email":    "doomed@example.test",
		"fullName": "Doomed Hire",
		"password": "correct horse battery",
	})
	if resp.StatusCode < 400 {
		t.Fatalf("status = %d, want a failure", resp.StatusCode)
	}

	if !h.Users.WasDeleted(squatterUID) {
		t.Fatalf("the provider account %s was left behind — an orphan that can "+
			"authenticate and resolve to nobody (§3.3 step 4)", squatterUID)
	}

	// And no half-written row survived.
	var count int
	if err := h.DB.Owner.Raw(`SELECT count(*) FROM users WHERE email = 'doomed@example.test'`).
		Scan(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("%d rows for the failed hire; the transaction did not roll back", count)
	}
}

// A tenant admin who could mint a superadmin would have escalated out of their
// own workspace entirely (§5.7).
func TestTenantAdminCannotCreateOrPromoteASuperadmin(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "No Escalation Ltd")
	admin := tenant.NewAdmin(t)

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/tenant/users", admin.FirebaseUID, map[string]any{
			"email":      "climber@example.test",
			"fullName":   "Social Climber",
			"password":   "correct horse battery",
			"tenantRole": "superadmin",
		}), http.StatusBadRequest, "malformed")

	if len(h.Users.Created) != 0 {
		t.Error("a provider account was created for a request that was refused")
	}

	testsupport.AssertErrorCode(t,
		h.Patch(t, userPath(tenant.User.ID), admin.FirebaseUID,
			map[string]any{"tenantRole": "superadmin"}),
		http.StatusBadRequest, "malformed")
}

func TestCreateUserRejectsDuplicateEmailAndWeakPassword(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Validation Ltd")
	admin := tenant.NewAdmin(t)

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/tenant/users", admin.FirebaseUID, map[string]any{
			"email":    tenant.User.Email, // already in this tenant
			"fullName": "Impostor",
			"password": "correct horse battery",
		}), http.StatusConflict, "in_use")

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/tenant/users", admin.FirebaseUID, map[string]any{
			"email":    "short@example.test",
			"fullName": "Short Password",
			"password": "abc",
		}), http.StatusBadRequest, "malformed")

	if len(h.Users.Created) != 0 {
		t.Error("a provider account was created for a request that was refused")
	}
}

// --------------------------------------------------------------------------
// The role matrix.
// --------------------------------------------------------------------------

// `none` deletes the row rather than storing itself (§5.3). The CHECK on
// role_level would refuse the string anyway, so a handler that tried to store it
// would 500 — which is what makes this worth asserting at the database.
func TestSettingALevelToNoneDeletesTheRow(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Revocation Ltd")
	admin := tenant.NewAdmin(t)
	user := tenant.NewUser(t, map[string]string{"procurement": "approver"})

	resp := h.Put(t, userPath(user.ID)+"/modules/procurement", admin.FirebaseUID,
		map[string]any{"roleLevel": "none"})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	var stored []string
	if err := h.DB.Owner.Raw(`
		SELECT role_level FROM user_module_roles
		WHERE user_id = ? AND module_code = 'procurement'`, user.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("read stored roles: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("row still present as %v; `none` must delete it, never be stored", stored)
	}

	// The detail response still describes the module, as "none" — the matrix
	// screen needs a dropdown for it (§10.6).
	detail := testsupport.Decode[userDetailBody](t, resp)
	found := false
	for _, m := range detail.Modules {
		if m.Code == "procurement" {
			found = true
			if m.RoleLevel != "none" || m.EffectiveLevel != "none" {
				t.Errorf("procurement = %+v, want none/none", m)
			}
		}
	}
	if !found {
		t.Error("the matrix omits procurement entirely")
	}

	// And the level is gone in fact, not just in the response.
	testsupport.AssertErrorCode(t,
		h.Get(t, testsupport.ProbePath("procurement", identity.RoleViewer), user.FirebaseUID),
		http.StatusForbidden, "insufficient_module_role")
}

// The bulk endpoint exists because the UI is a matrix: six dropdowns should be
// one request and one transaction, not six that can half-fail (§9.3).
func TestBulkMatrixIsOneRequestAndOneTransaction(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Matrix Ltd")
	admin := tenant.NewAdmin(t)
	user := tenant.NewUser(t, map[string]string{"finance": "admin"})

	resp := h.Put(t, userPath(user.ID)+"/modules", admin.FirebaseUID, map[string]any{
		"moduleRoles": map[string]string{
			"procurement": "approver",
			"inventory":   "user",
			"finance":     "none",
		},
	})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	detail := testsupport.Decode[userDetailBody](t, resp)
	want := map[string]string{"procurement": "approver", "inventory": "user"}
	if len(detail.ModuleRoles) != len(want) {
		t.Fatalf("moduleRoles = %v, want %v", detail.ModuleRoles, want)
	}
	for module, level := range want {
		if detail.ModuleRoles[module] != level {
			t.Errorf("moduleRoles[%s] = %q, want %q", module, detail.ModuleRoles[module], level)
		}
	}

	// One bad level rejects the whole request — the half-failure this endpoint
	// exists to prevent. Asserted by checking nothing moved.
	testsupport.AssertErrorCode(t,
		h.Put(t, userPath(user.ID)+"/modules", admin.FirebaseUID, map[string]any{
			"moduleRoles": map[string]string{"procurement": "viewer", "inventory": "managr"},
		}), http.StatusBadRequest, "malformed")

	after := testsupport.Decode[userDetailBody](t, h.Get(t, userPath(user.ID), admin.FirebaseUID))
	if after.ModuleRoles["procurement"] != "approver" {
		t.Errorf("procurement = %q after a rejected bulk write, want the previous approver",
			after.ModuleRoles["procurement"])
	}
}

// An admin's stored rows are accepted and kept but have no effect while they are
// an admin (§9.3). `effectiveRoles` is what the screens must render, or they
// misrepresent the model.
func TestEffectiveRolesShowTheImplicitAdmin(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Effective Ltd")
	admin := tenant.NewAdmin(t)

	detail := testsupport.Decode[userDetailBody](t, h.Get(t, userPath(admin.ID), admin.FirebaseUID))

	if len(detail.ModuleRoles) != 0 {
		t.Errorf("moduleRoles = %v, want empty — an admin holds no rows", detail.ModuleRoles)
	}
	for _, module := range []string{"procurement", "inventory", "finance"} {
		if detail.EffectiveRoles[module] != "admin" {
			t.Errorf("effectiveRoles[%s] = %q, want admin",
				module, detail.EffectiveRoles[module])
		}
	}
}
