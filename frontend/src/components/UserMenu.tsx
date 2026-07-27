import { useEffect, useRef, useState } from "react";

import { useAuth, useMe } from "../hooks/useAuth";
import type { Me, ModuleCode } from "../lib/api";

/** Written out rather than built from a template string, for the same reason as
 *  `AppShell`'s copy: Tailwind scans source text. */
const MODULES: { code: ModuleCode; label: string }[] = [
  { code: "procurement", label: "Procurement" },
  { code: "inventory", label: "Inventory" },
  { code: "finance", label: "Finance" },
];

/**
 * Up to two initials, from the first and last word of the name.
 *
 * Not `name[0]` alone: "Budi Santoso" and "Budi Prakoso" are two people at the
 * same workspace in the demo data, and a single letter makes the avatar say
 * nothing about which one is signed in.
 */
function initials(fullName: string): string {
  const words = fullName.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "?";
  const first = words[0][0];
  const last = words.length > 1 ? words[words.length - 1][0] : "";
  return (first + last).toUpperCase();
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

function SignOutIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="size-[18px] shrink-0"
    >
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" />
    </svg>
  );
}

/** The levels this identity holds, as the quiet typographic labels §10.8.1 asks
 *  for rather than saturated pills. They moved off the header bar and into here
 *  with everything else that describes *who you are* rather than what you can
 *  do next. */
function RoleRows({ me }: { me: Me }) {
  const held = MODULES.filter((module) => me.moduleRoles[module.code]);
  if (held.length === 0) return null;

  return (
    <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1.5 px-4 py-3 text-sm">
      {held.map((module) => (
        <div key={module.code} className="contents">
          <dt className="text-secondary">{module.label}</dt>
          <dd className="tabular text-right">{me.moduleRoles[module.code]}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * The avatar button and the menu behind it.
 *
 * WHAT IT REPLACED. The header used to spell all of this out along the bar: two
 * role badges, the full name, and a Sign out button, which at 1280px left the
 * middle of the header empty and at 360px wrapped onto a second row. Identity
 * detail is not something a person reads on every screen — it is something they
 * check once and then act on — so it belongs behind one control.
 *
 * **There is no Edit profile and no Change password.** Both would be real
 * screens with real server work: a profile edit needs a `PATCH /api/me` that
 * does not exist, and a password change belongs to Firebase, which already owns
 * the reset flow reachable from the sign-in screen. An item that opens nothing
 * is worse than an absent one.
 */
export function UserMenu() {
  const me = useMe();
  const { signOut } = useAuth();
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;

    function onPointerDown(event: MouseEvent) {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") return;
      setOpen(false);
      // Escape must put focus back where it came from, or a keyboard user is
      // dropped at the top of the document with no idea what they dismissed.
      trigger.current?.focus();
    }

    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  return (
    <div className="relative" ref={container}>
      <button
        ref={trigger}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex min-h-11 items-center gap-2 rounded-full border border-hairline bg-surface py-1 pl-1 pr-2 text-sm transition-colors hover:bg-subtle sm:pr-3"
      >
        <span
          aria-hidden="true"
          className="grid size-8 shrink-0 place-items-center rounded-full bg-accent text-xs font-semibold text-canvas"
        >
          {initials(me.user.fullName)}
        </span>
        {/* The name is the button's accessible name in both layouts. Below `sm`
            it is not drawn — an avatar and a chevron are enough at 360px — but
            it stays in the tree, because "button" is not a description of
            anything. */}
        <span className="max-sm:sr-only">{me.user.fullName}</span>
        <ChevronIcon open={open} />
      </button>

      {open && (
        <div
          role="menu"
          aria-label="Account"
          className="absolute right-0 top-full z-40 mt-2 w-64 overflow-hidden rounded-xl border border-hairline bg-surface shadow-lg"
        >
          <div className="px-4 py-3">
            <p className="font-medium">{me.user.fullName}</p>
            <p className="truncate text-sm text-secondary">{me.user.email}</p>
            {/* One text node, not a name plus a styled span. The workspace and
                the role are read as a single phrase — "Nusantara Retail ·
                staff" — and `tabular` is for numbers and document numbers, not
                for a word that happens to be an enum. */}
            <p className="mt-1 text-xs text-secondary">
              {`${me.tenant ? me.tenant.name : "Platform"} · ${me.user.tenantRole}`}
            </p>
          </div>

          <div className="border-t border-hairline" />
          <RoleRows me={me} />

          <div className="border-t border-hairline" />
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              void signOut();
            }}
            className="flex min-h-11 w-full items-center gap-3 px-4 text-left text-sm transition-colors hover:bg-subtle"
          >
            <SignOutIcon />
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
