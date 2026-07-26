import { useAsync } from "../hooks/useAsync";
import {
  listProducts,
  listSuppliers,
  listWarehouses,
  type Product,
  type RequisitionLineWrite,
  type Supplier,
  type Warehouse,
} from "./api";

/**
 * The state behind the requisition form, shared by `/new` and by editing a draft
 * in place.
 *
 * It is separate from the component that renders it because there are two callers,
 * which is the bar §4 sets for abstracting — and because the alternative is two
 * line editors that drift, one of which would quietly stop sending `estUnitCost`
 * as text.
 */

/** One line being edited. Quantities and costs stay as the text the user typed
 *  until they are sent: parsing them into JS numbers would round a decimal before
 *  the server, which stores NUMERIC, ever saw it (I8). */
export type DraftLine = { productId: string; qty: string; estUnitCost: string };

export const emptyLine: DraftLine = { productId: "", qty: "1", estUnitCost: "" };

export type RequisitionFormValues = {
  warehouseId: string;
  supplierId: string;
  notes: string;
  lines: DraftLine[];
};

export type Pickers = {
  warehouses: Warehouse[];
  suppliers: Supplier[];
  products: Product[];
};

/**
 * The three pickers, in one request each.
 *
 * `pageSize: 100` is the §9.0 maximum. A tenant with more than a hundred products
 * needs a searching picker, which is a real screen rather than a bigger dropdown.
 *
 * TODO(post-mvp): replace the product select with a search-as-you-type picker once
 * a tenant can plausibly exceed a hundred products.
 */
export function useRequisitionPickers(key: string) {
  return useAsync<Pickers>(key, async () => {
    const [warehouses, suppliers, products] = await Promise.all([
      listWarehouses({ pageSize: 100, sort: "code" }),
      listSuppliers({ pageSize: 100, sort: "code" }),
      listProducts({ pageSize: 100, sort: "sku" }),
    ]);
    return {
      warehouses: warehouses.data,
      suppliers: suppliers.data,
      // Discontinued products are excluded from the picker but not from history
      // (§6.9.1): this is where a new document is started, and starting one for
      // something being wound down is not what the field is for. A line that
      // already names one keeps it — nothing here removes an existing line.
      products: products.data.filter((product) => product.isActive),
    };
  });
}

/** The lines with a product chosen, in the shape the API takes. A row the user
 *  added and left blank is not an error — it is a row they have not filled in —
 *  so it is dropped rather than refused. */
export function usableLines(lines: DraftLine[]): RequisitionLineWrite[] {
  return lines
    .filter((line) => line.productId !== "")
    .map((line) => ({
      productId: line.productId,
      qty: line.qty.trim() || "0",
      ...(line.estUnitCost.trim() !== ""
        ? { estUnitCost: line.estUnitCost.trim() }
        : {}),
    }));
}

/** The cost that will actually be stored when the user has typed none: the
 *  product's standard cost (§8.3). Shown as the placeholder rather than silently
 *  applied, so the estimate on screen is the estimate that lands. */
export function costFor(line: DraftLine, products: Product[]): string {
  if (line.estUnitCost.trim() !== "") return line.estUnitCost;
  const product = products.find((candidate) => candidate.id === line.productId);
  return product ? String(product.standardCost) : "";
}

/** The running total, for the reader's benefit only. The number that counts is
 *  computed by PostgreSQL when the requisition is saved (I8); this one is a
 *  preview, and it is allowed to be a float because nothing is decided by it. */
export function previewTotal(lines: DraftLine[], products: Product[]): number {
  return lines.reduce((sum, line) => {
    const qty = Number(line.qty);
    const cost = Number(costFor(line, products));
    if (!Number.isFinite(qty) || !Number.isFinite(cost)) return sum;
    return sum + qty * cost;
  }, 0);
}
