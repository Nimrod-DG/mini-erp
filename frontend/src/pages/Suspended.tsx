import { ThemeToggle } from "../components/ThemeToggle";
import { useAuth } from "../hooks/useAuth";

/** 403 tenant_suspended. The user is authenticated; their workspace is not
 *  open. Saying so plainly is the point — an empty application would send them
 *  to support with the wrong question. */
export function Suspended() {
  const { signOut } = useAuth();

  return (
    <div className="min-h-screen bg-canvas text-primary">
      <header className="flex items-center justify-between px-6 py-4">
        <span className="text-lg font-semibold">mini-erp</span>
        <ThemeToggle />
      </header>

      <main className="mx-auto max-w-md px-6 py-16">
        <h1 className="text-xl font-semibold">This workspace is suspended</h1>
        <p className="mt-2 text-sm text-secondary">
          Your sign-in worked. Access is paused for your organisation — contact
          your administrator to have it restored.
        </p>
        <button
          type="button"
          onClick={() => void signOut()}
          className="mt-6 min-h-11 rounded-md border border-hairline px-4 text-sm font-medium"
        >
          Sign out
        </button>
      </main>
    </div>
  );
}
