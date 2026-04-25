#!/usr/bin/env bash
# E2E smoke for the rvs CLI. Runs inside the Dockerfile next to it.
# Required env: RVS_TOKEN. Optional: RVS_API_BASE (defaults to prod).
set -euo pipefail

API_BASE="${RVS_API_BASE:-https://agents.rvs.solutions}"
INSTALL_URL="${RVS_INSTALL_URL:-${API_BASE}/cli/install.sh}"

if [ -z "${RVS_TOKEN:-}" ]; then
  echo "smoke: RVS_TOKEN must be set" >&2
  exit 2
fi

echo "smoke: installing from ${INSTALL_URL}"
curl -fsSL "${INSTALL_URL}" | sh

echo "smoke: rvs version"
rvs version

echo "smoke: rvs login --token ..."
rvs login --token "${RVS_TOKEN}" --api "${API_BASE}"

echo "smoke: rvs me"
rvs me

echo "smoke: rvs models (expect at least one row)"
rvs models | head -5

echo "smoke: rvs list"
rvs list | head -10

echo "smoke: ok"
