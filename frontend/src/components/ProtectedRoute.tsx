import { Navigate, useLocation } from "react-router-dom";
import type { ReactNode } from "react";

import { useAuth } from "../hooks/useAuth";
import { Suspended } from "../pages/Suspended";

/**
 * The route guard. It is a rendering decision and nothing more: every screen it
 * hides is independently enforced by the backend, which resolves authorization
 * from the database on every request (I9, I12). Deleting this component would
 * make the app ugly, not insecure.
 */
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const location = useLocation();

  switch (auth.status) {
    case "loading":
      // Firebase restores a session asynchronously on every page load. Without
      // this state, a reload bounces a signed-in user to the login screen for a
      // frame — the single most common bug in this pattern.
      return (
        <div
          role="status"
          aria-live="polite"
          className="grid min-h-screen place-items-center bg-canvas text-sm text-secondary"
        >
          Loading…
        </div>
      );

    case "suspended":
      // Authenticated, and deliberately not let in. Sending them to the login
      // form instead would suggest their password was wrong.
      return <Suspended />;

    case "signedOut":
      return <Navigate to="/login" replace state={{ from: location.pathname }} />;

    case "signedIn":
      return <>{children}</>;
  }
}
