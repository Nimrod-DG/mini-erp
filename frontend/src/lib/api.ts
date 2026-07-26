import { auth } from "./firebase";

const BASE = import.meta.env.VITE_API_BASE_URL;

/** Module codes and role levels are naming contracts — they appear identically
 *  in the migrations, the Go types, this file, and the tests. */
export type ModuleCode = "procurement" | "inventory" | "finance";
export type RoleLevel = "viewer" | "user" | "approver" | "admin";
export type TenantRole = "staff" | "admin" | "superadmin";

export type Me = {
  user: {
    id: string;
    email: string;
    fullName: string;
    tenantRole: TenantRole;
  };
  /** null for a superadmin, who belongs to no tenant. */
  tenant: {
    id: string;
    name: string;
    slug: string;
    status: string;
    /** Business dates render in this zone, never the browser's (FE15). */
    timezone: string;
  } | null;
  /** Only modules the tenant is entitled to. An absent module means `none` —
   *  the level is the absence of a row, so this never contains "none". */
  moduleRoles: Partial<Record<ModuleCode, RoleLevel>>;
};

/** ApiError carries the machine-readable code from the §9.8 envelope. Branch on
 *  `code`, never on `message` — the prose is written for people and gets
 *  reworded. */
export class ApiError extends Error {
  // Declared and assigned rather than written as constructor parameter
  // properties: `erasableSyntaxOnly` is on, and that syntax has to be compiled
  // away rather than erased.
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

type ErrorEnvelope = { error?: string; message?: string };

/**
 * The single way this app talks to the backend. Every call carries the Firebase
 * ID token.
 *
 * `getIdToken()` is the whole of token lifecycle management: it returns the
 * cached token and silently refreshes when it is close to expiry. Storing one
 * ourselves would mean reimplementing that badly, and putting it in
 * localStorage would make it stealable by any injected script.
 */
export async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const user = auth.currentUser;
  if (!user) {
    throw new ApiError(401, "unauthenticated", "Not signed in.");
  }
  const token = await user.getIdToken();

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  if (init.body !== undefined) headers.set("Content-Type", "application/json");

  const response = await fetch(`${BASE}${path}`, { ...init, headers });

  if (response.status === 204) return undefined as T;

  const body: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const envelope = (body ?? {}) as ErrorEnvelope;
    throw new ApiError(
      response.status,
      envelope.error ?? "unknown",
      envelope.message ?? response.statusText,
    );
  }
  return body as T;
}

export function getMe(): Promise<Me> {
  return apiFetch<Me>("/api/me");
}

/** `none` is a value the API accepts and never stores — setting it deletes the
 *  row (§5.3). So it is legal in a request and in a matrix cell, but never in
 *  `moduleRoles`, which is why it is a separate type. */
export type RoleLevelOrNone = RoleLevel | "none";

/** The §9.0 list envelope. `totalItems` is mandatory: "Page 3 of ?" strands
 *  people (§10.7.4). */
export type ListResponse<T> = {
  data: T[];
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
};

export type ListQuery = {
  page?: number;
  pageSize?: number;
  q?: string;
  sort?: string;
};

function queryString(query: ListQuery): string {
  const params = new URLSearchParams();
  if (query.page && query.page > 1) params.set("page", String(query.page));
  if (query.pageSize) params.set("pageSize", String(query.pageSize));
  if (query.q) params.set("q", query.q);
  if (query.sort) params.set("sort", query.sort);
  const rendered = params.toString();
  return rendered ? `?${rendered}` : "";
}

// --------------------------------------------------------------------------
// Platform plane — superadmin only (§5.7).
// --------------------------------------------------------------------------

export type ModuleCatalogueEntry = {
  code: ModuleCode;
  name: string;
  description: string;
  sortOrder: number;
};

export type TenantModule = {
  code: ModuleCode;
  name: string;
  description: string;
  enabled: boolean;
};

export type TenantSummary = {
  id: string;
  name: string;
  slug: string;
  status: "active" | "suspended";
  timezone: string;
  /** Active users only — a workspace whose people are all deactivated is empty
   *  in the sense the console is asking about. */
  userCount: number;
  adminCount: number;
  moduleCount: number;
  enabledModules: ModuleCode[];
  createdAt: string;
};

export type TenantDetail = TenantSummary & { modules: TenantModule[] };

export type CreateTenantRequest = {
  name: string;
  slug: string;
  timezone: string;
  modules: ModuleCode[];
  admin: { email: string; fullName: string; password: string };
};

export function listTenants(query: ListQuery = {}) {
  return apiFetch<ListResponse<TenantSummary>>(
    `/api/admin/tenants${queryString(query)}`,
  );
}

export function getTenant(id: string) {
  return apiFetch<TenantDetail>(`/api/admin/tenants/${id}`);
}

export function createTenant(body: CreateTenantRequest) {
  return apiFetch<{ tenant: TenantDetail; admin: { id: string; email: string } }>(
    "/api/admin/tenants",
    { method: "POST", body: JSON.stringify(body) },
  );
}

export function patchTenant(
  id: string,
  body: { name?: string; timezone?: string; status?: "active" | "suspended" },
) {
  return apiFetch<TenantDetail>(`/api/admin/tenants/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function setTenantModule(id: string, code: ModuleCode, enabled: boolean) {
  return apiFetch<TenantModule[]>(`/api/admin/tenants/${id}/modules/${code}`, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export function listModuleCatalogue() {
  return apiFetch<ModuleCatalogueEntry[]>("/api/admin/modules");
}

// --------------------------------------------------------------------------
// Tenant plane — tenant admin only (§5.7).
// --------------------------------------------------------------------------

export type TenantUser = {
  id: string;
  email: string;
  fullName: string;
  tenantRole: TenantRole;
  isActive: boolean;
  createdAt: string;
  /** What is stored. Empty for an admin, who holds `admin` implicitly. */
  moduleRoles: Partial<Record<ModuleCode, RoleLevel>>;
  /** What LevelFor actually resolves — entitlement ceiling and implicit-admin
   *  rule applied. Render this one, or the screen misrepresents the model. */
  effectiveRoles: Partial<Record<ModuleCode, RoleLevel>>;
};

export type UserModuleCell = {
  code: ModuleCode;
  name: string;
  roleLevel: RoleLevelOrNone;
  effectiveLevel: RoleLevelOrNone;
};

export type TenantUserDetail = TenantUser & { modules: UserModuleCell[] };

export type CreateUserRequest = {
  email: string;
  fullName: string;
  password: string;
  tenantRole: "staff" | "admin";
  moduleRoles: Partial<Record<ModuleCode, RoleLevelOrNone>>;
};

export function listTenantUsers(query: ListQuery = {}) {
  return apiFetch<ListResponse<TenantUser>>(
    `/api/tenant/users${queryString(query)}`,
  );
}

export function getTenantUser(id: string) {
  return apiFetch<TenantUserDetail>(`/api/tenant/users/${id}`);
}

export function createTenantUser(body: CreateUserRequest) {
  return apiFetch<TenantUserDetail>("/api/tenant/users", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function patchTenantUser(
  id: string,
  body: { fullName?: string; isActive?: boolean; tenantRole?: "staff" | "admin" },
) {
  return apiFetch<TenantUserDetail>(`/api/tenant/users/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

/** The whole matrix in one request and one transaction. Six dropdowns must not
 *  be six requests that can half-fail (§9.3). */
export function setTenantUserModules(
  id: string,
  moduleRoles: Partial<Record<ModuleCode, RoleLevelOrNone>>,
) {
  return apiFetch<TenantUserDetail>(`/api/tenant/users/${id}/modules`, {
    method: "PUT",
    body: JSON.stringify({ moduleRoles }),
  });
}
