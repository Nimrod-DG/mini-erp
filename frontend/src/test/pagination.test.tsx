/**
 * The pagination contract, at both levels it has one.
 *
 * §10.7.4 asks for a visible total and a reachable end. The backend has always
 * supported `?page=&pageSize=` (`httpx.ParseList`), but until now the frontend
 * sent neither the size nor anything but Previous/Next — so a forty-page list
 * had its last row forty presses away, and the page size the server clamps to
 * 100 was a parameter nobody could reach.
 *
 * The two halves are tested separately on purpose. `pageWindow` is arithmetic
 * with edge cases at both ends and is worth pinning directly; everything else is
 * about what a request carries after a click, which only a rendered screen can
 * answer.
 */

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { DEFAULT_PAGE_SIZE, pageWindow } from "../lib/pagination";
import { page, product, rina } from "./fixtures";
import { renderApp } from "./render";
import { apiUrl, http, HttpResponse, server } from "./server";

// ---------------------------------------------------------------------------
// FE27 — the window arithmetic.
// ---------------------------------------------------------------------------

describe("FE27 — the page window is bounded, and both ends stay reachable", () => {
  it("shows every page when there are few enough to fit", () => {
    expect(pageWindow(1, 1)).toEqual([1]);
    expect(pageWindow(3, 7)).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it("never draws more than seven buttons, however long the list", () => {
    // Seven 44px targets plus two arrows is what fits at 360px. A window that
    // grew with the list would wrap onto a second line and push the table up.
    for (const current of [1, 2, 5, 50, 499, 500]) {
      const drawn = pageWindow(current, 500).filter((entry) => entry !== "gap");
      expect(drawn.length).toBeLessThanOrEqual(7);
    }
  });

  it("always offers the first and the last page", () => {
    // The whole reason for numbered buttons: the oldest record in a list sorted
    // by date is on the last page, and Next alone makes it 499 presses away.
    for (const current of [1, 4, 250, 500]) {
      const drawn = pageWindow(current, 500);
      expect(drawn).toContain(1);
      expect(drawn).toContain(500);
    }
  });

  it("marks the skipped stretches, and only those", () => {
    expect(pageWindow(1, 20)).toEqual([1, 2, 3, 4, "gap", 20]);
    expect(pageWindow(10, 20)).toEqual([1, "gap", 9, 10, 11, "gap", 20]);
    expect(pageWindow(20, 20)).toEqual([1, "gap", 17, 18, 19, 20]);

    // 8 is the first total that cannot be drawn whole, and the one place an
    // off-by-one would put a "…" over a single hidden page.
    expect(pageWindow(1, 8)).toEqual([1, 2, 3, 4, "gap", 8]);
  });

  it("keeps the current page inside the window wherever it is", () => {
    for (let current = 1; current <= 20; current++) {
      expect(pageWindow(current, 20)).toContain(current);
    }
  });
});

// ---------------------------------------------------------------------------
// FE28 — the controls, against a real screen.
// ---------------------------------------------------------------------------

/** Every request the product list made, in order. */
let requests: URL[] = [];

/** A products endpoint holding `totalItems` rows, which honours `page` and
 *  `pageSize` the way `httpx.ParseList` does. */
function productsTotalling(totalItems: number) {
  server.use(
    http.get(apiUrl("/api/inventory/products"), ({ request }) => {
      const url = new URL(request.url);
      requests.push(url);

      const pageSize = Number(url.searchParams.get("pageSize") ?? DEFAULT_PAGE_SIZE);
      const current = Number(url.searchParams.get("page") ?? 1);
      const offset = (current - 1) * pageSize;
      const rows = Array.from(
        { length: Math.max(0, Math.min(pageSize, totalItems - offset)) },
        (_, i) =>
          product({
            id: `44444444-4444-4444-8444-${String(offset + i).padStart(12, "0")}`,
            sku: `SKU-${String(offset + i + 1).padStart(4, "0")}`,
          }),
      );

      return HttpResponse.json(
        page(rows, { page: current, pageSize, totalItems }),
      );
    }),
  );
}

/** The last request's value for a parameter. `page` is omitted at 1 by
 *  `lib/api.ts`, which is why the absent case reads as "1" here. */
function lastRequest(param: "page" | "pageSize"): string {
  const url = requests[requests.length - 1];
  return url.searchParams.get(param) ?? (param === "page" ? "1" : "");
}

function pagination() {
  return screen.getByRole("navigation", { name: "Pagination" });
}

/** Scoped to the pagination bar, because the product list also carries a "Show
 *  deleted" checkbox and a bare /Show/ matches both. */
function sizePicker() {
  return within(pagination()).getByRole("combobox");
}

beforeEach(() => {
  requests = [];
});

describe("FE28 — the page size is a control, and it reaches the server", () => {
  it("asks for a small first page, and says so in the picker", async () => {
    productsTotalling(132);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    // Five, deliberately not `httpx.DefaultPageSize`'s 25 — see the note on
    // DEFAULT_PAGE_SIZE. The size is always sent rather than left to the
    // server's default, which is what lets the picker show a value on first
    // paint instead of a blank until the response lands.
    expect(DEFAULT_PAGE_SIZE).toBe(5);
    expect(lastRequest("pageSize")).toBe("5");
    expect(sizePicker()).toHaveValue("5");

    // The name comes from the words around the control — "Show [25] entries" —
    // rather than an aria-label that says something else. WCAG 2.5.3: what is
    // announced has to contain what is written, or voice control cannot name it.
    expect(sizePicker()).toHaveAccessibleName("Show entries");
  });

  it("sends the chosen size, and goes back to page 1 when it changes", async () => {
    productsTotalling(132);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    await userEvent.click(within(pagination()).getByRole("button", { name: "Page 3" }));
    await screen.findByText("SKU-0011");
    expect(lastRequest("page")).toBe("3");

    await userEvent.selectOptions(sizePicker(), "50");
    await screen.findByText("SKU-0001");

    // 132 rows is 27 pages at 5 and 3 pages at 50, so page 3 survives here by
    // luck and page 20 would not have. Resetting is the only answer that cannot
    // strand the reader, and it belongs in the hook rather than in each screen.
    expect(lastRequest("pageSize")).toBe("50");
    expect(lastRequest("page")).toBe("1");
  });

  it("offers no size larger than the server will honour", async () => {
    // `httpx.MaxPageSize` is 100 and the server clamps silently. An option for
    // 250 would be a control that appears to work and does not.
    productsTotalling(132);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    const sizes = within(sizePicker())
      .getAllByRole("option")
      .map((option) => Number(option.textContent));
    expect(Math.max(...sizes)).toBe(100);
  });
});

describe("FE29 — a page is reachable by number, and the reader is told where they are", () => {
  it("names the rows on screen and the total", async () => {
    productsTotalling(132);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    // §10.7.4: the total is mandatory. "Page 3 of ?" strands people, and so does
    // a range with no denominator.
    expect(within(pagination()).getByText(/Showing/)).toHaveTextContent(
      "Showing 1–5 of 132 entries",
    );

    // 132 rows at 5 a page is 27 pages, and the last one is reachable from the
    // first because the window always carries it — this is the whole argument
    // for numbered buttons over Next alone.
    await userEvent.click(within(pagination()).getByRole("button", { name: "Page 27" }));
    await screen.findByText("SKU-0131");

    // The last page is short, and the range says so rather than claiming 135.
    expect(within(pagination()).getByText(/Showing/)).toHaveTextContent(
      "Showing 131–132 of 132 entries",
    );
  });

  it("marks the current page for a screen reader, not only with a colour", async () => {
    productsTotalling(132);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    expect(within(pagination()).getByRole("button", { name: "Page 1" })).toHaveAttribute(
      "aria-current",
      "page",
    );

    await userEvent.click(within(pagination()).getByRole("button", { name: "Page 2" }));
    await screen.findByText("SKU-0006");

    expect(within(pagination()).getByRole("button", { name: "Page 2" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      within(pagination()).getByRole("button", { name: "Page 1" }),
    ).not.toHaveAttribute("aria-current");
  });

  it("keeps Previous and Next named, and disabled at the ends", async () => {
    productsTotalling(132);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    // The glyphs below `sm` are aria-hidden and the word is the accessible name
    // in both layouts, so this assertion holds at 360px as well as at 1280.
    expect(within(pagination()).getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(within(pagination()).getByRole("button", { name: "Next" })).toBeEnabled();

    await userEvent.click(within(pagination()).getByRole("button", { name: "Page 27" }));
    await screen.findByText("SKU-0131");

    expect(within(pagination()).getByRole("button", { name: "Previous" })).toBeEnabled();
    expect(within(pagination()).getByRole("button", { name: "Next" })).toBeDisabled();
  });

  it("still draws the controls on a single page, disabled", async () => {
    productsTotalling(3);
    await renderApp("/inventory/products", rina);
    await screen.findByText("SKU-0001");

    // Hiding them was the first version, and on a nine-row seeded workspace it
    // read as a missing feature: the right-hand half of the bar was empty and
    // nothing said the bar was pagination. A disabled control still says what
    // the thing is.
    expect(within(pagination()).getByText(/Showing/)).toHaveTextContent(
      "Showing 1–3 of 3 entries",
    );
    expect(within(pagination()).getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(within(pagination()).getByRole("button", { name: "Next" })).toBeDisabled();
    expect(within(pagination()).getByRole("button", { name: "Page 1" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("shows nothing at all when there are no rows", async () => {
    // The empty state says what is going on (§10.7.6). "Showing 0–0 of 0"
    // underneath it is noise.
    productsTotalling(0);
    await renderApp("/inventory/products", rina);
    await screen.findByText(/No products yet/);

    expect(
      screen.queryByRole("navigation", { name: "Pagination" }),
    ).not.toBeInTheDocument();
  });
});
