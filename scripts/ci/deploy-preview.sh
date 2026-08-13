#!/usr/bin/env bash
# Publish a per-developer dasm-burner preview via GitOps (dasmlab-live-cicd).
# Pattern copied from mock-me/scripts/ci/deploy-preview.sh — main→live/, else→previews/{owner}.yaml
# Does NOT oc-apply from the runner — Argo CD syncs the previews/ path.
set -euo pipefail

VERSION_TAG="${VERSION_TAG:?}"
ACTOR="${PREVIEW_ACTOR:?}"
FRESH="${PREVIEW_FRESH:-false}"
CLUSTER_APPS="${CLUSTER_APPS_DOMAIN:-apps.2026-prod-1.ocp.dasmlab.org}"

OWNER="$(echo "${ACTOR}" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-+/-/g' | cut -c1-20)"
OWNER="${OWNER:-dev}"
NS="dasm-burner-dev-${OWNER}"
HOST="dev-${OWNER}-dasm-burner.${CLUSTER_APPS}"
PREVIEW_URL="https://${HOST}"

PVC_NAME="dasm-burner-data"
if [[ "${FRESH}" == "true" || "${FRESH}" == "1" ]]; then
  PVC_NAME="dasm-burner-data-${VERSION_TAG//./-}"
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RENDERED="$(mktemp)"
sed \
  -e "s|__VERSION__|${VERSION_TAG}|g" \
  -e "s|__PREVIEW_NS__|${NS}|g" \
  -e "s|__PREVIEW_HOST__|${HOST}|g" \
  -e "s|__PREVIEW_OWNER__|${OWNER}|g" \
  -e "s|claimName: dasm-burner-data|claimName: ${PVC_NAME}|g" \
  -e "s|name: dasm-burner-data$|name: ${PVC_NAME}|g" \
  "${ROOT}/k8s_envelope/dasm-burner_preview-ocp.yaml" > "${RENDERED}"

echo "Preview owner=${OWNER} ns=${NS} host=${HOST} version=${VERSION_TAG} fresh=${FRESH} pvc=${PVC_NAME}"

if [[ "${SKIP_PREVIEW_BOOTSTRAP:-}" == "true" ]]; then
  echo "SKIP_PREVIEW_BOOTSTRAP=true — not copying preview secrets"
elif command -v oc >/dev/null 2>&1; then
  if oc whoami >/dev/null 2>&1; then
    echo "oc available — ensuring preview NS secrets via bootstrap-preview-ns.sh"
    bash "${ROOT}/scripts/ci/bootstrap-preview-ns.sh" "${OWNER}" || {
      echo "WARN: bootstrap-preview-ns.sh failed; pod may stay Pending until secrets exist" >&2
    }
  else
    echo "WARN: oc present but not logged in — skip preview secret bootstrap" >&2
  fi
else
  echo "WARN: oc not on PATH — skip preview secret bootstrap" >&2
fi

if [[ "${SKIP_PREVIEW_CERT:-}" != "true" ]]; then
  bash "${ROOT}/scripts/ci/ensure-preview-cert.sh" "${HOST}"
fi

if [[ "${SKIP_KEYCLOAK_URIS:-}" != "true" ]]; then
  bash "${ROOT}/scripts/ci/ensure-keycloak-preview-uris.sh" "${HOST}" || {
    echo "WARN: Keycloak URI ensure failed — add redirect URIs manually for ${PREVIEW_URL}" >&2
  }
fi

DEPLOY_TOKEN=""
if [ -f "/home/dasm/gh_token" ]; then
  DEPLOY_TOKEN="$(tr -d '\n\r' < /home/dasm/gh_token)"
fi
if [ -z "${DEPLOY_TOKEN}" ]; then
  DEPLOY_TOKEN="${DASMLAB_GHCR_PAT:-${GH_TOKEN:-}}"
fi
if [ -z "${DEPLOY_TOKEN}" ]; then
  echo "ERROR: deploy token not set (gh_token / DASMLAB_GHCR_PAT / GH_TOKEN)" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}" "${RENDERED}"' EXIT
git clone --depth 1 "https://x-access-token:${DEPLOY_TOKEN}@github.com/lmcdasm/dasmlab-live-cicd.git" "${WORK}/live-cicd"
PREVIEW_DIR="${WORK}/live-cicd/clusters/2026-prod-1/dasm-burner/previews"
mkdir -p "${PREVIEW_DIR}"
# Keep a README for the Argo path (exclude from include filter via name — Argo include is *.yaml)
if [[ ! -f "${PREVIEW_DIR}/README.md" ]]; then
  cat > "${PREVIEW_DIR}/README.md" <<'EOF'
# dasm-burner developer previews

Per-developer preview manifests land here via CI (`scripts/ci/deploy-preview.sh`).

- Host: `dev-{owner}-dasm-burner.apps.2026-prod-1.ocp.dasmlab.org`
- Namespace: `dasm-burner-dev-{owner}`
- Argo Application = `dasm-burner-previews` (auto-sync + prune)

Only `main` publishes to `../live/`. The `development` branch (and any non-main branch) publishes here.
EOF
fi
cp "${RENDERED}" "${PREVIEW_DIR}/${OWNER}.yaml"

# Ensure Argo Application for previews exists alongside prod.
mkdir -p "${WORK}/live-cicd/clusters/2026-prod-1/argocd/applications"
cp "${ROOT}/k8s_envelope/argocd-application-previews.yaml" \
  "${WORK}/live-cicd/clusters/2026-prod-1/argocd/applications/dasm-burner-previews.yaml"

cd "${WORK}/live-cicd"
git config user.name "dasmlab-bot"
git config user.email "ci@dasmlab.org"
git add \
  "clusters/2026-prod-1/dasm-burner/previews/${OWNER}.yaml" \
  "clusters/2026-prod-1/dasm-burner/previews/README.md" \
  "clusters/2026-prod-1/argocd/applications/dasm-burner-previews.yaml"
if git diff --cached --quiet; then
  echo "No GitOps preview changes (manifest identical)"
else
  git commit -m "preview(${OWNER}): dasm-burner ${VERSION_TAG}"
  git push
fi

if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "PREVIEW_URL=${PREVIEW_URL}" >> "${GITHUB_ENV}"
  echo "PREVIEW_NS=${NS}" >> "${GITHUB_ENV}"
fi
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "## dasm-burner preview (GitOps)"
    echo ""
    echo "- **URL:** ${PREVIEW_URL}"
    echo "- **Namespace:** \`${NS}\`"
    echo "- **GitOps file:** \`clusters/2026-prod-1/dasm-burner/previews/${OWNER}.yaml\`"
    echo "- **Argo app:** \`dasm-burner-previews\` (auto-sync)"
    echo "- **Image:** \`ghcr.io/dasmlab/dasm-burner:${VERSION_TAG}\`"
    echo "- **Owner:** \`${OWNER}\`"
  } >> "${GITHUB_STEP_SUMMARY}"
fi

echo "Preview GitOps published: ${PREVIEW_URL}"
