import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

export type ThemePreference = "light" | "dark" | "system";

const STORAGE_KEY = "theme";

function prefersDark() {
  return matchMedia("(prefers-color-scheme: dark)").matches;
}

function resolve(preference: ThemePreference) {
  return preference === "dark" || (preference === "system" && prefersDark());
}

function apply(preference: ThemePreference) {
  document.documentElement.classList.toggle("dark", resolve(preference));
}

function stored(): ThemePreference {
  const value = localStorage.getItem(STORAGE_KEY);
  return value === "light" || value === "dark" ? value : "system";
}

type ThemeContextValue = {
  preference: ThemePreference;
  isDark: boolean;
  setPreference: (next: ThemePreference) => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  // The class is already correct — index.html set it before first paint. This
  // only mirrors the same decision into React state.
  const [preference, setPreferenceState] = useState<ThemePreference>(stored);
  const [isDark, setIsDark] = useState(() => resolve(stored()));

  const setPreference = useCallback((next: ThemePreference) => {
    localStorage.setItem(STORAGE_KEY, next);
    apply(next);
    setPreferenceState(next);
    setIsDark(resolve(next));
  }, []);

  // On "system", follow the OS without requiring a reload.
  useEffect(() => {
    if (preference !== "system") return;
    const query = matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      apply("system");
      setIsDark(resolve("system"));
    };
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, [preference]);

  const value = useMemo(
    () => ({ preference, isDark, setPreference }),
    [preference, isDark, setPreference],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used inside <ThemeProvider>");
  return ctx;
}
