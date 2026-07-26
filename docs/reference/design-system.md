# Reference — Design direction and theming

> Phase 0 (tokens + theme wiring), then consulted by every UI phase.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 10.0 Design direction and references

The screens below specify *what* exists. This section specifies *how it should look*, because "a working ERP that looks like an unstyled admin template" and "a working ERP that looks considered" are judged very differently in a portfolio.

#### 10.0.1 The governing tension

Enterprise software is one of the few places where **familiarity is a feature**. A warehouse clerk should not have to learn a novel interface to receive a delivery. So the rule here is: **be conventional everywhere, and spend all the boldness in one place.**

That one place is the **cross-module confirmation panel** (Section 10.3) — the moment a goods receipt lands and the UI names what just happened across procurement, inventory, and finance. It is the thesis of the whole project, and it is the screenshot that goes in the portfolio. Everything else stays quiet and disciplined.

#### 10.0.2 Grounding the aesthetic in the subject

The subject world here is **documents and ledgers**: things with reference numbers, states, and a trail. Two design consequences worth taking seriously:

**Numbers are the content.** Quantities, costs, debits, and credits are what users actually read. Set every numeric column in a **tabular-figure** face so digits align vertically — `font-variant-numeric: tabular-nums`. Right-align numerics, left-align text. This single decision does more for perceived quality in an ERP than any amount of decoration, and most templates get it wrong.

**Document numbers are identity, not metadata.** `PR-202607-0001` already encodes type, period, and sequence (Section 8.1). Treat it as the primary label on document screens — monospaced, prominent — rather than hiding it as grey small print under a generic "Purchase Requisition" heading.

#### 10.0.3 Palette

The tokens in Section 10.8 are structural; these are the values to fill them with. The direction is **ink on paper** — a document system, not a dashboard.

| Role | Light | Dark | Notes |
|---|---|---|---|
| Canvas | `#FBFBFA` | `#0E1114` | Warm-neutral paper, not cream — cream plus a serif is a recognisable AI-template look |
| Surface | `#FFFFFF` | `#181B1F` | Cards, table backgrounds |
| Text primary | `#17191C` | `#F4F4F3` | Near-black with a blue undertone, not pure black |
| Text secondary | `#64676C` | `#A3A3A0` | |
| Border | `#E5E5E3` | `#33383E` | Hairlines, never heavy rules |
| Accent | `#0F6E6E` | `#4FB3B3` | Petrol/teal — reads as ledger ink rather than default SaaS blue |
| Success | `#15803D` | `#4ADE80` | |
| Warning | `#B45309` | `#FBBF24` | |
| Danger | `#B91C1C` | `#F87171` | |

Deliberately avoided: the near-black-plus-acid-green look, and the cream-plus-terracotta look. Both are currently everywhere and read as unconsidered.

#### 10.0.4 Typography

Three roles, and the third is the one most projects skip:

| Role | Suggestion | Used for |
|---|---|---|
| UI / body | Inter, or your system stack | Everything conversational |
| **Numeric / document** | **JetBrains Mono** or **IBM Plex Mono** | All quantities, money, document numbers, SKUs, ledger rows |
| Display | The body face at 600–700 weight | Page titles — no separate display face |

Resisting a decorative display face is the right call here. An ERP earns its character from **alignment discipline and numeric precision**, not from a personality typeface. The monospace face carries the identity.

Set a restrained scale — 12 / 14 / 16 / 20 / 28 — and use weight rather than size to establish hierarchy in dense screens.

#### 10.0.5 Status treatment

Status is the most repeated element in this interface (five requisition states, four PO states). Get it right once.

- **Text label plus a shape or icon, never colour alone** — required for colour-blind users and for both themes (Section 10.8.4).
- Prefer a **quiet typographic label with a subtle left border or a small dot** over a saturated filled pill. Eight saturated pills in a table row is visual noise; the eye should land on the *exceptional* status, not on all of them equally.
- Reserve saturated colour for states that need action: `submitted` awaiting approval, `cancelled`, `rejected`. Neutral states (`draft`, `open`, `received`) stay grey.

#### 10.0.6 References worth studying

Look at how these solve the specific problems in this build rather than copying their look:

| Reference | Study it for |
|---|---|
| **IBM Carbon** | Data-dense enterprise layout, table density options, the discipline of a strict grid |
| **Atlassian Design System** | Workflow and status patterns, approval UI, empty states |
| **Shopify Polaris** | Inventory and commerce conventions, and the best writing guidance of any public design system |
| **Ant Design** | The most common admin/ERP visual language — worth knowing the conventions you are choosing to follow or break |
| **shadcn/ui** | The practical component base for React + Tailwind; unstyled enough to take the palette above |
| **Tremor** | Dashboard and chart components that fit the same stack |

For the data-table behaviour specifically, Section 10.7.4 already covers the four mobile strategies and when each applies.

#### 10.0.7 Copy

Interface words are design material, and they are cheap to get right:

- **Name things by what the user controls**, not by how the system works. "Receive goods", not "Create goods receipt record".
- **An action keeps its name through the whole flow.** The button says "Post receipt", the toast says "Receipt posted", the audit entry says posted. Consistency is how people learn an interface.
- **Errors say what happened and what to do.** `409 in_use` should render as "This supplier has 3 open purchase orders. Close or cancel them before deleting." — not "Operation failed".
- **Empty states are invitations.** "No requisitions yet — create one to get started" with the create button, not a blank panel (Section 10.7.6).
- Sentence case throughout. Active voice. No filler.


### 10.8 Theming — light and dark mode

Both themes are first-class. Dark mode is an accessibility and comfort feature, not a style flourish, and a poorly-built dark mode is worse than none.

#### 10.8.1 Semantic tokens, not `dark:` modifiers

**Do not scatter `dark:` variants through component markup.** Writing `text-slate-900 dark:text-slate-100` on every element produces a codebase where a single colour change means touching hundreds of files, and it guarantees drift.

Instead define **semantic tokens** — named by role, not by appearance — that resolve differently per theme. Components reference the role and never know which theme is active:

```css
/* globals.css */
:root {
  --color-bg-canvas:      251 251 250;   /* #FBFBFA */
  --color-bg-surface:     255 255 255;
  --color-bg-raised:      255 255 255;
  --color-text-primary:    23  25  28;   /* #17191C */
  --color-text-secondary: 100 103 108;
  --color-border:         229 229 227;
  --color-accent:          15 110 110;   /* #0F6E6E petrol */
  --color-success:         21 128  61;
  --color-warning:        180  83   9;
  --color-danger:         185  28  28;
}

.dark {
  --color-bg-canvas:       14  17  20;   /* #0E1114 */
  --color-bg-surface:      24  27  31;   /* #181B1F */
  --color-bg-raised:       32  36  41;
  --color-text-primary:   244 244 243;
  --color-text-secondary: 163 163 160;
  --color-border:          51  56  62;
  --color-accent:          79 179 179;   /* #4FB3B3 petrol, lightened for dark */
  --color-success:         74 222 128;
  --color-warning:        251 191  36;
  --color-danger:         248 113 113;
}
```

Note that `--color-accent`, `--color-success`, `--color-danger` and friends have **different values per theme**, not the same value reused. Saturated colours that read well on white vibrate uncomfortably on near-black, so dark variants are lighter and less saturated. Reusing one value for both themes is the most common dark-mode mistake.

Raw channel values (`23 25 28` rather than `#17191C`) let Tailwind apply opacity modifiers like `bg-surface/60`. The values above are the Section 10.0.3 palette.

#### 10.8.2 Tailwind v4 wiring — the gotcha

If using Tailwind v4's `@theme`, **do not use `@theme inline` for colours.** Inline bakes values in at build time, which breaks runtime theme switching — the classic symptom is a toggle that changes nothing. The working shape is two-stage: raw channel values in `:root` / `.dark`, mapped by a **non-inline** `@theme`:

```css
@theme {
  --color-canvas:   rgb(var(--color-bg-canvas) / <alpha-value>);
  --color-surface:  rgb(var(--color-bg-surface) / <alpha-value>);
  --color-primary:  rgb(var(--color-text-primary) / <alpha-value>);
}
```

CSS custom properties resolve at runtime rather than build time, which is precisely why they are the right tool for instant theme switching.

#### 10.8.3 Three-state toggle and no flash

Offer **light / dark / system**, defaulting to system. Persist the choice in `localStorage`.

Apply the theme **before first paint** with a small blocking script in `index.html`. Without this, a dark-mode user gets a white flash on every page load:

```html
<script>
  (function () {
    var s = localStorage.getItem('theme') || 'system';
    var dark = s === 'dark' ||
      (s === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
    document.documentElement.classList.toggle('dark', dark);
  })();
</script>
```

When set to `system`, listen for changes to `prefers-color-scheme` so the app follows the OS without a reload.

#### 10.8.4 Contrast and status colours

- Body text: **4.5:1** minimum against its background. Large text and non-text UI (borders, icons, focus rings): **3:1**.
- **Never encode meaning in colour alone.** Every status needs a shape, icon, or text label alongside its colour. A red PO status badge must also say "Cancelled". This matters for colour-blind users in both themes and is a hard requirement for the status badges used throughout Procurement.
- Focus rings must be visible in both themes and must never rely on colour alone. Use `:focus-visible` so keyboard users get a ring without mouse users seeing one on every click.
- Honour `prefers-reduced-motion` for any transition, including the theme switch itself.

#### 10.8.5 Third-party components

Charting and UI libraries do not know about your tokens. **Recharts** in particular takes explicit colour props — read the resolved CSS variables at render time (or pass theme-aware values from a hook) so charts re-colour with the theme instead of staying stuck in light mode. Verify every third-party surface in both themes: charts, date pickers, toasts, and any dropdown that renders in a portal.
