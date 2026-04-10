#!/usr/bin/env bash
# KAI-354 — Dispatch per-integrator mobile builds.
#
# Walks whitelabel/integrators/*.json and fires one
# `gh workflow run integrator-mobile-build.yml` per integrator. Useful for:
#   * Local dry-runs ("rebuild integrator X on my branch")
#   * Cron / release hooks that don't live in .github/workflows/
#   * The integrator-mobile-build-all workflow (which uses reusable workflows
#     by default, but can be flipped to call this script for gh-cli fan-out).
#
# Usage:
#   ./scripts/dispatch-integrator-builds.sh                 # all integrators, main
#   ./scripts/dispatch-integrator-builds.sh --ref feat/xyz  # all integrators, branch
#   ./scripts/dispatch-integrator-builds.sh acme globex     # specific integrators
#   ./scripts/dispatch-integrator-builds.sh --skip-upload   # artifacts only
#
# Requires: gh (GitHub CLI) authenticated with workflow:write.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST_DIR="${REPO_ROOT}/whitelabel/integrators"
WORKFLOW="integrator-mobile-build.yml"

REF=""
SKIP_UPLOAD="false"
IDS=()

while (( $# > 0 )); do
  case "$1" in
    --ref)
      REF="$2"; shift 2 ;;
    --ref=*)
      REF="${1#--ref=}"; shift ;;
    --skip-upload)
      SKIP_UPLOAD="true"; shift ;;
    -h|--help)
      sed -n '2,20p' "$0"; exit 0 ;;
    --*)
      echo "unknown flag: $1" >&2; exit 2 ;;
    *)
      IDS+=("$1"); shift ;;
  esac
done

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI is required" >&2
  exit 1
fi

if (( ${#IDS[@]} == 0 )); then
  while IFS= read -r manifest; do
    IDS+=("$(jq -r '.id' "${manifest}")")
  done < <(find "${MANIFEST_DIR}" -maxdepth 1 -type f -name '*.json' | sort)
fi

if (( ${#IDS[@]} == 0 )); then
  echo "error: no integrator manifests found under ${MANIFEST_DIR}" >&2
  exit 1
fi

echo "Dispatching ${#IDS[@]} integrator build(s) via ${WORKFLOW}"
for id in "${IDS[@]}"; do
  echo "  -> ${id}"
  args=(workflow run "${WORKFLOW}" -f "integrator_id=${id}" -f "skip_upload=${SKIP_UPLOAD}")
  if [[ -n "${REF}" ]]; then
    args+=(--ref "${REF}" -f "ref=${REF}")
  fi
  gh "${args[@]}"
done

echo "Done. Watch runs with: gh run list --workflow=${WORKFLOW}"
