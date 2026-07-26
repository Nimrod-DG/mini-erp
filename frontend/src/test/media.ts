/**
 * `window.matchMedia`, which jsdom does not implement at all.
 *
 * Two things in this application are decided in JavaScript rather than in CSS,
 * and both go through it: `useCompact` asks whether the viewport is below `md`
 * (§10.7.4), and `useTheme` asks whether the OS prefers dark (§10.8.3). Neither
 * is testable without a media-query engine, so this is a small real one.
 *
 * WHY THE LISTS ARE CACHED BY QUERY STRING. A browser returns a fresh
 * `MediaQueryList` from every `matchMedia` call, and both hooks call it more than
 * once — `useCompact`'s subscribe and its cleanup each make their own. With fresh
 * objects, `removeEventListener` would be called on an object that never held the
 * listener, so every unmounted component would leak one and later tests would
 * update dead ones. One object per query string makes add and remove meet.
 */

type Listener = (event: { matches: boolean; media: string }) => void;

type FakeMediaQueryList = {
  media: string;
  matches: boolean;
  addEventListener: (type: "change", listener: Listener) => void;
  removeEventListener: (type: "change", listener: Listener) => void;
  onchange: null;
  addListener: (listener: Listener) => void;
  removeListener: (listener: Listener) => void;
  dispatchEvent: () => boolean;
};

/** A desktop, and light. The defaults a test does not have to state. */
const DEFAULT_WIDTH = 1280;

let width = DEFAULT_WIDTH;
let prefersDark = false;

const lists = new Map<string, { list: FakeMediaQueryList; listeners: Set<Listener> }>();

/** The three query forms this application actually writes. Anything else is a
 *  test asking a question the app does not ask, and answering `false` silently
 *  would hide it — so it throws. */
function evaluate(media: string): boolean {
  const max = /\(\s*max-width:\s*([\d.]+)px\s*\)/.exec(media);
  if (max) return width <= Number.parseFloat(max[1]);

  const min = /\(\s*min-width:\s*([\d.]+)px\s*\)/.exec(media);
  if (min) return width >= Number.parseFloat(min[1]);

  if (/prefers-color-scheme:\s*dark/.test(media)) return prefersDark;

  throw new Error(`the matchMedia fake does not understand "${media}"`);
}

function entry(media: string) {
  const existing = lists.get(media);
  if (existing) {
    // Re-evaluated on every lookup, so a query created before a viewport change
    // still answers correctly when it is read after one.
    existing.list.matches = evaluate(media);
    return existing;
  }

  const listeners = new Set<Listener>();
  const list: FakeMediaQueryList = {
    media,
    matches: evaluate(media),
    addEventListener: (_type, listener) => void listeners.add(listener),
    removeEventListener: (_type, listener) => void listeners.delete(listener),
    onchange: null,
    // The deprecated pair, in case a dependency reaches for it.
    addListener: (listener) => void listeners.add(listener),
    removeListener: (listener) => void listeners.delete(listener),
    dispatchEvent: () => true,
  };

  const created = { list, listeners };
  lists.set(media, created);
  return created;
}

/** Re-evaluate every live query and notify the ones whose answer changed. A
 *  query whose answer did not change fires nothing, exactly as a browser does. */
function notifyChanged(): void {
  for (const [media, { list, listeners }] of lists) {
    const next = evaluate(media);
    if (next === list.matches) continue;
    list.matches = next;
    for (const listener of [...listeners]) listener({ matches: next, media });
  }
}

export function installMatchMedia(): void {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    writable: true,
    value: (media: string) => entry(media).list,
  });
}

/**
 * Set the viewport width. 360 is the phone §10.7 designs down to; 1024 and up is
 * `lg`, where the table and the persistent sidebar live.
 *
 * Callers must wrap this in `act()` — `useCompact` is a `useSyncExternalStore`
 * subscription, so a change here re-renders.
 */
export function setViewportWidth(next: number): void {
  width = next;
  notifyChanged();
}

/** Flip the OS colour-scheme preference, for FE11. Same `act()` rule. */
export function setPrefersDark(next: boolean): void {
  prefersDark = next;
  notifyChanged();
}

export function resetMedia(): void {
  width = DEFAULT_WIDTH;
  prefersDark = false;
  lists.clear();
}
