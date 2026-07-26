import { useState, type ReactNode } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";

import { useAuth, useMe } from "../hooks/useAuth";
import type { Me, ModuleCode } from "../lib/api";
import { ThemeToggle } from "./ThemeToggle";

/** Written out rather than built from a template string: Tailwind scans source
 *  text, so a class it cannot see literally is a class it does not generate. */
const MODULES: { code: ModuleCode; label: string }[] = [
  { code: "procurement", label: "Procurement" },
  { code: "inventory", label: "Inventory" },
  { code: "finance", label: "Finance" },
];

type NavItem = {
  to: string;
  label: string;
  /** Shown only while the module is the one being looked at. A module's screens
   *  are not top-level destinations — they are places inside it — and listing
   *  all of them all of the time would make the sidebar a table of contents. */
  children?: { to: string; label: string }[];
};

/**
 * The sidebar, driven entirely by `/api/me` (§10.1).
 *
 * A module the user holds `none` in, or that the tenant is not entitled to, is
 * **hidden entirely** rather than disabled — a greyed-out Finance link tells
 * every employee what their employer has not bought, and tells an attacker what
 * to go looking for.
 *
 * All of this is cosmetic (I12). Every hidden destination is independently
 * enforced by RequireModule and RequireTenantAdmin on the server, so deleting
 * this function would make the app ugly, not insecure.
 */
function navItems(me: Me): NavItem[] {
  // Superadmins see no business modules at all: they administer tenants, not
  // tenant data (§5.5).
  if (me.user.tenantRole === "superadmin") {
    return [{ to: "/admin/tenants", label: "Workspaces" }];
  }

  const items: NavItem[] = [{ to: "/", label: "Dashboard" }];

  // One entry per module the tenant is entitled to and the user holds a level
  // in. Finance has no children: the module is a single page (§10.5), and a
  // sub-list of one repeats the item above it.
  const modulePaths: Partial<Record<ModuleCode, NavItem>> = {
    procurement: {
      // Requisitions first, because that is where buying starts: an order is not
      // created by hand, it is what approving a requisition produces.
      to: "/procurement/requisitions",
      label: "Procurement",
      children: [
        { to: "/procurement/requisitions", label: "Requisitions" },
        { to: "/procurement/orders", label: "Purchase orders" },
        { to: "/procurement/suppliers", label: "Suppliers" },
      ],
    },
    inventory: {
      to: "/inventory/products",
      label: "Inventory",
      children: [
        { to: "/inventory/products", label: "Products" },
        { to: "/inventory/stock", label: "Stock on hand" },
        { to: "/inventory/ledger", label: "Ledger" },
        { to: "/inventory/warehouses", label: "Warehouses" },
      ],
    },
    finance: {
      to: "/finance",
      label: "Finance",
    },
  };
  for (const module of MODULES) {
    const item = modulePaths[module.code];
    if (item && me.moduleRoles[module.code]) {
      items.push(item);
    }
  }

  // The tenant plane. Not a module role — administering people is not any one
  // module's business (§5.7).
  if (me.user.tenantRole === "admin") {
    items.push({ to: "/settings/users", label: "Users" });
  }

  return items;
}

/**
 * The bottom tab bar of §10.7.3: the three or four most-used destinations, below
 * `md`, where the sidebar is a drawer behind a hamburger.
 *
 * "The bottom bar is worth the effort: on the goods-receipt flow the user is
 * holding a phone one-handed while looking at boxes, and top-of-screen navigation
 * is out of thumb reach."
 *
 * Tabs respect entitlements exactly like the sidebar — a user with nothing in
 * Inventory gets three tabs, not a disabled fourth. Cosmetic, like the rest of
 * this file (I12).
 *
 * This is a *shortcut*, not the whole map: the drawer still holds everything.
 * Suppliers and Warehouses are deliberately absent, because master data is not
 * what anyone reaches for one-handed in a warehouse aisle.
 */
function tabItems(me: Me): { to: string; label: string; icon: string }[] {
  if (me.user.tenantRole === "superadmin") return [];

  const tabs = [{ to: "/", label: "Home", icon: "◉" }];
  if (me.moduleRoles.procurement) {
    tabs.push({ to: "/procurement/requisitions", label: "Requests", icon: "▤" });
    tabs.push({ to: "/procurement/orders", label: "Orders", icon: "▦" });
  }
  if (me.moduleRoles.inventory) {
    tabs.push({ to: "/inventory/stock", label: "Stock", icon: "▣" });
  }
  // One tab is not a navigation aid — it is a button that says where you already
  // are. The drawer covers that case.
  return tabs.length > 1 ? tabs : [];
}

function BottomTabs({ me }: { me: Me }) {
  const tabs = tabItems(me);
  if (tabs.length === 0) return null;

  return (
    <nav
      aria-label="Quick navigation"
      className="fixed inset-x-0 bottom-0 z-30 flex border-t border-hairline bg-surface md:hidden"
      // Clears the iOS home indicator, which otherwise sits on top of the tabs.
      style={{ paddingBottom: "env(safe-area-inset-bottom)" }}
    >
      {tabs.map((tab) => (
        <NavLink
          key={tab.to}
          to={tab.to}
          end={tab.to === "/"}
          className={({ isActive }) =>
            `flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 text-xs ${
              isActive ? "text-accent" : "text-secondary"
            }`
          }
        >
          {/* The glyph is decorative: the label underneath is the accessible name,
              and an icon-only tab bar is a guessing game (§10.7.5). */}
          <span aria-hidden="true" className="text-base leading-none">
            {tab.icon}
          </span>
          {tab.label}
        </NavLink>
      ))}
    </nav>
  );
}

/** RoleBadges renders the per-module levels the top bar carries (§10.1). */
function RoleBadges({ me }: { me: Me }) {
  const held = MODULES.filter((m) => me.moduleRoles[m.code]);
  if (held.length === 0) return null;

  return (
    <ul className="hidden items-center gap-1.5 sm:flex">
      {held.map((module) => (
        <li
          key={module.code}
          className="rounded border border-hairline px-2 py-0.5 text-xs text-secondary"
          title={`${module.label}: ${me.moduleRoles[module.code]}`}
        >
          {module.label.slice(0, 4)}
          <span className="ml-1 tabular">{me.moduleRoles[module.code]}</span>
        </li>
      ))}
    </ul>
  );
}

/** `/inventory/products` -> `/inventory`, so every screen in the module keeps
 *  the module's sub-navigation open. */
function modulePrefix(path: string): string {
  return "/" + path.split("/")[1];
}

function navLinkClass({ isActive }: { isActive: boolean }): string {
  // min-h-11 is the 44px touch target of §10.7.5, on every item.
  const base =
    "flex min-h-11 items-center rounded-md px-3 text-sm transition-colors";
  return isActive
    ? `${base} bg-raised font-medium text-primary`
    : `${base} text-secondary hover:bg-raised hover:text-primary`;
}

/**
 * AppShell is the frame every signed-in screen renders inside.
 *
 * `lg` and up: a persistent 240px sidebar. Below that: a slide-over drawer
 * behind a hamburger (§10.7.3). The bottom tab bar that section also describes
 * is deliberately absent — its three or four destinations are the module
 * screens, which arrive in Phases 4-6, and a tab bar with one tab in it is not
 * a navigation aid.
 */
export function AppShell({
  title,
  actions,
  children,
}: {
  title: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  const me = useMe();
  const { signOut } = useAuth();
  const location = useLocation();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const items = navItems(me);

  const nav = (
    <nav aria-label="Main" className="flex flex-col gap-1 p-3">
      {items.map((item) => {
        // A module's own screens appear only once you are inside it. Matched on
        // the module's prefix rather than on the exact path, or the sub-items
        // would vanish the moment you opened one of them.
        const inside =
          item.children !== undefined &&
          location.pathname.startsWith(modulePrefix(item.to));

        return (
          <div key={item.to}>
            <NavLink
              to={item.to}
              end={item.to === "/"}
              className={navLinkClass}
              onClick={() => setDrawerOpen(false)}
            >
              {item.label}
            </NavLink>

            {inside && (
              <div className="mt-1 flex flex-col gap-1 border-l border-hairline pl-3">
                {item.children?.map((child) => (
                  <NavLink
                    key={child.to}
                    to={child.to}
                    className={navLinkClass}
                    onClick={() => setDrawerOpen(false)}
                  >
                    {child.label}
                  </NavLink>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </nav>
  );

  return (
    <div className="min-h-screen bg-canvas text-primary">
      <header className="sticky top-0 z-30 flex flex-wrap items-center gap-3 border-b border-hairline bg-canvas px-4 py-3">
        <button
          type="button"
          aria-label="Open navigation"
          aria-expanded={drawerOpen}
          onClick={() => setDrawerOpen((open) => !open)}
          className="grid size-11 place-items-center rounded-md border border-hairline lg:hidden"
        >
          <span aria-hidden="true">☰</span>
        </button>

        <Link to={items[0]?.to ?? "/"} className="text-base font-semibold">
          mini-erp
        </Link>

        {me.tenant && (
          <span className="text-sm text-secondary">{me.tenant.name}</span>
        )}
        {me.user.tenantRole === "superadmin" && (
          <span className="rounded border border-hairline px-2 py-0.5 text-xs text-secondary">
            platform
          </span>
        )}

        <div className="ml-auto flex items-center gap-3">
          <RoleBadges me={me} />
          <span className="hidden text-sm text-secondary md:inline">
            {me.user.fullName}
          </span>
          <ThemeToggle />
          <button
            type="button"
            onClick={() => void signOut()}
            className="min-h-11 rounded-md border border-hairline px-3 text-sm"
          >
            Sign out
          </button>
        </div>
      </header>

      <div className="flex">
        <aside className="hidden w-60 shrink-0 border-r border-hairline lg:block">
          <div className="sticky top-[57px]">{nav}</div>
        </aside>

        {drawerOpen && (
          <>
            {/* Dismissible by keyboard as well as by pointer (§10.7.5). */}
            <button
              type="button"
              aria-label="Close navigation"
              onClick={() => setDrawerOpen(false)}
              className="fixed inset-0 z-30 bg-black/40 lg:hidden"
            />
            <aside
              className="fixed inset-y-0 left-0 z-40 w-64 border-r border-hairline bg-surface pt-16 lg:hidden"
              // The overlay above is the escape hatch; the pt-16 keeps the
              // hamburger itself visible and clickable underneath.
            >
              {nav}
            </aside>
          </>
        )}

        {/* pb-20 below md leaves room for the fixed tab bar, which would
            otherwise cover the last row of every table. */}
        <main
          key={location.pathname}
          className="min-w-0 flex-1 px-4 pb-20 pt-6 sm:px-6 md:pb-6"
        >
          <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
            <h1 className="text-xl font-semibold">{title}</h1>
            {actions}
          </div>
          {children}
        </main>
      </div>

      <BottomTabs me={me} />
    </div>
  );
}
