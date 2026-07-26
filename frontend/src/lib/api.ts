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

function queryString(query: ListQuery, extra: Record<string, string | undefined> = {}): string {
  const params = new URLSearchParams();
  if (query.page && query.page > 1) params.set("page", String(query.page));
  if (query.pageSize) params.set("pageSize", String(query.pageSize));
  if (query.q) params.set("q", query.q);
  if (query.sort) params.set("sort", query.sort);
  for (const [key, value] of Object.entries(extra)) {
    if (value) params.set(key, value);
  }
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

// --------------------------------------------------------------------------
// Inventory (§9.5, §10.4).
//
// Quantities and money arrive as JSON numbers. On the server they are NUMERIC
// end to end and never a float (I8) — every sum and every comparison, including
// `belowReorderPoint`, is computed by PostgreSQL. What arrives here is the
// answer, for display. Do not re-derive one of these numbers in TypeScript: a
// second implementation of a rule is one that can disagree with the one that
// counts.
// --------------------------------------------------------------------------

export type Product = {
  id: string;
  sku: string;
  name: string;
  uom: string;
  reorderPoint: number;
  standardCost: number;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
  /** Set means soft-deleted: hidden from lists, still resolvable by id, and
   *  restorable (§6.9.1). Distinct from `isActive`, which means discontinued. */
  deletedAt: string | null;
  qtyOnHand: number;
  belowReorderPoint: boolean;
};

export type ProductBalance = {
  warehouseId: string;
  warehouseCode: string;
  warehouseName: string;
  qtyOnHand: number;
};

export type ProductDetail = Product & { balances: ProductBalance[] };

export type Warehouse = {
  id: string;
  code: string;
  name: string;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
  qtyOnHand: number;
  /** Products with a non-zero balance here — what blocks a delete (G5). */
  productCount: number;
};

export type StockCell = {
  productId: string;
  sku: string;
  productName: string;
  uom: string;
  /** The product is in the recycle bin, but the goods are still on the shelf, so
   *  the balance is shown and marked. The grid, the ledger, and the warehouse's
   *  own count all have to agree about this or one of them lies. */
  productDeleted: boolean;
  warehouseId: string;
  warehouseCode: string;
  warehouseName: string;
  qtyOnHand: number;
};

export type LowStockRow = {
  productId: string;
  sku: string;
  name: string;
  uom: string;
  qtyOnHand: number;
  reorderPoint: number;
  shortfall: number;
};

export type EntryType = "receipt" | "issue" | "adjustment";
export type SourceType = "goods_receipt" | "manual_adjustment";

export type LedgerEntry = {
  id: string;
  occurredAt: string;
  entryType: EntryType;
  qtyDelta: number;
  unitCost: number;
  sourceType: SourceType;
  sourceId: string | null;
  note: string | null;
  productId: string;
  sku: string;
  productName: string;
  /** The product has since been deleted. The row stays — a movement that
   *  happened is still a movement that happened (§6.9.3). */
  productDeleted: boolean;
  warehouseId: string;
  warehouseCode: string;
  createdById: string;
  createdByName: string;
};

export type ProductWrite = {
  sku?: string;
  name?: string;
  uom?: string;
  /** Sent as strings so a decimal never crosses the wire as a float the browser
   *  rounded on the way out. The endpoint accepts both forms. */
  reorderPoint?: string;
  standardCost?: string;
  isActive?: boolean;
};

export type WarehouseWrite = {
  code?: string;
  name?: string;
  isActive?: boolean;
};

/** `includeDeleted` is module `admin` only (§9.0) — the server refuses it for
 *  anyone else, so the toggle that sets it is hidden rather than disabled. */
export type MasterDataQuery = ListQuery & { includeDeleted?: boolean };

export function listProducts(query: MasterDataQuery = {}) {
  return apiFetch<ListResponse<Product>>(
    `/api/inventory/products${queryString(query, {
      includeDeleted: query.includeDeleted ? "true" : undefined,
    })}`,
  );
}

export function getProduct(id: string) {
  return apiFetch<ProductDetail>(`/api/inventory/products/${id}`);
}

export function createProduct(body: ProductWrite) {
  return apiFetch<ProductDetail>("/api/inventory/products", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function patchProduct(id: string, body: ProductWrite) {
  return apiFetch<ProductDetail>(`/api/inventory/products/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

/** Soft delete. The row keeps its identity so history still resolves (§6.9.1);
 *  nothing is removed from the database. */
export function deleteProduct(id: string) {
  return apiFetch<ProductDetail>(`/api/inventory/products/${id}`, {
    method: "DELETE",
  });
}

export function restoreProduct(id: string) {
  return apiFetch<ProductDetail>(`/api/inventory/products/${id}/restore`, {
    method: "POST",
  });
}

export function listWarehouses(query: MasterDataQuery = {}) {
  return apiFetch<ListResponse<Warehouse>>(
    `/api/inventory/warehouses${queryString(query, {
      includeDeleted: query.includeDeleted ? "true" : undefined,
    })}`,
  );
}

export function createWarehouse(body: WarehouseWrite) {
  return apiFetch<Warehouse>("/api/inventory/warehouses", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function patchWarehouse(id: string, body: WarehouseWrite) {
  return apiFetch<Warehouse>(`/api/inventory/warehouses/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteWarehouse(id: string) {
  return apiFetch<Warehouse>(`/api/inventory/warehouses/${id}`, {
    method: "DELETE",
  });
}

export function restoreWarehouse(id: string) {
  return apiFetch<Warehouse>(`/api/inventory/warehouses/${id}/restore`, {
    method: "POST",
  });
}

export type StockQuery = ListQuery & {
  productId?: string;
  warehouseId?: string;
};

export function listStock(query: StockQuery = {}) {
  return apiFetch<ListResponse<StockCell>>(
    `/api/inventory/stock${queryString(query, {
      productId: query.productId,
      warehouseId: query.warehouseId,
    })}`,
  );
}

export function listLowStock(query: ListQuery = {}) {
  return apiFetch<ListResponse<LowStockRow>>(
    `/api/inventory/stock/low${queryString(query)}`,
  );
}

export type LedgerQuery = ListQuery & {
  productId?: string;
  warehouseId?: string;
  entryType?: EntryType | "";
  sourceType?: SourceType | "";
};

export function listLedger(query: LedgerQuery = {}) {
  return apiFetch<ListResponse<LedgerEntry>>(
    `/api/inventory/ledger${queryString(query, {
      productId: query.productId,
      warehouseId: query.warehouseId,
      entryType: query.entryType || undefined,
      sourceType: query.sourceType || undefined,
    })}`,
  );
}

export type AdjustmentRequest = {
  productId: string;
  warehouseId: string;
  /** Signed decimal text: `+` in, `-` out. A string, so the browser cannot
   *  round it before the server sees it. */
  qtyDelta: string;
  note?: string;
};

export function postAdjustment(body: AdjustmentRequest) {
  return apiFetch<{ entry: LedgerEntry; qtyOnHand: number }>(
    "/api/inventory/adjustments",
    { method: "POST", body: JSON.stringify(body) },
  );
}
