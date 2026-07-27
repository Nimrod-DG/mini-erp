import type { ReactNode } from "react";

import { useTheme, type ThemePreference } from "../hooks/useTheme";

/**
 * The three glyphs. Inline rather than from an icon package: this application
 * has no icon dependency, and three 24px paths are not a reason to acquire one.
 *
 * `stroke="currentColor"` throughout, so the selected state's colour change
 * carries the icon with it and neither theme needs a second copy.
 */
function Glyph({ children }: { children: ReactNode }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="size-[18px]"
    >
      {children}
    </svg>
  );
}

const ICONS: Record<ThemePreference, ReactNode> = {
  light: (
    <Glyph>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
    </Glyph>
  ),
  dark: (
    <Glyph>
      <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />
    </Glyph>
  ),
  system: (
    <Glyph>
      <rect x="2.5" y="4" width="19" height="12.5" rx="2" />
      <path d="M9 20.5h6M12 16.5v4" />
    </Glyph>
  ),
};

const OPTIONS: { value: ThemePreference; label: string }[] = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "system", label: "System" },
];

/**
 * Three states, defaulting to system. The choice persists in localStorage and
 * is re-applied before first paint by the script in index.html.
 *
 * WHY THIS IS THREE ICONS AND NOT ONE MOON. The header this was restyled
 * towards carries a single circular button that flips between light and dark,
 * and collapsing to that would be closer still. It would also throw away
 * `system`, which §10.8.3 requires and which is the *default* — an application
 * whose only theme control is a two-way flip cannot express "follow the OS", so
 * the first press would silently opt the user out of it for good.
 *
 * The label survives as `sr-only` text rather than an `aria-label`. It is what
 * a screen reader announces either way, but as real text it is also what
 * `textContent` reports, so the control cannot quietly lose its names to an
 * icon-only rewrite without a test noticing.
 */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();

  return (
    <div
      role="radiogroup"
      aria-label="Colour theme"
      className="inline-flex rounded-full border border-hairline bg-surface p-0.5"
    >
      {OPTIONS.map((option) => {
        const selected = preference === option.value;
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={selected}
            title={option.label}
            onClick={() => setPreference(option.value)}
            className={
              // min-h-10 inside a p-0.5 border makes the control 44px overall,
              // which is §10.7.5's floor. It was px-3 py-1 — about 26px, and one
              // of four things the Phase 7 touch-target audit found.
              "inline-flex min-h-10 min-w-10 items-center justify-center rounded-full transition-colors " +
              (selected
                ? "bg-accent text-canvas"
                : "text-secondary hover:bg-subtle hover:text-primary")
            }
          >
            {ICONS[option.value]}
            <span className="sr-only">{option.label}</span>
          </button>
        );
      })}
    </div>
  );
}
