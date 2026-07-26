/**
 * FE2 and FE6 — what a level lets you see, and what happens when it goes away.
 *
 * Both sit on the cosmetic side of I12. FE2's button is refused server-side by an
 * `approver` route and again by C2's self-approval rule; FE6's screen is refused
 * by `RequireModule`. What is tested here is that the browser agrees with the
 * server rather than that it enforces anything.
 */

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { budi, dewi, requisition, sari } from "./fixtures";
import { renderApp } from "./render";
import { apiUrl, failure, http, HttpResponse, server } from "./server";

const APPROVE = "Approve and create order";

/** One submitted requisition, raised by Sari. Everything FE2 turns on is who is
 *  reading it. */
function submittedBySari() {
  const pr = requisition({ status: "submitted", supplierId: null, supplierCode: null, supplierName: null });
  server.use(
    http.get(apiUrl(`/api/procurement/requisitions/${pr.id}`), () =>
      HttpResponse.json({
        ...pr,
        lines: [
          {
            id: "line-1",
            lineNo: 1,
            productId: "p1",
            sku: "HND-GLOVE",
            productName: "Nitrile gloves, box of 100",
            uom: "box",
            productDeleted: false,
            qty: 100,
            estUnitCost: 12.5,
            lineTotal: 1250,
          },
        ],
      }),
    ),
  );
  return pr;
}

describe("FE2 — Approve is absent below approver and present at approver", () => {
  it("renders it for an approver who did not raise the requisition", async () => {
    const pr = submittedBySari();
    await renderApp(`/procurement/requisitions/${pr.id}`, budi);

    expect(await screen.findByRole("button", { name: APPROVE })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reject" })).toBeInTheDocument();
  });

  it("does not render it for a `user`-level account", async () => {
    const pr = submittedBySari();
    // Sari holds `procurement: user`. She may raise a requisition and may not
    // decide one, and this is the same document Budi was just offered.
    await renderApp(`/procurement/requisitions/${pr.id}`, sari);

    await screen.findByText("PR-202607-0001");
    expect(screen.queryByRole("button", { name: APPROVE })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reject" })).not.toBeInTheDocument();
  });

  it("does not render it for a `viewer` either", async () => {
    const pr = submittedBySari();
    await renderApp(`/procurement/requisitions/${pr.id}`, dewi);

    await screen.findByText("PR-202607-0001");
    expect(screen.queryByRole("button", { name: APPROVE })).not.toBeInTheDocument();
    // A viewer gets no actions panel at all, not an empty one.
    expect(screen.queryByRole("heading", { name: "Actions" })).not.toBeInTheDocument();
  });

  it("withholds it from an approver reading their own requisition, and says why", async () => {
    // C2 forbids approving your own requisition for everybody, tenant admins
    // included. Hiding the button is the courtesy; the rule is the server's. The
    // sentence matters as much as the absence — §10.7.6's point about a blank
    // panel reading as broken applies to a missing button too.
    const pr = requisition({ status: "submitted", requestedById: budi.user.id, requestedByName: budi.user.fullName });
    server.use(
      http.get(apiUrl(`/api/procurement/requisitions/${pr.id}`), () =>
        HttpResponse.json({ ...pr, lines: [] }),
      ),
    );

    await renderApp(`/procurement/requisitions/${pr.id}`, budi);

    expect(
      await screen.findByText(
        "You raised this requisition, so somebody else has to approve it.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: APPROVE })).not.toBeInTheDocument();
  });

  it("withholds it once the requisition has been decided", async () => {
    // Not a level rule but the same button: an approved requisition has no
    // decision left to take, and the server refuses one with `state_conflict`.
    const pr = requisition({ status: "approved", decidedById: budi.user.id, decidedByName: budi.user.fullName, decidedAt: "2026-07-21T02:00:00Z" });
    server.use(
      http.get(apiUrl(`/api/procurement/requisitions/${pr.id}`), () =>
        HttpResponse.json({ ...pr, lines: [] }),
      ),
    );

    await renderApp(`/procurement/requisitions/${pr.id}`, budi);

    await screen.findByText("PR-202607-0001");
    expect(screen.queryByRole("button", { name: APPROVE })).not.toBeInTheDocument();
  });
});

describe("FE6 — a 403 module_not_enabled leaves the module, with a message", () => {
  const MESSAGE = "This workspace does not have the finance module enabled.";

  it("redirects to the dashboard and shows the server's sentence", async () => {
    // The stale-identity case, which is the only way this is reachable: Dewi's
    // /api/me says she holds `finance: admin`, so the cosmetic guard lets her
    // through, and the workspace lost the entitlement a minute ago.
    server.use(
      http.get(apiUrl("/api/finance/journal-entries"), () =>
        failure(403, "module_not_enabled", MESSAGE, { module: "finance" }),
      ),
      http.get(apiUrl("/api/finance/accounts"), () =>
        failure(403, "module_not_enabled", MESSAGE, { module: "finance" }),
      ),
    );

    await renderApp("/finance", dewi);

    expect(await screen.findByText(MESSAGE)).toBeInTheDocument();
    await waitFor(() => expect(window.location.pathname).toBe("/"));
  });

  it("re-reads /api/me, so the nav stops offering the module it just refused", async () => {
    let meReads = 0;
    server.use(
      http.get(apiUrl("/api/me"), () => {
        meReads += 1;
        // The second read is the truth: the entitlement is gone.
        return HttpResponse.json(
          meReads === 1 ? dewi : { ...dewi, moduleRoles: { procurement: "viewer" } },
        );
      }),
      http.get(apiUrl("/api/finance/journal-entries"), () =>
        failure(403, "module_not_enabled", MESSAGE, { module: "finance" }),
      ),
      http.get(apiUrl("/api/finance/accounts"), () =>
        failure(403, "module_not_enabled", MESSAGE, { module: "finance" }),
      ),
    );

    await renderApp("/finance", dewi);

    // Without the refresh the reader is bounced to the dashboard past a Finance
    // link that is still there, and clicking it bounces them again.
    await waitFor(() => {
      const nav = screen.getByRole("navigation", { name: "Main" });
      expect(nav.textContent).not.toContain("Finance");
    });
    expect(meReads).toBeGreaterThan(1);
  });

  it("does not throw the reader off the screen for insufficient_module_role", async () => {
    // A different refusal with a different answer. "You are in the right module
    // and not senior enough for this one action" is reported where the action
    // was; redirecting would lose the document the reader is looking at.
    //
    // This requisition names a supplier, unlike the one FE2 uses: without one the
    // Approve button is disabled until a supplier is chosen (§8.3), and a click
    // on a disabled button would make the test pass for the wrong reason.
    const pr = requisition({ status: "submitted" });
    server.use(
      http.get(apiUrl(`/api/procurement/requisitions/${pr.id}`), () =>
        HttpResponse.json({ ...pr, lines: [] }),
      ),
      http.post(apiUrl(`/api/procurement/requisitions/${pr.id}/approve`), () =>
        failure(403, "insufficient_module_role", "You do not have permission to do this."),
      ),
    );

    await renderApp(`/procurement/requisitions/${pr.id}`, budi);
    await userEvent.click(await screen.findByRole("button", { name: APPROVE }));

    expect(
      await screen.findByText("You do not have permission to do this."),
    ).toBeInTheDocument();
    expect(window.location.pathname).toBe(`/procurement/requisitions/${pr.id}`);
  });

  it("keeps a module out of the router for someone whose /api/me never held it", async () => {
    // The ordinary case, and the reason FE6's case is rare: Dewi holds nothing in
    // Inventory, so `RequireModule` redirects before a request is ever made.
    await renderApp("/inventory/products", dewi);

    await waitFor(() => expect(window.location.pathname).toBe("/"));
  });
});
