/**
 * MSW, standing in for the backend at the network boundary (§12.5).
 *
 * WHY MSW RATHER THAN STUBBING `lib/api.ts`. Every endpoint in `lib/api.ts` is a
 * URL, a method, a query string, and an error envelope, and all four are part of
 * the contract with the Go handlers. Stubbing the wrapper functions would test the
 * screens against a second, friendlier API that no server implements — a
 * `?status=` that is silently ignored, or a 409 whose `error` code is spelled
 * differently, would both stay green. Intercepting `fetch` means the request the
 * assertion sees is the request the server would have received.
 *
 * The default handlers below answer every endpoint the app can reach, with the
 * emptiest legal §9.0 response. A test that cares states its own with
 * `server.use(...)`; a test that does not is spared four lines of scaffolding.
 * `onUnhandledRequest: "error"` in `setup.ts` means an endpoint nobody thought
 * about fails loudly instead of hanging.
 */

import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";

import type { Me } from "../lib/api";
import { page, rina } from "./fixtures";

const BASE = "http://localhost:8080";

/** An absolute URL for a handler. The base has to match `VITE_API_BASE_URL` in
 *  `vite.config.ts`, which is why both are pinned rather than read from a file a
 *  developer might edit. */
export function apiUrl(path: string): string {
  return `${BASE}${path}`;
}

// Who /api/me answers with. Set by `renderApp`, so a test names an identity once.
let identity: Me = rina;

export function setMe(me: Me): void {
  identity = me;
}

/**
 * The §9.8 error envelope, exactly as `httpx.Fail` writes it: a machine-readable
 * `error` code and a `message` written for people. `ApiError` branches on the
 * code, and every toast shows the sentence.
 */
export function failure(
  status: number,
  code: string,
  message: string,
  details?: Record<string, unknown>,
) {
  return HttpResponse.json(
    details ? { error: code, message, details } : { error: code, message },
    { status },
  );
}

const emptyPage = () => HttpResponse.json(page([]));

const defaults = [
  http.get(apiUrl("/api/me"), () => HttpResponse.json(identity)),
  http.get(apiUrl("/api/dashboard/summary"), () => HttpResponse.json({})),

  // Platform plane.
  http.get(apiUrl("/api/admin/tenants"), emptyPage),
  http.get(apiUrl("/api/admin/modules"), () => HttpResponse.json([])),

  // Tenant plane.
  http.get(apiUrl("/api/tenant/users"), emptyPage),

  // Inventory.
  http.get(apiUrl("/api/inventory/products"), emptyPage),
  http.get(apiUrl("/api/inventory/warehouses"), emptyPage),
  http.get(apiUrl("/api/inventory/stock"), emptyPage),
  http.get(apiUrl("/api/inventory/stock/low"), emptyPage),
  http.get(apiUrl("/api/inventory/ledger"), emptyPage),

  // Procurement.
  http.get(apiUrl("/api/procurement/requisitions"), emptyPage),
  http.get(apiUrl("/api/procurement/purchase-orders"), emptyPage),
  http.get(apiUrl("/api/procurement/suppliers"), emptyPage),
  http.get(apiUrl("/api/procurement/goods-receipts"), emptyPage),

  // Finance.
  http.get(apiUrl("/api/finance/journal-entries"), emptyPage),
  http.get(apiUrl("/api/finance/accounts"), emptyPage),
];

export const server = setupServer(...defaults);

/** Re-export so a test file needs one import for its handlers. */
export { http, HttpResponse };
