import { auth } from "./firebase";

const BASE = import.meta.env.VITE_API_BASE_URL;

/** Module codes and role levels are naming contracts — they appear identically
 *  in the migrations, the Go types, this file, and the tests. */
export type ModuleCode = "procurement" | "inventory" | "finance";
export type RoleLevel = "viewer" | "user" | "approver" | "admin";
export type TenantRole = "staff" | "admin" | "superadmin";

export type Me = {
  user: {
    id: string;
    email: string;
    fullName: string;
    tenantRole: TenantRole;
  };
  /** null for a superadmin, who belongs to no tenant. */
  tenant: {
    id: string;
    name: string;
    slug: string;
    status: string;
    /** Business dates render in this zone, never the browser's (FE15). */
    timezone: string;
  } | null;
  /** Only modules the tenant is entitled to. An absent module means `none` —
   *  the level is the absence of a row, so this never contains "none". */
  moduleRoles: Partial<Record<ModuleCode, RoleLevel>>;
};

/** ApiError carries the machine-readable code from the §9.8 envelope. Branch on
 *  `code`, never on `message` — the prose is written for people and gets
 *  reworded. */
export class ApiError extends Error {
  // Declared and assigned rather than written as constructor parameter
  // properties: `erasableSyntaxOnly` is on, and that syntax has to be compiled
  // away rather than erased.
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

type ErrorEnvelope = { error?: string; message?: string };

/**
 * The single way this app talks to the backend. Every call carries the Firebase
 * ID token.
 *
 * `getIdToken()` is the whole of token lifecycle management: it returns the
 * cached token and silently refreshes when it is close to expiry. Storing one
 * ourselves would mean reimplementing that badly, and putting it in
 * localStorage would make it stealable by any injected script.
 */
export async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const user = auth.currentUser;
  if (!user) {
    throw new ApiError(401, "unauthenticated", "Not signed in.");
  }
  const token = await user.getIdToken();

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  if (init.body !== undefined) headers.set("Content-Type", "application/json");

  const response = await fetch(`${BASE}${path}`, { ...init, headers });

  if (response.status === 204) return undefined as T;

  const body: unknown = await response.json().catch(() => null);

  if (!response.ok) {
    const envelope = (body ?? {}) as ErrorEnvelope;
    throw new ApiError(
      response.status,
      envelope.error ?? "unknown",
      envelope.message ?? response.statusText,
    );
  }
  return body as T;
}

export function getMe(): Promise<Me> {
  return apiFetch<Me>("/api/me");
}
