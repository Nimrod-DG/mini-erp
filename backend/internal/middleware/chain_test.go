// Tests for the §7 chain. They drive the *real* application, built by api.New,
// with only the Firebase Verifier faked (§12.4) — a test that assembles its own
// middleware stack proves nothing about the one that ships.
//
// What is under test is the middleware, never Firebase: that invalid tokens are
// rejected, that valid ones resolve to the right user, that deactivated users
// are refused, and that the tenant a request runs as comes from the database
// and from nowhere else.
package middleware_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/api"
	"github.com/DGosal/mini-erp/backend/internal/db"
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

type errBody struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

// harness is the application under test plus the database behind it.
type harness struct {
	tdb *testsupport.TestDB
	app *fiber.App
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	tdb := testsupport.NewTestDB(t)
	pools := db.NewPools(tdb.AppURL, tdb.AdminURL)
	t.Cleanup(func() { _ = pools.Close() })

	app := api.New(api.Deps{
		Pools:       pools,
		Verifier:    &testsupport.FakeVerifier{},
		CORSOrigins: []string{"http://localhost:5173"},
		Quiet:       true,
	})
	h := &harness{tdb: tdb, app: app}
	h.addProbeRoute()
	return h
}

// addProbeRoute mounts a route that reads tenant data through the transaction
// TenantTx opened. /api/me alone cannot show that TenantTx works: it reads
// platform tables, which have no RLS, so it would pass with TenantTx deleted.
func (h *harness) addProbeRoute() {
	h.app.Get("/api/warehouses", func(c *fiber.Ctx) error {
		tx := middleware.TxFrom(c)
		if tx == nil {
			return fiber.NewError(fiber.StatusInternalServerError, "no tenant transaction")
		}
		var codes []string
		if err := tx.Raw(`SELECT code FROM warehouses ORDER BY code`).Scan(&codes).Error; err != nil {
			return err
		}
		if codes == nil {
			codes = []string{}
		}
		return c.JSON(codes)
	})
}

// get issues a request. An empty token sends no Authorization header at all.
func (h *harness) get(t *testing.T, path, token string, headers ...[2]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, kv := range headers {
		req.Header.Set(kv[0], kv[1])
	}
	resp, err := h.app.Test(req, -1)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var out T
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %q: %v", string(body), err)
	}
	return out
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, want, body)
	}
}

// --------------------------------------------------------------------------
// Rejections. Every one of these is reachable in normal operation.
// --------------------------------------------------------------------------

func TestNoAuthorizationHeaderIs401(t *testing.T) {
	h := newHarness(t)
	assertStatus(t, h.get(t, "/api/me", ""), http.StatusUnauthorized)
}

func TestMalformedAuthorizationHeaderIs401(t *testing.T) {
	h := newHarness(t)
	for _, header := range []string{"", "Bearer", "Bearer ", "Basic dXNlcjpwYXNz", "token abc"} {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := h.app.Test(req, -1)
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
	tdb := testsupport.NewTestDB(t)
	pools := db.NewPools(tdb.AppURL, tdb.AdminURL)
	t.Cleanup(func() { _ = pools.Close() })

	// Only this one token verifies; anything else is rejected, exactly as an
	// expired or forged token would be.
	app := api.New(api.Deps{
		Pools:    pools,
		Verifier: &testsupport.FakeVerifier{Valid: map[string]bool{"good-token": true}},
		Quiet:    true,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	assertStatus(t, resp, http.StatusUnauthorized)
	if got := decode[errBody](t, resp).Code; got != "unauthenticated" {
		t.Errorf("error code = %q, want unauthenticated", got)
	}
}

// A verified token naming nobody is the orphaned-Firebase-account case (§3.3):
// a real state, reachable whenever provisioning half-failed. It must be 401.
func TestValidTokenWithNoUserRowIs401NotAnError(t *testing.T) {
	h := newHarness(t)
	resp := h.get(t, "/api/me", "uid-that-exists-in-firebase-only")

	assertStatus(t, resp, http.StatusUnauthorized)
	if resp.StatusCode >= 500 {
		t.Fatal("orphaned account produced a server error, not a 401")
	}
	if got := decode[errBody](t, resp).Code; got != "unauthenticated" {
		t.Errorf("error code = %q, want unauthenticated", got)
	}
}

func TestDeactivatedUserIs401(t *testing.T) {
	h := newHarness(t)
	tenant := h.tdb.NewTenant(t, "Deactivation Ltd")
	user := tenant.User

	// Same token, before and after. Only the database row changes — which is
	// the point: authorization is a database fact, not a token fact (I9).
	assertStatus(t, h.get(t, "/api/me", user.FirebaseUID), http.StatusOK)

	h.tdb.Deactivate(t, user.ID)
	resp := h.get(t, "/api/me", user.FirebaseUID)

	assertStatus(t, resp, http.StatusUnauthorized)
	if got := decode[errBody](t, resp).Code; got != "unauthenticated" {
		t.Errorf("error code = %q, want unauthenticated", got)
	}
}

// A suspended tenant is 403, not 401: the caller is authenticated, and saying
// so is what stops them filing a bug about a login that "worked but is empty".
func TestSuspendedTenantIs403(t *testing.T) {
	h := newHarness(t)
	tenant := h.tdb.NewTenant(t, "Suspended Ltd")
	tenant.Suspend(t)

	resp := h.get(t, "/api/me", tenant.User.FirebaseUID)
	assertStatus(t, resp, http.StatusForbidden)
	if got := decode[errBody](t, resp).Code; got != "tenant_suspended" {
		t.Errorf("error code = %q, want tenant_suspended", got)
	}
}

// --------------------------------------------------------------------------
// Resolution.
// --------------------------------------------------------------------------

func TestMeReturnsTenantAndModuleRoles(t *testing.T) {
	h := newHarness(t)
	tenant := h.tdb.NewTenant(t, "Nusantara Trading")
	user := tenant.NewUser(t, map[string]string{
		"procurement": "approver",
		"inventory":   "viewer",
		// finance omitted: the level `none` is the absence of a row (§5.3).
	})

	resp := h.get(t, "/api/me", user.FirebaseUID)
	assertStatus(t, resp, http.StatusOK)
	body := decode[meBody](t, resp)

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
	h := newHarness(t)
	tenant := h.tdb.NewTenant(t, "Two Modules Ltd")
	tenant.DisableModule(t, "finance")

	body := decode[meBody](t, h.get(t, "/api/me", tenant.User.FirebaseUID))
	if _, present := body.ModuleRoles["finance"]; present {
		t.Error("moduleRoles includes finance, which the tenant is not entitled to")
	}
	if body.ModuleRoles["procurement"] != "admin" {
		t.Errorf("moduleRoles[procurement] = %q, want admin", body.ModuleRoles["procurement"])
	}
}

func TestMeForSuperadminHasNoTenantAndNoModules(t *testing.T) {
	h := newHarness(t)
	super := h.tdb.NewSuperadmin(t)

	resp := h.get(t, "/api/me", super.FirebaseUID)
	assertStatus(t, resp, http.StatusOK)
	body := decode[meBody](t, resp)

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
	h := newHarness(t)
	mine := h.tdb.NewTenant(t, "Mine Ltd")
	theirs := h.tdb.NewTenant(t, "Theirs Ltd")

	// Every channel a caller controls, all pointing at someone else's tenant:
	// a claim inside the token, a header, and a query parameter.
	token := fmt.Sprintf("%s;tenant=%s", mine.User.FirebaseUID, theirs.ID)
	path := fmt.Sprintf("/api/me?tenantId=%s&tenant_id=%s", theirs.ID, theirs.ID)

	headers := [][2]string{
		{"X-Tenant-Id", theirs.ID.String()},
		{"X-Tenant", theirs.Slug},
	}

	// What ResolveIdentity reports...
	resp := h.get(t, path, token, headers...)
	assertStatus(t, resp, http.StatusOK)
	body := decode[meBody](t, resp)

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

	resp = h.get(t, "/api/warehouses?tenantId="+theirs.ID.String(), token, headers...)
	assertStatus(t, resp, http.StatusOK)
	codes := decode[[]string](t, resp)

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
	h := newHarness(t)
	a := h.tdb.NewTenant(t, "Tenant A")
	b := h.tdb.NewTenant(t, "Tenant B")

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
			resp := h.get(t, "/api/warehouses", tc.user.FirebaseUID)
			assertStatus(t, resp, http.StatusOK)
			codes := decode[[]string](t, resp)

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
	h := newHarness(t)
	resp := h.get(t, "/api/health", "")
	assertStatus(t, resp, http.StatusOK)
}

func TestRequestIDIsEchoedAndGeneratedWhenAbsent(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/api/health", "")
	if resp.Header.Get(middleware.HeaderRequestID) == "" {
		t.Error("no request ID generated")
	}

	resp = h.get(t, "/api/health", "", [2]string{middleware.HeaderRequestID, "caller-supplied-id"})
	if got := resp.Header.Get(middleware.HeaderRequestID); got != "caller-supplied-id" {
		t.Errorf("request ID = %q, want the caller's", got)
	}
}
