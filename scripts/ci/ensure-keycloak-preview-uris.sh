#!/usr/bin/env bash
# Ensure Keycloak client dasm-burner has Valid Redirect URIs + Web Origins for a preview host.
# Keycloak does NOT support subdomain wildcards (dev-*-…); each preview host must be explicit.
# Pattern from dasmlab_home/scripts/ci/ensure-keycloak-preview-uris.sh
#
# Usage: ensure-keycloak-preview-uris.sh <preview-host-or-full-https-origin>
set -euo pipefail

RAW="${1:?preview host or https://origin required}"
if [[ "${RAW}" == https://* ]]; then
  ORIGIN="${RAW%/}"
else
  ORIGIN="https://${RAW}"
fi

KC="${KEYCLOAK_URL:-https://keycloak.apps.2026-prod-1.ocp.dasmlab.org}"
REALM="${KEYCLOAK_REALM:-dasmlab}"
CLIENT_ID="${KEYCLOAK_CLIENT_ID:-dasm-burner}"

ADMIN_USER="${KEYCLOAK_ADMIN_USER:-}"
ADMIN_PASS="${KEYCLOAK_ADMIN_PASSWORD:-}"
if [[ -z "${ADMIN_USER}" || -z "${ADMIN_PASS}" ]]; then
  if command -v oc >/dev/null 2>&1 && oc whoami >/dev/null 2>&1; then
    ADMIN_USER="$(oc get secret dasmlab-keycloak-initial-admin -n keycloak-operator -o jsonpath='{.data.username}' 2>/dev/null | base64 -d || true)"
    ADMIN_PASS="$(oc get secret dasmlab-keycloak-initial-admin -n keycloak-operator -o jsonpath='{.data.password}' 2>/dev/null | base64 -d || true)"
  fi
fi
if [[ -z "${ADMIN_USER}" || -z "${ADMIN_PASS}" ]]; then
  echo "WARN: no Keycloak admin creds — add redirect URIs manually for ${ORIGIN}" >&2
  echo "  ${ORIGIN}/api/v1/auth/callback" >&2
  echo "  ${ORIGIN}/*" >&2
  echo "  web origin: ${ORIGIN}" >&2
  exit 0
fi

TOKEN="$(curl -sk -X POST "${KC}/realms/master/protocol/openid-connect/token" \
  -d "client_id=admin-cli" \
  -d "username=${ADMIN_USER}" \
  -d "password=${ADMIN_PASS}" \
  -d "grant_type=password" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("access_token",""))')"
if [[ -z "${TOKEN}" ]]; then
  echo "ERROR: Keycloak admin login failed" >&2
  exit 1
fi

CID="$(curl -sk -H "Authorization: Bearer ${TOKEN}" \
  "${KC}/admin/realms/${REALM}/clients?clientId=${CLIENT_ID}" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d[0]["id"] if d else "")')"
if [[ -z "${CID}" ]]; then
  echo "ERROR: client ${CLIENT_ID} not found in realm ${REALM}" >&2
  exit 1
fi

TMP="$(mktemp)"
trap 'rm -f "${TMP}"' EXIT
curl -sk -H "Authorization: Bearer ${TOKEN}" \
  "${KC}/admin/realms/${REALM}/clients/${CID}" > "${TMP}"

CHANGED="$(python3 - "${TMP}" "${ORIGIN}" <<'PY'
import json, sys
path, origin = sys.argv[1], sys.argv[2].rstrip("/")
need_redirects = [f"{origin}/api/v1/auth/callback", f"{origin}/*"]
c = json.load(open(path))
ru = list(c.get("redirectUris") or [])
wo = list(c.get("webOrigins") or [])
changed = False
for u in need_redirects:
    if u not in ru:
        ru.append(u)
        changed = True
if origin not in wo and "+" not in wo:
    wo.append(origin)
    changed = True
attrs = dict(c.get("attributes") or {})
post = attrs.get("post.logout.redirect.uris") or ""
post_parts = [p for p in post.replace("##", " ").split() if p]
for u in (f"{origin}/*", origin):
    if u not in post_parts:
        post_parts.append(u)
        changed = True
attrs["post.logout.redirect.uris"] = "##".join(post_parts) if "##" in post or not post else " ".join(post_parts)
c["redirectUris"] = ru
c["webOrigins"] = wo
c["attributes"] = attrs
json.dump(c, open(path, "w"))
print("1" if changed else "0")
PY
)"

if [[ "${CHANGED}" == "1" ]]; then
  curl -sk -X PUT -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
    -d @"${TMP}" "${KC}/admin/realms/${REALM}/clients/${CID}" >/dev/null
  echo "Keycloak client ${CLIENT_ID}: added preview URIs for ${ORIGIN}"
else
  echo "Keycloak client ${CLIENT_ID}: preview URIs already present for ${ORIGIN}"
fi
