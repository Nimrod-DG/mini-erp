# Deploying mini-erp

The Phase 9 runbook. Follow it top to bottom the first time; after that, only
[§11 Redeploying](#11-redeploying) matters.

Everything here has been chosen for **this** deployment, which is unusual in one
way worth stating up front: **the frontend and the backend live under two
different Google accounts.** Firebase Hosting and Firebase Authentication stay in
the existing `erp-project-b66ce` project; Cloud Run and its secrets go in a
Google Cloud project owned by a different account. That works, and §5 is the one
place it costs anything.

---

## 0. The shape

```
   browser
     │
     │  1. static assets
     ▼
   Firebase Hosting  ──────────────  erp-project-b66ce   (account A)
   erp-project-b66ce.web.app         also: Firebase Authentication
     │
     │  2. fetch() with a Firebase ID token
     ▼
   Cloud Run  ─────────────────────  banded-torus-476311-q1  (account B)
   mini-erp-api, asia-southeast1     also: Secret Manager, Artifact Registry
     │
     │  3. SQL over TLS
     ▼
   Neon Postgres 17  ──────────────  ap-southeast-1 (AWS Singapore)
   database `erp`
```

> **Where the backend actually went.** §6 describes Cloud Run and is still
> correct, but the GCP billing account for this project turned out to be closed,
> so the backend is on **Render's free tier** instead — [§6B](#6b-render-instead-of-cloud-run).
> Everything else on this page is unchanged: same Neon database, same Firebase
> Hosting, same environment variables, same container.

| Piece | Where | Why there |
|---|---|---|
| Frontend | Firebase Hosting, `erp-project-b66ce` | The site already exists — it was created automatically when the web app was registered, and has served nothing since |
| Backend | **Render** (free, Singapore) — or Cloud Run, `banded-torus-476311-q1`, `asia-southeast1` | Both take the same container. Region is Singapore either way, because Neon is in Singapore |
| Database | Neon, `ap-southeast-1` | Free tier that persists, and §2.3.3 already accounts for its quirks |
| Auth | Firebase Auth, `erp-project-b66ce` | The eight seeded demo accounts already live there |
| Secrets | Secret Manager, `banded-torus-476311-q1` | Never in the image, never in the repo |

**Regions must match.** Every request in this design makes several round trips —
identity resolution, then the tenant transaction — so cross-region latency
multiplies rather than adds. Neon in `ap-southeast-1` (AWS Singapore) pairs with
Cloud Run in `asia-southeast1` (Google Singapore).

### The order is forced

1. **Database** — because the API cannot boot without it.
2. **Backend** — because `VITE_API_BASE_URL` is compiled *into* the frontend
   bundle and cannot be edited afterwards.
3. **Frontend** — last, once the Cloud Run URL is known.

A frontend built before the backend exists points at `localhost:8080`, and every
request fails in a way that looks like a CORS problem.

---

## 1. Accounts, projects, and the rename

### 1.1 Two accounts, two CLIs

`gcloud` and `firebase` authenticate separately, which is convenient here — they
are meant to be different people.

```bash
gcloud auth login                 # the account that owns banded-torus-476311-q1
gcloud config set project banded-torus-476311-q1
gcloud auth list                  # confirm the active account

firebase login                    # the account that owns erp-project-b66ce
firebase projects:list            # erp-project-b66ce must appear
```

If `firebase login` is already signed in as the wrong account:
`firebase logout` then `firebase login`.

### 1.2 Renaming "My First Project"

**The display name can change; the project ID cannot.** `banded-torus-476311-q1`
is permanent — it was assigned at creation and no console setting will alter it.
Renaming changes only the friendly label in the picker.

```bash
gcloud projects update banded-torus-476311-q1 --name="mini-erp"
```

Or: Console → **IAM & Admin → Settings** → *Project name* → Edit → Save.

**This matters less than it looks.** The project ID appears in Artifact Registry
paths and `gcloud` commands, and nowhere a visitor will ever see. The public URL
is built from the *service* name:

```
https://mini-erp-api-<project-number>.asia-southeast1.run.app
```

So `mini-erp-api` — chosen in §6 — is the name that shows up, not
`banded-torus`. If you want a tidy project ID anyway, the only way is a new
project (`gcloud projects create mini-erp-<something>`), and it is worth doing
*now* rather than after the secrets exist.

### 1.3 Billing and APIs

Cloud Run, Cloud Build, Artifact Registry, and Secret Manager all require a
billing account on the project. **The expected bill for this deployment is
zero** — Cloud Run's free tier is 2M requests and 180k vCPU-seconds a month,
Cloud Build gives 120 build-minutes a day, and Artifact Registry's first 0.5 GB
is free. A demo that nobody is hammering stays inside all three. Set a budget
alert anyway: Console → **Billing → Budgets & alerts**.

```bash
gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  secretmanager.googleapis.com
```

---

## 2. Neon: create the project

Console → **New project**.

| Setting | Value | Why |
|---|---|---|
| Postgres version | **17** | `security_invoker` views need 15+ (I4). Start current |
| Region | **AWS ap-southeast-1** (Singapore) | Matches Cloud Run's `asia-southeast1` |
| Database name | leave the default `neondb` | §3 creates `erp` alongside it |

Then take **both** connection strings from the dashboard's *Connect* dialog and
keep them somewhere safe for the next two steps:

- **Direct** — the plain endpoint. Migrations, seeding and verification use this.
- **Pooled** — the same endpoint with `-pooler` in the hostname, exposed by the
  *Connection pooling* toggle. The Cloud Run service uses this.

**Copy the pooled one from the dashboard rather than editing the hostname by
hand.** Neon's host format has changed more than once.

> **Why the split.** `golang-migrate`-style migrations take session-level locks
> that do not survive transaction-mode pooling, so migrations must use the direct
> endpoint. The *application* can use the pooled one specifically because §4.3
> mandates `SET LOCAL` inside an explicit transaction (I2) — a session-scoped
> `SET` would leak tenant context between requests through a shared connection.
> This is the second reason `SET LOCAL` is non-negotiable.

---

## 3. Neon: create the roles and the database

This is the step with the sharpest edge in the whole deployment.

> **Create the roles with SQL, never through the Neon Console, API, or CLI.**
> A role created through Neon's own tooling is granted membership in
> `neon_superuser`, which carries `BYPASSRLS`. Every RLS policy in this schema
> would then stop applying — with nothing visibly wrong. Tenant isolation would
> be decorative, and the application would look perfectly healthy.

1. Copy `deploy/neon-bootstrap.sql` to `deploy/prod.filled.sql`
   (that name is gitignored) and replace the three password placeholders with
   generated passwords. Do not reuse the Neon owner's password.
2. Open the Neon **SQL Editor**, database `neondb`, and run the whole file as
   `neondb_owner`.
3. The final `SELECT` prints three rows. **`rolsuper` and `rolbypassrls` must
   both be `f` on all three.** If either is `t`, the role came from the console —
   `DROP ROLE` it and run the file again.

What it creates: `erp_migrate`, `erp_app`, `erp_admin`, each with its session
timezone pinned to UTC, and a database `erp` owned by `erp_migrate`.

The file is annotated with the two PostgreSQL rules that make it look strange
(`ALTER DATABASE … OWNER TO` needing `SET ROLE`, and grants issued after an
ownership transfer being silent no-ops). Read them if you have to debug it.

**This bootstrap was rehearsed, not reasoned about.** The whole sequence —
bootstrap, `migrate` twice, `seed`, `dbverify` — was run against a Postgres 17
container configured with a non-superuser owner, exactly Neon's shape. That is
why `000_roles.sql` is now written to skip the statements a managed host refuses
and to *assert* I3 rather than force it.

---

## 4. Migrate, seed, and verify — from your laptop

Create `backend/.env.production` (gitignored). **All three URLs use the DIRECT
endpoint**: these are short-lived local commands, and one of them must not be
pooled at all.

```bash
DATABASE_URL=postgresql://erp_app:<app-pw>@ep-xxxx.ap-southeast-1.aws.neon.tech/erp?sslmode=require
ADMIN_DATABASE_URL=postgresql://erp_admin:<admin-pw>@ep-xxxx.ap-southeast-1.aws.neon.tech/erp?sslmode=require
MIGRATE_DATABASE_URL=postgresql://erp_migrate:<migrate-pw>@ep-xxxx.ap-southeast-1.aws.neon.tech/erp?sslmode=require

FIREBASE_PROJECT_ID=erp-project-b66ce
GOOGLE_APPLICATION_CREDENTIALS=./secrets/erp-project-b66ce-firebase-adminsdk-fbsvc-47d25660f5.json
```

Note `sslmode=require` on all three, and the database name `erp` — not `neondb`.

Then, from `backend/`:

```bash
go run ./cmd/migrate  .env.production   # applies 001..006, then 000_roles.sql
go run ./cmd/seed     .env.production   # the §15 demo — optional, see below
go run ./cmd/dbverify .env.production   # the gate
```

The env-file argument overrides whatever is exported in the shell, so there is
no way to run these against your local database by accident.

`dbverify` must end in **`all 11 checks passed`**, with no `warn` line. It is
what Phase 9 means by "test A10 and test J1 run against the production
database", plus I4, plus RLS forced on all fourteen tenant tables, plus the
end-to-end proof that a query with no tenant context returns nothing:

```
ok    A10 erp_app/erp_admin not elevated           erp_admin, erp_app
ok    erp_migrate not elevated                     super=false bypassrls=false
ok    J1  erp_app session timezone                 UTC
ok    I4 views are security_invoker                stock_balances, po_line_status
ok    RLS enabled+forced on 14 tenant tables       14 tables
ok    no tenant context returns no rows            0 without tenant context, 10 with it
```

A `warn` on `erp_migrate not elevated` is expected locally and is a **finding**
here: it means the owner was not created by the bootstrap file.

### About seeding a deployed database

`docs/phases/phase-9-deployment.md` says "do not seed demo accounts into it",
which assumes a separate `erp-prod` Firebase project. **This deployment
deliberately does the opposite** — see the Decision recorded in `PROGRESS.md`.
The deployed app authenticates against the same `erp-project-b66ce` pool the
demo accounts already live in, so seeding is what makes the live URL walkable by
someone who has no credentials from you. The password is `password123` and the
data is fictional; treat the deployment as a demo, never as a place to put real
records.

`cmd/seed` is idempotent (UUIDv5 row IDs, deterministic Firebase UIDs), so
running it twice is safe.

---

## 5. The cross-account bridge: one service account key

This is the only place two accounts costs anything.

The API verifies Firebase ID tokens and provisions users, both of which need
Admin SDK credentials **for `erp-project-b66ce`**. Cloud Run's own service
account belongs to the other project and cannot be granted that; the answer is
the service account key you already have.

1. The key is at
   `backend/secrets/erp-project-b66ce-firebase-adminsdk-fbsvc-47d25660f5.json`.
   If it is missing: Firebase Console → gear → **Project settings → Service
   accounts → Generate new private key**.
2. Store it, plus the two pooled connection strings, in Secret Manager **in the
   Cloud Run project**:

```bash
gcloud secrets create erp-firebase-key --data-file=backend/secrets/erp-project-b66ce-firebase-adminsdk-fbsvc-47d25660f5.json

printf '%s' 'postgresql://erp_app:<app-pw>@ep-xxxx-pooler.ap-southeast-1.aws.neon.tech/erp?sslmode=require' \
  | gcloud secrets create erp-database-url --data-file=-

printf '%s' 'postgresql://erp_admin:<admin-pw>@ep-xxxx-pooler.ap-southeast-1.aws.neon.tech/erp?sslmode=require' \
  | gcloud secrets create erp-admin-database-url --data-file=-
```

Note **`-pooler`** in both — the reverse of §4.

3. Let the Cloud Run runtime service account read them:

```bash
PROJECT_NUMBER=$(gcloud projects describe banded-torus-476311-q1 --format='value(projectNumber)')
SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"

for s in erp-firebase-key erp-database-url erp-admin-database-url; do
  gcloud secrets add-iam-policy-binding "$s" \
    --member="serviceAccount:${SA}" --role=roles/secretmanager.secretAccessor
done
```

> **That key is a real credential.** It can mint a valid token for any user in
> `erp-project-b66ce`. It never goes in the image, in the repository, in a
> screenshot, or in a chat. `.gitignore` blocks it by three separate patterns.

---

## 6. Deploy the backend to Cloud Run

`deploy/cloudrun.env.yaml` holds the non-secret environment. Nothing in it is
confidential — it is a project ID, a file path, and two origins — so it is
committed.

```bash
gcloud run deploy mini-erp-api \
  --source backend \
  --region asia-southeast1 \
  --allow-unauthenticated \
  --min-instances 0 \
  --max-instances 3 \
  --cpu 1 --memory 512Mi \
  --env-vars-file deploy/cloudrun.env.yaml \
  --set-secrets "DATABASE_URL=erp-database-url:latest,ADMIN_DATABASE_URL=erp-admin-database-url:latest,/secrets/firebase/key.json=erp-firebase-key:latest"
```

`--source backend` builds `backend/Dockerfile` with Cloud Build and pushes the
image to Artifact Registry for you; there is no local `docker build` and no
registry to create by hand. The build takes two to three minutes the first time.

Four things about that command are load-bearing:

| Flag | Why |
|---|---|
| `--allow-unauthenticated` | Cloud Run's IAM is not this application's auth. Every route is behind `FirebaseAuth` + `ResolveIdentity`, and a browser cannot send a Google IAM token. Locking it here would lock out the app |
| `--env-vars-file` | `CORS_ORIGINS` contains a comma, which `--set-env-vars` parses as a separator. A YAML file sidesteps the escaping entirely |
| `/secrets/firebase/key.json=…` | Mounts the secret as a **file**, because `GOOGLE_APPLICATION_CREDENTIALS` is a path. The other two are mounted as environment variables |
| no `MIGRATE_DATABASE_URL` | Deliberate. The deployed service has no use for the schema owner's credential, and `config.Load` no longer demands it — the API cannot be made to hold a connection string that can drop its own tables |

`PORT` is set by Cloud Run and must not be passed; `TZ=UTC` is already in the
image (I7).

Read the URL off the last line of the output, or:

```bash
gcloud run services describe mini-erp-api --region asia-southeast1 --format='value(status.url)'
```

Check it before going further:

```bash
curl https://mini-erp-api-<n>.asia-southeast1.run.app/api/health   # {"status":"ok"}
curl -i https://mini-erp-api-<n>.asia-southeast1.run.app/api/me    # 401, not 500
```

A **401** on `/api/me` is the good outcome: it means the Admin SDK initialised
and rejected a missing token. A **500** means the key did not mount — check
`gcloud run services logs read mini-erp-api --region asia-southeast1`.

---

## 6B. Render instead of Cloud Run

**This is the path actually taken.** Cloud Run needs an open billing account;
this project's was a Free Trial that had closed (`gcloud billing accounts list`
→ `OPEN: False`), and `services enable` succeeds on a *linked but closed*
account while every real API call is refused with `BILLING_DISABLED`. Render's
free tier needs no card. Nothing in the Go code changed — the container reads
its whole configuration from the environment either way.

What differs from §5–§6:

| | Cloud Run | Render |
|---|---|---|
| Source | `--source backend`, uploaded from your laptop | **A GitHub push.** Render builds from the repo |
| Secrets | Secret Manager + `--set-secrets` | Dashboard env vars + a **Secret File** |
| Key mount path | `/secrets/firebase/key.json` | `/etc/secrets/firebase-key.json` |
| Idle behaviour | Scales to zero, ~1–2 s cold start | **Sleeps after 15 min**, ~30 s cold start |
| Cost | Free tier, card required | Free, no card |

### Steps

1. **Push.** Render deploys what is on the branch, so the working tree has to be
   committed and pushed first. This is the one place the "do I need GitHub?"
   question is answered *yes*.

2. **Render → New → Blueprint**, pointed at `Nimrod-DG/mini-erp`. It reads
   [`render.yaml`](../render.yaml), which fixes the region, the root directory,
   the health check and the three non-secret variables. Doing it through *New
   Web Service* instead means filling eight fields by hand, and `rootDir` or the
   region being wrong is a silent problem rather than an error.

3. Render prompts for the two `sync: false` variables. Paste the Neon **pooled**
   URLs — the same values as `PROD_DATABASE_URL` and `PROD_ADMIN_DATABASE_URL`
   in `deploy/prod.env`.

4. **Add the Firebase key as a Secret File.** Service → *Environment* → *Secret
   Files* → filename exactly **`firebase-key.json`**, contents = the whole of
   `backend/secrets/erp-project-b66ce-firebase-adminsdk-*.json`. Render mounts it
   at `/etc/secrets/firebase-key.json`, which is what
   `GOOGLE_APPLICATION_CREDENTIALS` already points at. It has to be a file, not
   a variable, because the Admin SDK reads a path.

5. Wait for the first build (a few minutes: it downloads the module graph,
   builds `api` and `migrate`, and ships a distroless image).

6. Check it, at `https://mini-erp-api.onrender.com` or whatever URL Render
   assigns:

```bash
curl https://<render-url>/api/health    # {"status":"ok"}
curl -i https://<render-url>/api/me     # 401, not 500
```

A **401** on `/api/me` is the good outcome — the Admin SDK initialised and
rejected a missing token. A **500** means the secret file did not mount: check
the filename is exactly `firebase-key.json`.

Then continue at §7, using the Render URL as `VITE_API_BASE_URL`.

### If you later fix the billing account

§5 and §6 still work unchanged, and `deploy/deploy-api.sh` is still correct.
Moving back is: deploy to Cloud Run, change `VITE_API_BASE_URL`, rebuild the
frontend. Neon does not care which one is calling it.

---

## 7. Build and deploy the frontend

Now that the URL exists:

```bash
cp frontend/.env.production.example frontend/.env.production
# edit VITE_API_BASE_URL to the Cloud Run URL — no trailing slash
```

```bash
cd frontend
npm run build
firebase deploy --only hosting
```

Or `make deploy-web`, which is those two commands.

`frontend/firebase.json` carries the one rule the whole SPA depends on:

```json
"rewrites": [{ "source": "**", "destination": "/index.html" }]
```

Every route in `App.tsx` is client-side. `/procurement/orders/<uuid>/receive` is
not a file, and the acceptance test pastes UUIDs into the address bar — without
that rewrite, every deep link and every reload is a 404. `nginx.conf` states the
same rule for the container image, which is **not** on this serving path; the
two have to agree and nothing checks that they do.

`index.html` is served `no-store` and `/assets/**` is `immutable`, for the
reason in `nginx.conf`: hashed asset names can be cached forever, and the one
filename that never changes must never be cached, or a stale copy points at
assets that no longer exist — a white screen only a hard refresh fixes.

Vite loads `.env.production` **on top of** `.env.local`, so the dev URL cannot
leak into a release. (Verified: a build with a probe value baked in the probe and
`localhost:8080` appeared nowhere in `dist/`.)

The site lands on both `https://erp-project-b66ce.web.app` and
`https://erp-project-b66ce.firebaseapp.com`. Both are in `CORS_ORIGINS`, and both
are pre-authorised in Firebase Auth.

---

## 8. Three settings in the Firebase console

Under account A, in `erp-project-b66ce`.

1. **Password reset action URL.** Authentication → **Templates** → Password
   reset → pencil → *Customize action URL* →
   `https://erp-project-b66ce.web.app/auth/action`.
   Until this is set the emailed link opens Firebase's own hosted page, and this
   application's `/auth/action` screen — built in Phase 2 — has never been
   reached by a real email.
2. **Authorized domains.** Authentication → Settings → Authorized domains.
   `web.app` and `firebaseapp.com` are there by default; confirm rather than add.
3. **Restrict the browser API key.** Google Cloud Console *for
   `erp-project-b66ce`* → APIs & Services → Credentials → the Browser key →
   **HTTP referrers** → `https://erp-project-b66ce.web.app/*` and
   `https://erp-project-b66ce.firebaseapp.com/*`.
   The key is public by design and authorises nothing on its own — self-signup
   is disabled and there is no public registration endpoint (§3.3) — but this
   limits abuse of the auth endpoints from other origins.

---

## 9. Verify the deployment

The gate is the full twenty-five-step [acceptance test](acceptance-test.md), run
against the deployed URLs rather than localhost. Sign in at
`https://erp-project-b66ce.web.app` as `rina@nusantara.test` / `password123`.

Before that, four things that fail fast:

| Check | Expected |
|---|---|
| `GET /api/health` on the Cloud Run URL | `{"status":"ok"}` |
| Sign in, then reload on a deep link like `/inventory/stock` | The page renders — not a 404. This is the rewrite |
| Open the browser console on any list screen | No CORS error. This is `CORS_ORIGINS` |
| `go run ./cmd/dbverify .env.production` | `all 11 checks passed`, no `warn` |

Then walk the acceptance test. The two people it needs are both seeded: Sari
raises the requisition, Budi approves it (C2 forbids approving your own, for
everybody).

**Expect the first request after an idle period to be slow.** Cloud Run scales to
zero and free-tier Neon suspends its compute after a few minutes; stacked, a demo
opened after lunch can take several seconds to render. Two cheap mitigations:
`--min-instances 1` while you are actively demoing (it costs money — put it back
afterwards), and hit the app once before showing it to anyone.

---

## 10. Environment reference

### Cloud Run service (`deploy/cloudrun.env.yaml` + `--set-secrets`)

| Variable | Source | Value |
|---|---|---|
| `DATABASE_URL` | secret `erp-database-url` | Neon **pooled**, role `erp_app` |
| `ADMIN_DATABASE_URL` | secret `erp-admin-database-url` | Neon **pooled**, role `erp_admin` |
| `GOOGLE_APPLICATION_CREDENTIALS` | env, file from secret `erp-firebase-key` | `/secrets/firebase/key.json` |
| `FIREBASE_PROJECT_ID` | env | `erp-project-b66ce` |
| `CORS_ORIGINS` | env | the two Hosting origins, comma-separated, no wildcard |
| `PORT` | Cloud Run | set automatically; never pass it |
| `TZ` | image | `UTC`, from the Dockerfile |
| `MIGRATE_DATABASE_URL` | — | **absent on purpose** |

### `backend/.env.production` (your laptop, gitignored)

The three **direct** Neon URLs, `FIREBASE_PROJECT_ID`, and
`GOOGLE_APPLICATION_CREDENTIALS` pointing at the local key file. Used only by
`migrate`, `seed` and `dbverify`.

### `frontend/.env.production` (gitignored)

The four public `VITE_FIREBASE_*` identifiers and `VITE_API_BASE_URL`. Compiled
into the bundle; nothing here is secret (§2.4.4).

---

## 11. Redeploying

```bash
make deploy-api     # gcloud run deploy --source, same flags carried forward
make deploy-web     # npm run build && firebase deploy --only hosting
```

Secrets, CORS origins and scaling settings persist on the service, so a redeploy
is only a new image. Two rules:

- **A schema change means running `migrate` before deploying the API**, from the
  laptop, against the direct endpoint. Migrations are never the API's job — a
  running service must not be able to change its own schema.
- **A backend URL change means rebuilding the frontend.** It is baked in.

---

## 12. Credential hygiene

| Credential | Rotate by |
|---|---|
| Neon `erp_app` / `erp_admin` / `erp_migrate` | `ALTER ROLE … PASSWORD …` in the SQL editor, then `gcloud secrets versions add`, then redeploy |
| Neon `neondb_owner` | Neon Console → Roles → Reset password. Nothing in the deployment uses it after §3 |
| Firebase service account key | Console → Service accounts → generate a new key, `gcloud secrets versions add erp-firebase-key`, redeploy, then delete the old key |

**Rotate the Neon owner password if the connection string was ever pasted
anywhere it should not have been** — a chat window, an issue, a screenshot. It is
free to do and it is the only credential the bootstrap needs.

---

## 13. Troubleshooting

| Symptom | Cause |
|---|---|
| `must be superuser to alter superuser roles` during migrate | An older `000_roles.sql`. The current one skips what a managed host refuses |
| `role "erp_app" does not exist` during migrate | `deploy/neon-bootstrap.sql` has not been run, or was run against the wrong database |
| `database "erp" does not exist` | The bootstrap's `CREATE DATABASE` did not run — some SQL editors wrap the file in a transaction, and `CREATE DATABASE` cannot run inside one. Run those two lines alone |
| `dbverify` warns `erp_migrate not elevated` | Locally: expected, it is the container's superuser. Against Neon: the owner was created through the console. Recreate it |
| API returns 500 on every authenticated request | The service account key did not mount. Check the `/secrets/firebase/key.json` entry in `--set-secrets` and the `secretAccessor` binding |
| CORS error in the browser console | `CORS_ORIGINS` does not exactly match the origin, scheme included. It is an allow-list with no wildcards |
| Deep links 404 but the home page works | The Hosting rewrite. Confirm `firebase.json` was deployed from `frontend/`, where it lives |
| The frontend calls `localhost:8080` in production | Built before `frontend/.env.production` existed. Rebuild and redeploy |
| `prepared statement "lrupsc_1_0" already exists` | Transaction-mode pooling disagreeing with pgx's statement cache. Append `&default_query_exec_mode=simple_protocol` to the two pooled URLs, or point the service at the direct endpoint |
| First request after idle takes 5+ seconds | Cloud Run cold start plus Neon compute wake-up. §9 |

---

## What is deliberately not here

- **No deploy job in CI.** §12.7: Phase 9 deploys by hand. The workflow builds
  and tests both applications and stops there.
- **No `erp-prod` Firebase project.** Recorded as a Decision in `PROGRESS.md` —
  a live demo anyone can sign into was worth more than a clean user pool for a
  portfolio deployment.
- **No Cloud SQL.** Neon's free tier persists; Cloud SQL has none. Nothing in
  this design is provider-specific, so it remains a drop-in swap (§2.3.3).
