/**
 * FE10 and FE11 — the three-state theme toggle of §10.8.3.
 *
 * A NOTE ON FE10's WORDING. §12.5 writes it as "the toggle *cycles* light → dark
 * → system", which describes a single button you press repeatedly. What §10.8.3
 * actually specifies is "offer light / dark / system, defaulting to system", and
 * what is built is a `radiogroup` of three — every state reachable in one press,
 * and the current one legible without pressing anything. The substance of FE10 is
 * the three states, the applied class, and the persistence, and all three are
 * asserted below. The deviation is recorded in `PROGRESS.md`.
 */

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ThemeToggle } from "../components/ThemeToggle";
import { ThemeProvider } from "../hooks/useTheme";
import { rina } from "./fixtures";
import { setPrefersDark } from "./media";
import { renderApp } from "./render";

function renderToggle() {
  return render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  );
}

const isDark = () => document.documentElement.classList.contains("dark");
const stored = () => localStorage.getItem("theme");

function option(name: "Light" | "Dark" | "System") {
  return screen.getByRole("radio", { name });
}

describe("FE10 — the toggle offers three states and persists the choice", () => {
  it("offers exactly light, dark and system", async () => {
    renderToggle();

    const group = screen.getByRole("radiogroup", { name: "Colour theme" });
    expect(group).toBeInTheDocument();
    expect(screen.getAllByRole("radio").map((radio) => radio.textContent)).toEqual([
      "Light",
      "Dark",
      "System",
    ]);
  });

  it("defaults to system when nothing is stored", async () => {
    renderToggle();

    expect(option("System")).toHaveAttribute("aria-checked", "true");
    expect(option("Dark")).toHaveAttribute("aria-checked", "false");
    // Nothing is written until a choice is made: an untouched install must follow
    // the OS, and storing "system" eagerly is indistinguishable from a choice.
    expect(stored()).toBeNull();
  });

  it("walks light → dark → system, applying and persisting each", async () => {
    setPrefersDark(false);
    renderToggle();

    await userEvent.click(option("Light"));
    expect(stored()).toBe("light");
    expect(isDark()).toBe(false);
    expect(option("Light")).toHaveAttribute("aria-checked", "true");

    await userEvent.click(option("Dark"));
    expect(stored()).toBe("dark");
    expect(isDark()).toBe(true);
    expect(option("Dark")).toHaveAttribute("aria-checked", "true");
    expect(option("Light")).toHaveAttribute("aria-checked", "false");

    await userEvent.click(option("System"));
    expect(stored()).toBe("system");
    // The OS says light, so system resolves light — the point of the third state
    // is that it is not a fixed value.
    expect(isDark()).toBe(false);
    expect(option("System")).toHaveAttribute("aria-checked", "true");
  });

  it("resolves `system` against the OS rather than to a fixed value", async () => {
    setPrefersDark(true);
    renderToggle();

    await userEvent.click(option("System"));
    expect(stored()).toBe("system");
    expect(isDark()).toBe(true);
  });

  it("reads the stored choice back on the next load", async () => {
    localStorage.setItem("theme", "dark");
    renderToggle();

    expect(option("Dark")).toHaveAttribute("aria-checked", "true");
  });

  it("treats a junk stored value as system rather than crashing", async () => {
    // The blocking script in `index.html` reads the same key before first paint,
    // and neither it nor this may trust what is in localStorage.
    localStorage.setItem("theme", "midnight");
    setPrefersDark(true);
    renderToggle();

    expect(option("System")).toHaveAttribute("aria-checked", "true");
  });

  it("declares a 44px target on every option", async () => {
    // §10.7.5's floor. These three were `px-3 py-1` — about 26px — and one of the
    // four things the Phase 7 touch-target audit found. `min-h-10` inside a
    // `p-0.5` border is 44px overall, which is why the class alone reads as 40.
    renderToggle();

    for (const radio of screen.getAllByRole("radio")) {
      expect(radio.className).toContain("min-h-10");
      expect(radio.parentElement?.className).toContain("p-0.5");
    }
  });

  it("is reachable from every signed-in screen", async () => {
    // It lives in `AppShell`'s header, so it is not a screen's own affordance —
    // and §10.8.5's warning about verifying every surface in both themes only
    // works if the switch is always to hand.
    await renderApp("/", rina);
    expect(screen.getByRole("radiogroup", { name: "Colour theme" })).toBeInTheDocument();
  });
});

describe("FE11 — on `system`, a change to prefers-color-scheme applies without a reload", () => {
  it("flips the applied class when the OS goes dark", async () => {
    localStorage.setItem("theme", "system");
    setPrefersDark(false);
    renderToggle();
    expect(isDark()).toBe(false);

    act(() => setPrefersDark(true));

    // No reload, no remount: §10.8.3 asks for the listener precisely so the app
    // follows the OS as it changes.
    expect(isDark()).toBe(true);
    expect(option("System")).toHaveAttribute("aria-checked", "true");
    // The preference is untouched — following the OS is not choosing dark.
    expect(stored()).toBe("system");
  });

  it("flips back when the OS goes light again", async () => {
    // Both directions through the listener, starting from light.
    //
    // Not "start with the OS dark and assert the class is already there":
    // `ThemeProvider` deliberately does not apply the class on mount, because the
    // blocking script in `index.html` has already done it before first paint —
    // that script is the whole of §10.8.3's no-flash requirement, and a provider
    // that re-applied on mount would be a second implementation of it. So the
    // initial class is that script's claim to make, not this component's.
    localStorage.setItem("theme", "system");
    setPrefersDark(false);
    renderToggle();
    expect(isDark()).toBe(false);

    act(() => setPrefersDark(true));
    expect(isDark()).toBe(true);

    act(() => setPrefersDark(false));
    expect(isDark()).toBe(false);
  });

  it("ignores the OS once an explicit choice has been made", async () => {
    // The whole reason there are three states rather than two. Somebody who chose
    // Light wants light at 9pm as well.
    localStorage.setItem("theme", "light");
    setPrefersDark(false);
    renderToggle();

    act(() => setPrefersDark(true));

    expect(isDark()).toBe(false);
    expect(stored()).toBe("light");
  });

  it("starts following the OS again the moment system is chosen", async () => {
    setPrefersDark(false);
    renderToggle();
    await userEvent.click(option("Dark"));
    expect(isDark()).toBe(true);

    await userEvent.click(option("System"));
    expect(isDark()).toBe(false);

    // The listener is subscribed on becoming `system`, not at mount, so this is
    // the case where an effect keyed on the wrong thing would leave a stale
    // subscription behind.
    act(() => setPrefersDark(true));
    expect(isDark()).toBe(true);
  });

  it("stops listening once an explicit choice replaces system", async () => {
    localStorage.setItem("theme", "system");
    setPrefersDark(false);
    renderToggle();

    await userEvent.click(option("Light"));
    act(() => setPrefersDark(true));

    expect(isDark()).toBe(false);
  });
});
