/**
 * FE12, FE13 and FE26 — the three presentation rules the design direction makes
 * hard requirements rather than preferences.
 *
 * FE12 is an accessibility floor (§10.8.4: "never encode meaning in colour
 * alone"). FE26 is §10.0.2's claim that alignment discipline is where an ERP gets
 * its perceived quality. FE13 is §10.7.6's reason for using skeletons instead of a
 * spinner at all.
 */

import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { SkeletonRows } from "../components/ListStates";
import { StatusChip } from "../components/StatusChip";
import type { DocumentStatus } from "../lib/format";
import { statusLabel } from "../lib/format";
import { journalEntry, ledgerEntry, page, requisition, rina, warehouse } from "./fixtures";
import { paddingTokens, TEXT_SM_LINE_PX, declaredHeightPx } from "./layout";
import { renderApp } from "./render";
import { apiUrl, http, HttpResponse, server } from "./server";

/** Every status in the naming contract: five for a requisition, four for an
 *  order, with `cancelled` shared. */
const ALL_STATUSES: DocumentStatus[] = [
  "draft",
  "submitted",
  "approved",
  "rejected",
  "cancelled",
  "open",
  "partially_received",
  "received",
];

describe("FE12 — every status badge carries text, not only colour", () => {
  it("renders a word for all eight statuses", () => {
    render(
      <>
        {ALL_STATUSES.map((status) => (
          <StatusChip key={status} status={status} />
        ))}
      </>,
    );

    for (const status of ALL_STATUSES) {
      // The accessible name is the word. A reader who cannot distinguish the
      // amber from the green still reads "Submitted" and "Received", which is
      // §10.8.4's hard requirement — and also the honest design, because "the
      // green one" is not a status.
      expect(screen.getByText(statusLabel(status))).toBeInTheDocument();
    }
  });

  it("says `partially_received` in English while the wire value stays exact", () => {
    // §3's naming contract fixes the wire value, and `statusLabel` is the only
    // place it is allowed to read as prose — so two screens cannot call the same
    // state different things.
    render(<StatusChip status="partially_received" />);

    expect(screen.getByText("Partly received")).toBeInTheDocument();
    expect(screen.queryByText("partially_received")).not.toBeInTheDocument();
  });

  it("never renders a chip whose only content is a colour", () => {
    const { container } = render(
      <>
        {ALL_STATUSES.map((status) => (
          <StatusChip key={status} status={status} />
        ))}
      </>,
    );

    const chips = [...container.querySelectorAll("span")];
    expect(chips).toHaveLength(ALL_STATUSES.length);
    for (const chip of chips) {
      // A chip carries a colour class *and* a word. The assertion is on the word,
      // because that is the half that can be dropped without the design looking
      // broken.
      expect(chip.className).toMatch(/text-(secondary|warning|success|danger|accent)/);
      expect(chip.textContent?.trim()).not.toBe("");
    }
  });

  it("names the chosen status in the filter rather than tinting it", async () => {
    // The same rule one level up, and it survived the chips. The filter is a
    // dropdown now, so which status is active is carried by the *word in the
    // trigger* and by `aria-selected` on the option — not by an accent tint on
    // one of six pills. FE31 covers the control itself; this asserts the FE12
    // half of it, that the state is legible without colour.
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json(page([])),
      ),
    );
    await renderApp("/procurement/requisitions", rina);

    const filter = await screen.findByRole("combobox", { name: "Status" });
    expect(filter).toHaveTextContent("All statuses");

    await userEvent.click(filter);
    await userEvent.click(screen.getByRole("option", { name: "Submitted" }));

    expect(filter).toHaveTextContent("Submitted");
  });

  it("keeps the word in the card view as well as the table", async () => {
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json(page([requisition({ status: "partially_received" as never })])),
      ),
    );
    await renderApp("/procurement/requisitions", rina);

    // Whichever view is rendered, the status is a word — the chip is shared, and
    // this is what stops a card view growing its own vocabulary.
    expect(await screen.findByText("Partly received")).toBeInTheDocument();
  });
});

describe("FE13 — skeleton rows do not move the layout when data arrives", () => {
  /**
   * WHAT THIS CAN AND CANNOT CLAIM. jsdom has no layout engine and Tailwind is
   * never compiled during a unit test, so nothing here measures rendered pixels —
   * see `layout.ts`. What is asserted is the three things that decide whether a
   * row changes size, read off the markup:
   *
   *   1. the same number of cells, so nothing moves sideways;
   *   2. the same cell padding, so the box around the content is identical;
   *   3. a placeholder bar exactly one `text-sm` line box tall, so a single-line
   *      cell is the same height loading as loaded.
   *
   * A cell with a second line under it — a SKU with a product name, a quantity
   * with a unit count — is taller than the skeleton by that second line, and
   * deliberately so: matching it would mean the skeleton knowing each screen's
   * cell shapes, which is exactly the coupling `ListStates` refuses (it shares the
   * heading row and never the cells). The real pixels are the browser walk's job.
   */
  it("stands a placeholder exactly one line of text-sm tall", () => {
    const { container } = render(
      <table>
        <SkeletonRows cols={4} />
      </table>,
    );

    const bars = [...container.querySelectorAll("td > div")];
    expect(bars).toHaveLength(4 * 5);
    for (const bar of bars) {
      expect(declaredHeightPx(bar)).toBe(TEXT_SM_LINE_PX);
    }
  });

  it("uses the same cell count and padding as the row it is standing in for", async () => {
    // Held in flight so the skeleton and the loaded row are both observable in one
    // test, against one real screen rather than a fixture of my own shape.
    let release: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), async () => {
        await held;
        return HttpResponse.json(page([warehouse()]));
      }),
    );

    await renderApp("/inventory/warehouses", rina);

    const table = await screen.findByRole("table");
    const skeletonRows = within(table).getAllByRole("row").slice(1);
    expect(skeletonRows).toHaveLength(5);
    const skeletonCells = [...skeletonRows[0].querySelectorAll("td")];

    release?.();

    await waitFor(() => expect(screen.getByText("Main warehouse")).toBeInTheDocument());
    const loadedCells = [...within(table).getAllByRole("row")[1].querySelectorAll("td")];

    expect(skeletonCells).toHaveLength(loadedCells.length);
    for (let index = 0; index < loadedCells.length; index += 1) {
      expect(paddingTokens(skeletonCells[index])).toEqual(
        paddingTokens(loadedCells[index]),
      );
    }
  });

  it("renders a skeleton rather than a spinner while a list loads", async () => {
    // §10.7.6 asks for skeletons *instead of* a spinner. A spinner tells the
    // reader to wait; a skeleton tells them what is coming and holds its place.
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () => new Promise(() => {})),
    );

    await renderApp("/inventory/warehouses", rina);

    const table = await screen.findByRole("table");
    expect(within(table).getAllByRole("row")).toHaveLength(6);
    expect(table.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});

describe("FE26 — numeric columns are right-aligned and tabular", () => {
  /**
   * §10.0.2: "Set every numeric column in a tabular-figure face so digits align
   * vertically. Right-align numerics, left-align text. This single decision does
   * more for perceived quality in an ERP than any amount of decoration, and most
   * templates get it wrong."
   *
   * The check is driven off the *headings*: `Column.align === "right"` is the only
   * alignment variant in the design system, and it exists because the only columns
   * that are not left-aligned are numbers. So a right-aligned heading is a
   * declaration that the column below it is numeric, and every cell under one has
   * to be both right-aligned and tabular. Reading it this way means a new numeric
   * column is covered the day it is added, without this test naming any column.
   */
  /**
   * The table, once its data has actually arrived.
   *
   * `findByRole("table")` on its own resolves immediately, because `SkeletonRows`
   * renders *inside* that same `<table>` — and a skeleton cell is a plain
   * `px-3 py-3` with no `text-right` and no `tabular`, so every assertion below
   * fails against it. That made these four an intermittent failure: green when
   * MSW answered inside the first render, red when it did not. Waiting for the
   * pulse to go is waiting for the real cells.
   */
  async function findLoadedTable(): Promise<HTMLElement> {
    const table = await screen.findByRole("table");
    await waitFor(() => expect(table.querySelector(".animate-pulse")).toBeNull());
    return table;
  }

  function assertNumericColumns(table: HTMLElement) {
    const headings = within(table).getAllByRole("columnheader");
    const numeric = headings
      .map((heading, index) => ({ heading, index }))
      .filter(({ heading }) => heading.className.includes("text-right"));

    expect(numeric.length).toBeGreaterThan(0);

    const bodyRows = within(table)
      .getAllByRole("row")
      .filter((row) => row.querySelector("td") !== null);
    expect(bodyRows.length).toBeGreaterThan(0);

    for (const row of bodyRows) {
      const cells = [...row.querySelectorAll("td")];
      // An empty-state row spans the table and is prose, not data.
      if (cells.length !== headings.length) continue;

      for (const { heading, index } of numeric) {
        const cell = cells[index];
        const label = heading.textContent;
        expect(cell.className, `"${label}" cell is right-aligned`).toContain(
          "text-right",
        );
        // `tabular` is the project's own utility for
        // `font-variant-numeric: tabular-nums`. It may sit on the cell or on the
        // span holding the digits, because several of these cells put a unit or a
        // currency beside the number in secondary text.
        const tabular =
          cell.className.includes("tabular") ||
          cell.querySelector(".tabular") !== null;
        expect(tabular, `"${label}" cell renders digits as tabular-nums`).toBe(true);
      }
    }
  }

  it("holds on the requisition list", async () => {
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json(page([requisition(), requisition({ id: "r2", prNumber: "PR-202607-0002" })])),
      ),
    );
    await renderApp("/procurement/requisitions", rina);

    assertNumericColumns(await findLoadedTable());
  });

  it("holds on the stock grid, including the frozen column", async () => {
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
    await renderApp("/inventory/stock", rina);

    assertNumericColumns(await findLoadedTable());
  });

  it("holds on the stock ledger, where the sign is the content", async () => {
    server.use(
      http.get(apiUrl("/api/inventory/ledger"), () =>
        HttpResponse.json(
          page([ledgerEntry(), ledgerEntry({ id: "l2", qtyDelta: -12, entryType: "issue" })]),
        ),
      ),
    );
    await renderApp("/inventory/ledger", rina);

    assertNumericColumns(await findLoadedTable());
  });

  it("holds on the journal, where debits and credits have to line up", async () => {
    // The one place in the application where two columns of money are read
    // against each other. If tabular figures matter anywhere, it is here.
    server.use(
      http.get(apiUrl("/api/finance/journal-entries"), () =>
        HttpResponse.json(page([journalEntry()])),
      ),
    );
    await renderApp("/finance", rina);

    // The journal's own table, loaded — same reason as `findLoadedTable`. The
    // page renders a nested table of lines per entry, hence the sweep.
    await findLoadedTable();

    const tables = await screen.findAllByRole("table");
    for (const table of tables) {
      if (within(table).queryAllByRole("columnheader").length === 0) continue;
      assertNumericColumns(table);
    }
  });

  it("keeps numbers right-aligned and tabular in the card view too", async () => {
    server.use(
      http.get(apiUrl("/api/procurement/requisitions"), () =>
        HttpResponse.json(page([requisition()])),
      ),
    );
    const { setViewportWidth } = await import("./media");
    setViewportWidth(360);
    await renderApp("/procurement/requisitions", rina);

    // A reader moving between the table and the cards is reading the same shape,
    // which is why `CardField.align` exists at all.
    const estimated = (await screen.findByText("Estimated")).parentElement;
    expect(estimated?.className).toContain("text-right");
    expect(estimated?.querySelector("dd")?.className).toContain("tabular");
  });
});
