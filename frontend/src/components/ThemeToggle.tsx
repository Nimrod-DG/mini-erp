import { useTheme, type ThemePreference } from "../hooks/useTheme";

const OPTIONS: { value: ThemePreference; label: string }[] = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "system", label: "System" },
];

/** Three states, defaulting to system. The choice persists in localStorage and
 *  is re-applied before first paint by the script in index.html. */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();

  return (
    <div
      role="radiogroup"
      aria-label="Colour theme"
      className="inline-flex rounded-md border border-hairline p-0.5"
    >
      {OPTIONS.map((option) => {
        const selected = preference === option.value;
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => setPreference(option.value)}
            className={
              // min-h-10 inside a p-0.5 border makes the control 44px overall,
              // which is §10.7.5's floor. It was px-3 py-1 — about 26px, and one
              // of four things the Phase 7 touch-target audit found.
              "inline-flex min-h-10 items-center rounded px-3 text-sm transition-colors " +
              (selected
                ? "bg-accent text-canvas"
                : "text-secondary hover:text-primary")
            }
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
