package testsupport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/DGosal/mini-erp/backend/internal/api"
	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/internal/middleware"
)

// Harness is the application under test plus the database behind it.
//
// It drives the *real* app, built by api.New, with only the two Firebase
// interfaces faked (§12.4). A test that assembles its own middleware chain
// proves nothing about the one that ships — and every permission test in this
// project is a claim about the shipped chain.
type Harness struct {
	DB       *TestDB
	App      *fiber.App
	Users    *FakeUsers
	Verifier *FakeVerifier
}

// NewHarness builds the app with a verifier that accepts every token as its own
// UID.
func NewHarness(t *testing.T) *Harness {
	t.Helper()
	return NewHarnessWith(t, &FakeVerifier{})
}

// NewHarnessWith builds the app with a specific verifier, for the tests where
// token rejection is the thing under test.
func NewHarnessWith(t *testing.T, v *FakeVerifier) *Harness {
	t.Helper()

	tdb := NewTestDB(t)
	pools := db.NewPools(tdb.AppURL, tdb.AdminURL)
	t.Cleanup(func() { _ = pools.Close() })

	users := NewFakeUsers()
	app := api.New(api.Deps{
		Pools:       pools,
		Verifier:    v,
		Users:       users,
		CORSOrigins: []string{"http://localhost:5173"},
		Quiet:       true,
	})

	h := &Harness{DB: tdb, App: app, Users: users, Verifier: v}
	h.addProbeRoutes()
	return h
}

// ProbePath is the URL of the probe route guarded by RequireModule(module,
// level) — see addProbeRoutes.
func ProbePath(module string, level identity.RoleLevel) string {
	return fmt.Sprintf("/api/probe/%s/%s", module, level)
}

// TenantTxPath is a tenant-scoped route with NO RequireModule on it, for the
// tests that are about the transaction rather than about permissions.
//
// It lives under `/api/probe/` with the rest of the test-only routes. It was
// `/api/warehouses` until Phase 5, which was fine while no such endpoint existed
// and misleading the moment `/api/inventory/warehouses` shipped: a reader
// checking what a failing test covered would find a real endpoint one path
// segment away and reasonably assume they were the same thing.
const TenantTxPath = "/api/probe/tenant-tx"

// addProbeRoutes mounts the routes the permission tests exercise.
//
// They are registered after api.New but under the same `/api` prefix, so the
// §7 chain — RequestID, FirebaseAuth, ResolveIdentity, TenantTx — runs in front
// of them exactly as it does for a shipped route.
//
// They stand in for Phase 4's module endpoints, which do not exist yet, and
// each one holds a real RequireModule instance bound at registration time. Each
// also reads tenant data through the transaction TenantTx opened: /api/me alone
// cannot show that TenantTx works, because it reads platform tables that carry
// no RLS and would pass with TenantTx deleted.
func (h *Harness) addProbeRoutes() {
	h.App.Get(TenantTxPath, tenantTxProbe)

	for _, module := range []string{"procurement", "inventory", "finance"} {
		for _, level := range []identity.RoleLevel{
			identity.RoleViewer, identity.RoleUser, identity.RoleApprover, identity.RoleAdmin,
		} {
			h.App.Get(ProbePath(module, level),
				middleware.RequireModule(module, level), tenantTxProbe)
		}
	}
}

// tenantTxProbe reads through the tenant transaction, so reaching it proves
// both that the gate let the caller past and that the transaction is live.
func tenantTxProbe(c *fiber.Ctx) error {
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
}

// --------------------------------------------------------------------------
// Requests.
// --------------------------------------------------------------------------

// Request issues one request against the app. An empty token sends no
// Authorization header at all; a nil body sends none.
func (h *Harness) Request(t *testing.T, method, path, token string, body any, headers ...[2]string) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, kv := range headers {
		req.Header.Set(kv[0], kv[1])
	}

	// -1 disables the test timeout: these requests wait on a real database.
	resp, err := h.App.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (h *Harness) Get(t *testing.T, path, token string, headers ...[2]string) *http.Response {
	t.Helper()
	return h.Request(t, http.MethodGet, path, token, nil, headers...)
}

func (h *Harness) Post(t *testing.T, path, token string, body any) *http.Response {
	t.Helper()
	return h.Request(t, http.MethodPost, path, token, body)
}

func (h *Harness) Patch(t *testing.T, path, token string, body any) *http.Response {
	t.Helper()
	return h.Request(t, http.MethodPatch, path, token, body)
}

func (h *Harness) Put(t *testing.T, path, token string, body any) *http.Response {
	t.Helper()
	return h.Request(t, http.MethodPut, path, token, body)
}

// Delete carries no body. Every DELETE this API answers is a soft delete whose
// only input is the path (§6.9.1).
func (h *Harness) Delete(t *testing.T, path, token string) *http.Response {
	t.Helper()
	return h.Request(t, http.MethodDelete, path, token, nil)
}

// --------------------------------------------------------------------------
// Reading responses.
// --------------------------------------------------------------------------

// ErrorBody is the §9.8 envelope. Tests branch on Code, never on Message —
// exactly as a client must.
type ErrorBody struct {
	Code    string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// Decode unmarshals a response body, failing the test with the raw body on a
// mismatch — otherwise a shape change reads as an inscrutable zero value.
func Decode[T any](t *testing.T, resp *http.Response) T {
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

// DecodeError reads the error envelope.
func DecodeError(t *testing.T, resp *http.Response) ErrorBody {
	t.Helper()
	return Decode[ErrorBody](t, resp)
}

// AssertStatus fails with the response body attached, which is the difference
// between "wanted 403 got 500" and knowing why.
func AssertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, want, body)
	}
}

// AssertErrorCode asserts both the status and the machine-readable code. The
// pair matters: a 403 with the wrong code sends the user to the wrong person.
func AssertErrorCode(t *testing.T, resp *http.Response, wantStatus int, wantCode string) ErrorBody {
	t.Helper()
	body := DecodeError(t, resp)
	if resp.StatusCode != wantStatus || body.Code != wantCode {
		t.Fatalf("got %d %q, want %d %q (message: %s)",
			resp.StatusCode, body.Code, wantStatus, wantCode, body.Message)
	}
	return body
}
