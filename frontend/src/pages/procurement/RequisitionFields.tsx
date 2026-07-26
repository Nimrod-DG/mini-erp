import { formatMoney } from "../../lib/format";
import {
  costFor,
  emptyLine,
  previewTotal,
  type Pickers,
  type DraftLine,
  type RequisitionFormValues,
} from "../../lib/requisitionForm";

const field =
  "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";
const label = "mb-1 block text-sm text-secondary";

/**
 * The requisition form, rendered for `/new` and for editing a draft in place. Its
 * state and helpers live in lib/requisitionForm.ts; this file is the markup.
 */
export function RequisitionFields({
  pickers,
  values,
  onChange,
}: {
  pickers: Pickers;
  values: RequisitionFormValues;
  onChange: (next: RequisitionFormValues) => void;
}) {
  const { warehouses, suppliers, products } = pickers;
  const { lines } = values;

  const setLines = (next: DraftLine[]) => onChange({ ...values, lines: next });

  return (
    <>
      {warehouses.length === 0 && (
        <p className="rounded-lg border border-warning/40 bg-surface p-4 text-sm text-warning">
          There are no warehouses yet. Goods have to be received somewhere, so add
          one under Inventory first.
        </p>
      )}

      <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className={label}>Deliver to</span>
            <select
              required
              value={values.warehouseId}
              onChange={(event) =>
                onChange({ ...values, warehouseId: event.target.value })
              }
              className={field}
            >
              <option value="">Choose a warehouse…</option>
              {warehouses.map((warehouse) => (
                <option key={warehouse.id} value={warehouse.id}>
                  {warehouse.code} — {warehouse.name}
                </option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className={label}>Supplier</span>
            <select
              value={values.supplierId}
              onChange={(event) =>
                onChange({ ...values, supplierId: event.target.value })
              }
              className={field}
            >
              <option value="">Decide at approval</option>
              {suppliers.map((supplier) => (
                <option key={supplier.id} value={supplier.id}>
                  {supplier.code} — {supplier.name}
                </option>
              ))}
            </select>
            <span className="mt-1 block text-xs text-secondary">
              Optional now, required to approve — an order has to be addressed to
              somebody.
            </span>
          </label>
        </div>

        <label className="block">
          <span className={label}>Notes</span>
          <textarea
            value={values.notes}
            onChange={(event) => onChange({ ...values, notes: event.target.value })}
            rows={2}
            className="w-full rounded-md border border-hairline bg-surface px-3 py-2 text-sm"
            placeholder="Why this is needed, or anything the approver should know."
          />
        </label>
      </section>

      <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
        <h2 className="text-sm font-medium">Lines</h2>

        <div className="space-y-3">
          {lines.map((line, index) => (
            <div
              key={index}
              className="grid gap-3 sm:grid-cols-[1fr_6rem_8rem_auto] sm:items-end"
            >
              <label className="block">
                <span className={label}>Product</span>
                <select
                  value={line.productId}
                  onChange={(event) =>
                    setLines(
                      lines.map((current, i) =>
                        i === index
                          ? { ...current, productId: event.target.value }
                          : current,
                      ),
                    )
                  }
                  className={field}
                >
                  <option value="">Choose a product…</option>
                  {products.map((product) => (
                    <option key={product.id} value={product.id}>
                      {product.sku} — {product.name}
                    </option>
                  ))}
                  {/* A line naming a product the picker filtered out — one that
                      has been discontinued since the draft was written — keeps
                      its option, or editing anything else on the draft would
                      silently change what was ordered. */}
                  {line.productId !== "" &&
                    !products.some((product) => product.id === line.productId) && (
                      <option value={line.productId}>
                        (no longer in the catalogue)
                      </option>
                    )}
                </select>
              </label>

              <label className="block">
                <span className={label}>Quantity</span>
                <input
                  inputMode="decimal"
                  value={line.qty}
                  onChange={(event) =>
                    setLines(
                      lines.map((current, i) =>
                        i === index ? { ...current, qty: event.target.value } : current,
                      ),
                    )
                  }
                  className={`${field} tabular`}
                />
              </label>

              <label className="block">
                <span className={label}>Unit cost</span>
                <input
                  inputMode="decimal"
                  value={line.estUnitCost}
                  onChange={(event) =>
                    setLines(
                      lines.map((current, i) =>
                        i === index
                          ? { ...current, estUnitCost: event.target.value }
                          : current,
                      ),
                    )
                  }
                  placeholder={costFor(line, products) || "standard cost"}
                  className={`${field} tabular`}
                />
              </label>

              <button
                type="button"
                aria-label={`Remove line ${index + 1}`}
                disabled={lines.length === 1}
                onClick={() => setLines(lines.filter((_, i) => i !== index))}
                className="min-h-11 rounded-md border border-hairline px-3 text-sm disabled:opacity-40"
              >
                Remove
              </button>
            </div>
          ))}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <button
            type="button"
            onClick={() => setLines([...lines, { ...emptyLine }])}
            className="min-h-11 rounded-md border border-hairline px-3 text-sm"
          >
            Add line
          </button>
          <p className="text-sm text-secondary">
            Estimated total{" "}
            <span className="tabular text-primary">
              {formatMoney(previewTotal(lines, products))}
            </span>
          </p>
        </div>

        <p className="text-xs text-secondary">
          One line per product. Leave the unit cost blank to use the product's
          standard cost — the estimate becomes the order's price, and from there the
          value posted to Inventory and Finance when the goods arrive.
        </p>
      </section>
    </>
  );
}
