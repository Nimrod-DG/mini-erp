/**
 * FE3, FE4, FE5 and FE9 — the goods receipt screen.
 *
 * This is the one screen §10.7.1 calls genuinely mobile, and §10.3's confirmation
 * panel is the thesis of the whole application. It is also the flow Phase 7.5
 * found unreachable from a browser while every server-side test was green, which
 * is the calibration for why these four are worth having.
 */

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import {
  budi,
  PO_LINE_1,
  PO_LINE_2,
  PO_OPEN,
  poLine,
  purchaseOrder,
  receiptResult,
  sari,
} from "./fixtures";
import { formatMoney } from "../lib/format";
import { declaredHeightPx, declaredWidthPx, describe as describeEl } from "./layout";
import { renderApp } from "./render";
import { apiUrl, failure, http, HttpResponse, server } from "./server";

const RECEIVE_PATH = `/procurement/orders/${PO_OPEN}/receive`;

/** Line 1: 100 of 100 outstanding. Line 2: 30 of 40, ten already received. */
function order(overrides = {}) {
  const po = purchaseOrder(overrides);
  server.use(
    http.get(apiUrl(`/api/procurement/purchase-orders/${po.id}`), () =>
      HttpResponse.json(po),
    ),
  );
  return po;
}

function quantityBox(lineNo: number): HTMLInputElement {
  return screen.getByLabelText(`Quantity received for line ${lineNo}`) as HTMLInputElement;
}

function postButton(): HTMLButtonElement {
  return screen.getByRole("button", { name: /Post receipt|Posting/ }) as HTMLButtonElement;
}

describe("FE4 — the receipt form pre-fills outstanding quantities", () => {
  beforeEach(() => order());

  it("defaults every box to what is still outstanding on its line", async () => {
    await renderApp(RECEIVE_PATH, budi);

    // A full delivery is the overwhelmingly common case, so the form opens ready
    // to record one — while leaving every box editable for a partial.
    expect(await screen.findByLabelText("Quantity received for line 1")).toHaveValue("100");
    // 40 ordered, 10 already received. The outstanding quantity is derived from
    // `po_line_status`, never stored (I6), and the form must show the derived
    // number rather than what was ordered.
    expect(quantityBox(2)).toHaveValue("30");
  });

  it("shows the outstanding quantity beside each box, so the default is checkable", async () => {
    await renderApp(RECEIVE_PATH, budi);
    await screen.findByLabelText("Quantity received for line 1");

    const rows = screen.getAllByRole("row");
    const lineTwo = rows.find((row) => row.textContent?.includes("PKG-BOX-L"));
    expect(lineTwo).toBeDefined();
    expect(within(lineTwo as HTMLElement).getByText("30")).toBeInTheDocument();
  });

  it("omits a line that has nothing left outstanding", async () => {
    // A fully received line is not a box to leave blank, it is a line that is
    // finished. Leaving it on the form invites an over-receipt the server would
    // refuse.
    order({
      lines: [
        poLine({ qtyOrdered: 100, qtyReceived: 100, qtyOutstanding: 0 }),
        poLine({
          id: PO_LINE_2,
          lineNo: 2,
          sku: "PKG-BOX-L",
          qtyOrdered: 40,
          qtyReceived: 10,
          qtyOutstanding: 30,
        }),
      ],
    });

    await renderApp(RECEIVE_PATH, budi);

    expect(await screen.findByLabelText("Quantity received for line 2")).toBeInTheDocument();
    expect(screen.queryByLabelText("Quantity received for line 1")).not.toBeInTheDocument();
  });

  it("sends the box values as strings, unrounded", async () => {
    // I8, at the boundary. A quantity that passes through a JavaScript number on
    // the way out has already lost whatever it is going to lose before the
    // server's NUMERIC(18,4) ever sees it.
    let body: unknown;
    server.use(
      http.post(apiUrl(`/api/procurement/purchase-orders/${PO_OPEN}/receipts`), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(receiptResult());
      }),
    );

    await renderApp(RECEIVE_PATH, budi);
    const box = await screen.findByLabelText("Quantity received for line 1");
    await userEvent.clear(box);
    await userEvent.type(box, "12.3456");
    await userEvent.click(postButton());

    // The panel's heading, not the toast — both say "posted."
    await screen.findByRole("link", { name: /2 stock ledger entries created/ });
    expect(body).toMatchObject({
      lines: [
        { poLineId: PO_LINE_1, qtyReceived: "12.3456" },
        { poLineId: PO_LINE_2, qtyReceived: "30" },
      ],
    });
  });
});

describe("FE3 — a quantity over outstanding is refused before submitting", () => {
  beforeEach(() => order());

  it("warns and disables Post receipt when a line is over", async () => {
    await renderApp(RECEIVE_PATH, budi);

    const box = await screen.findByLabelText("Quantity received for line 1");
    await userEvent.clear(box);
    await userEvent.type(box, "101");

    expect(
      screen.getByText(/more entered than is outstanding/),
    ).toBeInTheDocument();
    expect(postButton()).toBeDisabled();
    // The box itself is marked, so the reader can tell *which* line is wrong on a
    // form of eight.
    expect(box).toHaveAttribute("aria-invalid", "true");
  });

  it("does not send the request at all", async () => {
    // The point of FE3: the courtesy check saves a round trip. It is not the rule
    // — `over_receipt` is computed server-side under a row lock and refused again
    // by a trigger (§6.10.6) — and two receipts posted at once can each pass this
    // check and still jointly over-receive. Which is exactly why the real check is
    // not here (I12).
    let posts = 0;
    server.use(
      http.post(apiUrl(`/api/procurement/purchase-orders/${PO_OPEN}/receipts`), () => {
        posts += 1;
        return failure(422, "over_receipt", "That is more than is outstanding.");
      }),
    );

    await renderApp(RECEIVE_PATH, budi);
    const box = await screen.findByLabelText("Quantity received for line 1");
    await userEvent.clear(box);
    await userEvent.type(box, "9999");
    await userEvent.click(postButton());

    expect(posts).toBe(0);
  });

  it("recovers as soon as the quantity is brought back down", async () => {
    await renderApp(RECEIVE_PATH, budi);

    const box = await screen.findByLabelText("Quantity received for line 1");
    await userEvent.clear(box);
    await userEvent.type(box, "101");
    expect(postButton()).toBeDisabled();

    await userEvent.clear(box);
    await userEvent.type(box, "100");
    expect(postButton()).toBeEnabled();
    expect(screen.queryByText(/more entered than is outstanding/)).not.toBeInTheDocument();
  });

  it("refuses to submit nothing", async () => {
    await renderApp(RECEIVE_PATH, budi);
    await screen.findByLabelText("Quantity received for line 1");

    for (const lineNo of [1, 2]) {
      await userEvent.clear(quantityBox(lineNo));
    }
    // A blank box means "none of this arrived", and a receipt of nothing is not a
    // receipt.
    expect(postButton()).toBeDisabled();
  });

  it("still shows the server's refusal if one gets through", async () => {
    // The check above is a courtesy, so the screen must remain able to report the
    // real refusal — a concurrent receipt having taken the outstanding quantity
    // between this form loading and this button being pressed.
    server.use(
      http.post(apiUrl(`/api/procurement/purchase-orders/${PO_OPEN}/receipts`), () =>
        failure(422, "over_receipt", "PKG-BOX-L: 30 outstanding, 30 already received."),
      ),
    );

    await renderApp(RECEIVE_PATH, budi);
    await userEvent.click(await screen.findByRole("button", { name: "Post receipt" }));

    expect(
      await screen.findByText("PKG-BOX-L: 30 outstanding, 30 already received."),
    ).toBeInTheDocument();
  });
});

describe("FE5 — the confirmation panel renders both cross-module links", () => {
  beforeEach(() => order());

  async function post(result = receiptResult()) {
    server.use(
      http.post(apiUrl(`/api/procurement/purchase-orders/${PO_OPEN}/receipts`), () =>
        HttpResponse.json(result),
      ),
    );
    await renderApp(RECEIVE_PATH, budi);
    await userEvent.click(await screen.findByRole("button", { name: "Post receipt" }));
    return result;
  }

  it("links the inventory line to the ledger rows this receipt wrote", async () => {
    const result = await post();

    const link = await screen.findByRole("link", { name: "2 stock ledger entries created" });
    // Not `/inventory/ledger` — the claim and the evidence have to be one click
    // apart, so the link lands on a list filtered to this one document.
    expect(link).toHaveAttribute("href", `/inventory/ledger?sourceId=${result.receipt.id}`);
  });

  it("links the finance line to the journal entry it posted", async () => {
    const result = await post();

    const link = await screen.findByRole("link", { name: /journal entry JE-202607-0001 posted/ });
    expect(link).toHaveAttribute("href", `/finance?sourceId=${result.receipt.id}`);
  });

  it("names all three modules and the balanced pair of accounts", async () => {
    await post();

    // One business event, in the words of every module it touched. Every number
    // here came from the server's response, itself rebuilt from committed rows —
    // so the panel cannot claim something the database does not say happened.
    expect(await screen.findByText(/Goods receipt/)).toBeInTheDocument();
    expect(screen.getByText("GR-202607-0001")).toBeInTheDocument();
    expect(screen.getByText("PO-202607-0001 is now fully received.")).toBeInTheDocument();
    expect(screen.getByText(/→ Inventory:/)).toBeInTheDocument();
    expect(screen.getByText(/→ Finance:/)).toBeInTheDocument();

    // The amount is composed with the application's own formatter rather than
    // written out as "1,340.00". `formatMoney` deliberately passes `undefined` as
    // the locale, so the group and decimal separators are the reader's — this
    // machine renders 1.340,00 — and a hardcoded expectation would be a test that
    // only passes where it was written. What is asserted is that the panel puts
    // the balanced pair in one sentence, which is the part that is ours.
    const money = formatMoney(1340);
    expect(
      screen.getByText(
        `(Dr Inventory ${money} / Cr Goods received not invoiced ${money})`,
      ),
    ).toBeInTheDocument();
  });

  it("says a replay was a replay, and still links to the original rows", async () => {
    // §8.6.1. A replay is a success the user should be told about: their first tap
    // worked, and nothing was written twice. The links have to keep working, or
    // the reassurance is unverifiable.
    const result = await post(receiptResult({ replayed: true }));

    expect(await screen.findByText(/was already posted\./)).toBeInTheDocument();
    expect(
      screen.getByText(/nothing was written a second time/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "2 stock ledger entries created" }),
    ).toHaveAttribute("href", `/inventory/ledger?sourceId=${result.receipt.id}`);
  });

  it("replaces the form, so the receipt cannot be posted twice by hand", async () => {
    await post();

    await screen.findByText("GR-202607-0001");
    expect(screen.queryByRole("button", { name: "Post receipt" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Quantity received for line 1")).not.toBeInTheDocument();
  });

  it("carries one idempotency key for the life of the form", async () => {
    // The whole mechanism of §8.6.1: the key is minted when the screen mounts,
    // not when the button is pressed. Minted per submit, every retry would be a
    // new receipt — stock credited twice, a second journal entry to match, and
    // nothing in the schema to flag it.
    const keys: (string | null)[] = [];
    server.use(
      http.post(apiUrl(`/api/procurement/purchase-orders/${PO_OPEN}/receipts`), ({ request }) => {
        keys.push(request.headers.get("Idempotency-Key"));
        // The first attempt fails at the network, which is the ordinary Tuesday
        // this exists for; the second is the retry.
        return keys.length === 1
          ? failure(503, "unavailable", "The server is briefly unavailable.")
          : HttpResponse.json(receiptResult());
      }),
    );

    await renderApp(RECEIVE_PATH, budi);
    const button = await screen.findByRole("button", { name: "Post receipt" });
    await userEvent.click(button);
    await screen.findByText("The server is briefly unavailable.");
    await userEvent.click(screen.getByRole("button", { name: "Post receipt" }));

    await screen.findByText("GR-202607-0001");
    expect(keys).toHaveLength(2);
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
  });
});

describe("FE9 — no control in the receipt form has a hit area below 44px", () => {
  /**
   * §10.7.5's floor, on the one screen §10.7.1 puts on a phone.
   *
   * Scoped to `<main>`, which is the screen: the header and the nav are the
   * frame, they are shared by every screen, and their targets were measured by
   * the Phase 7 audit. Scoped to the *form* rather than the confirmation panel,
   * because the panel's two links are inline inside sentences, where a 44px block
   * would break the prose — those are FE5's business.
   *
   * See `layout.ts` before trusting this: jsdom has no layout engine, so what is
   * measured is the minimum each control *declares*, not pixels a browser
   * produced. That is the check that catches the regression, which is a control
   * shipping with no declared floor at all — every one of the four the Phase 7
   * audit found was exactly that.
   */
  const FLOOR = 44;

  beforeEach(() => order());

  it("measures every interactive control on the form", async () => {
    await renderApp(RECEIVE_PATH, budi);
    await screen.findByLabelText("Quantity received for line 1");

    const main = document.querySelector("main");
    expect(main).not.toBeNull();

    const controls = [
      ...(main as HTMLElement).querySelectorAll(
        "button, a, input, textarea, select",
      ),
    ];
    // Six: two quantity boxes, the note, Post receipt, the back link, and the
    // module's own sub-navigation is outside <main>. If this number changes, the
    // list below changed and the new control needs measuring too.
    expect(controls.length).toBeGreaterThanOrEqual(5);

    const tooSmall = controls.filter((control) => {
      // HEIGHT IS THE AXIS THAT COLLAPSES. All four controls the Phase 7 audit
      // found were text or a glyph with no vertical minimum — `px-3 py-1` around
      // a ✕, a bare `text-sm` link. So every control has to declare one.
      const height = declaredHeightPx(control);
      if (height === null || height < FLOOR) return true;

      // Width is set by the label on anything that has one: "Back to the order"
      // at 14px is about 120px wide however little the class list says. It only
      // needs declaring where there is no text to set it — an icon-only control,
      // whose accessible name comes from `aria-label`. Those are the ones that
      // end up 24px square.
      const labelled = (control.textContent ?? "").trim().length > 1;
      if (labelled) return false;

      const width = declaredWidthPx(control);
      return width === null || width < FLOOR;
    });

    expect(
      tooSmall.map(describeEl),
      "every control on the receipt form declares a 44px minimum in both axes",
    ).toEqual([]);
  });

  it("keeps the primary action reachable without scrolling to the end of the lines", async () => {
    // The other half of §10.7.5's sentence about this screen. `sticky`, not
    // `fixed`, so nothing needs a spacer and nothing can hide behind it, and
    // `bottom-14` clears the tab bar.
    await renderApp(RECEIVE_PATH, budi);

    const bar = (await screen.findByRole("button", { name: "Post receipt" }))
      .parentElement as HTMLElement;
    expect(bar.className).toContain("sticky");
    expect(bar.className).toContain("bottom-14");
    expect(bar.className).toContain("md:static");
  });

  it("uses a decimal keypad rather than a number input for quantities", async () => {
    // §10.7.5 asks for `inputmode="decimal"`. `type="number"` would bring the
    // right keyboard and also let the browser reformat the value, and this string
    // goes to a NUMERIC(18,4) unchanged (I8).
    await renderApp(RECEIVE_PATH, budi);

    const box = await screen.findByLabelText("Quantity received for line 1");
    expect(box).toHaveAttribute("inputmode", "decimal");
    expect(box).not.toHaveAttribute("type", "number");
  });
});

describe("the receipt screen refuses itself to a reader who cannot post one", () => {
  it("explains rather than showing a form to a `user`-level account", async () => {
    // Cosmetic (I12): the endpoint is an `approver` route. The screen is reachable
    // because the *module* guard is what the route carries, so it has to say
    // something rather than render a form whose button always fails.
    order();
    await renderApp(RECEIVE_PATH, sari);

    expect(
      await screen.findByText("Receiving goods needs the approver level in procurement."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Post receipt" })).not.toBeInTheDocument();
  });

  it("explains when the order has nothing left to receive", async () => {
    order({ status: "received" });
    await renderApp(RECEIVE_PATH, budi);

    await waitFor(() =>
      expect(
        screen.getByText(/is received, so nothing more can be received against it/),
      ).toBeInTheDocument(),
    );
  });
});
