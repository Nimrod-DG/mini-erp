import type { ModuleCode, RoleLevel } from "./api";

/** The levels, ranked, exactly as `identity.RoleLevel` ranks them (§5.3). A copy
 *  of a naming contract, not a second implementation of a rule. */
const RANK: Record<RoleLevel, number> = {
  viewer: 1,
  user: 2,
  approver: 3,
  admin: 4,
};

/**
 * `holds` answers "may this screen show the control?" — and nothing more (I12).
 * Every control it hides is independently refused by the server.
 *
 * `me.moduleRoles` is already the *effective* map: the server intersected the
 * user's levels with the tenant's entitlements and applied the implicit-admin
 * rule before sending it, so an admin's implicit `admin` and an entitlement the
 * tenant lacks are both accounted for without this file knowing the rules.
 */
export function holds(
  roles: Partial<Record<ModuleCode, RoleLevel>>,
  module: ModuleCode,
  min: RoleLevel,
): boolean {
  const level = roles[module];
  return level !== undefined && RANK[level] >= RANK[min];
}
