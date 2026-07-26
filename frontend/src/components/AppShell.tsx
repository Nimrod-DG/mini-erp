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

type NavItem = { to: string; label: string };

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

  // The module screens arrive in Phases 4-6. The entitlement filter is already
  // the real one, so each phase adds a path here and nothing else: an item with
  // nowhere to go is left out rather than linking at a route that redirects.
  const modulePaths: Partial<Record<ModuleCode, string>> = {};
  for (const module of MODULES) {
    const path = modulePaths[module.code];
    if (path && me.moduleRoles[module.code]) {
      items.push({ to: path, label: module.label });
    }
  }

  // The tenant plane. Not a module role — administering people is not any one
  // module's business (§5.7).
  if (me.user.tenantRole === "admin") {
    items.push({ to: "/settings/users", label: "Users" });
  }

  return items;
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
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.to === "/"}
          className={navLinkClass}
          onClick={() => setDrawerOpen(false)}
        >
          {item.label}
        </NavLink>
      ))}
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

        <main key={location.pathname} className="min-w-0 flex-1 px-4 py-6 sm:px-6">
          <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
            <h1 className="text-xl font-semibold">{title}</h1>
            {actions}
          </div>
          {children}
        </main>
      </div>
    </div>
  );
}
