// Tests for the §7 chain. They drive the *real* application, built by api.New,
// with only the Firebase interfaces faked (§12.4) — a test that assembles its
// own middleware stack proves nothing about the one that ships.
//
// What is under test is the middleware, never Firebase: that invalid tokens are
// rejected, that valid ones resolve to the right user, that deactivated users
// are refused, and that the tenant a request runs as comes from the database
// and from nowhere else.
package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/middleware"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// meBody is the /api/me response, as the frontend sees it.
type meBody struct {
	User struct {
		ID         string `json:"id"`
		Email      string `json:"email"`
		FullName   string `json:"fullName"`
		TenantRole string `json:"tenantRole"`
	} `json:"user"`
	Tenant *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Status   string `json:"status"`
		Timezone string `json:"timezone"`
	} `json:"tenant"`
	ModuleRoles map[string]string `json:"moduleRoles"`
}

// --------------------------------------------------------------------------
// Rejections. Every one of these is reachable in normal operation.
// --------------------------------------------------------------------------

func TestNoAuthorizationHeaderIs401(t *testing.T) {
	h := testsupport.NewHarness(t)
	testsupport.AssertStatus(t, h.Get(t, "/api/me", ""), http.StatusUnauthorized)
}

func TestMalformedAuthorizationHeaderIs401(t *testing.T) {
	h := testsupport.NewHarness(t)
	for _, header := range []string{"", "Bearer", "Bearer ", "Basic dXNlcjpwYXNz", "token abc"} {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := h.App.Test(req, -1)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization %q → %d, want 401", header, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestInvalidTokenIs401(t *testing.T) {
	// Only this one token verifies; anything else is rejected, exactly as an
	// expired or forged token would be.
	h := testsupport.NewHarnessWith(t, &testsupport.FakeVerifier{
		Valid: map[string]bool{"good-token": true},
	})
	resp := h.Get(t, "/api/me", "expired-token")
	testsupport.AssertErrorCode(t, resp, http.StatusUnauthorized, "unauthenticated")
}

// A verified token naming nobody is the orphaned-Firebase-account case (§3.3):
// a real state, reachable whenever provisioning half-failed. It must be 401.
func TestValidTokenWithNoUserRowIs401NotAnError(t *testing.T) {
	h := testsupport.NewHarness(t)
	resp := h.Get(t, "/api/me", "uid-that-exists-in-firebase-only")

	if resp.StatusCode >= 500 {
		t.Fatal("orphaned account produced a server error, not a 401")
	}
	testsupport.AssertErrorCode(t, resp, http.StatusUnauthorized, "unauthenticated")
}

func TestDeactivatedUserIs401(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Deactivation Ltd")
	user := tenant.User

	// Same token, before and after. Only the database row changes — which is
	// the point: authorization is a database fact, not a token fact (I9).
	testsupport.AssertStatus(t, h.Get(t, "/api/me", user.FirebaseUID), http.StatusOK)

	h.DB.Deactivate(t, user.ID)
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/me", user.FirebaseUID), http.StatusUnauthorized, "unauthenticated")
}

// A suspended tenant is 403, not 401: the caller is authenticated, and saying
// so is what stops them filing a bug about a login that "worked but is empty".
func TestSuspendedTenantIs403(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Suspended Ltd")
	tenant.Suspend(t)

	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/me", tenant.User.FirebaseUID), http.StatusForbidden, "tenant_suspended")
}

// --------------------------------------------------------------------------
// Resolution.
// --------------------------------------------------------------------------

func TestMeReturnsTenantAndModuleRoles(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Nusantara Trading")
	user := tenant.NewUser(t, map[string]string{
		"procurement": "approver",
		"inventory":   "viewer",
		// finance omitted: the level `none` is the absence of a row (§5.3).
	})

	resp := h.Get(t, "/api/me", user.FirebaseUID)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	body := testsupport.Decode[meBody](t, resp)

	if body.User.ID != user.ID.String() {
		t.Errorf("user.id = %q, want %q", body.User.ID, user.ID)
	}
	if body.User.Email != user.Email {
		t.Errorf("user.email = %q, want %q", body.User.Email, user.Email)
	}
	if body.User.TenantRole != "staff" {
		t.Errorf("user.tenantRole = %q, want staff", body.User.TenantRole)
	}
	if body.Tenant == nil {
		t.Fatal("tenant is null for a staff user")
	}
	if body.Tenant.ID != tenant.ID.String() {
		t.Errorf("tenant.id = %q, want %q", body.Tenant.ID, tenant.ID)
	}
	if body.Tenant.Name != "Nusantara Trading" {
		t.Errorf("tenant.name = %q, want Nusantara Trading", body.Tenant.Name)
	}
	if body.Tenant.Timezone != "Asia/Jakarta" {
		t.Errorf("tenant.timezone = %q, want Asia/Jakarta", body.Tenant.Timezone)
	}

	want := map[string]string{"procurement": "approver", "inventory": "viewer"}
	if len(body.ModuleRoles) != len(want) {
		t.Fatalf("moduleRoles = %v, want %v", body.ModuleRoles, want)
	}
	for module, level := range want {
		if body.ModuleRoles[module] != level {
			t.Errorf("moduleRoles[%s] = %q, want %q", module, body.ModuleRoles[module], level)
		}
	}
	if _, present := body.ModuleRoles["finance"]; present {
		t.Error("moduleRoles carries a module the user holds no row in")
	}
}

// The nav is driven by this map (§10.1, FE1), so a module the tenant lost its
// entitlement to must leave it — holding a role level in a module the tenant
// does not have is not access to it.
func TestMeOmitsModulesTheTenantIsNotEntitledTo(t *testing.T) {
	h := testsupport.NewHarness(t)
	tenant := h.DB.NewTenant(t, "Two Modules Ltd")
	tenant.DisableModule(t, "finance")

	body := testsupport.Decode[meBody](t, h.Get(t, "/api/me", tenant.User.FirebaseUID))
	if _, present := body.ModuleRoles["finance"]; present {
		t.Error("moduleRoles includes finance, which the tenant is not entitled to")
	}
	if body.ModuleRoles["procurement"] != "admin" {
		t.Errorf("moduleRoles[procurement] = %q, want admin", body.ModuleRoles["procurement"])
	}
}

func TestMeForSuperadminHasNoTenantAndNoModules(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	resp := h.Get(t, "/api/me", super.FirebaseUID)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	body := testsupport.Decode[meBody](t, resp)

	if body.Tenant != nil {
		t.Errorf("tenant = %+v, want null for a superadmin", body.Tenant)
	}
	if body.User.TenantRole != "superadmin" {
		t.Errorf("tenantRole = %q, want superadmin", body.User.TenantRole)
	}
	if len(body.ModuleRoles) != 0 {
		t.Errorf("moduleRoles = %v, want empty", body.ModuleRoles)
	}
}

// --------------------------------------------------------------------------
// The tenant comes from the database. Nothing a client sends can change it.
// --------------------------------------------------------------------------

// The Verifier interface returns a UID and nothing else, so a claim cannot
// physically reach an authorization decision — this asserts that the guarantee
// holds end to end, by sending one and watching it be ignored.
func TestTenantIgnoresClaimsHeadersAndParameters(t *testing.T) {
	h := testsupport.NewHarness(t)
	mine := h.DB.NewTenant(t, "Mine Ltd")
	theirs := h.DB.NewTenant(t, "Theirs Ltd")

	// Every channel a caller controls, all pointing at someone else's tenant:
	// a claim inside the token, a header, and a query parameter.
	token := fmt.Sprintf("%s;tenant=%s", mine.User.FirebaseUID, theirs.ID)
	path := fmt.Sprintf("/api/me?tenantId=%s&tenant_id=%s", theirs.ID, theirs.ID)

	headers := [][2]string{
		{"X-Tenant-Id", theirs.ID.String()},
		{"X-Tenant", theirs.Slug},
	}

	// What ResolveIdentity reports...
	resp := h.Get(t, path, token, headers...)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	body := testsupport.Decode[meBody](t, resp)

	if body.Tenant == nil {
		t.Fatal("tenant is null")
	}
	if body.Tenant.ID != mine.ID.String() {
		t.Fatalf("tenant.id = %q — resolved from the request, not the database; "+
			"want %q from users.tenant_id", body.Tenant.ID, mine.ID)
	}
	if body.User.ID != mine.User.ID.String() {
		t.Errorf("user.id = %q, want %q", body.User.ID, mine.User.ID)
	}

	// ...and, separately, what TenantTx actually sets app.current_tenant to.
	// /api/me never enters the transaction — it renders platform data — so
	// asserting on it alone leaves a TenantTx that trusts a request parameter
	// completely undetected. Only a tenant-scoped route can catch that.
	wantCode := warehouseCode(t, mine)
	denyCode := warehouseCode(t, theirs)

	resp = h.Get(t, testsupport.TenantTxPath+"?tenantId="+theirs.ID.String(), token, headers...)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	codes := testsupport.Decode[[]string](t, resp)

	if len(codes) != 1 || codes[0] != wantCode {
		t.Fatalf("warehouses = %v, want exactly [%s] — the transaction was scoped "+
			"to a tenant the caller supplied, not to users.tenant_id", codes, wantCode)
	}
	for _, code := range codes {
		if code == denyCode {
			t.Fatalf("saw the other tenant's warehouse %q", code)
		}
	}
}

// warehouseCode reads the fixture's one warehouse code directly, as the tenant.
func warehouseCode(t *testing.T, f *testsupport.TenantFixture) string {
	t.Helper()
	var code string
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT code FROM warehouses`).Row().Scan(&code)
	})
	return code
}

// TenantTx must open the transaction for the tenant the identity names — Group
// A's isolation guarantee, now reached through the HTTP chain rather than
// through db.WithTenant directly. Two tenants, because a single-tenant test
// cannot detect an isolation failure (§12.2).
func TestTenantTxScopesQueriesToTheCallersTenant(t *testing.T) {
	h := testsupport.NewHarness(t)
	a := h.DB.NewTenant(t, "Tenant A")
	b := h.DB.NewTenant(t, "Tenant B")

	wantA, wantB := warehouseCode(t, a), warehouseCode(t, b)

	for _, tc := range []struct {
		name string
		user *testsupport.UserFixture
		want string
		deny string
	}{
		{"tenant A's user", a.User, wantA, wantB},
		{"tenant B's user", b.User, wantB, wantA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.Get(t, testsupport.TenantTxPath, tc.user.FirebaseUID)
			testsupport.AssertStatus(t, resp, http.StatusOK)
			codes := testsupport.Decode[[]string](t, resp)

			if len(codes) != 1 || codes[0] != tc.want {
				t.Fatalf("warehouses = %v, want exactly [%s]", codes, tc.want)
			}
			for _, code := range codes {
				if code == tc.deny {
					t.Fatalf("saw the other tenant's warehouse %q", code)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// Chain wiring.
// --------------------------------------------------------------------------

// Health is liveness. If it ever needs a token, every deployment that restarts
// on a failed probe restarts forever.
func TestHealthNeedsNoToken(t *testing.T) {
	h := testsupport.NewHarness(t)
	testsupport.AssertStatus(t, h.Get(t, "/api/health", ""), http.StatusOK)
}

func TestRequestIDIsEchoedAndGeneratedWhenAbsent(t *testing.T) {
	h := testsupport.NewHarness(t)

	resp := h.Get(t, "/api/health", "")
	if resp.Header.Get(middleware.HeaderRequestID) == "" {
		t.Error("no request ID generated")
	}

	resp = h.Get(t, "/api/health", "", [2]string{middleware.HeaderRequestID, "caller-supplied-id"})
	if got := resp.Header.Get(middleware.HeaderRequestID); got != "caller-supplied-id" {
		t.Errorf("request ID = %q, want the caller's", got)
	}
}
