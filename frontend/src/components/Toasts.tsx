import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { ApiError } from "../lib/api";

/**
 * Toasts, for the outcome of something the user just did.
 *
 * The line between this and `ErrorNotice` is worth keeping straight:
 *
 *   - **A load failed** → `ErrorNotice`, inline, where the data would have been.
 *     A toast there leaves an empty screen with no explanation once it fades.
 *   - **An action succeeded or was refused** → a toast. The user pressed a
 *     button and is looking for the answer; putting it inline pushes the layout
 *     around and, for a refusal on a row, ends up somewhere their eye is not.
 *
 * Refusals last much longer than confirmations and can be dismissed by hand. A
 * message like "another product now uses SKU-001" is the whole explanation of
 * why the click did nothing, and a refusal that fades before it is read is
 * worse than no refusal at all.
 */
type ToastKind = "success" | "error";

type Toast = { id: number; kind: ToastKind; message: string };

const SUCCESS_MS = 4000;
const ERROR_MS = 12000;

type ToastApi = {
  /** Confirm something happened. Say what, in the user's words, not the API's. */
  success: (message: string) => void;
  /**
   * Report a refusal or a failure. Takes the caught value, so a call site never
   * has to narrow `unknown` itself.
   *
   * An ApiError's `message` is the §9.8 envelope's human sentence, written to be
   * shown. Anything else is a network or programming failure, and saying so
   * plainly beats leaking "TypeError: fetch failed" at somebody.
   */
  failure: (caught: unknown, fallback?: string) => void;
};

const ToastContext = createContext<ToastApi | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(0);
  const timers = useRef(new Set<ReturnType<typeof setTimeout>>());

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const push = useCallback(
    (kind: ToastKind, message: string) => {
      const id = nextId.current++;
      setToasts((current) => [...current, { id, kind, message }]);

      const timer = setTimeout(
        () => {
          timers.current.delete(timer);
          dismiss(id);
        },
        kind === "error" ? ERROR_MS : SUCCESS_MS,
      );
      timers.current.add(timer);
    },
    [dismiss],
  );

  // Unmounting with timers outstanding would have them fire against a dead
  // setState. Only reachable on a hot reload in practice, which is exactly when
  // a stray warning is most confusing.
  useEffect(() => {
    const pending = timers.current;
    return () => {
      for (const timer of pending) clearTimeout(timer);
      pending.clear();
    };
  }, []);

  const api = useMemo<ToastApi>(
    () => ({
      success: (message) => push("success", message),
      failure: (caught, fallback) => {
        if (caught instanceof ApiError) {
          push("error", caught.message);
          return;
        }
        // Logged in full, shown as a sentence. The stack is for whoever is
        // debugging; the user gets something they can act on.
        console.warn("toast: unexpected failure", caught);
        push(
          "error",
          fallback ?? "Something went wrong. Try again in a moment.",
        );
      },
    }),
    [push],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}

      {/*
        aria-live on the container rather than on each toast: the region has to
        exist in the DOM before a message is put into it, or a screen reader has
        nothing to watch and announces nothing.

        `polite`, not `assertive` — these follow the user's own action, so they
        are never an interruption.
      */}
      {/*
        Top right, but BELOW the header rather than over it. The header is
        `sticky top-0` and this container is z-50, so anchoring at top-0 would
        put a toast on top of Sign out and the theme toggle — and the page's own
        action button (Add product, Add warehouse) sits in the same corner just
        underneath, which is often the button that raised the toast.

        top-14 clears the 57px header; the container's own p-4 gives the gap.
        New toasts stack downwards from there, away from both.
      */}
      <div
        aria-live="polite"
        aria-atomic="false"
        className="pointer-events-none fixed inset-x-0 top-14 z-50 flex flex-col items-center gap-2 p-4 sm:items-end"
      >
        {toasts.map((toast) => (
          <output
            key={toast.id}
            className={`pointer-events-auto flex w-full max-w-md items-start gap-3 rounded-lg border bg-surface p-4 text-sm shadow-lg ${
              toast.kind === "error"
                ? "border-danger/40 text-danger"
                : "border-success/40 text-success"
            }`}
          >
            <span className="min-w-0 grow">{toast.message}</span>
            <button
              type="button"
              onClick={() => dismiss(toast.id)}
              aria-label="Dismiss"
              // A declared 44×44 rather than padding around a glyph. The
              // previous `-m-2 p-2` claimed the §10.7.5 target in its comment
              // and delivered about 36px — the Phase 7 audit measured it. The
              // negative margin still keeps the button from making the toast
              // taller than its text needs.
              className="-my-3 -mr-2 grid size-11 shrink-0 place-items-center text-secondary hover:text-primary"
            >
              <span aria-hidden="true">✕</span>
            </button>
          </output>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const api = useContext(ToastContext);
  if (!api) {
    throw new Error("useToast must be used inside a ToastProvider");
  }
  return api;
}
