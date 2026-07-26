/**
 * FE22–FE25 — the master-data CRUD loop.
 *
 * §12.5 calls these "the automation of acceptance steps 21–23", which the MVP
 * verified by hand. §9.6.1 is why they exist at all: "a half-built entity —
 * creatable but not editable — is the most common way a demo falls over."
 *
 * The three entities are deliberately not tested through one shared helper. They
 * are the same *scaffolding* — `MasterDataList` — with genuinely different
 * affordances: warehouses and suppliers edit inline, a product edits on a detail
 * route, and FE22's claim is about each list rather than about the component they
 * share.
 */

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { dewi, page, product, rina, sari, supplier, warehouse } from "./fixtures";
import { renderApp } from "./render";
import { apiUrl, failure, http, HttpResponse, server } from "./server";

/** The `<tr>` a piece of text sits in. Async, because every caller is waiting for
 *  the list to arrive first and a synchronous lookup would just be a race. */
async function row(text: string): Promise<HTMLElement> {
  const cell = await screen.findByText(text);
  const tr = cell.closest("tr");
  if (tr === null) throw new Error(`"${text}" is not in a table row`);
  return tr;
}

describe("FE22 — create, edit and delete for an admin; none of them for a viewer", () => {
  describe("warehouses", () => {
    it("offers all three to an inventory admin", async () => {
      server.use(
        http.get(apiUrl("/api/inventory/warehouses"), () =>
          HttpResponse.json(page([warehouse()])),
        ),
      );
      await renderApp("/inventory/warehouses", rina);

      expect(await screen.findByRole("button", { name: "Add warehouse" })).toBeInTheDocument();
      const cells = within(await row("Main warehouse"));
      expect(cells.getByRole("button", { name: "Edit" })).toBeInTheDocument();
      expect(cells.getByRole("button", { name: "Delete" })).toBeInTheDocument();
    });

    it("offers none of them below admin", async () => {
      // Sari holds `inventory: user`. The server refuses each of these
      // independently (§9.5), so their absence is a courtesy — but a screen that
      // showed them would be a screen whose every button fails.
      server.use(
        http.get(apiUrl("/api/inventory/warehouses"), () =>
          HttpResponse.json(page([warehouse()])),
        ),
      );
      await renderApp("/inventory/warehouses", sari);

      await screen.findByText("Main warehouse");
      expect(screen.queryByRole("button", { name: "Add warehouse" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
      // And no recycle bin: `includeDeleted` is module `admin` only (§9.0), so the
      // toggle that sets it is hidden rather than shown and refused.
      expect(screen.queryByLabelText(/Show deleted/)).not.toBeInTheDocument();
    });
  });

  describe("suppliers", () => {
    it("offers all three to a procurement admin", async () => {
      server.use(
        http.get(apiUrl("/api/procurement/suppliers"), () =>
          HttpResponse.json(page([supplier()])),
        ),
      );
      await renderApp("/procurement/suppliers", rina);

      expect(await screen.findByRole("button", { name: "Add supplier" })).toBeInTheDocument();
      const cells = within(await row("Acme Supplies"));
      expect(cells.getByRole("button", { name: "Edit" })).toBeInTheDocument();
      expect(cells.getByRole("button", { name: "Delete" })).toBeInTheDocument();
    });

    it("offers none of them to a procurement viewer", async () => {
      server.use(
        http.get(apiUrl("/api/procurement/suppliers"), () =>
          HttpResponse.json(page([supplier()])),
        ),
      );
      await renderApp("/procurement/suppliers", dewi);

      await screen.findByText("Acme Supplies");
      expect(screen.queryByRole("button", { name: "Add supplier" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
    });
  });

  describe("products", () => {
    it("offers create on the list and edit and delete on the detail route", async () => {
      // Products are the one master-data entity with a detail screen, because a
      // product has a ledger history and per-warehouse balances to show. So the
      // list's edit affordance is the link to that screen, and this is the case
      // FE22 would miss if it only looked for a button called Edit.
      server.use(
        http.get(apiUrl("/api/inventory/products"), () =>
          HttpResponse.json(page([product()])),
        ),
        http.get(apiUrl(`/api/inventory/products/${product().id}`), () =>
          HttpResponse.json({ ...product(), balances: [] }),
        ),
      );
      await renderApp("/inventory/products", rina);

      expect(await screen.findByRole("link", { name: "Add product" })).toBeInTheDocument();
      const link = screen.getByRole("link", { name: "HND-GLOVE" });
      expect(link).toHaveAttribute("href", `/inventory/products/${product().id}`);

      await userEvent.click(link);
      expect(await screen.findByRole("button", { name: "Save changes" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
      // And the third state of §6.9.1, which is not a delete: a discontinued
      // product stays in every report and keeps its stock.
      expect(screen.getByRole("button", { name: "Discontinue" })).toBeInTheDocument();
    });

    it("shows a `user`-level reader the facts and no controls", async () => {
      server.use(
        http.get(apiUrl(`/api/inventory/products/${product().id}`), () =>
          HttpResponse.json({ ...product(), balances: [] }),
        ),
      );
      await renderApp(`/inventory/products/${product().id}`, sari);

      expect(await screen.findByText("SKU")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
    });
  });
});

describe("FE23 — the edit form pre-populates from the detail response", () => {
  const detail = { ...product(), balances: [] };

  function withProduct(patched?: (body: unknown) => void) {
    server.use(
      http.get(apiUrl(`/api/inventory/products/${detail.id}`), () =>
        HttpResponse.json(detail),
      ),
      http.patch(apiUrl(`/api/inventory/products/${detail.id}`), async ({ request }) => {
        patched?.(await request.json());
        return HttpResponse.json(detail);
      }),
    );
  }

  it("fills every field from the response rather than leaving them blank", async () => {
    withProduct();
    await renderApp(`/inventory/products/${detail.id}`, rina);

    expect(await screen.findByLabelText("SKU")).toHaveValue("HND-GLOVE");
    expect(screen.getByLabelText("Name")).toHaveValue("Nitrile gloves, box of 100");
    expect(screen.getByLabelText("Unit of measure")).toHaveValue("box");
    expect(screen.getByLabelText("Reorder point")).toHaveValue("20");
    expect(screen.getByLabelText("Standard cost")).toHaveValue("12.5");
  });

  it("submits only the field that changed", async () => {
    let body: unknown;
    withProduct((sent) => {
      body = sent;
    });
    await renderApp(`/inventory/products/${detail.id}`, rina);

    const cost = await screen.findByLabelText("Standard cost");
    await userEvent.clear(cost);
    await userEvent.type(cost, "13.75");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await screen.findByText("Changes saved.");
    // PATCH treats an absent field as "leave it alone". Sending the other four
    // would put a minute-old `name` over the top of a rename somebody else made
    // in the meantime.
    expect(body).toEqual({ standardCost: "13.75" });
  });

  it("sends nothing at all when nothing was touched", async () => {
    let patches = 0;
    server.use(
      http.get(apiUrl(`/api/inventory/products/${detail.id}`), () =>
        HttpResponse.json(detail),
      ),
      http.patch(apiUrl(`/api/inventory/products/${detail.id}`), () => {
        patches += 1;
        return HttpResponse.json(detail);
      }),
    );
    await renderApp(`/inventory/products/${detail.id}`, rina);

    await screen.findByLabelText("SKU");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(await screen.findByText("Nothing to save — no field was changed.")).toBeInTheDocument();
    expect(patches).toBe(0);
  });

  it("sends the quantity fields as strings, unrounded", async () => {
    // I8 again. `reorderPoint` is NUMERIC(18,4) and the browser must not be the
    // thing that decides how many of those four digits survive.
    let body: unknown;
    withProduct((sent) => {
      body = sent;
    });
    await renderApp(`/inventory/products/${detail.id}`, rina);

    const reorder = await screen.findByLabelText("Reorder point");
    await userEvent.clear(reorder);
    await userEvent.type(reorder, "20.5000");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await screen.findByText("Changes saved.");
    expect(body).toEqual({ reorderPoint: "20.5000" });
  });

  it("pre-populates the inline editors from the row", async () => {
    // Warehouses and suppliers have no detail response to read — the list row is
    // the record — but the same rule applies: an edit that opens blank is an edit
    // that silently clears whatever you do not retype.
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () =>
        HttpResponse.json(page([warehouse()])),
      ),
    );
    await renderApp("/inventory/warehouses", rina);

    await userEvent.click(within(await row("Main warehouse")).getByRole("button", { name: "Edit" }));

    expect(screen.getByLabelText("Warehouse code")).toHaveValue("WH-MAIN");
    expect(screen.getByLabelText("Warehouse name")).toHaveValue("Main warehouse");
  });
});

describe("FE24 — a delete removes the row, and a refused delete does not", () => {
  /**
   * A DELIBERATE DEVIATION FROM FE24's WORDING, recorded in `PROGRESS.md`.
   *
   * §12.5 writes FE24 as "deleting from a list **optimistically** removes the row
   * and restores it if the request fails". This application deletes
   * pessimistically: the button disables, the request goes, and on success the
   * list refetches.
   *
   * The reason is that a refused delete is the *ordinary* case here, not the edge
   * one. A warehouse holding stock is refused with `in_use` (G5) and so is a
   * supplier with open orders (G4) — the seeded demo has both, and the acceptance
   * test walks them. Optimistic removal would make the common outcome a row that
   * vanishes and comes back, which reads as a bug, in exchange for hiding about
   * 200ms of latency in the uncommon one.
   *
   * What FE24 exists to guarantee is unaffected and is what is asserted here: a
   * successful delete takes the row away, and a refused one leaves it exactly
   * where it was with the reason on screen.
   */
  it("takes the row away once the server has agreed", async () => {
    let deleted = false;
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () =>
        HttpResponse.json(page(deleted ? [] : [warehouse()])),
      ),
      http.delete(apiUrl(`/api/inventory/warehouses/${warehouse().id}`), () => {
        deleted = true;
        return HttpResponse.json({ ...warehouse(), deletedAt: "2026-07-26T00:00:00Z" });
      }),
    );
    await renderApp("/inventory/warehouses", rina);

    await userEvent.click(within(await row("Main warehouse")).getByRole("button", { name: "Delete" }));

    // The confirmation says how to undo it, because a soft delete is undoable and
    // a message that does not say so invites a support ticket.
    expect(
      await screen.findByText("WH-MAIN deleted. Tick Show deleted to restore it."),
    ).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Main warehouse")).not.toBeInTheDocument());
  });

  it("leaves the row in place when the delete is refused, and says why", async () => {
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () =>
        HttpResponse.json(page([warehouse()])),
      ),
      http.delete(apiUrl(`/api/inventory/warehouses/${warehouse().id}`), () =>
        failure(409, "in_use", "WH-MAIN still holds stock of 7 products. Move or write it off first."),
      ),
    );
    await renderApp("/inventory/warehouses", rina);

    await userEvent.click(within(await row("Main warehouse")).getByRole("button", { name: "Delete" }));

    // The envelope's sentence, not "Operation failed". Reading the refusal is what
    // turns a button that did nothing into a delete that was correctly refused.
    expect(
      await screen.findByText(
        "WH-MAIN still holds stock of 7 products. Move or write it off first.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Main warehouse")).toBeInTheDocument();
    expect(within(await row("Main warehouse")).getByRole("button", { name: "Delete" })).toBeEnabled();
  });

  it("does not leave the button disabled after a refusal", async () => {
    // The `finally` in `useRowActions`. A refusal that also bricks the button
    // means the reader cannot retry after fixing the cause.
    server.use(
      http.get(apiUrl("/api/procurement/suppliers"), () =>
        HttpResponse.json(page([supplier({ openOrders: 3 })])),
      ),
      http.delete(apiUrl(`/api/procurement/suppliers/${supplier().id}`), () =>
        failure(409, "in_use", "Acme Supplies has 3 open purchase orders. Close or cancel them before deleting."),
      ),
    );
    await renderApp("/procurement/suppliers", rina);

    const button = within(await row("Acme Supplies")).getByRole("button", { name: "Delete" });
    await userEvent.click(button);

    expect(
      await screen.findByText(
        "Acme Supplies has 3 open purchase orders. Close or cancel them before deleting.",
      ),
    ).toBeInTheDocument();
    expect(button).toBeEnabled();
    // The count was on the row before the click, so the refusal is not the first
    // the reader hears of it.
    expect(screen.getByText("3 open orders")).toBeInTheDocument();
  });
});

describe("FE25 — Show deleted reveals soft-deleted rows with a restore action", () => {
  it("asks the server for them rather than filtering a page in the browser", async () => {
    const queries: (string | null)[] = [];
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), ({ request }) => {
        const includeDeleted = new URL(request.url).searchParams.get("includeDeleted");
        queries.push(includeDeleted);
        return HttpResponse.json(
          page(
            includeDeleted === "true"
              ? [warehouse(), warehouse({ id: "w2", code: "WH-OLD", name: "Old depot", deletedAt: "2026-07-01T00:00:00Z" })]
              : [warehouse()],
          ),
        );
      }),
    );
    await renderApp("/inventory/warehouses", rina);

    await screen.findByText("Main warehouse");
    expect(screen.queryByText("Old depot")).not.toBeInTheDocument();

    await userEvent.click(screen.getByLabelText(/Show deleted/));

    expect(await screen.findByText("Old depot")).toBeInTheDocument();
    // A deleted row is marked as such, or the list is two kinds of thing that look
    // identical.
    expect(within(await row("Old depot")).getByText("deleted")).toBeInTheDocument();
    expect(queries).toEqual([undefined, "true"].map((v) => v ?? null));
  });

  it("offers Restore on a deleted row, and only Restore", async () => {
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () =>
        HttpResponse.json(
          page([warehouse({ deletedAt: "2026-07-01T00:00:00Z" })]),
        ),
      ),
    );
    await renderApp("/inventory/warehouses", rina);

    await userEvent.click(screen.getByLabelText(/Show deleted/));
    const cells = within(await row("Main warehouse"));

    expect(await cells.findByRole("button", { name: "Restore" })).toBeInTheDocument();
    // Editing or re-deleting something already in the recycle bin is not an
    // action; there is one thing to do with it.
    expect(cells.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(cells.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("brings the row back", async () => {
    let restored = false;
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () =>
        HttpResponse.json(
          page([warehouse(restored ? {} : { deletedAt: "2026-07-01T00:00:00Z" })]),
        ),
      ),
      http.post(apiUrl(`/api/inventory/warehouses/${warehouse().id}/restore`), () => {
        restored = true;
        return HttpResponse.json(warehouse());
      }),
    );
    await renderApp("/inventory/warehouses", rina);

    await userEvent.click(screen.getByLabelText(/Show deleted/));
    await userEvent.click(await within(await row("Main warehouse")).findByRole("button", { name: "Restore" }));

    expect(await screen.findByText("WH-MAIN restored.")).toBeInTheDocument();
    await waitFor(async () =>
      expect(within(await row("Main warehouse")).queryByText("deleted")).not.toBeInTheDocument(),
    );
  });

  it("reports the refusal when the code was taken while the row was deleted", async () => {
    // G3, and the refusal most worth reading on this screen: there cannot be two
    // live rows holding one code, so a restore can legitimately fail long after
    // the delete succeeded.
    server.use(
      http.get(apiUrl("/api/inventory/warehouses"), () =>
        HttpResponse.json(page([warehouse({ deletedAt: "2026-07-01T00:00:00Z" })])),
      ),
      http.post(apiUrl(`/api/inventory/warehouses/${warehouse().id}/restore`), () =>
        failure(409, "in_use", "Another warehouse now uses WH-MAIN."),
      ),
    );
    await renderApp("/inventory/warehouses", rina);

    await userEvent.click(screen.getByLabelText(/Show deleted/));
    await userEvent.click(await within(await row("Main warehouse")).findByRole("button", { name: "Restore" }));

    expect(await screen.findByText("Another warehouse now uses WH-MAIN.")).toBeInTheDocument();
  });

  it("restores a product from its own detail screen", async () => {
    // The product's recycle bin is on the detail route, because §6.9.1 requires a
    // deleted product to stay *resolvable* — the ledger rows below it link here,
    // and a 404 would make last quarter's movements unreadable.
    const deleted = { ...product(), deletedAt: "2026-07-01T00:00:00Z", balances: [] };
    let restored = false;
    server.use(
      http.get(apiUrl(`/api/inventory/products/${deleted.id}`), () =>
        HttpResponse.json(restored ? { ...product(), balances: [] } : deleted),
      ),
      http.post(apiUrl(`/api/inventory/products/${deleted.id}/restore`), () => {
        restored = true;
        return HttpResponse.json({ ...product(), balances: [] });
      }),
      http.get(apiUrl("/api/inventory/ledger"), () => HttpResponse.json(page([]))),
    );
    await renderApp(`/inventory/products/${deleted.id}`, rina);

    expect(
      await screen.findByText(/This product is deleted\./),
    ).toBeInTheDocument();
    // And no edit form while it is deleted: the offer is Restore, not Save.
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Restore" }));

    expect(await screen.findByText("Restored.")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByText(/This product is deleted\./)).not.toBeInTheDocument(),
    );
  });
});
