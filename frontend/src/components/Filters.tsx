import { useEffect, useId, useRef, useState, type ReactNode } from "react";

/**
 * The filter row every list screen puts above its table.
 *
 * ONE LINE, AND THE SEARCH BOX IS WHAT GIVES. The dropdowns hold their width —
 * a filter whose current value is clipped is a filter you have to open to read —
 * and the search box takes whatever is left. Below its `min-w`, the row wraps
 * rather than crushing anything: at 360px a search box and three dropdowns
 * cannot share a line, and pretending otherwise produces four unusable controls
 * instead of a second row.
 */
export function FilterBar({ children }: { children: ReactNode }) {
  return <div className="mb-4 flex flex-wrap items-center gap-3">{children}</div>;
}

function SearchIcon() {
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
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </svg>
  );
}

function ChevronIcon({ open }: { open: boolean }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`size-4 shrink-0 text-secondary transition-transform ${
        open ? "rotate-180" : ""
      }`}
    >
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}

/**
 * The search box: an icon inside the field, and the label carried by `aria-label`
 * rather than printed above it.
 *
 * The visible "Search" caption these replaced said nothing the placeholder did
 * not — "SKU or name" already tells you both that it is a search and what it
 * searches — and it forced every filter row two lines tall before any control
 * had been placed. The accessible name is still there, which is the half that
 * was doing work.
 */
export function SearchInput({
  label,
  value,
  onChange,
  placeholder,
}: {
  /** The accessible name. Says *what* is being searched — "Search requisitions",
   *  not "Search" — because a screen reader user landing on it has no column
   *  headings for context. */
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <div className="relative min-w-[14rem] flex-1">
      <span className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-secondary">
        <SearchIcon />
      </span>
      <input
        type="search"
        aria-label={label}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="min-h-11 w-full rounded-xl border border-hairline bg-surface pl-11 pr-3 text-sm placeholder:text-secondary"
      />
    </div>
  );
}

export type FilterOption = { value: string; label: string };

/**
 * A filter dropdown.
 *
 * WHY THIS IS NOT A `<select>`. It very nearly is, and a styled native select
 * would have been free. What it could not do is the part that matters on these
 * screens: show the *current* value in the trigger while the list itself renders
 * an explicit "All suppliers" row that is visibly the selected one. A native
 * select's popup is drawn by the OS, so the open state cannot be made to match
 * the rest of the application in either theme — and on the two dense grids the
 * filter row is the only chrome above a table of numbers, which is exactly where
 * an OS-coloured menu looks like a different program.
 *
 * The cost is that the keyboard behaviour has to be written out, so it is:
 * Enter/Space/ArrowDown open, arrows and Home/End move, Enter picks, Escape
 * closes and returns focus, Tab closes. `aria-activedescendant` carries the
 * highlight, so focus itself never leaves the trigger.
 */
export function FilterDropdown({
  label,
  value,
  options,
  allLabel,
  onChange,
}: {
  /** The accessible name of the control — "Status", "Supplier". */
  label: string;
  /** `""` means no filter, and renders as `allLabel`. */
  value: string;
  options: FilterOption[];
  allLabel: string;
  onChange: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const listId = useId();

  const all: FilterOption = { value: "", label: allLabel };
  const items = [all, ...options];
  const selectedIndex = Math.max(
    0,
    items.findIndex((item) => item.value === value),
  );
  const selected = items[selectedIndex];

  function close(refocus = true) {
    setOpen(false);
    if (refocus) trigger.current?.focus();
  }

  function choose(index: number) {
    onChange(items[index].value);
    close();
  }

  useEffect(() => {
    if (!open) return;
    // Opening starts on whatever is currently chosen, not at the top: arrowing
    // once from "All suppliers" when Acme is selected would be a silent jump.
    setActive(selectedIndex);

    function onPointerDown(event: MouseEvent) {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onPointerDown);
    return () => document.removeEventListener("mousedown", onPointerDown);
    // selectedIndex is read once, at open. Re-running on every change would
    // move the highlight under the user while they are arrowing.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  function onKeyDown(event: React.KeyboardEvent) {
    if (!open) {
      if (event.key === "Enter" || event.key === " " || event.key === "ArrowDown") {
        event.preventDefault();
        setOpen(true);
      }
      return;
    }

    switch (event.key) {
      case "Escape":
        event.preventDefault();
        close();
        break;
      case "Tab":
        // Not preventDefault: Tab should both close this and move on, which is
        // what a reader expects from a menu they have finished with.
        setOpen(false);
        break;
      case "ArrowDown":
        event.preventDefault();
        setActive((current) => Math.min(current + 1, items.length - 1));
        break;
      case "ArrowUp":
        event.preventDefault();
        setActive((current) => Math.max(current - 1, 0));
        break;
      case "Home":
        event.preventDefault();
        setActive(0);
        break;
      case "End":
        event.preventDefault();
        setActive(items.length - 1);
        break;
      case "Enter":
      case " ":
        event.preventDefault();
        choose(active);
        break;
    }
  }

  return (
    // 176px from `sm` up, where it is a fixed shape and the search box absorbs
    // the slack. Below that it shares the row instead — two of these fit at
    // 360px and three do not, so the ledger's third wraps and the two document
    // lists stay on one line. A hard `w-44` on a phone put every dropdown on a
    // row of its own and pushed the table three rows down.
    <div
      className="relative min-w-[9rem] flex-1 sm:w-44 sm:flex-none"
      ref={container}
    >
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={open ? `${listId}-${active}` : undefined}
        aria-label={label}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={onKeyDown}
        className={`flex min-h-11 w-full items-center justify-between gap-2 rounded-xl border bg-surface px-3 text-left text-sm transition-colors ${
          open ? "border-accent" : "border-hairline hover:bg-subtle"
        }`}
      >
        {/* The chosen value, not the label. The label is the accessible name;
            printing it here as well would cost half the width to say "Status"
            next to the status. */}
        <span
          className={`truncate ${value === "" ? "text-secondary" : "text-primary"}`}
        >
          {selected.label}
        </span>
        <ChevronIcon open={open} />
      </button>

      {open && (
        <ul
          id={listId}
          role="listbox"
          aria-label={label}
          className="absolute left-0 top-full z-40 mt-2 max-h-72 w-full min-w-44 overflow-auto rounded-xl border border-hairline bg-surface p-1.5 shadow-lg"
        >
          {items.map((item, index) => {
            const isSelected = item.value === value;
            return (
              <li
                key={item.value || "all"}
                id={`${listId}-${index}`}
                role="option"
                aria-selected={isSelected}
                // A pointer press must not blur the trigger — focus stays there
                // for the whole interaction, and `aria-activedescendant` is what
                // moves.
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(index)}
                onMouseEnter={() => setActive(index)}
                className={`flex min-h-11 cursor-pointer items-center rounded-lg px-3 text-sm ${
                  index === active ? "bg-subtle" : ""
                } ${isSelected ? "font-medium text-accent" : ""}`}
              >
                {item.label}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
