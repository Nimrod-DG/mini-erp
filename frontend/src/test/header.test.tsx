/**
 * FE30 — the account menu in `AppShell`'s header.
 *
 * The header used to spell identity out along the bar: two role badges, the full
 * name, and a Sign out button. That is four controls' worth of chrome for
 * something a person checks once a session, and at 360px it wrapped onto a
 * second row and pushed every screen down. It is now one avatar button with a
 * menu behind it.
 *
 * Which makes signing out a *disclosed* action rather than a visible one, and
 * that is the risk worth testing: a sign-out nobody can find is worse than an
 * untidy header. So the assertions are about the menu being reachable, saying
 * who you are once open, and closing the way a menu is expected to.
 */

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { budi, rina, superadmin } from "./fixtures";
import { renderApp } from "./render";

/** The avatar button. Its accessible name is the person's name, which is the
 *  only thing that makes an avatar a control rather than a decoration. */
function trigger(name: string) {
  return screen.getByRole("button", { name: new RegExp(name) });
}

function menu() {
  return screen.getByRole("menu", { name: "Account" });
}

describe("FE30 — identity lives behind one control, and sign-out is inside it", () => {
  it("shows the person's name on the trigger and nothing else on the bar", async () => {
    await renderApp("/", budi);

    expect(trigger("Budi Santoso")).toHaveAttribute("aria-expanded", "false");

    // Closed, so the menu's contents are out of the tree entirely rather than
    // hidden with CSS — a screen reader must not be able to reach a Sign out
    // that a pointer cannot.
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Sign out" }),
    ).not.toBeInTheDocument();
  });

  it("says who you are, where you are, and what you hold", async () => {
    await renderApp("/", budi);
    await userEvent.click(trigger("Budi Santoso"));

    const panel = within(menu());
    expect(panel.getByText("budi@nusantara.test")).toBeInTheDocument();
    expect(panel.getByText(/Nusantara Retail/)).toBeInTheDocument();

    // Budi holds `procurement: approver` and `inventory: viewer` and nothing in
    // Finance — the module he cannot see is absent, not listed as "none". Same
    // rule as the sidebar (§10.1): a greyed-out Finance tells every employee
    // what their employer has not bought.
    expect(panel.getByText("Procurement")).toBeInTheDocument();
    expect(panel.getByText("approver")).toBeInTheDocument();
    expect(panel.getByText("Inventory")).toBeInTheDocument();
    expect(panel.getByText("viewer")).toBeInTheDocument();
    expect(panel.queryByText("Finance")).not.toBeInTheDocument();
  });

  it("reads sensibly for the platform account, which belongs to no workspace", async () => {
    // A superadmin has `tenant: null` and holds no module. The panel would read
    // "undefined · superadmin" with an empty level list if either case had been
    // written for the tenant user only.
    await renderApp("/admin/tenants", superadmin);
    await userEvent.click(trigger("Platform Operator"));

    const panel = within(menu());
    // One line, so the assertion is that the two halves agree: "Platform" where
    // a workspace name would go, and the platform role beside it.
    expect(panel.getByText("Platform · superadmin")).toBeInTheDocument();
    expect(panel.queryByText("Procurement")).not.toBeInTheDocument();
  });

  it("signs out from inside the menu", async () => {
    await renderApp("/", rina);
    await userEvent.click(trigger("Rina Wijaya"));
    await userEvent.click(screen.getByRole("menuitem", { name: "Sign out" }));

    // The fake drops the Firebase session, `AuthProvider` follows it, and
    // ProtectedRoute redirects. Asserting the destination rather than a spy:
    // what matters is that the user ends up signed out, not that a function ran.
    expect(
      await screen.findByRole("heading", { name: "Sign in" }),
    ).toBeInTheDocument();
  });

  it("closes on Escape, and gives the focus back", async () => {
    await renderApp("/", rina);
    const button = trigger("Rina Wijaya");

    await userEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");

    await userEvent.keyboard("{Escape}");

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    // Without this a keyboard user is dropped at the top of the document with
    // no idea what they just dismissed.
    expect(button).toHaveFocus();
  });

  it("closes when something else is clicked", async () => {
    await renderApp("/", rina);
    await userEvent.click(trigger("Rina Wijaya"));
    expect(menu()).toBeInTheDocument();

    await userEvent.click(screen.getByRole("heading", { level: 1 }));

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });
});
