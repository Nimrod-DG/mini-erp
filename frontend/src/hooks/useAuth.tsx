import {
  onAuthStateChanged,
  sendPasswordResetEmail,
  signInWithEmailAndPassword,
  signOut as firebaseSignOut,
} from "firebase/auth";
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

import { ApiError, getMe, type Me } from "../lib/api";
import { auth } from "../lib/firebase";

/**
 * Signing in has two halves, and both must succeed.
 *
 * Firebase says who you are. The backend says whether you are anybody here —
 * an orphaned account, a deactivated user, and a suspended tenant all
 * authenticate perfectly and then resolve to nothing or to a refusal. So
 * "signed in" means /api/me returned 200, not that Firebase has a session.
 */
export type AuthState =
  | { status: "loading" }
  | { status: "signedOut"; notice?: string }
  | { status: "signedIn"; me: Me }
  | { status: "suspended" };

type AuthContextValue = AuthState & {
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  sendReset: (email: string) => Promise<void>;
  /** Re-read /api/me for the current session. Call it after changing something
   *  /api/me reports about the signed-in user — their tenant role or their
   *  module levels — because the whole navigation is derived from it. */
  refreshMe: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  // onAuthStateChanged can fire again while a /api/me is still in flight —
  // a sign-out during a slow request is the ordinary case. Only the newest
  // resolution may write state, or a stale success reinstates a signed-out user.
  const generation = useRef(0);

  useEffect(() => {
    return onAuthStateChanged(auth, async (firebaseUser) => {
      const current = ++generation.current;
      const commit = (next: AuthState) => {
        if (generation.current === current) setState(next);
      };

      if (!firebaseUser) {
        commit({ status: "signedOut" });
        return;
      }

      commit({ status: "loading" });
      try {
        commit({ status: "signedIn", me: await getMe() });
      } catch (error) {
        if (error instanceof ApiError && error.code === "tenant_suspended") {
          commit({ status: "suspended" });
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          // The credentials were valid and the account still resolves to
          // nobody. Dropping the Firebase session too keeps the app from
          // looping on a token that can never work.
          await firebaseSignOut(auth);
          commit({
            status: "signedOut",
            notice:
              "That account is not active in this workspace. Contact your administrator.",
          });
          return;
        }
        await firebaseSignOut(auth);
        commit({
          status: "signedOut",
          notice: "Could not reach the server. Try again in a moment.",
        });
      }
    });
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    // No state written here: the onAuthStateChanged listener above owns the
    // transition, so there is exactly one path into "signedIn".
    await signInWithEmailAndPassword(auth, email.trim(), password);
  }, []);

  const signOut = useCallback(async () => {
    await firebaseSignOut(auth);
  }, []);

  const sendReset = useCallback(async (email: string) => {
    await sendPasswordResetEmail(auth, email.trim(), {
      // Where the "Continue" button goes *after* the reset completes.
      //
      // This does NOT decide which page the emailed link opens. That is a
      // Firebase Console setting — Authentication → Templates → Password reset
      // → Customize action URL — and it must point at `/auth/action` on this
      // origin, or the link keeps landing on Firebase's hosted page and
      // ResetPassword is never reached.
      url: `${window.location.origin}/login`,
      handleCodeInApp: false,
    });
  }, []);

  // Authorization is resolved from the database on every request (I9), so the
  // server is never stale — but the *navigation* is built from one /api/me read
  // taken at sign-in. Change your own role and the sidebar, the bottom tabs and
  // the role badges all keep describing who you were until the page is reloaded
  // by hand: a Users link that 403s is the visible symptom.
  //
  // Guarded by the same generation counter as the listener above, so a refresh
  // resolving after a sign-out cannot reinstate a signed-in user, and written
  // through a state updater so it is a no-op unless we are still signed in.
  const refreshMe = useCallback(async () => {
    const current = generation.current;
    try {
      const me = await getMe();
      if (generation.current !== current) return;
      setState((previous) =>
        previous.status === "signedIn" ? { status: "signedIn", me } : previous,
      );
    } catch {
      // A failed refresh leaves the last known identity in place. The screen
      // still works, and the next real request surfaces the actual problem —
      // signing someone out over a background refetch would be the worse bug.
    }
  }, []);

  const value = useMemo(
    () => ({ ...state, signIn, signOut, sendReset, refreshMe }),
    [state, signIn, signOut, sendReset, refreshMe],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside <AuthProvider>");
  return ctx;
}

/** The signed-in caller. Throws if used outside a protected route, which makes
 *  a missing guard a loud failure rather than a screen full of "undefined". */
export function useMe(): Me {
  const ctx = useAuth();
  if (ctx.status !== "signedIn") {
    throw new Error("useMe used outside a protected route");
  }
  return ctx.me;
}
