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
 * The entitlement-went-away channel (FE6).
 *
 * WHY THIS EXISTS. Authorization is resolved from the database on every request
 * (I9), so the *server* is never stale — but the navigation and every cosmetic
 * guard are built from one `/api/me` read taken at sign-in. Disable Finance for a
 * workspace while somebody is reading `/finance` and `RequireModule` in the
 * browser still says yes, because `me.moduleRoles` is a minute old. The next
 * request comes back `403 module_not_enabled`, and without this the reader is left
 * on a screen of a module they no longer have, holding an inline error, with a
 * sidebar still offering the link that produced it.
 *
 * `insufficient_module_role` is deliberately NOT routed here. That refusal means
 * you are in the right module and not senior enough for one action, and the answer
 * to it is the inline refusal where the action was — not being thrown off the
 * screen.
 *
 * A Set rather than a single slot: StrictMode mounts the subscriber twice, and a
 * single slot would be left holding whichever effect happened to clean up last.
 */
type EntitlementListener = (error: ApiError) => void;

const entitlementListeners = new Set<EntitlementListener>();

export function onEntitlementRevoked(listener: EntitlementListener): () => void {
  entitlementListeners.add(listener);
  return () => entitlementListeners.delete(listener);
}

/**
 * The single way this app talks to the backend. Every call carries the Firebase
 * ID token.
 *
 * **Not exported.** Every endpoint in this file has a named wrapper with a return
 * type, and a screen reaching past them would be a request whose shape nothing
 * checks — the first step towards a URL spelled two ways.
 *
 * `getIdToken()` is the whole of token lifecycle management: it returns the
 * cached token and silently refreshes when it is close to expiry. Storing one
 * ourselves would mean reimplementing that badly, and putting it in
 * localStorage would make it stealable by any injected script.
 */
async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
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
    const error = new ApiError(
      response.status,
      envelope.error ?? "unknown",
      envelope.message ?? response.statusText,
    );
    // Announced before it is thrown, so the caller's own `.catch` still runs and
    // still gets to say what it was doing. The listener handles the part no
    // individual call site can: the screen is now the wrong screen.
    if (error.status === 403 && error.code === "module_not_enabled") {
      for (const listener of [...entitlementListeners]) listener(error);
    }
    throw error;
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

function queryString(
  query: ListQuery,
  extra: Record<string, string | undefined> = {},
): string {
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

/* fallow reports the four types below as unused exports. They are not: each is the
   return type of an exported function just above or below it, so removing the
   export makes that signature unusable by any caller. Phase 4 identified this as a
   false-positive class and said to suppress rather than "fix" it — a dead-code
   report that cries wolf four times is one nobody reads the fifth time. */
// The return type of listModuleCatalogue.
// fallow-ignore-next-line unused-type
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
  return apiFetch<{
    tenant: TenantDetail;
    admin: { id: string; email: string };
  }>("/api/admin/tenants", { method: "POST", body: JSON.stringify(body) });
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

export function setTenantModule(
  id: string,
  code: ModuleCode,
  enabled: boolean,
) {
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
  body: {
    fullName?: string;
    isActive?: boolean;
    tenantRole?: "staff" | "admin";
  },
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

// The return type of listStock.
// fallow-ignore-next-line unused-type
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

// The return type of listLowStock.
// fallow-ignore-next-line unused-type
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
  /** Resolved from the source document, so a row can name where it came from
   *  instead of showing a UUID. Null for a manual adjustment, which has no
   *  document behind it — the person is the source (§6.3). */
  sourceNumber: string | null;
  sourcePoId: string | null;
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
  /** The rows one document wrote. This is what the goods receipt confirmation
   *  panel links to — "2 stock ledger entries created" has to be followed by the
   *  two entries themselves. */
  sourceId?: string;
};

export function listLedger(query: LedgerQuery = {}) {
  return apiFetch<ListResponse<LedgerEntry>>(
    `/api/inventory/ledger${queryString(query, {
      productId: query.productId,
      warehouseId: query.warehouseId,
      entryType: query.entryType || undefined,
      sourceType: query.sourceType || undefined,
      sourceId: query.sourceId,
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

// --------------------------------------------------------------------------
// Procurement (§9.4, §10.3).
//
// The same rule as inventory applies to every number below: it arrives as the
// answer PostgreSQL computed, for display. `estimatedTotal`, `totalAmount`,
// `lineTotal`, and `qtyOutstanding` are all summed server-side where the values
// are still NUMERIC (I8) — do not re-derive one of them here, because a second
// implementation of a rule is one that can disagree with the one that counts.
// --------------------------------------------------------------------------

export type Supplier = {
  id: string;
  code: string;
  name: string;
  contactEmail: string | null;
  contactPhone: string | null;
  leadTimeDays: number;
  paymentTerms: string;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
  /** Orders that would refuse a delete (G4). Shown in the list so the refusal is
   *  not the first the user hears of it. */
  openOrders: number;
};

export type SupplierWrite = {
  code?: string;
  name?: string;
  /** An empty string clears these two; omitting them leaves them alone. */
  contactEmail?: string;
  contactPhone?: string;
  leadTimeDays?: number;
  paymentTerms?: string;
  isActive?: boolean;
};

export function listSuppliers(query: MasterDataQuery = {}) {
  return apiFetch<ListResponse<Supplier>>(
    `/api/procurement/suppliers${queryString(query, {
      includeDeleted: query.includeDeleted ? "true" : undefined,
    })}`,
  );
}

export function createSupplier(body: SupplierWrite) {
  return apiFetch<Supplier>("/api/procurement/suppliers", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function patchSupplier(id: string, body: SupplierWrite) {
  return apiFetch<Supplier>(`/api/procurement/suppliers/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteSupplier(id: string) {
  return apiFetch<Supplier>(`/api/procurement/suppliers/${id}`, {
    method: "DELETE",
  });
}

export function restoreSupplier(id: string) {
  return apiFetch<Supplier>(`/api/procurement/suppliers/${id}/restore`, {
    method: "POST",
  });
}

export type RequisitionStatus =
  "draft" | "submitted" | "approved" | "rejected" | "cancelled";

export type RequisitionLine = {
  id: string;
  lineNo: number;
  productId: string;
  sku: string;
  productName: string;
  uom: string;
  /** The product has since been deleted. The line stays — a requisition is a
   *  historical record (§6.9.1). */
  productDeleted: boolean;
  qty: number;
  estUnitCost: number;
  lineTotal: number;
};

export type Requisition = {
  id: string;
  prNumber: string;
  status: RequisitionStatus;
  warehouseId: string;
  warehouseCode: string;
  warehouseName: string;
  supplierId: string | null;
  supplierCode: string | null;
  supplierName: string | null;
  notes: string | null;

  requestedById: string;
  requestedByName: string;
  submittedAt: string | null;
  decidedById: string | null;
  decidedByName: string | null;
  decidedAt: string | null;
  rejectReason: string | null;
  cancelledById: string | null;
  cancelledByName: string | null;
  cancelledAt: string | null;
  cancelReason: string | null;

  createdAt: string;
  updatedAt: string;
  lineCount: number;
  estimatedTotal: number;

  /** The order approval generated, so the detail screen can link straight to it. */
  purchaseOrderId: string | null;
  purchaseOrderNumber: string | null;
};

export type RequisitionDetail = Requisition & { lines: RequisitionLine[] };

/** One line as the API takes it. Quantities and costs are strings, so the
 *  browser cannot round a decimal before the server sees it. */
export type RequisitionLineWrite = {
  productId: string;
  qty: string;
  /** Absent means the product's standard cost, which is what stops a purchase
   *  order — and the journal entry behind its receipt — being valued at zero. */
  estUnitCost?: string;
};

export type RequisitionWrite = {
  warehouseId?: string;
  /** An empty string clears it: a requisition may legitimately not name a
   *  supplier until approval. */
  supplierId?: string;
  notes?: string;
  /** When present, REPLACES the whole set of lines. Drafts only. */
  lines?: RequisitionLineWrite[];
};

export type RequisitionQuery = ListQuery & { status?: RequisitionStatus | "" };

export function listRequisitions(query: RequisitionQuery = {}) {
  return apiFetch<ListResponse<Requisition>>(
    `/api/procurement/requisitions${queryString(query, {
      status: query.status || undefined,
    })}`,
  );
}

export function getRequisition(id: string) {
  return apiFetch<RequisitionDetail>(`/api/procurement/requisitions/${id}`);
}

export function createRequisition(body: RequisitionWrite) {
  return apiFetch<RequisitionDetail>("/api/procurement/requisitions", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function patchRequisition(id: string, body: RequisitionWrite) {
  return apiFetch<RequisitionDetail>(`/api/procurement/requisitions/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function submitRequisition(id: string) {
  return apiFetch<RequisitionDetail>(
    `/api/procurement/requisitions/${id}/submit`,
    { method: "POST" },
  );
}

/** `supplierId` is required only when the requisition does not already name one
 *  (§8.3) — a purchase order has to be addressed to somebody. */
export function approveRequisition(id: string, supplierId?: string) {
  return apiFetch<RequisitionDetail>(
    `/api/procurement/requisitions/${id}/approve`,
    { method: "POST", body: JSON.stringify(supplierId ? { supplierId } : {}) },
  );
}

export function rejectRequisition(id: string, reason: string) {
  return apiFetch<RequisitionDetail>(
    `/api/procurement/requisitions/${id}/reject`,
    { method: "POST", body: JSON.stringify({ reason }) },
  );
}

export function cancelRequisition(id: string, reason: string) {
  return apiFetch<RequisitionDetail>(
    `/api/procurement/requisitions/${id}/cancel`,
    { method: "POST", body: JSON.stringify({ reason }) },
  );
}

export type PurchaseOrderStatus =
  "open" | "partially_received" | "received" | "cancelled";

export type PurchaseOrderLine = {
  id: string;
  lineNo: number;
  productId: string;
  sku: string;
  productName: string;
  uom: string;
  productDeleted: boolean;
  qtyOrdered: number;
  unitCost: number;
  lineTotal: number;
  /** Derived from the goods receipt lines through `po_line_status`, never stored
   *  (I6). */
  qtyReceived: number;
  qtyOutstanding: number;
};

export type PurchaseOrder = {
  id: string;
  poNumber: string;
  status: PurchaseOrderStatus;
  supplierId: string;
  supplierCode: string;
  supplierName: string;
  warehouseId: string;
  warehouseCode: string;
  warehouseName: string;
  requisitionId: string | null;
  requisitionNumber: string | null;
  totalAmount: number;
  orderedAt: string;
  /** A business date as `YYYY-MM-DD`, not an instant — rendered as it arrives, so
   *  no timezone can move it a day (§2.5.3). */
  expectedAt: string | null;
  createdById: string;
  createdByName: string;
  cancelledById: string | null;
  cancelledByName: string | null;
  cancelledAt: string | null;
  cancelReason: string | null;
  updatedAt: string;
  lineCount: number;
  qtyOrdered: number;
  qtyReceived: number;
  qtyOutstanding: number;
};

// The return type of getPurchaseOrder. No suppression needed since Phase 5B: the
// order detail screen names it directly, now that its sections are components.
export type PurchaseOrderDetail = PurchaseOrder & {
  lines: PurchaseOrderLine[];
};

export type PurchaseOrderQuery = ListQuery & {
  status?: PurchaseOrderStatus | "";
  supplierId?: string;
};

export function listPurchaseOrders(query: PurchaseOrderQuery = {}) {
  return apiFetch<ListResponse<PurchaseOrder>>(
    `/api/procurement/purchase-orders${queryString(query, {
      status: query.status || undefined,
      supplierId: query.supplierId,
    })}`,
  );
}

export function getPurchaseOrder(id: string) {
  return apiFetch<PurchaseOrderDetail>(
    `/api/procurement/purchase-orders/${id}`,
  );
}

export function cancelPurchaseOrder(id: string, reason: string) {
  return apiFetch<PurchaseOrderDetail>(
    `/api/procurement/purchase-orders/${id}/cancel`,
    { method: "POST", body: JSON.stringify({ reason }) },
  );
}

// --------------------------------------------------------------------------
// Goods receipts — the cross-module transaction (§8.4).
// --------------------------------------------------------------------------

export type GoodsReceiptLine = {
  id: string;
  poLineId: string;
  lineNo: number;
  productId: string;
  sku: string;
  productName: string;
  uom: string;
  productDeleted: boolean;
  qtyReceived: number;
  unitCost: number;
  lineTotal: number;
};

export type GoodsReceipt = {
  id: string;
  grNumber: string;
  poId: string;
  poNumber: string;
  /** Where the order was left by this receipt. */
  poStatus: PurchaseOrderStatus;
  supplierId: string;
  supplierName: string;
  warehouseId: string;
  warehouseCode: string;
  warehouseName: string;
  receivedById: string;
  receivedByName: string;
  receivedAt: string;
  note: string | null;
  lineCount: number;
  qtyReceived: number;
  /** SUM(qty × unit cost) — the amount the journal entry was posted for, summed
   *  server-side in the same expression finance used. */
  totalValue: number;
};

// Part of ReceiptResult below, and what a receipt detail screen would read.
// fallow-ignore-next-line unused-type
export type GoodsReceiptDetail = GoodsReceipt & { lines: GoodsReceiptLine[] };

/** What the receipt wrote, across all three modules. This is the shape the
 *  confirmation panel of §10.3 renders — the screenshot the project exists for. */
export type ReceiptResult = {
  receipt: GoodsReceiptDetail;
  purchaseOrder: { id: string; poNumber: string; status: PurchaseOrderStatus };
  inventory: { ledgerEntryIds: string[]; entryCount: number };
  finance: {
    journalEntryId: string;
    entryNumber: string;
    amount: number;
    debitAccountId: string;
    debitAccountCode: string;
    debitAccountName: string;
    creditAccountId: string;
    creditAccountCode: string;
    creditAccountName: string;
  };
  /** True when this response was rebuilt for a repeated Idempotency-Key: the
   *  receipt had already been posted and nothing was written twice (§8.6.1). */
  replayed: boolean;
};

export type ReceiptLineWrite = {
  poLineId: string;
  /** A decimal string, so the browser cannot round a quantity before the server
   *  sees it (I8). */
  qtyReceived: string;
};

export function listGoodsReceipts(query: ListQuery & { poId?: string } = {}) {
  return apiFetch<ListResponse<GoodsReceipt>>(
    `/api/procurement/goods-receipts${queryString(query, { poId: query.poId })}`,
  );
}

/* There is deliberately no `getGoodsReceipt` wrapper. `GET /goods-receipts/:id`
   exists and is tested — §9.4 specifies it — but no screen reads a receipt on its
   own: the confirmation panel shows the one just posted, and the order screen shows
   the history. A client function nothing calls is the kind of symmetry §9.6.1 warns
   about, and adding it back is one line whenever a receipt detail screen exists. */

/**
 * Post a goods receipt (§8.4). One request, one transaction, three modules.
 *
 * `idempotencyKey` MUST be generated when the form opens and stay the same across
 * every retry of that form (§8.6.1) — see `useIdempotencyKey`. A receipt is posted
 * from a loading dock on warehouse wifi, where a request that times out
 * client-side but succeeded server-side is an ordinary Tuesday: without a stable
 * key, tapping "Post receipt" again credits the stock twice with two journal
 * entries to match, and nothing in the schema would flag it.
 */
export function postGoodsReceipt(
  poId: string,
  idempotencyKey: string,
  lines: ReceiptLineWrite[],
  note?: string,
) {
  return apiFetch<ReceiptResult>(
    `/api/procurement/purchase-orders/${poId}/receipts`,
    {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({ lines, note }),
    },
  );
}

// --------------------------------------------------------------------------
// Finance (§9.6, §10.5).
//
// Reads only. The module is a stub: there is no manual journal entry and no
// account CRUD in the MVP, so a `postJournalEntry` here would be a client
// function for an endpoint that does not exist. Every entry the list returns was
// written by a goods receipt inside the transaction that received the goods.
// --------------------------------------------------------------------------

export type AccountType =
  | "asset"
  | "liability"
  | "equity"
  | "revenue"
  | "expense";

export type Account = {
  id: string;
  code: string;
  name: string;
  type: AccountType;
  isActive: boolean;
};

export type JournalEntryLine = {
  id: string;
  journalEntryId: string;
  accountId: string;
  accountCode: string;
  accountName: string;
  accountType: AccountType;
  /** One of these is zero on every line — `journal_entry_lines` has a CHECK that
   *  says so. Both are the exact decimals PostgreSQL holds (I8). */
  debit: number;
  credit: number;
  memo: string | null;
};

export type JournalSourceType = "goods_receipt" | "manual";

export type JournalEntry = {
  id: string;
  entryNumber: string;
  postedAt: string;
  description: string;
  sourceType: JournalSourceType;
  sourceId: string | null;
  /** The GR number and the order behind a `goods_receipt` entry, resolved
   *  server-side so the screen links to a document rather than printing a UUID. */
  sourceNumber: string | null;
  sourcePoId: string | null;
  /** The total of the debit side, summed in SQL. The credit side equals it by
   *  construction — `jel_balanced` refuses to commit anything else — so do not
   *  add the lines up here to check: a second implementation of a rule is one
   *  that can disagree with the one that counts. */
  amount: number;
  createdById: string;
  createdByName: string;
  lines: JournalEntryLine[];
};

export type JournalEntryQuery = ListQuery & {
  sourceType?: JournalSourceType | "";
  /** The entry one document posted — the counterpart of `LedgerQuery.sourceId`,
   *  and what the goods receipt confirmation panel links to (§10.3). */
  sourceId?: string;
  accountId?: string;
};

export function listJournalEntries(query: JournalEntryQuery = {}) {
  return apiFetch<ListResponse<JournalEntry>>(
    `/api/finance/journal-entries${queryString(query, {
      sourceType: query.sourceType || undefined,
      sourceId: query.sourceId,
      accountId: query.accountId,
    })}`,
  );
}

export function listAccounts(query: ListQuery = {}) {
  return apiFetch<ListResponse<Account>>(
    `/api/finance/accounts${queryString(query)}`,
  );
}

// --------------------------------------------------------------------------
// Dashboard (§9.7, §10.2).
//
// One request for the whole home screen. Every field is optional because the
// server omits a widget the caller cannot read — and that is the whole design:
// `{ lowStock: { count: 0 } }` says "nothing is low", while the widget's absence
// says "you cannot see Inventory". A screen that could not tell those apart
// would show a stock panel reporting zero to somebody with no stock access.
//
// So these are `?:` rather than `| null`, and the page renders `summary.lowStock
// && <LowStockWidget …>`. Do not default them.
// --------------------------------------------------------------------------

export type OpenOrdersWidget = {
  /** `open` **and** `partially_received`: an order half of which has arrived is
   *  still an order somebody is waiting on. */
  count: number;
  totalValue: number;
};

export type PendingApprovalsWidget = {
  /** Everybody's number, including requisitions this caller may not approve. */
  count: number;
  /** Whether this caller holds `approver`, and so whether `queue` is populated.
   *  Sent explicitly: an empty queue is otherwise indistinguishable from
   *  "nothing is waiting". */
  canApprove: boolean;
  /** The oldest few, for the inline decision §10.2 gives an approver. Includes
   *  the caller's own submissions — hiding them would make the count disagree
   *  with the list underneath it, and C2 refuses them server-side anyway. */
  queue: Requisition[];
};

export type LowStockWidget = {
  count: number;
  /** The worst few by shortfall. `count` is the real total, so a reader knows
   *  when there is more to see. */
  products: LowStockRow[];
};

export type RecentActivityWidget = {
  entries: LedgerEntry[];
};

// fallow-ignore-next-line unused-type
export type DashboardSummary = {
  openOrders?: OpenOrdersWidget;
  pendingApprovals?: PendingApprovalsWidget;
  lowStock?: LowStockWidget;
  recentActivity?: RecentActivityWidget;
};

export function getDashboardSummary() {
  return apiFetch<DashboardSummary>("/api/dashboard/summary");
}
