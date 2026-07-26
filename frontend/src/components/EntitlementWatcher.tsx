import { useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import { useAuth } from "../hooks/useAuth";
import { onEntitlementRevoked } from "../lib/api";
import { useToast } from "./Toasts";

/**
 * What happens when a module is taken away while somebody is inside it (FE6).
 *
 * Renders nothing. It sits once, at the top of the router, and turns every
 * `403 module_not_enabled` anywhere in the application into the three things that
 * refusal actually calls for:
 *
 *   1. **Say what happened**, in the server's own sentence — "This workspace does
 *      not have the finance module enabled." A silent redirect looks like the app
 *      losing its place.
 *   2. **Re-read `/api/me`**, because the sidebar, the bottom tabs and every
 *      cosmetic guard are built from it and all of them are now wrong. Without
 *      this the reader is bounced to the dashboard past a Finance link that is
 *      still there, and clicking it bounces them again.
 *   3. **Leave**, to the dashboard. There is nothing on the screen they are on
 *      that they are still allowed to read.
 *
 * This is the one place the *cosmetic* half of I12 gets a correction rather than a
 * refusal. Nothing here grants anything: the server had already said no, and this
 * only stops the browser from continuing to claim otherwise.
 *
 * `insufficient_module_role` is deliberately not handled — see `onEntitlementRevoked`.
 */
export function EntitlementWatcher() {
  const navigate = useNavigate();
  const location = useLocation();
  const { refreshMe } = useAuth();
  const toast = useToast();

  /**
   * One event, one message.
   *
   * A screen makes more than one request — `/finance` reads its entries and its
   * accounts in parallel — and on a revocation every one of them comes back 403.
   * Untracked, that is the same sentence stacked as many times as the screen
   * happened to be curious, for a single thing that happened once.
   *
   * Cleared on navigating anywhere other than the dashboard we redirect to, which
   * is both races-free and the only moment a *second* revocation could be news:
   * the requests still in flight from the screen we just left cannot clear it,
   * because they resolve while the path is "/".
   */
  const announced = useRef<string | null>(null);

  useEffect(() => {
    if (location.pathname !== "/") announced.current = null;
  }, [location.pathname]);

  useEffect(
    () =>
      onEntitlementRevoked((error) => {
        if (announced.current !== error.message) {
          announced.current = error.message;
          toast.failure(error);
        }
        void refreshMe();
        // `replace`, so Back does not return to a screen that will refuse again.
        navigate("/", { replace: true });
      }),
    [navigate, refreshMe, toast],
  );

  return null;
}
