/**
 * FE31 — the filter row: one search box and the dropdowns beside it.
 *
 * `FilterDropdown` is the first control in this application that is a custom
 * widget rather than a native element, and that is the whole reason this file
 * exists. A `<select>` gets its keyboard behaviour, its focus handling and its
 * `role` from the platform for free; a `<button>` plus a `<ul>` gets none of
 * them, and every one has to be written out and then held in place. So the
 * assertions here are mostly about the parts a native element would have given
 * us — the roles, the keyboard, where the focus ends up — rather than about the
 * filtering, which is the easy half.
 *
 * What it buys is the open state matching the rest of the application in both
 * themes, which an OS-drawn popup cannot.
 */

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { page, rina, supplier } from "./fixtures";
import { renderApp } from "./render";
import { apiUrl, http, HttpResponse, server } from "./server";

/** Every requisition request the screen made. */
let requests: URL[] = [];

const ACME = "55555555-5555-4555-8555-555555555551";
const BUDI_CO = "55555555-5555-4555-8555-555555555552";

function requisitionScreen() {
  server.use(
    http.get(apiUrl("/api/procurement/requisitions"), ({ request }) => {
      requests.push(new URL(request.url));
      return HttpResponse.json(page([]));
    }),
    http.get(apiUrl("/api/procurement/suppliers"), () =>
      HttpResponse.json(
        page([
          supplier({ id: ACME, code: "SUP-ACME", name: "Acme Supplies" }),
          supplier({ id: BUDI_CO, code: "SUP-BUDI", name: "Budi Trading" }),
        ]),
      ),
    ),
  );
}

function last(param: string): string | null {
  return requests[requests.length - 1]?.searchParams.get(param) ?? null;
}

function dropdown(name: string) {
  return screen.getByRole("combobox", { name });
}

beforeEach(() => {
  requests = [];
});

describe("FE31 — the search box and the dropdowns are one row of controls", () => {
  it("gives the search box a name that says what it searches", async () => {
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    // The visible "Search" caption is gone — it forced the filter row two lines
    // tall and said nothing the placeholder did not. The accessible name is the
    // half that was doing work, and it names the *noun*: a screen reader user
    // landing here has no column headings for context, so "Search" alone would
    // be a control that could be searching anything.
    const box = screen.getByRole("searchbox", { name: "Search requisitions" });
    expect(box).toHaveAttribute("placeholder", "Number, supplier, or notes");

    await userEvent.type(box, "gloves");
    expect(last("q")).toBe("gloves");
  });

  it("sends the chosen status and supplier to the server, not to a .filter()", async () => {
    // The rule the removed chips existed to enforce. Filtering the fetched page
    // would report "3 drafts" for a tenant with forty, because only one page
    // arrived — the count in the pagination line and the rows in the table have
    // to answer the same question.
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    await userEvent.click(dropdown("Status"));
    await userEvent.click(screen.getByRole("option", { name: "Approved" }));
    expect(last("status")).toBe("approved");

    await userEvent.click(dropdown("Supplier"));
    await userEvent.click(
      screen.getByRole("option", { name: "SUP-ACME — Acme Supplies" }),
    );
    expect(last("supplierId")).toBe(ACME);
    // Both at once: choosing a supplier must not quietly drop the status.
    expect(last("status")).toBe("approved");
  });

  it("puts the filter back to everything from its own first row", async () => {
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    await userEvent.click(dropdown("Status"));
    await userEvent.click(screen.getByRole("option", { name: "Draft" }));
    expect(last("status")).toBe("draft");

    await userEvent.click(dropdown("Status"));
    await userEvent.click(screen.getByRole("option", { name: "All statuses" }));

    // Absent, not `status=`: an empty parameter and no parameter are the same
    // thing to the handler, but only one of them is what an unfiltered list
    // should send.
    expect(last("status")).toBeNull();
    expect(dropdown("Status")).toHaveTextContent("All statuses");
  });

  it("declares the roles a native select would have declared", async () => {
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    const trigger = dropdown("Supplier");
    expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await userEvent.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");

    const list = screen.getByRole("listbox", { name: "Supplier" });
    // The two suppliers plus the "all" row the component owns.
    expect(within(list).getAllByRole("option")).toHaveLength(3);
    expect(
      within(list).getByRole("option", { name: "All suppliers" }),
    ).toHaveAttribute("aria-selected", "true");
  });

  it("opens, moves and picks from the keyboard alone", async () => {
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    const trigger = dropdown("Status");
    trigger.focus();

    await userEvent.keyboard("{ArrowDown}");
    expect(trigger).toHaveAttribute("aria-expanded", "true");

    // Focus never leaves the trigger — `aria-activedescendant` is what moves,
    // which is why the highlight has to be readable from the attribute.
    expect(trigger).toHaveFocus();

    await userEvent.keyboard("{ArrowDown}{Enter}");

    // One step down from "All statuses" is the first real status.
    expect(last("status")).toBe("draft");
    expect(trigger).toHaveTextContent("Draft");
  });

  it("closes on Escape without choosing, and gives the focus back", async () => {
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    const trigger = dropdown("Status");
    await userEvent.click(trigger);
    await userEvent.keyboard("{ArrowDown}{Escape}");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    // Escape is a cancel, so the highlighted row must not have been applied.
    expect(last("status")).toBeNull();
    expect(trigger).toHaveTextContent("All statuses");
  });

  it("opens on the current value rather than at the top", async () => {
    // Arrowing once from "All suppliers" when Acme is chosen would be a silent
    // jump to a neighbour of a row the reader was not on.
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    await userEvent.click(dropdown("Supplier"));
    await userEvent.click(
      screen.getByRole("option", { name: "SUP-BUDI — Budi Trading" }),
    );

    const trigger = dropdown("Supplier");
    await userEvent.click(trigger);

    const active = trigger.getAttribute("aria-activedescendant");
    expect(document.getElementById(active ?? "")).toHaveTextContent(
      "SUP-BUDI — Budi Trading",
    );
  });

  it("closes when something else is clicked", async () => {
    requisitionScreen();
    await renderApp("/procurement/requisitions", rina);

    await userEvent.click(dropdown("Status"));
    expect(screen.getByRole("listbox")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("heading", { level: 1 }));

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
});
