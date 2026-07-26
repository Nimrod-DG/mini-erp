// Group M — the CORS preflight (§9.8).
//
// This group exists because of one bug, and it is the only bug in the project
// that every other test in this suite was structurally incapable of catching.
//
// The browser will not send a cross-origin request carrying a header the
// preflight did not bless. It does not fail loudly: the OPTIONS returns 204, the
// real request is then never sent, nothing reaches a handler, and the server log
// shows a preflight with no method behind it. `Idempotency-Key` was missing from
// AllowHeaders, so posting a goods receipt — §8.4, the cross-module transaction
// this application is built to demonstrate — was impossible from a browser while
// Groups D and H were green.
//
// They were green because `app.Test` speaks directly to Fiber. There is no
// origin, so there is no preflight, so the allow-list is never consulted. Any
// test that posts a receipt the normal way proves nothing about this.
//
// M1 therefore asserts the allow-list from the outside, and asserts it for every
// header the frontend actually sends rather than for a hardcoded string.
package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DGosal/mini-erp/backend/internal/middleware"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// preflightPath is the receipts route. A preflight is answered before routing,
// so the path only has to be shaped like the real one — the id never resolves.
const preflightPath = "/api/procurement/purchase-orders/" +
	"00000000-0000-0000-0000-000000000000/receipts"

// M1 — the preflight blesses every header the frontend sends.
//
// A header added to a request without being added here is the failure this
// catches, and the assertion is per-header so the message names the missing one.
func TestM1PreflightAllowsEveryHeaderTheFrontendSends(t *testing.T) {
	h := testsupport.NewHarness(t)

	// The origin the harness is configured with. A preflight from anywhere else
	// is a different question, asserted in M2.
	const origin = "http://localhost:5173"

	// Content-Type and Authorization are on every authenticated write.
	// Idempotency-Key is on exactly one, and is the header that was missing.
	// X-Request-Id is optional but honoured inbound, so a client may send it.
	for _, header := range []string{
		"Content-Type",
		"Authorization",
		middleware.HeaderRequestID,
		middleware.HeaderIdempotencyKey,
	} {
		t.Run(header, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodOptions,
				preflightPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			req.Header.Set("Access-Control-Request-Headers", header)

			resp, err := h.App.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()

			// A preflight is answered by the CORS middleware and never reaches the
			// auth chain, so it must not be a 401 either.
			if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
				t.Fatalf("preflight returned %d, want 204 or 200", resp.StatusCode)
			}

			// The comparison is case-insensitive: header names are, and Fiber
			// echoes back the list as configured rather than as requested.
			allowed := resp.Header.Get("Access-Control-Allow-Headers")
			if !strings.Contains(strings.ToLower(allowed), strings.ToLower(header)) {
				t.Errorf("Access-Control-Allow-Headers omits %q, so a browser will "+
					"answer the preflight and then silently refuse to send the "+
					"request.\ngot: %s", header, allowed)
			}
		})
	}
}

// M2 — the allow-list is an allow-list. An unconfigured origin is not blessed,
// or the CORS configuration would be decoration.
func TestM2PreflightDoesNotBlessAnUnknownOrigin(t *testing.T) {
	h := testsupport.NewHarness(t)

	req, err := http.NewRequest(http.MethodOptions,
		preflightPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://not-our-frontend.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", middleware.HeaderIdempotencyKey)

	resp, err := h.App.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got == "https://not-our-frontend.example" {
		t.Errorf("an unconfigured origin was echoed back as allowed: %s", got)
	}
}
