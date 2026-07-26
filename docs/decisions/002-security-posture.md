# Decision 002 — Where the real security boundaries are

> Rationale. Read before writing the README or describing the project.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

### 2.4 Security posture — what actually protects this system

Worth stating explicitly, because stack choices are often credited with security properties they do not have.

#### 2.4.1 A compiled binary is not an encrypted one

**Go binaries are compiled, not encrypted.** Compilation is not a security control. Go is in fact comparatively easy to reverse-engineer among compiled languages, because the runtime needs type information and stack-trace metadata, so a stripped-looking binary still ships with function names, type descriptors, and plaintext string literals. `strings` recovers a great deal, and tooling exists specifically for recovering structure from Go binaries.

The backend is protected because **it runs on infrastructure the user does not control and its code is never sent to the client.** That is a property of the deployment, not of the language — equally true of Node, Python, or Java. Never describe the choice of Go as a form of encryption.

Corollary that does matter: **never embed a secret in the binary.** Credentials come from Secret Manager at runtime, never from a constant, and never from a committed `.env`.

#### 2.4.2 Why Go is nonetheless a defensible choice

| Property | Benefit |
|---|---|
| Memory safety | Bounds-checked slices, GC, no manual memory management — buffer overflows and use-after-free are not reachable |
| No runtime interpreter | A static binary in a distroless image has far less attack surface than an image carrying an interpreter and a package tree |
| Static typing | Status enums and role levels become compile errors instead of runtime bugs |
| `govulncheck` | Reports vulnerabilities the code actually reaches, rather than every CVE in the dependency graph |
| Race detector | `go test -race` in CI catches concurrency bugs — relevant to the document-numbering and last-admin locks |

#### 2.4.3 Where the real security boundaries are

| Boundary | Mechanism |
|---|---|
| Tenant isolation | **PostgreSQL RLS** (Section 4) — the primary boundary, enforced by the database |
| Identity | Firebase ID token verification via Admin SDK (Section 3) |
| Authorization | Entitlement + module role + record rules, all server-side (Section 5) |
| In transit | TLS terminated by Cloud Run and Firebase Hosting |
| At rest | Provider-managed encryption at rest (Neon and Cloud SQL both enable this by default) |
| Credentials | Secret Manager, injected at runtime |
| Audit | Append-only log with grant-level immutability (Section 6.7) |

If asked "is it encrypted?", the accurate answer is: **in transit via TLS, and at rest by the database provider** — not because of the language.

#### 2.4.4 The frontend is public

Everything shipped to the browser — React code, environment variables baked at build time, API URLs — is readable by any user. Two consequences that shape the design:

- **No secrets in frontend code, ever.** Firebase Web SDK config keys are identifiers, not secrets, and are safe to ship; a service-account key or database credential never is.
- **Hiding is not a permission.** Nav items hidden by entitlement (Section 10.1) are a convenience. Every one of them is independently enforced server-side (Section 8.5), and the acceptance test deliberately calls blocked endpoints directly rather than trusting that the button was absent.

---
