import { useState } from "react";

import { MasterDataList, type Column } from "../../components/MasterDataList";
import { useToast } from "../../components/Toasts";
import { useMe } from "../../hooks/useAuth";
import { useRowActions } from "../../hooks/useRowActions";
import {
  createWarehouse,
  deleteWarehouse,
  listWarehouses,
  patchWarehouse,
  restoreWarehouse,
  type Warehouse,
} from "../../lib/api";
import { formatQty } from "../../lib/format";
import { holds } from "../../lib/levels";

const field =
  "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";

const COLUMNS: Column[] = [
  { label: "Code" },
  { label: "Name" },
  { label: "Stock held", align: "right" },
  { label: "Actions", hidden: true },
];

/**
 * `/inventory/warehouses` — warehouse master data.
 *
 * §10.4 lists four inventory screens and this is not one of them, but §9.6.1 is
 * explicit that every entity with CRUD endpoints needs a working UI: "a half-built
 * entity — creatable but not editable — is the most common way a demo falls over".
 * Warehouses have the full endpoint set, the stock grid and the adjustment form
 * both pick from them, and there would otherwise be no way to make one without
 * curl.
 *
 * A flat list with inline editing rather than a detail route: a warehouse is a code
 * and a name, and a page of its own would be two fields on an empty screen.
 */
export function WarehouseList() {
  const me = useMe();
  const canManage = holds(me.moduleRoles, "inventory", "admin");

  return (
    <MasterDataList<Warehouse>
      title="Warehouses"
      cacheKey="warehouses"
      canManage={canManage}
      columns={COLUMNS}
      minWidthClass="min-w-[38rem]"
      searchPlaceholder="Code or name"
      firstRun="No warehouses yet. Stock is always held somewhere, so add at least one before receiving anything."
      noResults="No warehouses match that search."
      addButton={canManage ? "Add warehouse" : undefined}
      form={
        canManage
          ? (onCreated) => <NewWarehouseForm onCreated={onCreated} />
          : undefined
      }
      load={(query) => listWarehouses({ ...query, sort: "code" })}
      row={(warehouse, onChanged) => (
        <WarehouseCells
          warehouse={warehouse}
          canManage={canManage}
          onChanged={onChanged}
        />
      )}
    />
  );
}

/**
 * One row's cells, editable in place.
 *
 * The delete button stays enabled even when the warehouse holds stock. The server
 * refuses it with `in_use` and a sentence naming how many products are in the way
 * (G5), and reading that refusal is more useful than a disabled button with no
 * explanation — and it is the server's answer either way (I12).
 */
function WarehouseCells({
  warehouse,
  canManage,
  onChanged,
}: {
  warehouse: Warehouse;
  canManage: boolean;
  onChanged: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [code, setCode] = useState(warehouse.code);
  const [name, setName] = useState(warehouse.name);
  const { busy, run } = useRowActions(onChanged);

  return (
    <>
      <td className="px-3 py-3">
        {editing ? (
          <input
            aria-label="Warehouse code"
            value={code}
            onChange={(event) => setCode(event.target.value)}
            className={`${field} tabular`}
          />
        ) : (
          <span className="tabular font-medium">{warehouse.code}</span>
        )}
      </td>
      <td className="px-3 py-3">
        {editing ? (
          <input
            aria-label="Warehouse name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            className={field}
          />
        ) : (
          <>
            {warehouse.name}
            {warehouse.deletedAt && (
              <span className="ml-2 text-xs text-secondary">deleted</span>
            )}
            {!warehouse.isActive && !warehouse.deletedAt && (
              <span className="ml-2 text-xs text-secondary">inactive</span>
            )}
          </>
        )}
      </td>
      <td className="px-3 py-3 text-right">
        <span className="tabular">{formatQty(warehouse.qtyOnHand)}</span>
        <div className="text-xs text-secondary">
          {warehouse.productCount === 1
            ? "1 product"
            : `${warehouse.productCount} products`}
        </div>
      </td>
      <td className="px-3 py-3">
        {canManage && (
          <div className="flex flex-wrap justify-end gap-2">
            {warehouse.deletedAt ? (
              <button
                type="button"
                disabled={busy}
                onClick={() =>
                  void run(
                    () => restoreWarehouse(warehouse.id),
                    `${warehouse.code} restored.`,
                  )
                }
                className="min-h-11 rounded-md border border-hairline px-3 text-sm"
              >
                Restore
              </button>
            ) : editing ? (
              <>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    void run(
                      () =>
                        patchWarehouse(warehouse.id, {
                          code: code.trim(),
                          name: name.trim(),
                        }),
                      "Changes saved.",
                    ).then((ok) => ok && setEditing(false))
                  }
                  className="min-h-11 rounded-md bg-accent px-3 text-sm font-medium text-white"
                >
                  Save
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setCode(warehouse.code);
                    setName(warehouse.name);
                    setEditing(false);
                  }}
                  className="min-h-11 rounded-md border border-hairline px-3 text-sm"
                >
                  Cancel
                </button>
              </>
            ) : (
              <>
                <button
                  type="button"
                  onClick={() => setEditing(true)}
                  className="min-h-11 rounded-md border border-hairline px-3 text-sm"
                >
                  Edit
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    void run(
                      () => deleteWarehouse(warehouse.id),
                      `${warehouse.code} deleted. Tick Show deleted to restore it.`,
                    )
                  }
                  className="min-h-11 rounded-md border border-danger/40 px-3 text-sm text-danger"
                >
                  Delete
                </button>
              </>
            )}
          </div>
        )}
      </td>
    </>
  );
}

function NewWarehouseForm({ onCreated }: { onCreated: () => void }) {
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  return (
    <form
      className="mb-6 space-y-4 rounded-lg border border-hairline bg-surface p-5"
      onSubmit={(event) => {
        event.preventDefault();
        setBusy(true);
        const label = code.trim();
        createWarehouse({ code: label, name: name.trim() })
          .then(() => {
            toast.success(`${label} added.`);
            onCreated();
          })
          .catch((caught: unknown) => {
            toast.failure(caught);
          })
          .finally(() => setBusy(false));
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">Code</span>
          <input
            required
            value={code}
            onChange={(event) => setCode(event.target.value)}
            placeholder="WH-1"
            className={`${field} tabular`}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">Name</span>
          <input
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Main warehouse"
            className={field}
          />
        </label>
      </div>

      <button
        type="submit"
        disabled={busy}
        className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
      >
        {busy ? "Adding…" : "Add warehouse"}
      </button>
    </form>
  );
}
