import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { AppShell } from "../../components/AppShell";
import { ErrorNotice } from "../../components/ListStates";
import { useToast } from "../../components/Toasts";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
import {
  deleteProduct,
  getProduct,
  listLedger,
  listWarehouses,
  patchProduct,
  postAdjustment,
  restoreProduct,
  type LedgerEntry,
  type ProductDetail as Detail,
} from "../../lib/api";
import {
  formatDateTime,
  formatDelta,
  formatMoney,
  formatQty,
} from "../../lib/format";
import { holds } from "../../lib/levels";

const field =
  "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";
const labelClass = "mb-1 block text-sm text-secondary";

/**
 * `/inventory/products/:id` — detail plus that product's ledger history (§10.4).
 *
 * The screen resolves a **deleted** product too, on purpose: the ledger rows
 * below it link here, and a 404 would make last quarter's movements unreadable
 * (§6.9.1). When it is deleted, the page says so and offers Restore in place of
 * the editing controls.
 */
export function ProductDetailPage() {
  const { id = "" } = useParams();
  const me = useMe();
  const [nonce, setNonce] = useState(0);

  const canManage = holds(me.moduleRoles, "inventory", "admin");
  const canAdjust = holds(me.moduleRoles, "inventory", "approver");

  const { state, reload } = useAsync(`product:${id}:${nonce}`, () =>
    getProduct(id),
  );
  // Refetched alongside the product, so posting an adjustment moves the balance,
  // the per-warehouse table, and the history together.
  const history = useAsync(`product-ledger:${id}:${nonce}`, () =>
    listLedger({ productId: id, pageSize: 25, sort: "-occurredAt" }),
  );

  const refresh = () => setNonce((n) => n + 1);

  if (state.status === "loading") {
    return (
      <AppShell title="Product">
        <p className="text-sm text-secondary">Loading…</p>
      </AppShell>
    );
  }
  if (state.status === "error") {
    return (
      <AppShell title="Product">
        <ErrorNotice error={state.error} onRetry={reload} />
      </AppShell>
    );
  }

  const product = state.data;

  return (
    <AppShell
      title={product.name}
      actions={
        <Link
          to="/inventory/products"
          className="min-h-11 rounded-md border border-hairline px-3 text-sm leading-[2.75rem]"
        >
          All products
        </Link>
      }
    >
      {product.deletedAt && (
        <div
          role="status"
          className="mb-6 rounded-lg border border-hairline bg-raised p-4 text-sm"
        >
          <p>
            This product is deleted. It no longer appears in lists or pickers,
            and its history below is intact.
          </p>
          {canManage && (
            <RestoreButton productId={product.id} onDone={refresh} />
          )}
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="space-y-6">
          {canManage && !product.deletedAt ? (
            <EditForm product={product} onSaved={refresh} />
          ) : (
            <ReadOnlyFacts product={product} />
          )}

          <LedgerHistory
            state={history.state}
            onRetry={history.reload}
            timezone={me.tenant?.timezone ?? "UTC"}
          />
        </div>

        <div className="space-y-6">
          <BalancesPanel product={product} />
          {canAdjust && !product.deletedAt && (
            <AdjustmentForm product={product} onPosted={refresh} />
          )}
        </div>
      </div>
    </AppShell>
  );
}

/** The facts, for someone who cannot edit them. */
function ReadOnlyFacts({ product }: { product: Detail }) {
  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-base font-semibold">Details</h2>
      <dl className="mt-4 grid grid-cols-2 gap-4 text-sm">
        <div>
          <dt className="text-secondary">SKU</dt>
          <dd className="tabular">{product.sku}</dd>
        </div>
        <div>
          <dt className="text-secondary">Unit</dt>
          <dd>{product.uom}</dd>
        </div>
        <div>
          <dt className="text-secondary">Reorder point</dt>
          <dd className="tabular">{formatQty(product.reorderPoint)}</dd>
        </div>
        <div>
          <dt className="text-secondary">Standard cost</dt>
          <dd className="tabular">{formatMoney(product.standardCost)}</dd>
        </div>
        <div>
          {/* Deleted wins the label. `is_active` is still true underneath and
              that is correct -- but a Status of "Active" beside a banner saying
              the product is deleted reads as a contradiction, and the reader
              cannot be expected to know there are two columns. */}
          <dt className="text-secondary">Status</dt>
          <dd>
            {product.deletedAt
              ? "Deleted"
              : product.isActive
                ? "Active"
                : "Discontinued"}
          </dd>
        </div>
      </dl>
    </section>
  );
}

/**
 * Edit, discontinue, and delete.
 *
 * "Discontinue" and "Delete" sit side by side because they are the two different
 * questions of §6.9.1 and the UI has to make the difference obvious: a
 * discontinued product stays in every report and keeps its stock; a deleted one
 * leaves the lists and can be brought back.
 */
function EditForm({
  product,
  onSaved,
}: {
  product: Detail;
  onSaved: () => void;
}) {
  const [sku, setSku] = useState(product.sku);
  const [name, setName] = useState(product.name);
  const [uom, setUom] = useState(product.uom);
  const [reorderPoint, setReorderPoint] = useState(String(product.reorderPoint));
  const [standardCost, setStandardCost] = useState(String(product.standardCost));
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  // `confirmation` is what the user is told on success. Saying nothing at all
  // was the worst part of this screen: Save changes refetched and looked
  // identical to having done nothing.
  async function run(action: () => Promise<unknown>, confirmation: string) {
    setBusy(true);
    try {
      await action();
      toast.success(confirmation);
      onSaved();
    } catch (caught) {
      toast.failure(caught);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-base font-semibold">Details</h2>

      <form
        className="mt-4 space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          void run(
            () =>
              patchProduct(product.id, {
                sku: sku.trim(),
                name: name.trim(),
                uom: uom.trim(),
                reorderPoint: reorderPoint.trim(),
                standardCost: standardCost.trim(),
              }),
            "Changes saved.",
          );
        }}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className={labelClass}>SKU</span>
            <input
              required
              value={sku}
              onChange={(event) => setSku(event.target.value)}
              className={`${field} tabular`}
            />
          </label>
          <label className="block">
            <span className={labelClass}>Unit of measure</span>
            <input
              value={uom}
              onChange={(event) => setUom(event.target.value)}
              className={field}
            />
          </label>
        </div>

        <label className="block">
          <span className={labelClass}>Name</span>
          <input
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            className={field}
          />
        </label>

        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className={labelClass}>Reorder point</span>
            <input
              inputMode="decimal"
              value={reorderPoint}
              onChange={(event) => setReorderPoint(event.target.value)}
              className={`${field} tabular`}
            />
          </label>
          <label className="block">
            <span className={labelClass}>Standard cost</span>
            <input
              inputMode="decimal"
              value={standardCost}
              onChange={(event) => setStandardCost(event.target.value)}
              className={`${field} tabular`}
            />
          </label>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <button
            type="submit"
            disabled={busy}
            className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
          >
            {busy ? "Saving…" : "Save changes"}
          </button>

          <button
            type="button"
            disabled={busy}
            onClick={() =>
              void run(
                () => patchProduct(product.id, { isActive: !product.isActive }),
                product.isActive
                  ? `${product.name} is discontinued. It stays in reports and keeps its stock.`
                  : `${product.name} is active again.`,
              )
            }
            className="min-h-11 rounded-md border border-hairline px-4 text-sm"
          >
            {product.isActive ? "Discontinue" : "Reinstate"}
          </button>

          <button
            type="button"
            disabled={busy}
            onClick={() =>
              void run(
                () => deleteProduct(product.id),
                `${product.name} deleted. You can restore it from this page.`,
              )
            }
            className="ml-auto min-h-11 rounded-md border border-danger/40 px-4 text-sm text-danger"
          >
            Delete
          </button>
        </div>

        <p className="text-xs text-secondary">
          Discontinuing keeps the product in reports and keeps its stock; it just
          cannot be used in new documents. Deleting hides it and can be undone —
          nothing is ever removed.
        </p>
      </form>
    </section>
  );
}

function RestoreButton({
  productId,
  onDone,
}: {
  productId: string;
  onDone: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  return (
    <div className="mt-3">
      <button
        type="button"
        disabled={busy}
        onClick={() => {
          setBusy(true);
          restoreProduct(productId)
            .then(() => {
              toast.success("Restored.");
              onDone();
            })
            // The refusal worth reading: another product took the SKU while
            // this one was deleted, and there cannot be two live rows holding
            // it (G3). The toast carries the server's own sentence.
            .catch((caught: unknown) => {
              toast.failure(caught);
            })
            .finally(() => setBusy(false));
        }}
        className="min-h-11 rounded-md border border-hairline px-4 text-sm"
      >
        {busy ? "Restoring…" : "Restore"}
      </button>
    </div>
  );
}

/** Stock by warehouse, straight from `stock_balances`. */
function BalancesPanel({ product }: { product: Detail }) {
  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-base font-semibold">Stock on hand</h2>

      <p className="mt-2">
        <span className="tabular text-2xl font-semibold">
          {formatQty(product.qtyOnHand)}
        </span>{" "}
        <span className="text-sm text-secondary">{product.uom} in total</span>
      </p>
      {product.belowReorderPoint && (
        <p className="mt-1 text-sm text-warning">
          Below the reorder point of {formatQty(product.reorderPoint)}.
        </p>
      )}

      {product.balances.length === 0 ? (
        <p className="mt-4 text-sm text-secondary">
          No warehouse holds any of this yet.
        </p>
      ) : (
        <ul className="mt-4 divide-y divide-hairline text-sm">
          {product.balances.map((balance) => (
            <li
              key={balance.warehouseId}
              className="flex items-center justify-between gap-3 py-2"
            >
              <span>
                <span className="tabular">{balance.warehouseCode}</span>{" "}
                <span className="text-secondary">{balance.warehouseName}</span>
              </span>
              <span className="tabular">{formatQty(balance.qtyOnHand)}</span>
            </li>
          ))}
        </ul>
      )}

      <p className="mt-4 text-xs text-secondary">
        Summed from the ledger on every load. There is no stored total — which is
        why every number here can be explained by the entries below.
      </p>
    </section>
  );
}

/**
 * Post a manual adjustment. `approver` and above (§9.5): correcting stock with
 * no document behind it is a decision, not data entry.
 */
function AdjustmentForm({
  product,
  onPosted,
}: {
  product: Detail;
  onPosted: () => void;
}) {
  const warehouses = useAsync("adjustment-warehouses", () =>
    listWarehouses({ pageSize: 100, sort: "code" }),
  );

  const [warehouseId, setWarehouseId] = useState("");
  const [qtyDelta, setQtyDelta] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  const options =
    warehouses.state.status === "ready" ? warehouses.state.data.data : [];
  const selected = warehouseId || options[0]?.id || "";

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const result = await postAdjustment({
        productId: product.id,
        warehouseId: selected,
        qtyDelta: qtyDelta.trim(),
        note: note.trim(),
      });
      // Names the warehouse: the balance that moved is a product-and-warehouse
      // balance, not the product's total.
      const where =
        options.find((option) => option.id === selected)?.code ??
        "that warehouse";
      toast.success(
        `Posted. ${formatQty(result.qtyOnHand)} ${product.uom} on hand in ${where}.`,
      );
      setQtyDelta("");
      setNote("");
      onPosted();
    } catch (caught) {
      toast.failure(caught);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-base font-semibold">Adjust stock</h2>
      <p className="mt-1 text-sm text-secondary">
        Appends one entry to the ledger. Nothing is overwritten — a correction is
        a new entry, attributed to you.
      </p>

      <form onSubmit={submit} className="mt-4 space-y-4">
        <label className="block">
          <span className={labelClass}>Warehouse</span>
          <select
            required
            value={selected}
            onChange={(event) => setWarehouseId(event.target.value)}
            className={field}
          >
            {options.map((warehouse) => (
              <option key={warehouse.id} value={warehouse.id}>
                {warehouse.code} — {warehouse.name}
              </option>
            ))}
          </select>
        </label>

        <label className="block">
          <span className={labelClass}>Quantity</span>
          <input
            required
            inputMode="decimal"
            value={qtyDelta}
            onChange={(event) => setQtyDelta(event.target.value)}
            placeholder="-3 to write off, 12 to add"
            className={`${field} tabular`}
          />
          <span className="mt-1 block text-xs text-secondary">
            Signed: a minus reduces stock. Zero is not a movement.
          </span>
        </label>

        <label className="block">
          <span className={labelClass}>Note</span>
          <input
            value={note}
            onChange={(event) => setNote(event.target.value)}
            placeholder="Annual count"
            className={field}
          />
        </label>

        <button
          type="submit"
          disabled={busy || options.length === 0}
          className="min-h-11 w-full rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
        >
          {busy ? "Posting…" : "Post adjustment"}
        </button>
        {options.length === 0 && warehouses.state.status === "ready" && (
          <p className="text-xs text-secondary">
            There are no warehouses yet, so there is nowhere to hold stock.
          </p>
        )}
      </form>
    </section>
  );
}

/** This product's movements, newest first. */
function LedgerHistory({
  state,
  onRetry,
  timezone,
}: {
  state: ReturnType<typeof useAsync<{ data: LedgerEntry[] }>>["state"];
  onRetry: () => void;
  timezone: string;
}) {
  return (
    <section className="rounded-lg border border-hairline bg-surface p-5">
      <h2 className="text-base font-semibold">Ledger history</h2>

      {state.status === "error" && (
        <div className="mt-4">
          <ErrorNotice error={state.error} onRetry={onRetry} />
        </div>
      )}
      {state.status === "loading" && (
        <p className="mt-4 text-sm text-secondary">Loading…</p>
      )}

      {state.status === "ready" && state.data.data.length === 0 && (
        <p className="mt-4 text-sm text-secondary">
          Nothing has moved yet. Receipts and adjustments will appear here.
        </p>
      )}

      {state.status === "ready" && state.data.data.length > 0 && (
        <ul className="mt-4 divide-y divide-hairline text-sm">
          {state.data.data.map((entry) => (
            <li key={entry.id} className="flex flex-wrap gap-x-3 gap-y-1 py-3">
              <span
                className={`tabular w-20 shrink-0 text-right font-medium ${
                  entry.qtyDelta < 0 ? "text-danger" : "text-success"
                }`}
              >
                {formatDelta(entry.qtyDelta)}
              </span>
              <span className="min-w-0 grow">
                <span className="tabular">{entry.warehouseCode}</span>
                <span className="text-secondary">
                  {" · "}
                  {entry.entryType}
                  {" · "}
                  {entry.createdByName}
                </span>
                {entry.note && (
                  <span className="block text-secondary">{entry.note}</span>
                )}
              </span>
              <span className="shrink-0 text-secondary">
                {formatDateTime(entry.occurredAt, timezone)}
              </span>
            </li>
          ))}
        </ul>
      )}

      <Link
        to="/inventory/ledger"
        className="mt-4 inline-block text-sm underline decoration-hairline underline-offset-2"
      >
        Full ledger
      </Link>
    </section>
  );
}
