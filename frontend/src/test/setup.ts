import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, vi } from "vitest";

import { resetFirebaseFake } from "./firebaseFake";
import { installMatchMedia, resetMedia } from "./media";
import { server } from "./server";

/**
 * Both Firebase entry points are replaced wholesale. The factories are hoisted
 * above every import in this file, so they cannot close over anything declared
 * beside them — hence the dynamic import, and hence `firebaseFake.ts` being a
 * module of its own rather than a few consts up here.
 */
vi.mock("firebase/app", async () => (await import("./firebaseFake")).appModule);
vi.mock("firebase/auth", async () => (await import("./firebaseFake")).authModule);

beforeAll(() => {
  // "error", not "warn": an endpoint no handler answers is a test asserting
  // against a request that never resolved, and the symptom is a timeout five
  // seconds later with nothing naming the URL.
  server.listen({ onUnhandledRequest: "error" });
});

afterEach(() => {
  cleanup();
  server.resetHandlers();
  resetFirebaseFake();
  resetMedia();
  installMatchMedia();
  localStorage.clear();
  // The theme is applied to <html>, which RTL's cleanup does not touch.
  document.documentElement.classList.remove("dark");
  // Every test that renders the app pushes its own path; leaving the last one
  // behind would make a test's result depend on the one before it.
  window.history.pushState({}, "", "/");
});

afterAll(() => server.close());

// jsdom has no matchMedia at all, and two hooks call it during their first
// render — so it has to exist before the first test, not just between them.
installMatchMedia();
