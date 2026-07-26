import { useState } from "react";

import { MasterDataList, type Column } from "../../components/MasterDataList";
import { useToast } from "../../components/Toasts";
import { useMe } from "../../hooks/useAuth";
import { useRowActions } from "../../hooks/useRowActions";
import {
  createSupplier,
  deleteSupplier,
  listSuppliers,
  patchSupplier,
  restoreSupplier,
  type Supplier,
} from "../../lib/api";
import { holds } from "../../lib/levels";

const field =
  "min-h-11 w-full rounded-md border border-hairline bg-surface px-3 text-sm";

const COLUMNS: Column[] = [
  { label: "Code" },
  { label: "Name" },
  { label: "Contact" },
  { label: "Lead time", align: "right" },
  { label: "Actions", hidden: true },
];

/**
 * `/procurement/suppliers` — supplier master data (§10.3).
 *
 * A flat list with inline editing, like `/inventory/warehouses`: a supplier is a
 * handful of fields, and a detail route would be a mostly-empty page. §10.3 calls
 * this "supplier list + create/edit modal"; a modal that holds six fields is a form
 * that has to be dismissed before you can see the list behind it, so the form opens
 * in place instead.
 */
export function SupplierList() {
  const me = useMe();
  const canManage = holds(me.moduleRoles, "procurement", "admin");

  return (
    <MasterDataList<Supplier>
      title="Suppliers"
      cacheKey="suppliers"
      canManage={canManage}
      columns={COLUMNS}
      minWidthClass="min-w-[48rem]"
      searchPlaceholder="Code or name"
      firstRun="No suppliers yet. A purchase order has to be addressed to somebody, so add one before raising a requisition."
      noResults="No suppliers match that search."
      addButton={canManage ? "Add supplier" : undefined}
      form={
        canManage
          ? (onCreated) => <NewSupplierForm onCreated={onCreated} />
          : undefined
      }
      load={(query) => listSuppliers({ ...query, sort: "code" })}
      row={(supplier, onChanged) => (
        <SupplierCells
          supplier={supplier}
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
 * The delete button stays enabled even when the supplier has open orders. The
 * server refuses it with `in_use` and a sentence naming which orders are in the
 * way (G4), and reading that refusal is more useful than a disabled button with no
 * explanation — and it is the server's answer either way (I12).
 */
function SupplierCells({
  supplier,
  canManage,
  onChanged,
}: {
  supplier: Supplier;
  canManage: boolean;
  onChanged: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [code, setCode] = useState(supplier.code);
  const [name, setName] = useState(supplier.name);
  const [email, setEmail] = useState(supplier.contactEmail ?? "");
  const [leadTime, setLeadTime] = useState(String(supplier.leadTimeDays));
  const { busy, run } = useRowActions(onChanged);

  return (
    <>
      <td className="px-3 py-3">
        {editing ? (
          <input
            aria-label="Supplier code"
            value={code}
            onChange={(event) => setCode(event.target.value)}
            className={`${field} tabular`}
          />
        ) : (
          <span className="tabular font-medium">{supplier.code}</span>
        )}
      </td>
      <td className="px-3 py-3">
        {editing ? (
          <input
            aria-label="Supplier name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            className={field}
          />
        ) : (
          <>
            {supplier.name}
            {supplier.deletedAt && (
              <span className="ml-2 text-xs text-secondary">deleted</span>
            )}
            {!supplier.isActive && !supplier.deletedAt && (
              <span className="ml-2 text-xs text-secondary">inactive</span>
            )}
            {supplier.openOrders > 0 && (
              <div className="text-xs text-secondary">
                {supplier.openOrders === 1
                  ? "1 open order"
                  : `${supplier.openOrders} open orders`}
              </div>
            )}
          </>
        )}
      </td>
      <td className="px-3 py-3">
        {editing ? (
          <input
            aria-label="Contact email"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            className={field}
          />
        ) : (
          <span className="text-secondary">{supplier.contactEmail ?? "—"}</span>
        )}
      </td>
      <td className="px-3 py-3 text-right">
        {editing ? (
          <input
            aria-label="Lead time in days"
            inputMode="numeric"
            value={leadTime}
            onChange={(event) => setLeadTime(event.target.value)}
            className={`${field} tabular text-right`}
          />
        ) : (
          <>
            <span className="tabular">{supplier.leadTimeDays}</span>
            <span className="text-secondary"> days</span>
            <div className="text-xs text-secondary">{supplier.paymentTerms}</div>
          </>
        )}
      </td>
      <td className="px-3 py-3">
        {canManage && (
          <div className="flex flex-wrap justify-end gap-2">
            {supplier.deletedAt ? (
              <button
                type="button"
                disabled={busy}
                onClick={() =>
                  void run(
                    () => restoreSupplier(supplier.id),
                    `${supplier.code} restored.`,
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
                        patchSupplier(supplier.id, {
                          code: code.trim(),
                          name: name.trim(),
                          contactEmail: email.trim(),
                          // Sent only when it parses, so a half-typed number does
                          // not silently become 0 days.
                          leadTimeDays: Number.isFinite(Number(leadTime))
                            ? Number(leadTime)
                            : undefined,
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
                    setCode(supplier.code);
                    setName(supplier.name);
                    setEmail(supplier.contactEmail ?? "");
                    setLeadTime(String(supplier.leadTimeDays));
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
                      () => deleteSupplier(supplier.id),
                      `${supplier.code} deleted. Tick Show deleted to restore it.`,
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

function NewSupplierForm({ onCreated }: { onCreated: () => void }) {
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [leadTime, setLeadTime] = useState("7");
  const [terms, setTerms] = useState("NET30");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  return (
    <form
      className="mb-6 space-y-4 rounded-lg border border-hairline bg-surface p-5"
      onSubmit={(event) => {
        event.preventDefault();
        setBusy(true);
        const label = code.trim();
        createSupplier({
          code: label,
          name: name.trim(),
          contactEmail: email.trim(),
          contactPhone: phone.trim(),
          leadTimeDays: Number(leadTime) || 0,
          paymentTerms: terms.trim() || "NET30",
        })
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
            placeholder="SUP-001"
            className={`${field} tabular`}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">Name</span>
          <input
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Sumber Makmur"
            className={field}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">
            Contact email
          </span>
          <input
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            className={field}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">
            Contact phone
          </span>
          <input
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
            className={field}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">
            Lead time (days)
          </span>
          <input
            inputMode="numeric"
            value={leadTime}
            onChange={(event) => setLeadTime(event.target.value)}
            className={`${field} tabular`}
          />
          <span className="mt-1 block text-xs text-secondary">
            Added to today's date to set the expected date on their orders.
          </span>
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-secondary">
            Payment terms
          </span>
          <input
            value={terms}
            onChange={(event) => setTerms(event.target.value)}
            placeholder="NET30"
            className={field}
          />
        </label>
      </div>

      <button
        type="submit"
        disabled={busy}
        className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-white disabled:opacity-50"
      >
        {busy ? "Adding…" : "Add supplier"}
      </button>
    </form>
  );
}
