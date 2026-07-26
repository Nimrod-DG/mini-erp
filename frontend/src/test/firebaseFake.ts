/**
 * The Firebase fake. §12.4's rule for the backend — "define the verifier as an
 * interface so tests never touch the network" — applied to the browser half.
 *
 * Nothing here tests Firebase. What it makes testable is the thing either side
 * of it: that `AuthProvider` turns a Firebase session into a `/api/me` read, and
 * that `apiFetch` puts the resulting token on every request. Both of those are
 * ours, and both are what every screen test depends on.
 *
 * The two `firebase/*` entry points are mocked as whole modules in `setup.ts`,
 * which is why the fake implementations live in a separate file: a `vi.mock`
 * factory is hoisted above the imports, so it cannot close over anything defined
 * beside it — it can only `await import` a module like this one.
 */

export type FakeUser = {
  uid: string;
  getIdToken: () => Promise<string>;
};

type Listener = (user: FakeUser | null) => void;

const listeners = new Set<Listener>();
let currentUser: FakeUser | null = null;

/**
 * The token is derived from the UID, exactly as the backend's `FakeVerifier`
 * maps a token straight back to one. A test that wants to assert what was sent
 * can therefore predict the header.
 */
function userFor(uid: string): FakeUser {
  return { uid, getIdToken: () => Promise.resolve(`token-${uid}`) };
}

/**
 * Sign a UID in, or `null` to sign out. `AuthProvider` is subscribed through the
 * mocked `onAuthStateChanged`, so this drives the real state machine — including
 * the `/api/me` read, which is the point.
 */
export function setFirebaseUser(uid: string | null): void {
  currentUser = uid === null ? null : userFor(uid);
  for (const listener of [...listeners]) listener(currentUser);
}

/** Between tests. Listeners are dropped without notifying: the components that
 *  registered them are already unmounted by RTL's cleanup. */
export function resetFirebaseFake(): void {
  currentUser = null;
  listeners.clear();
}

/** Stands in for the `Auth` instance `getAuth()` returns. `currentUser` is a
 *  getter because `apiFetch` reads it at call time, on every request. */
export const fakeAuth = {
  get currentUser(): FakeUser | null {
    return currentUser;
  },
};

/** The `firebase/app` surface this application uses: one function. */
export const appModule = {
  initializeApp: () => ({ name: "test" }),
};

/** The `firebase/auth` surface this application uses. */
export const authModule = {
  getAuth: () => fakeAuth,

  onAuthStateChanged: (_auth: unknown, listener: Listener) => {
    listeners.add(listener);
    // Firebase calls back once with the current state on subscribe, and
    // `ProtectedRoute` depends on it: without that first call the app would sit
    // in `loading` forever.
    listener(currentUser);
    return () => listeners.delete(listener);
  },

  signOut: async () => setFirebaseUser(null),

  // The email is used as the UID, so a test signs in as `sari@nusantara.test`
  // and the MSW handler for `/api/me` keys off the same string.
  signInWithEmailAndPassword: async (_auth: unknown, email: string) => {
    setFirebaseUser(email);
    return { user: currentUser };
  },

  sendPasswordResetEmail: async () => undefined,
};
