import { useEffect, useState } from "react";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { ThemeToggle } from "./components/ThemeToggle";
import { ThemeProvider } from "./hooks/useTheme";

type HealthState = "checking" | "ok" | "unreachable";

// Written out rather than built from a template string: Tailwind scans source
// text, so a class it cannot see literally is a class it does not generate.
const SWATCHES = [
  { role: "accent", className: "text-accent" },
  { role: "success", className: "text-success" },
  { role: "warning", className: "text-warning" },
  { role: "danger", className: "text-danger" },
];

/** Phase 0 sample page. It exists to prove three things work: the token/theme
 *  wiring, the numeric face, and the API reaching the browser. Real screens
 *  arrive from Phase 4 onward. */
function SamplePage() {
  const [health, setHealth] = useState<HealthState>("checking");

  useEffect(() => {
    const controller = new AbortController();
    fetch(`${import.meta.env.VITE_API_BASE_URL}/api/health`, {
      signal: controller.signal,
    })
      .then((r) => setHealth(r.ok ? "ok" : "unreachable"))
      .catch(() => {
        if (!controller.signal.aborted) setHealth("unreachable");
      });
    return () => controller.abort();
  }, []);

  return (
    <div className="min-h-screen bg-canvas text-primary">
      <header className="flex items-center justify-between border-b border-hairline px-6 py-4">
        <h1 className="text-lg font-semibold">mini-erp</h1>
        <ThemeToggle />
      </header>

      <main className="mx-auto max-w-3xl px-6 py-10">
        <section className="rounded-lg border border-hairline bg-surface p-6">
          <h2 className="text-base font-semibold">Phase 0 — foundations</h2>
          <p className="mt-1 text-sm text-secondary">
            Database, API, and frontend, with the theme wired before the first
            component exists.
          </p>

          <dl className="mt-6 grid grid-cols-[auto_1fr] gap-x-8 gap-y-2 text-sm">
            <dt className="text-secondary">API health</dt>
            <dd className="tabular">
              {health === "checking" && "checking…"}
              {health === "ok" && <span className="text-success">● 200 ok</span>}
              {health === "unreachable" && (
                <span className="text-danger">● unreachable</span>
              )}
            </dd>

            <dt className="text-secondary">Document number</dt>
            <dd className="tabular">PR-202607-0001</dd>

            <dt className="text-secondary">Quantity</dt>
            <dd className="tabular">1,250.0000</dd>

            <dt className="text-secondary">Amount</dt>
            <dd className="tabular">18,430.75</dd>
          </dl>
        </section>

        <section className="mt-6 flex flex-wrap gap-2">
          {SWATCHES.map(({ role, className }) => (
            <span
              key={role}
              className={`rounded border border-hairline px-3 py-1 text-xs ${className}`}
            >
              {role}
            </span>
          ))}
        </section>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<SamplePage />} />
        </Routes>
      </BrowserRouter>
    </ThemeProvider>
  );
}
