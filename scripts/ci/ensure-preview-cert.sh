#!/usr/bin/env bash
# Ensure HAProxy on 10.20.1.10 has a Let's Encrypt CERT entry for FQDN.
# Preview/dev FQDNs. Uses scripts/ci/remotessh (Go) — no system ssh on bld-249.
set -euo pipefail

FQDN="${1:?usage: ensure-preview-cert.sh <fqdn>}"
# Guard: never write a bare username (e.g. lmcdasm) into CERTn.
if [[ "${FQDN}" != *.* ]]; then
  echo "ERROR: ensure-preview-cert.sh expects a FQDN (got ${FQDN})" >&2
  exit 1
fi

PROXY_HOST="${PREVIEW_PROXY_HOST:-10.20.1.10}"
PROXY_USER="${PREVIEW_PROXY_USER:-dasm}"
PROXY_DIR="${PREVIEW_PROXY_DIR:-/home/dasm/dasmlab-internal/new_haproxy}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REMOTE_SSH=(go run "${ROOT}/scripts/ci/remotessh")

cleanup_key() {
  if [[ -n "${_DB_SSH_KEY_TMP:-}" && -f "${_DB_SSH_KEY_TMP}" ]]; then
    rm -f "${_DB_SSH_KEY_TMP}"
  fi
}
trap cleanup_key EXIT

if [[ -n "${SSH_IDENTITY_FILE:-}" && -f "${SSH_IDENTITY_FILE}" ]]; then
  REMOTE_SSH+=(-i "${SSH_IDENTITY_FILE}")
elif [[ -n "${PREVIEW_PROXY_SSH_KEY:-}" ]]; then
  _DB_SSH_KEY_TMP="$(mktemp)"
  printf '%s\n' "${PREVIEW_PROXY_SSH_KEY}" | tr -d '\r' > "${_DB_SSH_KEY_TMP}"
  chmod 600 "${_DB_SSH_KEY_TMP}"
  REMOTE_SSH+=(-i "${_DB_SSH_KEY_TMP}")
elif command -v ssh >/dev/null 2>&1; then
  # Local/dev host with system ssh (not the org runner).
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
    "${PROXY_USER}@${PROXY_HOST}" bash -s -- "${FQDN}" "${PROXY_DIR}" <<'EOS'
set -euo pipefail
FQDN="$1"
DIR="$2"
cd "$DIR"
if grep -Fq "=${FQDN}" runme.sh; then
  echo "HAProxy CERT already present for ${FQDN}"
  exit 0
fi
last="$(grep -oE 'CERT[0-9]+=' runme.sh | grep -oE '[0-9]+' | sort -n | tail -1)"
next=$((last + 1))
echo "Adding CERT${next}=${FQDN} to runme.sh and recreating new-haproxy"
tmp="$(mktemp)"
awk -v n="$next" -v h="$FQDN" '
  / -e EMAIL=/ && !done {
    print "    -e CERT" n "=" h " \\"
    done=1
  }
  { print }
' runme.sh > "$tmp"
mv "$tmp" runme.sh
chmod +x runme.sh
./runme.sh
echo "HAProxy updated for ${FQDN} (CERT${next})"
EOS
  exit 0
else
  echo "ERROR: set PREVIEW_PROXY_SSH_KEY or SSH_IDENTITY_FILE (runner has no system ssh)" >&2
  exit 1
fi

"${REMOTE_SSH[@]}" "${PROXY_USER}@${PROXY_HOST}" bash -s -- "${FQDN}" "${PROXY_DIR}" <<'EOS'
set -euo pipefail
FQDN="$1"
DIR="$2"
cd "$DIR"
if grep -Fq "=${FQDN}" runme.sh; then
  echo "HAProxy CERT already present for ${FQDN}"
  exit 0
fi

last="$(grep -oE 'CERT[0-9]+=' runme.sh | grep -oE '[0-9]+' | sort -n | tail -1)"
next=$((last + 1))
echo "Adding CERT${next}=${FQDN} to runme.sh and recreating new-haproxy"

tmp="$(mktemp)"
awk -v n="$next" -v h="$FQDN" '
  / -e EMAIL=/ && !done {
    print "    -e CERT" n "=" h " \\"
    done=1
  }
  { print }
' runme.sh > "$tmp"
mv "$tmp" runme.sh
chmod +x runme.sh

./runme.sh
echo "HAProxy updated for ${FQDN} (CERT${next})"
EOS
