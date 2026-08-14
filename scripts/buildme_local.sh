#!/usr/bin/env bash
# Prove the product Containerfile builds (same stages CI uses) before push.
# Deletes the local image afterward so this does not leave GHCR-sized clutter.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="localhost/dasm-burner:local-check"
FILE="deployments/containers/Containerfile"

engine=""
if command -v podman >/dev/null 2>&1; then
  engine=podman
elif command -v docker >/dev/null 2>&1; then
  engine=docker
else
  echo "need podman or docker on PATH" >&2
  exit 1
fi

echo "==> ${engine} build -f ${FILE} -t ${TAG}"
"${engine}" build \
  --file "${FILE}" \
  --tag "${TAG}" \
  --build-arg "BUILD_VERSION=local-check" \
  "${ROOT}"

echo "==> removing ${TAG}"
"${engine}" rmi -f "${TAG}" >/dev/null

echo "OK: container build succeeded (image removed)"
