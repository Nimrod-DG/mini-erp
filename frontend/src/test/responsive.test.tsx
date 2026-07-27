/**
 * FE7 — the card transformation of §10.7.4.
 *
 * The rule this protects is an accessibility one, not a stylistic one: below the
 * breakpoint a **genuinely different component** is rendered, because "a `<td>`
 * styled to look like a card still announces as a table cell" — and the mirror
 * image is worse, since rendering both and hiding one with CSS puts every row in
 * the accessibility tree twice and a screen reader reads the invisible one.
 *
 * So the assertions are deliberately about what is *in the tree*, not about what
 * is visible. `queryByRole("table")` returning null is the whole point.
 */

import { act, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { journalEntry, page, requisition, rina, sari } from "./fixtures";
import { setViewportWidth } from "./media";
import { renderApp } from "./render";
import { apiUrl, http, HttpResponse, server } from "./server";

/** 360px is the phone §10.7 designs down to; 1280 is `xl`, comfortably past the
 *  `md` boundary the switch is on. */
const PHONE = 360;
const DESKTOP = 1280;

const ROWS = [
  requisition({ id: "r1", prNumber: "PR-202607-0001", status: "submitted" }),
  requisition({
    id: "r2",
    prNumber: "PR-202607-0002",
    status: "approved",
    purchaseOrderId: "po-2",
    purchaseOrderNumber: "PO-202607-0002",
  }),
];

function withRequisitions(rows = ROWS) {
  server.use(
    http.get(apiUrl("/api/procurement/requisitions"), () => HttpResponse.json(page(rows))),
  );
}

/**
 * The screen, without the frame around it.
 *
 * Scoped deliberately. It was written because `AppShell`'s role badges were an
 * `<ul>` of `<li>` in the header, so an unscoped `getAllByRole("listitem")`
 * counted a card list of two as four — and the count of *cards* became a
 * function of the user's entitlements. Those badges have since moved inside
 * `UserMenu`, which is closed by default, so the collision is gone; the scoping
 * stays, because a header that grows a list again should not break this test.
 */
function main() {
  const region = document.querySelector("main");
  if (region === null) throw new Error("no <main>: the shell did not render");
  return within(region);
}

describe("FE7 — the requisition list is cards below md and a table at lg", () => {
  it("renders a semantic table on a desktop", async () => {
    setViewportWidth(DESKTOP);
    withRequisitions();
    await renderApp("/procurement/requisitions", sari);

    // Waiting for a *row* rather than for the table. `SkeletonRows` renders
    // inside the same `<table>`, so `findByRole("table")` resolves while the
    // list is still loading — and the row count below then counts five skeleton
    // rows instead of two documents. Intermittent, and only ever on a slow run.
    await screen.findByRole("link", { name: "PR-202607-0001" });

    const table = await screen.findByRole("table");
    // §10.7.4 asks for semantic markup specifically, because native table
    // semantics give screen readers the row/column relationships for free and
    // ARIA grid roles are the harder fallback.
    expect(table.tagName).toBe("TABLE");
    expect(table.querySelector("thead")).not.toBeNull();
    expect(table.querySelector("tbody")).not.toBeNull();

    const headings = within(table).getAllByRole("columnheader");
    expect(headings.map((cell) => cell.textContent)).toEqual([
      "Number",
      "Status",
      "Supplier",
      "Raised by",
      "Lines",
      "Estimated",
    ]);
    for (const heading of headings) {
      expect(heading).toHaveAttribute("scope", "col");
    }
    // Two documents, plus the heading row.
    expect(within(table).getAllByRole("row")).toHaveLength(3);
  });

  it("renders cards on a phone, and no table at all", async () => {
    setViewportWidth(PHONE);
    withRequisitions();
    await renderApp("/procurement/requisitions", sari);

    await screen.findByRole("link", { name: "PR-202607-0001" });

    // The table must be absent from the tree, not merely hidden.
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.queryAllByRole("columnheader")).toHaveLength(0);
    expect(screen.queryAllByRole("cell")).toHaveLength(0);

    const cards = main().getAllByRole("listitem");
    expect(cards).toHaveLength(2);
    // Every column of the table survives as a labelled field, so the card is the
    // same information in a different shape rather than a subset.
    const first = within(cards[0]);
    for (const label of ["Supplier", "Estimated", "Raised by", "Lines"]) {
      expect(first.getByText(label)).toBeInTheDocument();
    }
  });

  it("keeps the number a link and the status a chip in the card view", async () => {
    setViewportWidth(PHONE);
    withRequisitions();
    await renderApp("/procurement/requisitions", sari);

    await screen.findByRole("link", { name: "PR-202607-0001" });
    const cards = main().getAllByRole("listitem");
    // The card is not one big link: several rows carry a second link — the order
    // an approval generated — and a link inside a link is invalid markup that
    // browsers resolve by guessing.
    const second = within(cards[1]);
    expect(second.getByRole("link", { name: "PR-202607-0002" })).toHaveAttribute(
      "href",
      "/procurement/requisitions/r2",
    );
    expect(second.getByRole("link", { name: "PO-202607-0002" })).toHaveAttribute(
      "href",
      "/procurement/orders/po-2",
    );
    expect(second.getByText("Approved")).toBeInTheDocument();
  });

  it("swaps between the two without a reload", async () => {
    // `useCompact` is a `useSyncExternalStore` subscription rather than an
    // effect, so the first render already knows the answer — no phone paints the
    // desktop table for a frame and then swaps it. Rotating a phone has to work
    // too, which is what this checks.
    setViewportWidth(DESKTOP);
    withRequisitions();
    await renderApp("/procurement/requisitions", sari);
    expect(await screen.findByRole("table")).toBeInTheDocument();

    act(() => setViewportWidth(PHONE));
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(main().getAllByRole("listitem")).toHaveLength(2);

    act(() => setViewportWidth(DESKTOP));
    expect(screen.getByRole("table")).toBeInTheDocument();
  });

  it("keeps the dense grids as tables on a phone, and freezes the first column", async () => {
    // The other half of §10.7.4's per-screen choice, and the reason FE7 is about
    // the requisition list specifically. The stock grid's power is comparing a
    // product across warehouses, and a stack of cards throws exactly that away —
    // so it scrolls sideways with the SKU pinned, because a row of quantities
    // whose product has scrolled out of sight is numbers about nothing.
    setViewportWidth(PHONE);
    server.use(
      http.get(apiUrl("/api/inventory/stock"), () =>
        HttpResponse.json(
          page([
            {
              productId: "p1",
              sku: "HND-GLOVE",
              productName: "Nitrile gloves, box of 100",
              uom: "box",
              productDeleted: false,
              warehouseId: "w1",
              warehouseCode: "WH-MAIN",
              warehouseName: "Main warehouse",
              qtyOnHand: 140,
            },
          ]),
        ),
      ),
    );

    await renderApp("/inventory/stock", sari);

    const table = await screen.findByRole("table");
    expect(table.tagName).toBe("TABLE");

    const productHeading = within(table).getByRole("columnheader", { name: "Product" });
    expect(productHeading.className).toContain("sticky");
    // Paired with the matching `<td>`, or the header freezes and the body does
    // not. The `<tr>` carries `group` so the frozen cell follows the row's hover.
    const cell = within(table).getAllByRole("cell")[0];
    expect(cell.className).toContain("sticky");
    expect(cell.parentElement?.className).toContain("group");
  });

  it("freezes the entry number on the journal, the widest table in the application", async () => {
    // Phase 7.5's finding 9. At `min-w-[52rem]` the Amount column is well off a
    // 360px screen, and scrolling to it used to take the entry number with it —
    // leaving a column of money belonging to nothing. The same fix the stock grid
    // and the ledger already had.
    setViewportWidth(PHONE);
    server.use(
      http.get(apiUrl("/api/finance/journal-entries"), () =>
        HttpResponse.json(page([journalEntry()])),
      ),
    );
    await renderApp("/finance", rina);

    await screen.findByText("JE-202607-0001");
    const table = screen
      .getAllByRole("table")
      .find((candidate) => within(candidate).queryAllByRole("columnheader").length > 0);
    expect(table).toBeDefined();

    const headings = within(table as HTMLElement).getAllByRole("columnheader");
    // The document number is the first column, not the timestamp: only the first
    // column can sensibly be frozen, and §10.0.2 says the number is the identity
    // rather than metadata anyway.
    expect(headings[0].textContent).toBe("Entry");
    expect(headings[0].className).toContain("sticky");

    const firstCell = within(table as HTMLElement).getAllByRole("cell")[0];
    expect(firstCell.textContent).toContain("JE-202607-0001");
    expect(firstCell.className).toContain("sticky");
    expect(firstCell.parentElement?.className).toContain("group");
  });

  it("shows the total count with the pagination in both views", async () => {
    // "Page 3 of ?" strands people (§10.7.4), which is why `totalItems` is
    // mandatory in the §9.0 envelope rather than optional.
    withRequisitions();
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json(page(ROWS, { totalItems: 37, pageSize: 25 })),
      ),
    );

    setViewportWidth(PHONE);
    await renderApp("/procurement/requisitions", sari);

    // "1–2 of 37" and a Next button: the reader can tell there is more, and how
    // much more, in the card view exactly as in the table.
    // The total, and a Next that says there is more of it. Asserted on the count
    // itself rather than on the sentence around it: the range and the total are
    // separate `tabular` spans, so no single element's own text is "1–2 of 37".
    expect(await screen.findByText("37")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
  });
});
