/**
 * FE14 and FE15 — the two list-screen rules that are about words and numbers
 * rather than about layout.
 *
 * FE14 is §10.7.6: "a blank panel for either reads as broken", and the two empty
 * states need different copy *and* different actions. FE15 is I7 and §2.5.3: a
 * timestamp is rendered in the tenant's business timezone, never the browser's.
 */

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import {
  agus,
  page,
  PO_OPEN,
  purchaseOrder,
  requisition,
  rina,
  sari,
  warehouse,
} from "./fixtures";
import { setViewportWidth } from "./media";
import { renderApp } from "./render";
import { apiUrl, http, HttpResponse, server } from "./server";

const FIRST_RUN =
  "No requisitions yet. A requisition is how buying starts here: raise one, have it approved, and a purchase order is created for you.";
const NO_RESULTS = "No requisitions match those filters.";

function emptyRequisitions() {
  server.use(
    http.get(apiUrl("/api/procurement/requisitions"), () => HttpResponse.json(page([]))),
  );
}

/** Narrow the list by status. Two presses now rather than one — the row of chips
 *  became a dropdown when the filter row had to share a line with a search box. */
async function chooseStatus(label: string) {
  await userEvent.click(screen.getByRole("combobox", { name: "Status" }));
  await userEvent.click(screen.getByRole("option", { name: label }));
}

describe("FE14 — first-run and no-results are different copy and different actions", () => {
  it("explains what a requisition is for when there are none at all", async () => {
    emptyRequisitions();
    await renderApp("/procurement/requisitions", sari);

    // §10.7.6: explain what will appear, and offer the action that creates one.
    expect(await screen.findByText(FIRST_RUN)).toBeInTheDocument();
    expect(screen.queryByText(NO_RESULTS)).not.toBeInTheDocument();

    // The create action appears *inside* the empty state, not only in the header —
    // an empty panel whose only way forward is a button somewhere else is the
    // blank-panel failure with extra steps.
    const table = screen.getByRole("table");
    expect(
      within(table).getByRole("link", { name: "New requisition" }),
    ).toHaveAttribute("href", "/procurement/requisitions/new");
  });

  it("offers no create action inside the empty state to someone who cannot create", async () => {
    // Dewi is a procurement viewer. Offering her the action would be offering a
    // route the server refuses, and §10.7.6's "offer the action that creates one"
    // presumes the reader may.
    emptyRequisitions();
    await renderApp("/procurement/requisitions", {
      ...sari,
      moduleRoles: { procurement: "viewer" },
    });

    expect(await screen.findByText(FIRST_RUN)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "New requisition" })).not.toBeInTheDocument();
  });

  it("says something different when a filter is what emptied the list", async () => {
    emptyRequisitions();
    await renderApp("/procurement/requisitions", sari);
    await screen.findByText(FIRST_RUN);

    await chooseStatus("Draft");

    // Different words, because it is a different problem: nothing is missing, the
    // question was too narrow.
    expect(await screen.findByText(NO_RESULTS)).toBeInTheDocument();
    expect(screen.queryByText(FIRST_RUN)).not.toBeInTheDocument();
    // And no create action, because creating a requisition would not answer the
    // question the reader just asked. Clearing the filter would.
    const table = screen.getByRole("table");
    expect(within(table).queryByRole("link", { name: "New requisition" })).not.toBeInTheDocument();

    // The way back. It was an "All" chip beside the others; it is now the
    // dropdown's first row, and the trigger says which narrowing is in force so
    // the reader can see what to undo.
    const filter = screen.getByRole("combobox", { name: "Status" });
    expect(filter).toHaveTextContent("Draft");
    await userEvent.click(filter);
    expect(screen.getByRole("option", { name: "All statuses" })).toBeInTheDocument();
  });

  it("tells the two apart on a search as well as on a filter chip", async () => {
    emptyRequisitions();
    await renderApp("/procurement/requisitions", sari);
    await screen.findByText(FIRST_RUN);

    await userEvent.type(screen.getByPlaceholderText("Number, supplier, or notes"), "zzz");

    expect(await screen.findByText(NO_RESULTS)).toBeInTheDocument();
  });

  it("keeps both states distinct in the card view", async () => {
    // The card view says exactly what the table's would — `EmptyCards` and
    // `EmptyState` share `EmptyMessage` for that reason, so the two views cannot
    // drift into different copy.
    setViewportWidth(360);
    emptyRequisitions();
    await renderApp("/procurement/requisitions", sari);

    expect(await screen.findByText(FIRST_RUN)).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();

    await chooseStatus("Submitted");
    expect(await screen.findByText(NO_RESULTS)).toBeInTheDocument();
  });

  it("tells them apart on a master-data list too", async () => {
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), ({ request }) =>
        HttpResponse.json(
          page(new URL(request.url).searchParams.get("q") ? [] : [warehouse()]),
        ),
      ),
    );
    await renderApp("/inventory/warehouses", rina);

    await screen.findByText("Main warehouse");
    await userEvent.type(screen.getByPlaceholderText("Code or name"), "zzz");

    expect(await screen.findByText("No warehouses match that search.")).toBeInTheDocument();
    expect(
      screen.queryByText(/No warehouses yet\./),
    ).not.toBeInTheDocument();
  });

  it("reports a failed load inline rather than as a toast", async () => {
    // The fourth of §10.7.6's states, and the distinction Phase 4 drew: a failed
    // *load* stays where the data would have been, because a toast fades and
    // leaves an empty screen with no explanation.
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json({ error: "internal", message: "Something went wrong on the server." }, { status: 500 }),
      ),
    );
    await renderApp("/procurement/requisitions", sari);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Something went wrong on the server.");
    expect(within(alert).getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });
});

describe("FE15 — timestamps render in the tenant's timezone, not the browser's", () => {
  /** The same instant as the tenant would render it. Composed with `Intl` rather
   *  than written out, because the *locale* is the reader's and only the timezone
   *  is under test — a hardcoded string would be a test that only passes on the
   *  machine it was written on. */
  function asTenantWouldRender(iso: string, timeZone: string): string {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone,
    }).format(new Date(iso));
  }

  // 23:30 UTC on the 20th is 06:30 on the 21st in Jakarta and 07:30 on the 21st
  // in Makassar — so the two tenants disagree about the time, and a browser in
  // UTC disagrees about the *day*.
  const INSTANT = "2026-07-20T23:30:00Z";

  function withOneRequisition() {
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json(page([requisition({ createdAt: INSTANT })])),
      ),
    );
  }

  it("renders Asia/Jakarta for a Jakarta workspace", async () => {
    withOneRequisition();
    await renderApp("/procurement/requisitions", rina);

    await screen.findByText("PR-202607-0001");
    expect(
      screen.getByText(asTenantWouldRender(INSTANT, "Asia/Jakarta")),
    ).toBeInTheDocument();
  });

  it("renders Asia/Makassar for a Makassar workspace — the same instant, an hour later", async () => {
    // The assertion that does not depend on where the test is run: two tenants,
    // one instant, two renderings that differ by the hour between their zones. A
    // screen reading the browser's zone would render both identically.
    withOneRequisition();
    await renderApp("/procurement/requisitions", agus);

    await screen.findByText("PR-202607-0001");
    const makassar = asTenantWouldRender(INSTANT, "Asia/Makassar");
    const jakarta = asTenantWouldRender(INSTANT, "Asia/Jakarta");

    expect(makassar).not.toBe(jakarta);
    expect(screen.getByText(makassar)).toBeInTheDocument();
    expect(screen.queryByText(jakarta)).not.toBeInTheDocument();
  });

  it("puts a receipt posted at 23:30 Jakarta on the Jakarta day", async () => {
    // I7's worked example, and why this matters beyond tidiness: a receipt posted
    // at 23:30 in Jakarta is on that day's books, and a colleague opening the same
    // ledger in London has to see the same date or the two of them disagree about
    // which month a movement fell in.
    const lateJakarta = "2026-07-31T16:30:00Z"; // 23:30 on the 31st in Jakarta
    server.use(
      http.get(apiUrl("/api/inventory/ledger"), () =>
        HttpResponse.json(
          page([
            {
              id: "l1",
              occurredAt: lateJakarta,
              entryType: "receipt" as const,
              qtyDelta: 100,
              unitCost: 12.5,
              sourceType: "goods_receipt" as const,
              sourceId: "gr1",
              sourceNumber: "GR-202607-0001",
              sourcePoId: PO_OPEN,
              note: null,
              productId: "p1",
              sku: "HND-GLOVE",
              productName: "Nitrile gloves, box of 100",
              productDeleted: false,
              warehouseId: "w1",
              warehouseCode: "WH-MAIN",
              createdById: "u1",
              createdByName: "Budi Santoso",
            },
          ]),
        ),
      ),
    );
    await renderApp("/inventory/ledger", rina);

    await screen.findByText("GR-202607-0001");
    const rendered = asTenantWouldRender(lateJakarta, "Asia/Jakarta");
    expect(screen.getByText(rendered)).toBeInTheDocument();
    // July, in Jakarta. In UTC the same instant is still the 31st, but the
    // rendering must be the tenant's either way.
    expect(rendered).toMatch(/31/);
  });

  it("renders a business date exactly as it arrives, with no zone applied", async () => {
    // §2.5.3. `expectedAt` is a `YYYY-MM-DD` date, not an instant: there is no
    // time of day to convert, so passing it through `new Date()` and a formatter
    // is how a delivery expected on the 28th shows up as the 27th.
    server.use(
      http.get(apiUrl("/api/procurement/purchase-orders"), () =>
        HttpResponse.json(page([purchaseOrder({ expectedAt: "2026-07-28" })])),
      ),
    );
    await renderApp("/procurement/orders", rina);

    expect(await screen.findByText("2026-07-28")).toBeInTheDocument();
  });
});
