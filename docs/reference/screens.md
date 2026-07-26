# Reference — Screen inventory

> Read only the subsection for the module you are building.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 10.1 Shell

- Left sidebar: Dashboard, Procurement, Inventory, Finance; Administration for superadmins.
- Nav items are driven entirely by `/api/me`. A module the user holds `none` in, or that the tenant lacks entitlement to, is **hidden entirely** — not disabled.
- Within a visible module, actions above the user's level are hidden (e.g. no "Approve" button for a `user`-level account).
- Top bar: tenant name, user name, per-module role badges, logout.
- Superadmins land on `/admin/tenants` and see no business modules at all.

### 10.2 Dashboard (`/`)

Four widgets, each omitted if the user cannot read its module:

1. **Open purchase orders** — count + total value, links to the PO list.
2. **Requisitions awaiting approval** — count; for `approver`+, an inline approve/reject queue.
3. **Low stock** — products below reorder point, with a "Create requisition" shortcut that pre-fills them.
4. **Recent activity** — last 15 stock ledger entries, each linking to its source document.

### 10.3 Procurement screens

| Route | Screen |
|---|---|
| `/procurement/requisitions` | List with status filter chips |
| `/procurement/requisitions/new` | Create: warehouse + supplier, product lines, save draft or submit |
| `/procurement/requisitions/:id` | Detail: lines, status timeline, role-appropriate actions |
| `/procurement/orders` | PO list, status + supplier filters |
| `/procurement/orders/:id` | Detail: ordered vs received per line, receipt history, "Receive goods" |
| `/procurement/orders/:id/receive` | Receipt form, per-line qty defaulting to outstanding, over-receipt validation |
| `/procurement/suppliers` | Supplier list + create/edit modal |

**The receipt confirmation screen matters for the demo.** After a successful receipt, show a panel that names what happened across modules:

> Goods receipt `GR-202607-0004` posted.
> → Inventory: 2 stock ledger entries created
> → Finance: journal entry `JE-202607-0004` posted (Dr Inventory 4,500,000 / Cr GRNI 4,500,000)

with both lines as links. This is the screenshot that goes in the portfolio.

### 10.4 Inventory screens

| Route | Screen |
|---|---|
| `/inventory/products` | Product list with current stock and reorder flag |
| `/inventory/products/:id` | Detail + that product's ledger history |
| `/inventory/stock` | Stock-on-hand grid, product × warehouse |
| `/inventory/ledger` | Full ledger, filterable, rows linked to source documents |

### 10.5 Finance screen

A single `/finance` page. Header reads "Finance — coming soon". Below it, a live read-only journal entry list from `/api/finance/journal-entries`, introduced with: *"Postings from other modules are already flowing in. Reporting and period close are not built yet."*

Better than an empty placeholder: it proves cross-module posting works while being honest that the module is incomplete.

### 10.6 Administration screens

| Route | Screen | Who |
|---|---|---|
| `/admin/tenants` | Tenant list: status, user count, module pills | superadmin |
| `/admin/tenants/new` | Create tenant + first admin | superadmin |
| `/admin/tenants/:id` | Module entitlement toggle matrix | superadmin |
| `/settings/users` | Tenant user list | tenant `admin` |
| `/settings/users/:id` | **Per-module role matrix** (staff only) — one dropdown per module | tenant `admin` |

The per-module role matrix is worth building carefully — it is the clearest visual explanation of the permission model, and a good screenshot.
