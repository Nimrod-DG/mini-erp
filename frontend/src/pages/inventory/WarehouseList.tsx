import { useState } from "react";

import { AppShell } from "../../components/AppShell";
import {
  EmptyState,
  ErrorNotice,
  Pagination,
  SkeletonRows,
} from "../../components/ListStates";
import { useToast } from "../../components/Toasts";
import { useAsync } from "../../hooks/useAsync";
import { useMe } from "../../hooks/useAuth";
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

const COLUMNS = 4;

const field =
  "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";

/**
 * `/inventory/warehouses` — warehouse master data.
 *
 * §10.4 lists four inventory screens and this is not one of them, but §9.6.1 is
 * explicit that every entity with CRUD endpoints needs a working UI: "a
 * half-built entity — creatable but not editable — is the most common way a demo
 * falls over". Warehouses have the full endpoint set, the stock grid and the
 * adjustment form both pick from them, and there would otherwise be no way to
 * make one without curl.
 *
 * A flat list with inline editing rather than a detail route: a warehouse is a
 * code and a name, and a page of its own would be two fields on an empty screen.
 */
export function WarehouseList() {
  const me = useMe();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [showDeleted, setShowDeleted] = useState(false);
  const [adding, setAdding] = useState(false);
  const [nonce, setNonce] = useState(0);

  const canManage = holds(me.moduleRoles, "inventory", "admin");

  const { state, reload } = useAsync(
    `warehouses:${page}:${search}:${showDeleted}:${nonce}`,
    () =>
      listWarehouses({
        page,
        q: search,
        sort: "code",
        includeDeleted: canManage && showDeleted,
      }),
  );

  const refresh = () => setNonce((n) => n + 1);

  const addButton = canManage ? (
    <button
      type="button"
      onClick={() => setAdding((open) => !open)}
      className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white"
    >
      {adding ? "Cancel" : "Add warehouse"}
    </button>
  ) : null;

  return (
    <AppShell title="Warehouses" actions={addButton}>
      {adding && canManage && (
        <NewWarehouseForm
          onCreated={() => {
            setAdding(false);
            refresh();
          }}
        />
      )}

      <div className="mb-4 flex flex-wrap items-end gap-4">
        <label className="block max-w-sm grow">
          <span className="mb-1 block text-sm text-secondary">Search</span>
          <input
            type="search"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
            placeholder="Code or name"
            className={field}
          />
        </label>

        {canManage && (
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={showDeleted}
              onChange={(event) => {
                setShowDeleted(event.target.checked);
                setPage(1);
              }}
              className="size-4"
            />
            Show deleted
          </label>
        )}
      </div>

      {state.status === "error" ? (
        <ErrorNotice error={state.error} onRetry={reload} />
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border border-hairline bg-surface">
            <table className="w-full min-w-[38rem] text-left text-sm">
              <thead className="text-xs uppercase tracking-wide text-secondary">
                <tr>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Code
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    Name
                  </th>
                  <th scope="col" className="px-3 py-2.5 text-right font-medium">
                    Stock held
                  </th>
                  <th scope="col" className="px-3 py-2.5 font-medium">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>

              {state.status === "loading" && <SkeletonRows cols={COLUMNS} />}

              {state.status === "ready" && state.data.data.length === 0 && (
                <EmptyState
                  colSpan={COLUMNS}
                  filtered={search !== ""}
                  firstRun="No warehouses yet. Stock is always held somewhere, so add at least one before receiving anything."
                  noResults="No warehouses match that search."
                  action={addButton}
                />
              )}

              {state.status === "ready" && state.data.data.length > 0 && (
                <tbody>
                  {state.data.data.map((warehouse) => (
                    <WarehouseRow
                      key={warehouse.id}
                      warehouse={warehouse}
                      canManage={canManage}
                      onChanged={refresh}
                    />
                  ))}
                </tbody>
              )}
            </table>
          </div>

          {state.status === "ready" && (
            <Pagination
              page={state.data.page}
              pageSize={state.data.pageSize}
              totalItems={state.data.totalItems}
              totalPages={state.data.totalPages}
              onPage={setPage}
            />
          )}
        </>
      )}
    </AppShell>
  );
}

/**
 * One row, editable in place.
 *
 * The delete button stays enabled even when the warehouse holds stock. The
 * server refuses it with `in_use` and a sentence naming how many products are in
 * the way (G5), and reading that refusal is more useful than a disabled button
 * with no explanation — and it is the server's answer either way (I12).
 */
function WarehouseRow({
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
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  async function run(action: () => Promise<unknown>, confirmation: string) {
    setBusy(true);
    try {
      await action();
      toast.success(confirmation);
      setEditing(false);
      onChanged();
    } catch (caught) {
      // The refusal this screen exists to surface: a warehouse holding stock
      // cannot be deleted, and the server's sentence names how many products
      // are in the way (G5). A toast puts it where the eye already is, rather
      // than in a row that shifts the table.
      toast.failure(caught);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <tr className="border-t border-hairline hover:bg-raised">
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
                      )
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
      </tr>
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
