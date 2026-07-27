/**
 * FE32 — the dashboard of §10.2.
 *
 * §10.2 lists the data on this screen; it does not fix the arrangement, and the
 * arrangement is what these are about. It settled at **three numbers and one
 * table**, after two versions that each said things twice: an approval queue
 * that was the "Awaiting approval" tile again, and a Low stock panel that was
 * the "Below reorder point" tile again. Both are gone, and what is left is the
 * summary plus the one thing that exists nowhere else — the last fifteen stock
 * movements.
 *
 * So these assert what the layout is *claiming*: that the numbers are one
 * comparable row in a fixed order, that each carries a fact and is the way in to
 * its own screen, that nothing is said twice, and — the one that could actually
 * mislead — that the activity table's search says which set it is narrowing.
 *
 * There were no tests here at all before this, which is how a screen ends up
 * with three big numbers a reader cannot compare and a dead link nobody noticed.
 */

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import {
  budi,
  dashboard,
  dewi,
  ledgerEntry,
  page,
  PRODUCT_BOX,
  PRODUCT_GLOVE,
  requisition,
  WH_MAIN,
} from "./fixtures";
import { renderApp } from "./render";
import { apiUrl, http, HttpResponse, server } from "./server";

function summary(body: Record<string, unknown>) {
  server.use(
    http.get(apiUrl("/api/dashboard/summary"), () => HttpResponse.json(body)),
  );
}

const LOW_STOCK = {
  count: 3,
  products: [
    {
      productId: PRODUCT_BOX,
      sku: "PKG-BOX-S",
      name: "Kardus kecil 20×20×15",
      uom: "pcs",
      qtyOnHand: 140,
      reorderPoint: 200,
      shortfall: 60,
    },
  ],
};

/** The strip, as a list, so its order can be read off the DOM. */
function strip() {
  return screen.getByRole("list", { name: "Summary" });
}

function tileLabels() {
  return within(strip())
    .getAllByRole("listitem")
    .map((tile) => tile.textContent);
}

describe("FE32 — the headline numbers are one comparable row", () => {
  it("orders the tiles by urgency, not by module", async () => {
    // Fixed order: what needs a decision, then what is at risk, then what is in
    // flight. A strip whose order depends on your entitlements is a strip you
    // have to read rather than recognise.
    summary(
      dashboard({
        openOrders: { count: 3, totalValue: 9310000 },
        pendingApprovals: { count: 3, canApprove: true, queue: [] },
        lowStock: LOW_STOCK,
      }),
    );
    await renderApp("/", budi);

    const labels = await screen.findAllByText(
      /Awaiting approval|Below reorder point|Open purchase orders/,
    );
    expect(labels.map((node) => node.textContent)).toEqual([
      "Awaiting approval",
      "Below reorder point",
      "Open purchase orders",
    ]);
  });

  it("keeps that order when a caller gets only some of the tiles", async () => {
    // Dewi holds procurement and finance and nothing in Inventory, so the server
    // omits `lowStock` entirely. The two that remain must not reshuffle.
    summary(
      dashboard({
        openOrders: { count: 2, totalValue: 500000 },
        pendingApprovals: { count: 1, canApprove: false, queue: [] },
      }),
    );
    await renderApp("/", dewi);
    await screen.findByText("Awaiting approval");

    const tiles = tileLabels();
    expect(tiles).toHaveLength(2);
    expect(tiles[0]).toContain("Awaiting approval");
    expect(tiles[1]).toContain("Open purchase orders");
  });

  it("gives every number a fact beside it rather than leaving it naked", async () => {
    summary(
      dashboard({
        openOrders: { count: 3, totalValue: 9310000 },
        pendingApprovals: { count: 3, canApprove: true, queue: [] },
        lowStock: LOW_STOCK,
      }),
    );
    await renderApp("/", budi);

    // A count on its own is a number the reader has to open something to
    // understand. The worst-off product by name is the thing to act on.
    expect(await screen.findByText(/PKG-BOX-S short 60 pcs/)).toBeInTheDocument();
    expect(screen.getByText(/Worth 9\.310\.000,00/)).toBeInTheDocument();
    expect(screen.getByText("3 requisitions need you")).toBeInTheDocument();
  });

  it("says something true when a number is zero", async () => {
    // §10.7.6 — a panel that just says "0" reads as broken or as unloaded.
    summary(
      dashboard({
        openOrders: { count: 0, totalValue: 0 },
        pendingApprovals: { count: 0, canApprove: true, queue: [] },
        lowStock: { count: 0, products: [] },
      }),
    );
    await renderApp("/", budi);

    expect(
      await screen.findByText("Nothing waiting on a decision"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Everything is above its reorder point"),
    ).toBeInTheDocument();
    expect(screen.getByText("Nothing is on order")).toBeInTheDocument();
  });

  it("lands the approval tile on a list that is actually filtered", async () => {
    // The tile said "3 awaiting approval" and linked to
    // `?status=submitted` — which the requisition list ignored, so the reader
    // arrived at all thirteen. A dashboard figure whose link disagrees with it
    // is worse than one with no link.
    summary(
      dashboard({
        pendingApprovals: { count: 3, canApprove: true, queue: [] },
      }),
    );
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), ({ request }) => {
        const asked = new URL(request.url).searchParams.get("status");
        return HttpResponse.json(
          page(asked === "submitted" ? [requisition({ id: "s1" })] : []),
        );
      }),
    );
    await renderApp("/", budi);

    await userEvent.click(
      await screen.findByRole("link", { name: /Awaiting approval/ }),
    );

    expect(
      await screen.findByRole("combobox", { name: "Status" }),
    ).toHaveTextContent("Submitted");
    expect(await screen.findByText("PR-202607-0001")).toBeInTheDocument();
  });

  it("makes every tile a way in", async () => {
    summary(dashboard({ lowStock: LOW_STOCK }));
    await renderApp("/", budi);
    await screen.findByText("Below reorder point");

    // A dashboard number that cannot be opened is a number the reader has to
    // take on trust.
    const tile = within(strip()).getByRole("link");
    expect(tile).toHaveAttribute("href", "/inventory/stock");
  });
});

/** One low product, in the shape `/api/inventory/stock/low` returns. */
const LOW_ROW = {
  productId: PRODUCT_BOX,
  sku: "PKG-BOX-S",
  name: "Kardus kecil 20×20×15",
  uom: "pcs",
  qtyOnHand: 140,
  reorderPoint: 200,
  shortfall: 60,
};

describe("FE32 — nothing on the page is said twice", () => {
  it("has no approval panel: the tile is the way to the decisions", async () => {
    // The queue panel was a heading, a list, and a link that went exactly where
    // the tile above it went. One of them had to go, and the tile is the one
    // that also carries the number.
    summary(
      dashboard({
        pendingApprovals: {
          count: 3,
          canApprove: true,
          queue: [requisition({ id: "q1", prNumber: "PR-202607-0004" })],
        },
      }),
    );
    await renderApp("/", budi);
    await screen.findByText("3 requisitions need you");

    expect(
      screen.queryByRole("heading", { name: "Needs your decision" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Approve" }),
    ).not.toBeInTheDocument();
    expect(
      within(strip()).getByRole("link", { name: /Awaiting approval/ }),
    ).toHaveAttribute("href", "/procurement/requisitions?status=submitted");
  });

  it("has no low-stock panel: the tile names the worst and links to the rest", async () => {
    summary(dashboard({ lowStock: LOW_STOCK }));
    await renderApp("/", budi);
    await screen.findByText("PKG-BOX-S short 60 pcs");

    // The panel listed the same rows `/inventory/stock` lists, under a number
    // the tile already carried.
    expect(
      screen.queryByRole("heading", { name: "Low stock" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Kardus kecil 20×20×15")).not.toBeInTheDocument();
  });

  it("keeps the requisition shortcut, on the screen the tile points at", async () => {
    // The one thing on the low-stock panel that was NOT a duplicate: products
    // pre-filled from their shortfalls. Removing the panel would have removed a
    // feature, so it moved to `/inventory/stock`.
    server.use(
      http.get(apiUrl("/api/inventory/stock/low"), () =>
        HttpResponse.json(page([LOW_ROW])),
      ),
    );
    await renderApp("/inventory/stock", budi);

    // The shortfall rides in the URL, so the requisition that opens would
    // actually clear the reorder point rather than ask for one of something
    // short by sixty.
    expect(
      await screen.findByRole("link", { name: "Create requisition" }),
    ).toHaveAttribute(
      "href",
      `/procurement/requisitions/new?products=${PRODUCT_BOX}:60`,
    );
  });

  it("does not offer that shortcut to somebody who cannot raise one", async () => {
    // Cross-module and cosmetic (I12): seeing that stock is low is Inventory,
    // asking for more of it is Procurement, and this needs somebody who holds
    // the first and not the second. None of the seeded eight is that shape —
    // the closest, Dewi, has no Inventory at all and so cannot reach this
    // screen — so the identity is built here rather than bent out of a fixture.
    const stockWatcher = {
      ...budi,
      moduleRoles: { procurement: "viewer", inventory: "viewer" },
    } as typeof budi;

    server.use(
      http.get(apiUrl("/api/inventory/stock/low"), () =>
        HttpResponse.json(page([LOW_ROW])),
      ),
    );
    await renderApp("/inventory/stock", stockWatcher);

    await screen.findByText(/below its reorder point/);
    expect(
      screen.queryByRole("link", { name: "Create requisition" }),
    ).not.toBeInTheDocument();
  });
});

describe("FE32 — the activity table, and the one way it could mislead", () => {
  const MOVEMENTS = [
    ledgerEntry({
      id: "a1",
      sku: "HND-GLOVE",
      productName: "Sarung tangan kerja",
      productId: PRODUCT_GLOVE,
      entryType: "receipt",
      qtyDelta: 30,
      warehouseId: WH_MAIN,
      warehouseCode: "GP",
    }),
    ledgerEntry({
      id: "a2",
      sku: "PKG-BOX-L",
      productName: "Kardus besar 60×40×40",
      productId: PRODUCT_BOX,
      entryType: "adjustment",
      qtyDelta: -12,
      sourceType: "manual_adjustment",
      sourceId: null,
      sourceNumber: null,
      sourcePoId: null,
      warehouseId: "99999999-9999-4999-8999-999999999991",
      warehouseCode: "GC",
    }),
  ];

  function withActivity() {
    summary(dashboard({ recentActivity: { entries: MOVEMENTS } }));
  }

  it("is a table, not a feed", async () => {
    // A narrow column of stacked rows several times taller than anything beside
    // it is what made every earlier version of this page look lopsided.
    withActivity();
    await renderApp("/", budi);

    const table = await screen.findByRole("table");
    expect(
      within(table).getAllByRole("columnheader").map((cell) => cell.textContent),
    ).toEqual(["When", "Product", "Warehouse", "Change", "Source"]);
    expect(within(table).getAllByRole("row")).toHaveLength(3);
  });

  it("says which set the filters are narrowing, before anything is typed", async () => {
    // THE POINT OF THIS TEST. Every other search box in the application is a
    // server parameter; this one narrows the widget's rows in the browser,
    // because the widget *is* the last fifteen movements. A box that looks the
    // same and silently searches less is the one way this screen could mislead,
    // so the sentence under the heading names the set on every render — and it
    // is separate from the pagination line, which counts the *filtered* set.
    withActivity();
    await renderApp("/", budi);

    expect(await screen.findByText(/The last/)).toHaveTextContent(
      "The last 2 stock movements — the full ledger goes back further.",
    );
  });

  it("narrows on a search, and the pagination follows", async () => {
    withActivity();
    await renderApp("/", budi);
    await screen.findByRole("table");

    await userEvent.type(
      screen.getByRole("searchbox", { name: "Search recent activity" }),
      "kardus",
    );

    expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(2);
    expect(
      within(screen.getByRole("navigation", { name: "Pagination" })).getByText(
        /Showing/,
      ),
    ).toHaveTextContent("Showing 1–1 of 1 entries");
  });

  it("shows five rows a page, and pages through the rest", async () => {
    // Five, like every other list in the application — the widget returns
    // fifteen and dumping all of them made the dashboard scroll for no reason.
    summary(
      dashboard({
        recentActivity: {
          entries: Array.from({ length: 12 }, (_, i) =>
            ledgerEntry({
              id: `m${i}`,
              sku: `SKU-${String(i + 1).padStart(4, "0")}`,
              productName: `Movement ${i + 1}`,
            }),
          ),
        },
      }),
    );
    await renderApp("/", budi);

    const rows = () =>
      within(screen.getByRole("table")).getAllByRole("row").length - 1;

    await screen.findByText("Movement 1");
    expect(rows()).toBe(5);
    expect(screen.queryByText("Movement 6")).not.toBeInTheDocument();

    const bar = screen.getByRole("navigation", { name: "Pagination" });
    await userEvent.click(within(bar).getByRole("button", { name: "Page 3" }));

    // 12 rows at 5 a page is 3 pages, and the last one is short.
    expect(screen.getByText("Movement 11")).toBeInTheDocument();
    expect(rows()).toBe(2);
  });

  it("narrows on a movement type", async () => {
    withActivity();
    await renderApp("/", budi);
    await screen.findByRole("table");

    await userEvent.click(screen.getByRole("combobox", { name: "Movement" }));
    await userEvent.click(screen.getByRole("option", { name: "Adjustments" }));

    expect(screen.getByText("Kardus besar 60×40×40")).toBeInTheDocument();
    expect(screen.queryByText("Sarung tangan kerja")).not.toBeInTheDocument();
  });

  it("offers only filter options that match one of the rows", async () => {
    // Over a fixed set of fifteen, an option that matches nothing is a dead end
    // rather than a filter. Both dropdowns are built from the rows themselves.
    withActivity();
    await renderApp("/", budi);
    await screen.findByRole("table");

    await userEvent.click(screen.getByRole("combobox", { name: "Movement" }));

    // Scoped to the listbox: the pagination bar's page-size `<select>` also
    // contains options, and an unscoped query counts 5/10/25/50/100 as movement
    // types.
    const list = screen.getByRole("listbox", { name: "Movement" });
    expect(
      within(list).getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["All movements", "Receipts", "Adjustments"]);
  });

  it("points at the full ledger when the filters find nothing here", async () => {
    // "No results" over a fifteen-row window is usually not "no results" — it is
    // "not in the last fifteen", and the answer to that is the ledger.
    withActivity();
    await renderApp("/", budi);
    await screen.findByRole("table");

    await userEvent.type(
      screen.getByRole("searchbox", { name: "Search recent activity" }),
      "zzz",
    );

    expect(
      screen.getByText(/Nothing in the last 2 movements matches those filters/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Search the full ledger" }),
    ).toHaveAttribute("href", "/inventory/ledger");
  });

  it("renders timestamps in the tenant's zone, not the browser's", async () => {
    // I7 and FE15. Nusantara is Asia/Jakarta, so the fixture's 04:15Z is 11.15
    // there and would be 05.15 in London.
    withActivity();
    await renderApp("/", budi);

    expect(await screen.findAllByText(/22 Jul 2026, 11\.15/)).not.toHaveLength(0);
  });

  it("explains itself when the server sends no widgets at all", async () => {
    summary(dashboard());
    await renderApp("/", budi);

    expect(
      await screen.findByText(/You have not been given access to any module yet/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("list", { name: "Summary" }),
    ).not.toBeInTheDocument();
  });
});
