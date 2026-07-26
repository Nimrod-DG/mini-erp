# Reference — Responsive design

> Phase 7. Nav and breakpoints are needed earlier; the rest is the responsive pass.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 10.7 Responsive design

The whole application is responsive down to a 360px-wide phone. This is not a "shrink the desktop grid" exercise — ERP tables are the hardest component to move to a small screen, because their power comes from horizontal comparison and phones are vertical.

#### 10.7.1 Decide which screens are actually mobile-first

Not every screen deserves equal mobile effort. In this domain the split is unusually clear, and it should drive where the work goes:

| Screen | Primary device | Why |
|---|---|---|
| **Goods receipt** | **Phone** | Performed at a loading dock, standing next to the delivery. This is the one screen genuinely used on mobile. |
| **Requisition approval** | Phone / tablet | Managers approve between meetings; it is a two-button decision. |
| Dashboard | Any | Glanceable by design. |
| Requisition create | Tablet / desktop | Multi-line entry; possible on a phone, not pleasant. |
| Stock ledger, product list | Desktop | Dense comparison across many columns. |
| Master data, user roles, entitlements | Desktop | Configuration work, done sitting down. |

Build the goods receipt and approval flows mobile-first. Make everything else *usable* on a phone without pretending it is the intended context. Being able to explain this split — rather than claiming everything is equally mobile-optimised — is itself the more credible position.

#### 10.7.2 Breakpoints

Use Tailwind's defaults; do not invent a scale.

| Name | Min width | Layout |
|---|---|---|
| base | 0 | Single column, bottom tab bar, cards instead of tables |
| `sm` | 640px | Two-column forms, denser cards |
| `md` | 768px | Collapsible sidebar, tables begin appearing |
| `lg` | 1024px | Persistent sidebar, full tables, multi-column dashboard |
| `xl` | 1280px | Wider content column, more table columns visible |

#### 10.7.3 Navigation

- **`lg` and up:** persistent 240px left sidebar.
- **`md`:** sidebar collapses to icons, expands on hover.
- **Below `md`:** sidebar becomes a slide-over drawer behind a hamburger, **plus** a bottom tab bar for the three or four most-used destinations (Dashboard, Requisitions, Orders, and Stock if entitled).

The bottom bar is worth the effort: on the goods-receipt flow the user is holding a phone one-handed while looking at boxes, and top-of-screen navigation is out of thumb reach. Tabs must respect entitlements and role levels exactly like the sidebar — a user with `none` in Inventory gets three tabs, not a disabled fourth.

#### 10.7.4 Tables on small screens

There are four established strategies. **Choose per screen; do not apply one globally.**

| Pattern | How it works | Use for |
|---|---|---|
| Horizontal scroll + frozen first column | Table stays intact, swipe sideways | Stock balances, stock ledger — data integrity matters more than fit |
| Card transformation | Each row becomes a stacked card | Requisition list, PO list, supplier list — read individually, modest counts |
| Hide non-critical columns | Show essentials, rest on detail | Product list |
| Priority+ | Show top columns, reveal rest behind a "more" control | Goods receipt lines |

A note on the trade-off, because it is genuinely contested: some practitioners argue against card transformation entirely, on the grounds that most dashboard users are on wide screens and horizontal scroll with a frozen first column is enough. That reasoning holds when mobile is incidental. Here it is not — the receipt flow is a real phone workflow — so cards earn their place on the document lists a manager scans on the move, while the dense analytical grids keep horizontal scroll. Pick per screen, and be able to say why.

Implementation notes:

- Use **semantic `<table>`, `<thead>`, `<tbody>`, `<th scope>`** markup. Native table semantics give screen readers row/column relationships for free; ARIA grid roles are the harder fallback, not the starting point.
- When transforming to cards, render a genuinely different component below the breakpoint rather than CSS-hiding cells — a `<td>` styled to look like a card still announces as a table cell.
- Sticky table headers on any table taller than one screen. Watch the z-index where a sticky header meets a frozen column.
- Always show a total count with pagination. "Page 3 of ?" strands people.

#### 10.7.5 Touch targets and forms

- Minimum interactive target **44×44px**. The documented floors are 24×24 CSS px (WCAG 2.2 Target Size Minimum) and 48dp (Material Design 3); 44px sits comfortably above the former and close to the latter.
- **No hover-only actions, anywhere.** An action that only appears on mouse hover is invisible to touch and keyboard users. This is an accessibility failure, not a space-saving technique. Row actions live in a persistent trailing menu.
- Numeric inputs use `inputmode="decimal"` so phones show a number pad — quantities are typed constantly in the receipt flow.
- Below `md`, forms are single-column with full-width inputs and a **sticky bottom action bar** holding the primary action, so "Post receipt" is always reachable without scrolling to the end of a long line list.
- Every destructive or irreversible action needs confirmation, and confirmation dialogs must be reachable and dismissible by keyboard.

#### 10.7.6 States

Every list and table needs all of: **default, loading, empty, error**. Use skeleton rows matching real row height rather than a spinner — skeletons hold the layout still so nothing lurches when data arrives.

Distinguish two different empty states, because they need different copy and different actions:

- **First-run empty** ("no requisitions yet") → explain what will appear and offer the action that creates one.
- **No results** ("no requisitions match these filters") → offer to clear the filters.

A blank panel for either reads as broken.
