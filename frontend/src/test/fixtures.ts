/**
 * The fixtures, named after the seeded accounts of §15.
 *
 * Using the demo's own people rather than `user1`/`user2` is not decoration: the
 * entitlement shapes these tests turn on are the ones the acceptance test walks,
 * so `budi` really is the approver who is not the author and `agus` really is a
 * tenant admin with no Finance entitlement. A test that reads
 * `renderApp("/finance", agus)` says what it is checking.
 */

import type {
  DashboardSummary,
  GoodsReceipt,
  JournalEntry,
  LedgerEntry,
  ListResponse,
  Me,
  Product,
  PurchaseOrderDetail,
  PurchaseOrderLine,
  ReceiptResult,
  Requisition,
  Supplier,
  Warehouse,
} from "../lib/api";

// UUIDs are written out rather than generated, so a failure message names a row a
// reader can find in this file.
export const NUSANTARA = "11111111-1111-4111-8111-111111111111";
export const BAHARI = "22222222-2222-4222-8222-222222222222";
export const WH_MAIN = "33333333-3333-4333-8333-333333333331";
export const PRODUCT_GLOVE = "44444444-4444-4444-8444-444444444441";
export const PRODUCT_BOX = "44444444-4444-4444-8444-444444444442";
export const SUPPLIER_ACME = "55555555-5555-4555-8555-555555555551";
export const PO_OPEN = "66666666-6666-4666-8666-666666666661";
export const PO_LINE_1 = "77777777-7777-4777-8777-777777777771";
export const PO_LINE_2 = "77777777-7777-4777-8777-777777777772";
export const RECEIPT = "88888888-8888-4888-8888-888888888881";

// ---------------------------------------------------------------------------
// Identities.
// ---------------------------------------------------------------------------

function nusantaraUser(
  id: string,
  email: string,
  fullName: string,
  tenantRole: Me["user"]["tenantRole"],
  moduleRoles: Me["moduleRoles"],
): Me {
  return {
    user: { id, email, fullName, tenantRole },
    tenant: {
      id: NUSANTARA,
      name: "Nusantara Retail",
      slug: "nusantara",
      status: "active",
      timezone: "Asia/Jakarta",
    },
    moduleRoles,
  };
}

/** Tenant admin. `moduleRoles` carries the *effective* map, so her implicit
 *  `admin` in all three modules is already resolved — the server does that
 *  before sending it, and `holds` relies on it. */
export const rina = nusantaraUser(
  "aaaaaaaa-0000-4000-8000-000000000001",
  "rina@nusantara.test",
  "Rina Wijaya",
  "admin",
  { procurement: "admin", inventory: "admin", finance: "admin" },
);

/** The approver. Holds nothing in Finance, which is what FE1 and FE8 read. */
export const budi = nusantaraUser(
  "aaaaaaaa-0000-4000-8000-000000000002",
  "budi@nusantara.test",
  "Budi Santoso",
  "staff",
  { procurement: "approver", inventory: "viewer" },
);

/** `user` in procurement: may raise a requisition, may not approve one (FE2),
 *  may not receive goods. */
export const sari = nusantaraUser(
  "aaaaaaaa-0000-4000-8000-000000000003",
  "sari@nusantara.test",
  "Sari Lestari",
  "staff",
  { procurement: "user", inventory: "user" },
);

/** Finance admin, procurement viewer, nothing in Inventory. */
export const dewi = nusantaraUser(
  "aaaaaaaa-0000-4000-8000-000000000004",
  "dewi@nusantara.test",
  "Dewi Anggraini",
  "staff",
  { procurement: "viewer", finance: "admin" },
);

/** Bahari's tenant admin. The workspace has **no Finance entitlement**, so even
 *  an admin's implicit `admin` does not appear for it — the entitlement ceiling
 *  is applied before `moduleRoles` is sent. */
export const agus: Me = {
  user: {
    id: "bbbbbbbb-0000-4000-8000-000000000001",
    email: "agus@bahari.test",
    fullName: "Agus Pratama",
    tenantRole: "admin",
  },
  tenant: {
    id: BAHARI,
    name: "Bahari Logistics",
    slug: "bahari",
    status: "active",
    timezone: "Asia/Makassar",
  },
  moduleRoles: { procurement: "admin", inventory: "admin" },
};

/** The platform account. Belongs to no tenant and holds no module. */
export const superadmin: Me = {
  user: {
    id: "cccccccc-0000-4000-8000-000000000001",
    email: "super@erp.test",
    fullName: "Platform Operator",
    tenantRole: "superadmin",
  },
  tenant: null,
  moduleRoles: {},
};

export const EVERYONE = [rina, budi, sari, dewi, agus, superadmin];

// ---------------------------------------------------------------------------
// Rows.
// ---------------------------------------------------------------------------

/** The §9.0 list envelope. `totalItems` is always the real total, because half
 *  of §10.7.4 depends on the count and the rows answering the same question. */
export function page<T>(rows: T[], overrides: Partial<ListResponse<T>> = {}): ListResponse<T> {
  const pageSize = overrides.pageSize ?? 25;
  const totalItems = overrides.totalItems ?? rows.length;
  return {
    data: rows,
    page: overrides.page ?? 1,
    pageSize,
    totalItems,
    totalPages: overrides.totalPages ?? Math.max(1, Math.ceil(totalItems / pageSize)),
  };
}

export function requisition(overrides: Partial<Requisition> = {}): Requisition {
  return {
    id: "99999999-9999-4999-8999-999999999991",
    prNumber: "PR-202607-0001",
    status: "submitted",
    warehouseId: WH_MAIN,
    warehouseCode: "WH-MAIN",
    warehouseName: "Main warehouse",
    supplierId: SUPPLIER_ACME,
    supplierCode: "SUP-ACME",
    supplierName: "Acme Supplies",
    notes: null,
    requestedById: sari.user.id,
    requestedByName: sari.user.fullName,
    // 2026-07-20T23:30:00Z is 2026-07-21 06:30 in Asia/Jakarta and still
    // 2026-07-20 in London. FE15 is that difference.
    submittedAt: "2026-07-20T23:30:00Z",
    decidedById: null,
    decidedByName: null,
    decidedAt: null,
    rejectReason: null,
    cancelledById: null,
    cancelledByName: null,
    cancelledAt: null,
    cancelReason: null,
    createdAt: "2026-07-20T23:30:00Z",
    updatedAt: "2026-07-20T23:30:00Z",
    lineCount: 2,
    estimatedTotal: 1250.5,
    purchaseOrderId: null,
    purchaseOrderNumber: null,
    ...overrides,
  };
}

export function poLine(overrides: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  const qtyOrdered = overrides.qtyOrdered ?? 100;
  const qtyReceived = overrides.qtyReceived ?? 0;
  return {
    id: PO_LINE_1,
    lineNo: 1,
    productId: PRODUCT_GLOVE,
    sku: "HND-GLOVE",
    productName: "Nitrile gloves, box of 100",
    uom: "box",
    productDeleted: false,
    qtyOrdered,
    unitCost: 12.5,
    lineTotal: 1250,
    qtyReceived,
    qtyOutstanding: overrides.qtyOutstanding ?? qtyOrdered - qtyReceived,
    ...overrides,
  };
}

export function purchaseOrder(
  overrides: Partial<PurchaseOrderDetail> = {},
): PurchaseOrderDetail {
  const lines = overrides.lines ?? [
    poLine(),
    poLine({
      id: PO_LINE_2,
      lineNo: 2,
      productId: PRODUCT_BOX,
      sku: "PKG-BOX-L",
      productName: "Cardboard box, large",
      uom: "each",
      qtyOrdered: 40,
      qtyReceived: 10,
      unitCost: 3,
      lineTotal: 120,
    }),
  ];
  return {
    id: PO_OPEN,
    poNumber: "PO-202607-0001",
    status: "open",
    supplierId: SUPPLIER_ACME,
    supplierCode: "SUP-ACME",
    supplierName: "Acme Supplies",
    warehouseId: WH_MAIN,
    warehouseCode: "WH-MAIN",
    warehouseName: "Main warehouse",
    requisitionId: null,
    requisitionNumber: null,
    totalAmount: 1370,
    orderedAt: "2026-07-21T02:00:00Z",
    expectedAt: "2026-07-28",
    createdById: budi.user.id,
    createdByName: budi.user.fullName,
    cancelledById: null,
    cancelledByName: null,
    cancelledAt: null,
    cancelReason: null,
    updatedAt: "2026-07-21T02:00:00Z",
    lineCount: lines.length,
    qtyOrdered: 140,
    qtyReceived: 10,
    qtyOutstanding: 130,
    ...overrides,
    lines,
  };
}

export function goodsReceipt(overrides: Partial<GoodsReceipt> = {}): GoodsReceipt {
  return {
    id: RECEIPT,
    grNumber: "GR-202607-0001",
    poId: PO_OPEN,
    poNumber: "PO-202607-0001",
    poStatus: "partially_received",
    supplierId: SUPPLIER_ACME,
    supplierName: "Acme Supplies",
    warehouseId: WH_MAIN,
    warehouseCode: "WH-MAIN",
    warehouseName: "Main warehouse",
    receivedById: budi.user.id,
    receivedByName: budi.user.fullName,
    receivedAt: "2026-07-22T04:15:00Z",
    note: null,
    lineCount: 2,
    qtyReceived: 130,
    totalValue: 1340,
    ...overrides,
  };
}

/**
 * What §8.4 wrote, across all three modules — the shape the confirmation panel
 * of §10.3 renders, and what FE5 reads.
 */
export function receiptResult(overrides: Partial<ReceiptResult> = {}): ReceiptResult {
  const receipt = goodsReceipt();
  return {
    receipt: {
      ...receipt,
      lines: [
        {
          id: "aaaa1111-1111-4111-8111-111111111111",
          poLineId: PO_LINE_1,
          lineNo: 1,
          productId: PRODUCT_GLOVE,
          sku: "HND-GLOVE",
          productName: "Nitrile gloves, box of 100",
          uom: "box",
          productDeleted: false,
          qtyReceived: 100,
          unitCost: 12.5,
          lineTotal: 1250,
        },
        {
          id: "aaaa1111-1111-4111-8111-111111111112",
          poLineId: PO_LINE_2,
          lineNo: 2,
          productId: PRODUCT_BOX,
          sku: "PKG-BOX-L",
          productName: "Cardboard box, large",
          uom: "each",
          productDeleted: false,
          qtyReceived: 30,
          unitCost: 3,
          lineTotal: 90,
        },
      ],
    },
    purchaseOrder: {
      id: PO_OPEN,
      poNumber: "PO-202607-0001",
      status: "received",
    },
    inventory: {
      ledgerEntryIds: [
        "bbbb1111-1111-4111-8111-111111111111",
        "bbbb1111-1111-4111-8111-111111111112",
      ],
      entryCount: 2,
    },
    finance: {
      journalEntryId: "cccc1111-1111-4111-8111-111111111111",
      entryNumber: "JE-202607-0001",
      amount: 1340,
      debitAccountId: "dddd1111-1111-4111-8111-111111111111",
      debitAccountCode: "1300",
      debitAccountName: "Inventory",
      creditAccountId: "dddd1111-1111-4111-8111-111111111112",
      creditAccountCode: "2150",
      creditAccountName: "Goods received not invoiced",
    },
    replayed: false,
    ...overrides,
  };
}

export function ledgerEntry(overrides: Partial<LedgerEntry> = {}): LedgerEntry {
  return {
    id: "bbbb1111-1111-4111-8111-111111111111",
    occurredAt: "2026-07-22T04:15:00Z",
    entryType: "receipt",
    qtyDelta: 100,
    unitCost: 12.5,
    sourceType: "goods_receipt",
    sourceId: RECEIPT,
    sourceNumber: "GR-202607-0001",
    sourcePoId: PO_OPEN,
    note: null,
    productId: PRODUCT_GLOVE,
    sku: "HND-GLOVE",
    productName: "Nitrile gloves, box of 100",
    productDeleted: false,
    warehouseId: WH_MAIN,
    warehouseCode: "WH-MAIN",
    createdById: budi.user.id,
    createdByName: budi.user.fullName,
    ...overrides,
  };
}

export function journalEntry(overrides: Partial<JournalEntry> = {}): JournalEntry {
  return {
    id: "cccc1111-1111-4111-8111-111111111111",
    entryNumber: "JE-202607-0001",
    postedAt: "2026-07-22T04:15:00Z",
    description: "Goods receipt GR-202607-0001",
    sourceType: "goods_receipt",
    sourceId: RECEIPT,
    sourceNumber: "GR-202607-0001",
    sourcePoId: PO_OPEN,
    amount: 1340,
    createdById: budi.user.id,
    createdByName: budi.user.fullName,
    lines: [
      {
        id: "eeee1111-1111-4111-8111-111111111111",
        journalEntryId: "cccc1111-1111-4111-8111-111111111111",
        accountId: "dddd1111-1111-4111-8111-111111111111",
        accountCode: "1300",
        accountName: "Inventory",
        accountType: "asset",
        debit: 1340,
        credit: 0,
        memo: null,
      },
      {
        id: "eeee1111-1111-4111-8111-111111111112",
        journalEntryId: "cccc1111-1111-4111-8111-111111111111",
        accountId: "dddd1111-1111-4111-8111-111111111112",
        accountCode: "2150",
        accountName: "Goods received not invoiced",
        accountType: "liability",
        debit: 0,
        credit: 1340,
        memo: null,
      },
    ],
    ...overrides,
  };
}

export function product(overrides: Partial<Product> = {}): Product {
  return {
    id: PRODUCT_GLOVE,
    sku: "HND-GLOVE",
    name: "Nitrile gloves, box of 100",
    uom: "box",
    reorderPoint: 20,
    standardCost: 12.5,
    isActive: true,
    createdAt: "2026-06-01T00:00:00Z",
    updatedAt: "2026-06-01T00:00:00Z",
    deletedAt: null,
    qtyOnHand: 140,
    belowReorderPoint: false,
    ...overrides,
  };
}

export function warehouse(overrides: Partial<Warehouse> = {}): Warehouse {
  return {
    id: WH_MAIN,
    code: "WH-MAIN",
    name: "Main warehouse",
    isActive: true,
    createdAt: "2026-06-01T00:00:00Z",
    updatedAt: "2026-06-01T00:00:00Z",
    deletedAt: null,
    qtyOnHand: 1420,
    productCount: 7,
    ...overrides,
  };
}

export function supplier(overrides: Partial<Supplier> = {}): Supplier {
  return {
    id: SUPPLIER_ACME,
    code: "SUP-ACME",
    name: "Acme Supplies",
    contactEmail: "sales@acme.test",
    contactPhone: "+62 21 555 0100",
    leadTimeDays: 7,
    paymentTerms: "NET30",
    isActive: true,
    createdAt: "2026-06-01T00:00:00Z",
    updatedAt: "2026-06-01T00:00:00Z",
    deletedAt: null,
    openOrders: 0,
    ...overrides,
  };
}

/** The dashboard omits a widget the caller cannot read, which is the whole
 *  design — `{ lowStock: { count: 0 } }` says "nothing is low" and the widget's
 *  absence says "you cannot see Inventory". So this defaults to empty. */
export function dashboard(overrides: DashboardSummary = {}): DashboardSummary {
  return { ...overrides };
}
