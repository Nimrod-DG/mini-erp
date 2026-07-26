import { FirebaseError } from "firebase/app";
import { useState, type FormEvent } from "react";

import { PasswordField } from "../components/PasswordField";
import { ThemeToggle } from "../components/ThemeToggle";
import { useAuth } from "../hooks/useAuth";

/** The raw Firebase code, for the console and for the dev-only hint. Collapsing
 *  every failure into one user-facing sentence is right for the user and awful
 *  for whoever is debugging — this keeps the real code reachable without
 *  putting it in front of an anonymous visitor. */
function errorCode(error: unknown): string {
  if (error instanceof FirebaseError) return error.code;
  if (error instanceof Error) return error.name;
  return "unknown";
}

/**
 * Firebase codes are stable and machine-readable; their default messages are
 * not meant for users. Everything that could distinguish "no such account" from
 * "wrong password" collapses into one sentence — a login form that tells you
 * which is an account-enumeration oracle.
 */
function signInMessage(error: unknown): string {
  if (!(error instanceof FirebaseError)) {
    return "Could not sign in. Try again.";
  }
  switch (error.code) {
    case "auth/invalid-credential":
    case "auth/invalid-email":
    case "auth/wrong-password":
    case "auth/user-not-found":
      return "Email or password is incorrect.";
    case "auth/user-disabled":
      return "That account has been disabled.";
    case "auth/too-many-requests":
      return "Too many attempts. Try again in a few minutes.";
    case "auth/network-request-failed":
      return "Could not reach the sign-in service. Check your connection.";
    case "auth/operation-not-allowed":
      // A configuration fault, not a user fault: email/password is not enabled
      // on the Firebase project.
      return "Email sign-in is not enabled for this project.";
    default:
      return "Could not sign in. Try again.";
  }
}

type Mode = "signIn" | "reset";

export function Login() {
  const authState = useAuth();
  const { signIn, sendReset } = authState;

  const [mode, setMode] = useState<Mode>("signIn");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [code, setCode] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  // The provider's notice explains a rejection the form could not have known
  // about — a deactivated user, an account with no row on this side.
  const banner =
    error ?? (authState.status === "signedOut" ? authState.notice : undefined);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    setCode(null);
    setBusy(true);
    try {
      if (mode === "signIn") {
        await signIn(email, password);
        // No navigation here. The auth listener flips the whole app to
        // "signedIn" and the router follows; navigating manually would race it.
      } else {
        await sendReset(email);
        setSent(true);
      }
    } catch (err) {
      console.warn("[auth] sign-in failed:", errorCode(err), err);
      // A failed reset is reported the same as a successful one — see below.
      if (mode === "reset") {
        setSent(true);
      } else {
        setError(signInMessage(err));
        setCode(errorCode(err));
      }
    } finally {
      setBusy(false);
    }
  }

  function switchMode(next: Mode) {
    setMode(next);
    setError(null);
    setCode(null);
    setSent(false);
  }

  return (
    <div className="min-h-screen bg-canvas text-primary">
      <header className="flex items-center justify-between px-6 py-4">
        <span className="text-lg font-semibold">mini-erp</span>
        <ThemeToggle />
      </header>

      <main className="mx-auto flex max-w-sm flex-col px-6 py-16">
        <h1 className="text-xl font-semibold">
          {mode === "signIn" ? "Sign in" : "Reset your password"}
        </h1>
        <p className="mt-1 text-sm text-secondary">
          {mode === "signIn"
            ? "Accounts are created by your administrator."
            : "We will email you a link to choose a new password."}
        </p>

        {banner && (
          <p
            role="alert"
            className="mt-6 rounded-md border border-hairline bg-surface px-4 py-3 text-sm text-danger"
          >
            {banner}
            {/* Dev only. `import.meta.env.DEV` is statically replaced, so this
                whole branch is dropped from the production bundle — the real
                code stays available while building and never reaches a user. */}
            {import.meta.env.DEV && code && (
              <span className="tabular mt-1 block text-xs text-secondary">
                {code}
              </span>
            )}
          </p>
        )}

        {sent && (
          <p
            role="status"
            className="mt-6 rounded-md border border-hairline bg-surface px-4 py-3 text-sm text-secondary"
          >
            If an account exists for {email}, a reset link is on its way.
          </p>
        )}

        <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4">
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="font-medium">Email</span>
            <input
              type="email"
              name="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="min-h-11 rounded-md border border-hairline bg-surface px-3 text-base"
            />
          </label>

          {mode === "signIn" && (
            <PasswordField value={password} onChange={setPassword} />
          )}

          <button
            type="submit"
            disabled={busy}
            className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-canvas disabled:opacity-60"
          >
            {busy
              ? "Working…"
              : mode === "signIn"
                ? "Sign in"
                : "Send reset link"}
          </button>
        </form>

        <button
          type="button"
          onClick={() => switchMode(mode === "signIn" ? "reset" : "signIn")}
          className="mt-2 inline-flex min-h-11 items-center self-start text-sm text-accent underline-offset-4 hover:underline"
        >
          {mode === "signIn" ? "Forgot your password?" : "Back to sign in"}
        </button>
      </main>
    </div>
  );
}
