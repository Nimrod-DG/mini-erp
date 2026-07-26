import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { useToast } from "../../components/Toasts";
import { createProduct } from "../../lib/api";

/**
 * `/inventory/products/new`.
 *
 * There is deliberately no opening-stock field. Stock exists only as ledger
 * entries (I6), so the way to give a new product a balance is to post an
 * adjustment from its detail screen — which records who said so and when. A
 * "starting quantity" box here would be a stock movement with no author.
 */
export function ProductNew() {
  const navigate = useNavigate();

  const [sku, setSku] = useState("");
  const [name, setName] = useState("");
  const [uom, setUom] = useState("pcs");
  const [reorderPoint, setReorderPoint] = useState("0");
  const [standardCost, setStandardCost] = useState("0");

  const [saving, setSaving] = useState(false);
  const toast = useToast();

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const created = await createProduct({
        sku: sku.trim(),
        name: name.trim(),
        uom: uom.trim(),
        // Sent as the text the user typed. Parsing it into a JS number first
        // would round it before the server, which stores NUMERIC, ever saw it.
        reorderPoint: reorderPoint.trim() || "0",
        standardCost: standardCost.trim() || "0",
      });
      // The toast lives outside the router, so it survives this navigation
      // and lands on the detail screen with the product it is about.
      toast.success(`${created.name} added.`);
      navigate(`/inventory/products/${created.id}`, { replace: true });
    } catch (caught) {
      toast.failure(caught);
    } finally {
      setSaving(false);
    }
  }

  const field =
    "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";
  const label = "mb-1 block text-sm text-secondary";

  return (
    <AppShell title="Add product">
      <form onSubmit={submit} className="max-w-xl space-y-6">
        <section className="space-y-4 rounded-lg border border-hairline bg-surface p-5">
          <label className="block">
            <span className={label}>SKU</span>
            <input
              required
              value={sku}
              onChange={(event) => setSku(event.target.value)}
              className={`${field} tabular`}
              placeholder="SKU-001"
            />
            <span className="mt-1 block text-xs text-secondary">
              Unique among live products. A SKU freed by deleting a product can
              be reused.
            </span>
          </label>

          <label className="block">
            <span className={label}>Name</span>
            <input
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              className={field}
            />
          </label>

          <label className="block">
            <span className={label}>Unit of measure</span>
            <input
              value={uom}
              onChange={(event) => setUom(event.target.value)}
              className={field}
              placeholder="pcs"
            />
          </label>

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block">
              <span className={label}>Reorder point</span>
              <input
                inputMode="decimal"
                value={reorderPoint}
                onChange={(event) => setReorderPoint(event.target.value)}
                className={`${field} tabular`}
              />
              <span className="mt-1 block text-xs text-secondary">
                Below this, the product is flagged as low. Leave at 0 for no
                threshold.
              </span>
            </label>

            <label className="block">
              <span className={label}>Standard cost</span>
              <input
                inputMode="decimal"
                value={standardCost}
                onChange={(event) => setStandardCost(event.target.value)}
                className={`${field} tabular`}
              />
              <span className="mt-1 block text-xs text-secondary">
                What a manual adjustment values a unit at, unless told otherwise.
              </span>
            </label>
          </div>
        </section>

        <div className="flex flex-wrap gap-3">
          <button
            type="submit"
            disabled={saving}
            className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            {saving ? "Adding…" : "Add product"}
          </button>
          <button
            type="button"
            onClick={() => navigate("/inventory/products")}
            className="min-h-11 rounded-md border border-hairline px-4 text-sm"
          >
            Cancel
          </button>
        </div>
      </form>
    </AppShell>
  );
}
