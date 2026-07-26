import { FirebaseError } from "firebase/app";
import { confirmPasswordReset, verifyPasswordResetCode } from "firebase/auth";
import { useEffect, useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { PasswordField } from "../components/PasswordField";
import { ThemeToggle } from "../components/ThemeToggle";
import { useAuth } from "../hooks/useAuth";
import { auth } from "../lib/firebase";

/**
 * Firebase's own floor is 6 characters and it rejects anything shorter with
 * `auth/weak-password`. This is the application's stricter rule, and it is a
 * courtesy rather than a control: Firebase owns credentials, so there is no
 * server of ours to enforce it on. Real policy belongs in Identity Platform's
 * password settings, not here.
 */
const MIN_PASSWORD_LENGTH = 8;

function resetMessage(error: unknown): string {
  if (!(error instanceof FirebaseError)) {
    return "Something went wrong. Request a new link and try again.";
  }
  switch (error.code) {
    case "auth/expired-action-code":
      return "This link has expired. Request a new one from the sign-in page.";
    case "auth/invalid-action-code":
      return "This link is invalid or has already been used. Request a new one.";
    case "auth/user-disabled":
      return "That account has been disabled. Contact your administrator.";
    case "auth/user-not-found":
      return "No account matches this link. Request a new one.";
    case "auth/weak-password":
      return `Choose a longer password — at least ${MIN_PASSWORD_LENGTH} characters.`;
    default:
      return "Could not set the new password. Try again.";
  }
}

type Status = "verifying" | "ready" | "done" | "invalid";

/**
 * The in-app handler for Firebase's password-reset action link — this is what
 * keeps people on our own domain instead of bouncing them to
 * `<project>.firebaseapp.com/__/auth/action`.
 *
 * Firebase opens it as `/auth/action?mode=resetPassword&oobCode=…`. Note that
 * the `url` passed to `sendPasswordResetEmail` does NOT decide which page the
 * emailed link opens — that is a Firebase Console setting (Authentication →
 * Templates → Password reset → Customize action URL). Without that change the
 * email still points at Firebase's hosted page and this route is never reached.
 */
export function ResetPassword() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { signOut } = useAuth();

  const mode = params.get("mode");
  const oobCode = params.get("oobCode") ?? "";

  const [status, setStatus] = useState<Status>("verifying");
  const [accountEmail, setAccountEmail] = useState("");
  const [banner, setBanner] = useState("");

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);

  // Verify before showing the form. A user who types a new password twice and
  // only then learns the link expired has wasted the one thing they came to do.
  useEffect(() => {
    let cancelled = false;

    if (mode !== "resetPassword" || !oobCode) {
      setBanner(
        "This link is not a password reset link. Request a new one from the sign-in page.",
      );
      setStatus("invalid");
      return;
    }

    verifyPasswordResetCode(auth, oobCode)
      .then((email) => {
        if (cancelled) return;
        setAccountEmail(email);
        setStatus("ready");
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        console.warn("[auth] reset code rejected:", error);
        setBanner(resetMessage(error));
        setStatus("invalid");
      });

    return () => {
      cancelled = true;
    };
  }, [mode, oobCode]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setBanner("");

    if (password.length < MIN_PASSWORD_LENGTH) {
      setBanner(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`);
      return;
    }
    if (password !== confirm) {
      setBanner("The two passwords do not match.");
      return;
    }

    setBusy(true);
    try {
      await confirmPasswordReset(auth, oobCode, password);
      // Firebase revokes the account's existing refresh tokens here, so any
      // session open in this tab is already dead. Clearing it locally keeps the
      // app from discovering that as a mystery 401 on the next request.
      await signOut();
      setStatus("done");
    } catch (error) {
      console.warn("[auth] reset failed:", error);
      setBanner(resetMessage(error));
    } finally {
      setBusy(false);
    }
  }

  // Navigating away also strips ?mode=&oobCode= from the URL, so a refresh
  // cannot re-enter the flow with a code that has now been consumed.
  const toSignIn = () => navigate("/login", { replace: true });

  return (
    <div className="min-h-screen bg-canvas text-primary">
      <header className="flex items-center justify-between px-6 py-4">
        <span className="text-lg font-semibold">mini-erp</span>
        <ThemeToggle />
      </header>

      <main className="mx-auto flex max-w-sm flex-col px-6 py-16">
        <h1 className="text-xl font-semibold">
          {status === "done" ? "Password updated" : "Choose a new password"}
        </h1>

        {status === "verifying" && (
          <p role="status" className="mt-2 text-sm text-secondary">
            Checking your link…
          </p>
        )}

        {status === "ready" && accountEmail && (
          <p className="mt-1 text-sm text-secondary">
            For <span className="font-medium text-primary">{accountEmail}</span>.
          </p>
        )}

        {status === "done" && (
          <p className="mt-1 text-sm text-secondary">
            You can sign in with your new password now.
          </p>
        )}

        {banner && (
          <p
            role="alert"
            className="mt-6 rounded-md border border-hairline bg-surface px-4 py-3 text-sm text-danger"
          >
            {banner}
          </p>
        )}

        {status === "ready" && (
          <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4">
            <PasswordField
              label="New password"
              name="new-password"
              autoComplete="new-password"
              value={password}
              onChange={setPassword}
              disabled={busy}
            />
            <PasswordField
              label="Confirm new password"
              name="confirm-password"
              autoComplete="new-password"
              value={confirm}
              onChange={setConfirm}
              disabled={busy}
            />
            <button
              type="submit"
              disabled={busy}
              className="min-h-11 rounded-md bg-accent px-4 text-sm font-medium text-canvas disabled:opacity-60"
            >
              {busy ? "Updating…" : "Set new password"}
            </button>
          </form>
        )}

        {status !== "verifying" && (
          <button
            type="button"
            onClick={toSignIn}
            className="mt-6 self-start text-sm text-accent underline-offset-4 hover:underline"
          >
            {status === "done" ? "Go to sign in" : "Back to sign in"}
          </button>
        )}
      </main>
    </div>
  );
}
