import { Navigate } from "react-router-dom";
import type { ReactNode } from "react";

import { useMe } from "../hooks/useAuth";
import type { ModuleCode } from "../lib/api";

/**
 * RequireModule keeps a module's screens out of the router for someone who
 * holds nothing in it, the same way the sidebar keeps them out of the nav.
 *
 * Cosmetic, like every guard in this file: RequireModule on the server refuses
 * the same requests with `module_not_enabled` or `insufficient_module_role`,
 * and deleting this component would make the app render empty screens full of
 * 403s rather than make anything reachable.
 */
export function RequireModule({
  module,
  children,
}: {
  module: ModuleCode;
  children: ReactNode;
}) {
  const me = useMe();
  if (!me.moduleRoles[module]) return <Navigate to="/" replace />;
  return <>{children}</>;
}

/**
 * Route guards for the two control planes (§5.7).
 *
 * These are rendering decisions and nothing more (I12). Every screen they hide
 * is independently enforced server-side: `/api/admin/*` asserts
 * `tenant_role = 'superadmin'` on the erp_admin pool, and `/api/tenant/*` asserts
 * the tenant admin role. Deleting this file would make the app ugly, not
 * insecure — the screens would render and then fill with 403s.
 *
 * A wrong-plane visitor is redirected rather than shown a refusal, because they
 * did not choose to go there: it is a stale bookmark or a link from another
 * account's session.
 */
export function RequireSuperadmin({ children }: { children: ReactNode }) {
  const me = useMe();
  if (me.user.tenantRole !== "superadmin") return <Navigate to="/" replace />;
  return <>{children}</>;
}

export function RequireTenantAdmin({ children }: { children: ReactNode }) {
  const me = useMe();
  if (me.user.tenantRole !== "admin") return <Navigate to="/" replace />;
  return <>{children}</>;
}

/**
 * Home sends a superadmin to `/admin/tenants`, which §10.1 makes their landing
 * screen: they see no business modules at all, so a dashboard of module widgets
 * would be an empty page.
 */
export function Home({ children }: { children: ReactNode }) {
  const me = useMe();
  if (me.user.tenantRole === "superadmin") {
    return <Navigate to="/admin/tenants" replace />;
  }
  return <>{children}</>;
}
