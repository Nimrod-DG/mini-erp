/**
 * FE1 and FE8 — the navigation is derived from `/api/me` and nothing else.
 *
 * Both are cosmetic (I12): every destination they hide is independently refused
 * by `RequireModule` on the server. What they protect is the §10.1 rule that a
 * module the caller holds nothing in is **absent**, not disabled — a greyed-out
 * Finance link tells every employee what their employer has not bought.
 */

import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { agus, budi, dewi, rina, superadmin } from "./fixtures";
import { renderApp } from "./render";

/** The sidebar and the drawer share one `<nav aria-label="Main">`. */
function mainNav() {
  return screen.getByRole("navigation", { name: "Main" });
}

function mainNavLinks(): string[] {
  return within(mainNav())
    .getAllByRole("link")
    .map((link) => link.textContent?.trim() ?? "");
}

describe("FE1 — nav renders only modules present in /api/me", () => {
  it("shows all three modules for a tenant admin entitled to all three", async () => {
    await renderApp("/", rina);

    expect(mainNavLinks()).toEqual([
      "Dashboard",
      "Procurement",
      "Inventory",
      // Rina is a tenant admin, so the tenant plane is hers as well (§5.7).
      "Finance",
      "Users",
    ]);
  });

  it("omits Finance for a user who holds nothing in it", async () => {
    await renderApp("/", budi);

    const links = mainNavLinks();
    expect(links).toContain("Procurement");
    expect(links).toContain("Inventory");
    expect(links).not.toContain("Finance");
  });

  it("omits Inventory for a user who holds nothing in it, and keeps Finance", async () => {
    // Dewi is the mirror image of Budi: viewer in procurement, admin in
    // finance, absent from inventory. Two identities with complementary gaps
    // catch an implementation that hides the *last* module rather than the
    // unheld one.
    await renderApp("/", dewi);

    const links = mainNavLinks();
    expect(links).toContain("Procurement");
    expect(links).toContain("Finance");
    expect(links).not.toContain("Inventory");
  });

  it("omits Finance for a tenant admin whose workspace is not entitled to it", async () => {
    // The distinction the whole entitlement model turns on. Agus holds implicit
    // `admin` in every module Bahari has, and Bahari has no Finance — so the
    // absence here is the tenant's, not the user's, and `/api/me` has already
    // applied the ceiling.
    await renderApp("/", agus);

    expect(mainNavLinks()).not.toContain("Finance");
    expect(screen.getByText("Bahari Logistics")).toBeInTheDocument();
  });

  it("gives a superadmin the platform plane and no business module at all", async () => {
    // A superadmin administers tenants, not tenant data (§5.5), so `/` redirects
    // to the workspace list rather than rendering a dashboard of empty widgets.
    await renderApp("/", superadmin);

    expect(mainNavLinks()).toEqual(["Workspaces"]);
    expect(window.location.pathname).toBe("/admin/tenants");
  });

  it("hides the tenant plane from staff", async () => {
    // Administering people is not any one module's business (§5.7), so it is the
    // tenant role that decides — Budi is `approver` in procurement and still has
    // no Users link.
    await renderApp("/", budi);
    expect(mainNavLinks()).not.toContain("Users");
  });
});

describe("FE8 — the bottom tab bar renders only entitled modules", () => {
  function tabLabels(): string[] {
    return within(screen.getByRole("navigation", { name: "Quick navigation" }))
      .getAllByRole("link")
      .map((link) => link.textContent?.replace(/[^\w\s]/g, "").trim() ?? "");
  }

  it("gives a fully entitled user Home, Requests, Orders and Stock", async () => {
    await renderApp("/", rina);
    expect(tabLabels()).toEqual(["Home", "Requests", "Orders", "Stock"]);
  });

  it("gives a user with nothing in inventory three tabs, not a disabled fourth", async () => {
    // §10.7.3, verbatim: "a user with `none` in Inventory gets three tabs, not a
    // disabled fourth". So the assertion is on the *number* of tabs as much as
    // on their names — a fourth tab rendered `disabled` would pass a
    // contains-check and fail this.
    await renderApp("/", dewi);

    const tabs = tabLabels();
    expect(tabs).toEqual(["Home", "Requests", "Orders"]);
    expect(screen.queryByText("Stock")).not.toBeInTheDocument();
  });

  it("renders no tab bar at all when there would be only one tab", async () => {
    // A single tab is not a navigation aid, it is a button that says where you
    // already are. The drawer covers that reader.
    await renderApp("/admin/tenants", superadmin);

    expect(
      screen.queryByRole("navigation", { name: "Quick navigation" }),
    ).not.toBeInTheDocument();
  });

  it("never offers a tab for a module the tenant is not entitled to", async () => {
    await renderApp("/", agus);

    // Bahari has no Finance, and there is no Finance tab to hide — the tab bar
    // is deliberately a shortcut to four destinations, and this asserts the
    // entitlement rule holds for the ones it does offer.
    const tabs = tabLabels();
    expect(tabs).toEqual(["Home", "Requests", "Orders", "Stock"]);
    expect(tabs).not.toContain("Finance");
  });
});
