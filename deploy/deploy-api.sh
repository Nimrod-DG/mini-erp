#!/usr/bin/env bash
# Deploy the backend to Cloud Run. Reads deploy/prod.env; see docs/DEPLOY.md.
#
#   bash deploy/deploy-api.sh            # secrets (if absent) + deploy
#   bash deploy/deploy-api.sh --secrets  # only create/update the secrets
#
# Everything here is idempotent: a secret that exists gets a new version rather
# than an error, an IAM binding that exists is a no-op, and a service that
# exists is updated in place. Re-running after a failure is safe.
#
# The one thing this cannot do for you is `gcloud auth login` — that is an OAuth
# token, not a value, and it needs a browser.
set -euo pipefail

cd "$(dirname "$0")/.."

[ -f deploy/prod.env ] || { echo "deploy/prod.env is missing — see docs/DEPLOY.md §10"; exit 1; }
set -a; . ./deploy/prod.env; set +a

# On Windows, the bash-shim `gcloud` shells out to `python`, which the Microsoft
# Store App Execution Alias intercepts — every call dies with "Python was not
# found" and nothing else. gcloud.cmd uses the SDK's own bundled Python and
# works from Git Bash, PowerShell and cmd alike.
GCLOUD=${GCLOUD:-gcloud}
if ! "$GCLOUD" version >/dev/null 2>&1; then
  # Tested by running it, not by [ -x ]: Git Bash does not report .cmd files as
  # executable, so a file test rejects the very thing that works.
  for candidate in \
    "/c/Program Files (x86)/Google/Cloud SDK/google-cloud-sdk/bin/gcloud.cmd" \
    "/c/Program Files/Google/Cloud SDK/google-cloud-sdk/bin/gcloud.cmd" \
    "$LOCALAPPDATA/Google/Cloud SDK/google-cloud-sdk/bin/gcloud.cmd"; do
    if [ -f "$candidate" ] && "$candidate" version >/dev/null 2>&1; then
      GCLOUD="$candidate"
      break
    fi
  done
fi
"$GCLOUD" version >/dev/null 2>&1 || {
  echo "cannot run gcloud. Set GCLOUD=/path/to/gcloud.cmd and retry."; exit 1; }

: "${GCP_PROJECT_ID:?}" "${GCP_REGION:?}" "${CLOUD_RUN_SERVICE:?}"
: "${PROD_DATABASE_URL:?}" "${PROD_ADMIN_DATABASE_URL:?}" "${FIREBASE_KEY_FILE:?}"
[ -f "$FIREBASE_KEY_FILE" ] || { echo "FIREBASE_KEY_FILE not found: $FIREBASE_KEY_FILE"; exit 1; }

"$GCLOUD" config set project "$GCP_PROJECT_ID" >/dev/null

# --- secrets ---------------------------------------------------------------
# `versions add` on a secret that does not exist fails, so create first and
# ignore the "already exists" case. A new version is the right outcome either
# way: rotating a password is `versions add` plus a redeploy.
#
# Values go through a file in a 0700 temp directory rather than a pipe: gcloud
# is a wrapper script that spawns Python, and handing it /dev/stdin means
# relying on which process in that chain consumes it.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

put_secret() {
  local name="$1" file="$2"
  "$GCLOUD" secrets create "$name" --replication-policy=automatic </dev/null >/dev/null 2>&1 || true
  "$GCLOUD" secrets versions add "$name" --data-file="$file" >/dev/null
  echo "  secret $name updated"
}

echo "creating secrets in $GCP_PROJECT_ID"
printf '%s' "$PROD_DATABASE_URL"       > "$TMP/app-url"
printf '%s' "$PROD_ADMIN_DATABASE_URL" > "$TMP/admin-url"
put_secret erp-database-url       "$TMP/app-url"
put_secret erp-admin-database-url "$TMP/admin-url"
put_secret erp-firebase-key       "$FIREBASE_KEY_FILE"

# The runtime service account has to be allowed to read them. This is the
# binding whose absence shows up as a container that will not start, with
# "Permission denied on secret" buried in the Cloud Run logs.
PROJECT_NUMBER=$("$GCLOUD" projects describe "$GCP_PROJECT_ID" --format='value(projectNumber)')
RUNTIME_SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"

echo "granting secretAccessor to $RUNTIME_SA"
for s in erp-database-url erp-admin-database-url erp-firebase-key; do
  "$GCLOUD" secrets add-iam-policy-binding "$s" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role=roles/secretmanager.secretAccessor >/dev/null
done

[ "${1:-}" = "--secrets" ] && { echo "secrets only — stopping here"; exit 0; }

# --- deploy ----------------------------------------------------------------
# --source builds backend/Dockerfile with Cloud Build and pushes the image to
# Artifact Registry. No local docker, no repository to connect, no git push.
#
# MIGRATE_DATABASE_URL is deliberately absent: the schema owner's credential has
# no business on a running service, and config.Load no longer requires it.
echo "deploying $CLOUD_RUN_SERVICE to $GCP_REGION (first build takes a few minutes)"
#
# --quiet accepts the one prompt the first run asks: --source needs an Artifact
# Registry repository to push to, and offers to create `cloud-run-source-deploy`
# in the region. Answering it by hand is fine; being unable to answer it, in a
# script, is a hang.
"$GCLOUD" run deploy "$CLOUD_RUN_SERVICE" \
  --quiet \
  --source backend \
  --region "$GCP_REGION" \
  --allow-unauthenticated \
  --min-instances 0 \
  --max-instances 3 \
  --cpu 1 --memory 512Mi \
  --env-vars-file deploy/cloudrun.env.yaml \
  --set-secrets "DATABASE_URL=erp-database-url:latest,ADMIN_DATABASE_URL=erp-admin-database-url:latest,/secrets/firebase/key.json=erp-firebase-key:latest"

URL=$("$GCLOUD" run services describe "$CLOUD_RUN_SERVICE" --region "$GCP_REGION" --format='value(status.url)')
echo
echo "service URL: $URL"
echo
echo "next: put that in frontend/.env.production as VITE_API_BASE_URL, then"
echo "      cd frontend && npm run build && firebase deploy --only hosting"
