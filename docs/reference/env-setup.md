# Reference — Environment variables and secrets

> Phase 0. Consolidates §2.3.1 (database URLs) and §3.5.2 (Firebase), which the
> original document specified in two separate places.
>
> Part of the mini-erp build docs. Index: [`../README.md`](../README.md)

---

## Where the files go

```
mini-erp/
├── .gitignore                          # must contain secrets/ and *.local BEFORE anything below exists
├── backend/
│   ├── .env                            # NEVER COMMIT
│   ├── .env.example                     # commit this — same keys, dummy values
│   └── secrets/
│       └── <project-id>-service-account.json   # NEVER COMMIT
└── frontend/
    ├── .env.local                      # public identifiers; gitignored by Vite's default *.local rule
    └── .env.example                     # commit this
```

Two files, two audiences. The backend `.env` holds **real credentials**. The
frontend `.env.local` holds **public identifiers** that ship inside the JS bundle
and are readable by any user — that is expected and safe (§2.4.4).

---

## `backend/.env`

```bash
# --- database (local Docker, Section 2.3.1) -----------------------------------
# Three roles, three URLs. The app never connects as the owner.
DATABASE_URL=postgres://erp_app:localdev@localhost:5432/erp?sslmode=disable
ADMIN_DATABASE_URL=postgres://erp_admin:localdev@localhost:5432/erp?sslmode=disable
MIGRATE_DATABASE_URL=postgres://erp_migrate:localdev@localhost:5432/erp?sslmode=disable

# --- firebase (Section 3.5.2) -------------------------------------------------
# The REAL project ID from the console URL, not the friendly display name
# ("erp project" is the display name; erp-project-b66ce is the ID).
FIREBASE_PROJECT_ID=erp-project-b66ce
GOOGLE_APPLICATION_CREDENTIALS=./secrets/erp-project-b66ce-service-account.json

# --- server -------------------------------------------------------------------
PORT=8080
CORS_ORIGINS=http://localhost:5173
```

`GOOGLE_APPLICATION_CREDENTIALS` is a **path**, not the key itself. The Firebase
Admin SDK reads that env var automatically — you do not pass it in code.

At Phase 9 only the *values* change: `sslmode=require`, `MIGRATE_DATABASE_URL`
pointed at an unpooled endpoint, and the credentials moved to Secret Manager
(§2.4.1). The keys stay identical.

---

## `frontend/.env.local`

Only variables prefixed `VITE_` are exposed to the browser by Vite. Anything
without that prefix is silently dropped at build time — a common half-hour of
confusion.

**This project's actual dev values — copy verbatim:**

```bash
VITE_FIREBASE_API_KEY=AIzaSyDixUw_ch3oJeBaHOiZTbMEmWrwoaq3TmM
VITE_FIREBASE_AUTH_DOMAIN=erp-project-b66ce.firebaseapp.com
VITE_FIREBASE_PROJECT_ID=erp-project-b66ce
VITE_FIREBASE_APP_ID=1:889259985673:web:1bd9b28a7769be3142e1d8
VITE_API_BASE_URL=http://localhost:8080
```

**Four keys from the console snippet are deliberately omitted:**

| Omitted | Why |
|---|---|
| `storageBucket` | Cloud Storage is not used — no file uploads (§1.3) |
| `messagingSenderId` | FCM is not used — no push, no notifications (§1.3) |
| `measurementId` | Google Analytics is not used — see below |
| `databaseURL` | Realtime Database is not used; Postgres is the database |

Carrying config for products you do not use is how a "why is this here?" question
lands in an interview with no good answer.

---

## `frontend/src/lib/firebase.ts`

Install first, from inside `frontend/`: `npm install firebase`

The console gives you a snippet with the values **hardcoded**. Do not use it as
written — read from `import.meta.env` instead, so Phase 9 switches to the prod
project by changing an env file rather than editing code:

```ts
import { initializeApp } from "firebase/app";
import { getAuth } from "firebase/auth";

const firebaseConfig = {
  apiKey:     import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId:  import.meta.env.VITE_FIREBASE_PROJECT_ID,
  appId:      import.meta.env.VITE_FIREBASE_APP_ID,
};

export const app  = initializeApp(firebaseConfig);
export const auth = getAuth(app);
```

**Drop `getAnalytics` from the console snippet.** Analytics is not in scope, adds
a dependency and a cookie-consent question, and breaks in test environments. Only
`firebase/app` and `firebase/auth` are imported anywhere in this project.

Type the env vars so a typo is a compile error rather than `undefined` at runtime
— `frontend/src/vite-env.d.ts`:

```ts
/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_FIREBASE_API_KEY: string;
  readonly VITE_FIREBASE_AUTH_DOMAIN: string;
  readonly VITE_FIREBASE_PROJECT_ID: string;
  readonly VITE_FIREBASE_APP_ID: string;
  readonly VITE_API_BASE_URL: string;
}
interface ImportMeta { readonly env: ImportMetaEnv }
```

---

## Getting the values from the console

**1. Web app config** → Project Overview → **Add app** → Web (`</>`) → nickname
`mini-erp-web`. **Do not tick "Also set up Firebase Hosting"** — that is Phase 9,
and doing it now scaffolds files you will fight with. Copy the `firebaseConfig`
object into `.env.local` as above.

**2. Service account key** → gear icon → Project settings → **Service accounts**
→ **Generate new private key**. Downloads a JSON file. Move it to
`backend/secrets/` and rename it to match `GOOGLE_APPLICATION_CREDENTIALS`.

**3. Enable the provider** → use the sidebar's **Search for products** box, type
*Authentication* → Get started → **Sign-in method** → **Email/Password** →
Enable. Leave *Email link (passwordless)* **off**, and leave Google OAuth off —
neither is in scope (§1.3), and OAuth adds a consent-screen step.

Without step 3, `signInWithEmailAndPassword` fails with
`auth/operation-not-allowed` and the error does not say why.

---

## The service account key is a real credential

It grants full admin control over authentication, **including minting a valid
token for any user in the project**. Treat it like a root password:

- `secrets/` and `*-service-account.json` go in `.gitignore` **before** the file
  exists on disk. If it is ever committed, rotating the key is the only fix —
  deleting the file from HEAD does not remove it from history.
- Never paste it into a chat, an issue, a screenshot, or a prompt.
- It never goes in the frontend, in the container image, or in a constant.

The `VITE_FIREBASE_*` values are the opposite — public by design, safe in the
bundle, safe in a screenshot. A Firebase Web API key identifies the project; it
does not authorise anything on its own. What protects this project is that
**self-signup is disabled and there is no public registration endpoint** (§3.3),
so possessing the key gets an attacker nothing but a login form they still need
credentials for.

*Phase 9 hardening, not now:* restrict the key to your Hosting origin under
Google Cloud Console → APIs & Services → Credentials → HTTP referrers. That
limits abuse of the auth endpoints from other origins.

---

## Two projects, not one (§3.5.1)

Firebase has no environment separation *within* a project: one project means one
user pool. Without a split, the seed script's `password123` accounts (§15) share
a pool with anything you later demo to an employer, and a reseed that deletes
users could delete real ones.

Project IDs are globally unique and **immutable**, so `erp-dev` and `erp-prod`
are usually taken and you will get something like `erp-project-b66ce`. That is
fine — the docs refer to them by role, not by literal ID. Record the real ones:

| Role in the docs | Actual project ID | Used by |
|---|---|---|
| `erp-dev` | **`erp-project-b66ce`** (display name "erp project") | Local development, Phases 0–8. Seeded demo users, freely resettable. |
| `erp-prod` | *(create at Phase 9)* | Deployment only. No seed accounts, ever. |

Wherever these docs say `erp-dev`, read `erp-project-b66ce`.

**A Hosting site is already linked to the web app** (`erp-project-b66ce`), created
automatically when the app was registered. Harmless now — it serves nothing until
you deploy — and it saves a step at Phase 9. No action either way.

---

## `.env.example` — commit these

Same keys, obviously-fake values. This is how the next person (or the next
session) knows what is required without you handing over secrets.

```bash
# backend/.env.example
DATABASE_URL=postgres://erp_app:localdev@localhost:5432/erp?sslmode=disable
ADMIN_DATABASE_URL=postgres://erp_admin:localdev@localhost:5432/erp?sslmode=disable
MIGRATE_DATABASE_URL=postgres://erp_migrate:localdev@localhost:5432/erp?sslmode=disable
FIREBASE_PROJECT_ID=your-project-id
GOOGLE_APPLICATION_CREDENTIALS=./secrets/your-project-id-service-account.json
PORT=8080
CORS_ORIGINS=http://localhost:5173
```

```bash
# frontend/.env.example
VITE_FIREBASE_API_KEY=your-api-key
VITE_FIREBASE_AUTH_DOMAIN=your-project-id.firebaseapp.com
VITE_FIREBASE_PROJECT_ID=your-project-id
VITE_FIREBASE_APP_ID=your-app-id
VITE_API_BASE_URL=http://localhost:8080
```

---

## Verify before moving on

```bash
git check-ignore -v backend/secrets/ backend/.env frontend/.env.local
# every line must print a matching .gitignore rule -- silence means it is tracked

git status --porcelain | grep -Ei 'service-account|\.env$'
# must print nothing
```
