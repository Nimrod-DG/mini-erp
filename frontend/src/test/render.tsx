import { render, screen, type RenderResult } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";

import App from "../App";
import { ToastProvider } from "../components/Toasts";
import { ThemeProvider } from "../hooks/useTheme";
import type { Me } from "../lib/api";
import { setFirebaseUser } from "./firebaseFake";
import { setMe } from "./server";

/**
 * Render the whole application at a path, signed in as somebody.
 *
 * WHY THE WHOLE APP AND NOT THE SCREEN. Most of what §12.5 asks about is not
 * inside a screen: FE1 and FE8 are about navigation derived from `/api/me`, FE2 is
 * about a control the route guard has already decided you can reach, FE6 is a
 * redirect. Rendering `<App/>` means the real router, the real `AuthProvider`, the
 * real guards and the real `/api/me` round trip are all in the picture, so a test
 * cannot pass because the harness assembled the providers more kindly than
 * `main.tsx` does.
 *
 * The path is pushed onto jsdom's history rather than passed to a `MemoryRouter`,
 * because `App` owns its `BrowserRouter` — and that is worth keeping: a harness
 * that had to be handed the router would be a harness testing a different tree
 * from the one that ships.
 *
 * Awaits the shell before returning. Without that, every test would start with a
 * `Loading…` screen and its first query would be a race.
 */
export async function renderApp(path: string, me: Me): Promise<RenderResult> {
  setMe(me);
  window.history.pushState({}, "", path);
  setFirebaseUser(me.user.email);

  const result = render(<App />);
  // The one element on every signed-in screen, and absent while ProtectedRoute
  // is waiting for Firebase and /api/me.
  await screen.findByRole("link", { name: "mini-erp" });
  return result;
}

/** Render the app signed out — the login screen, and the redirect onto it. */
export function renderAppSignedOut(path: string): RenderResult {
  window.history.pushState({}, "", path);
  setFirebaseUser(null);
  return render(<App />);
}

/**
 * Render one component with the providers it needs and nothing else.
 *
 * For the pieces whose behaviour is entirely their own — a status chip, a
 * skeleton, a theme toggle. A screen goes through `renderApp`, because a screen's
 * behaviour includes which identity reached it.
 */
export function renderWithProviders(
  ui: ReactNode,
  { route = "/" }: { route?: string } = {},
): RenderResult {
  return render(
    <ThemeProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>
      </ToastProvider>
    </ThemeProvider>,
  );
}
