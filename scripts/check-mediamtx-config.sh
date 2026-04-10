#!/usr/bin/env bash
#
# check-mediamtx-config.sh — CI guard enforcing the CLAUDE.md directive that
# the NVR runtime settings in mediamtx.yml must NEVER be modified or cleared
# to make tests (notably TestSampleConfFile) pass.
#
# WHY THIS EXISTS:
#   CLAUDE.md (project root) has a standing "Critical: Do NOT modify
#   mediamtx.yml runtime settings" directive. Past sessions have tried to
#   "fix" TestSampleConfFile by flipping nvr/api/playback/logLevel or
#   clearing nvrJWTSecret. That is destructive:
#
#     - nvrJWTSecret encrypts the database keys. Clearing or changing it
#       bricks the installation — encrypted keys on disk become unreadable.
#     - nvr/api/playback must stay true for the NVR subsystem to function.
#     - logLevel: debug is required for the NVR troubleshooting workflow.
#
#   Fix the TEST, not the config. This script enforces that rule in CI so
#   the directive is not just documentation.
#
# USAGE:
#   ./scripts/check-mediamtx-config.sh [path/to/mediamtx.yml]
#
# EXIT CODES:
#   0 — all required NVR settings present and correct
#   1 — at least one required setting missing or flipped
#
set -euo pipefail

CONFIG_FILE="${1:-mediamtx.yml}"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "ERROR: config file not found: $CONFIG_FILE" >&2
  exit 1
fi

# Each entry: "regex|human-readable description"
# Regex is an ERE anchored to start/end of line.
REQUIRED=(
  '^nvr: true$|nvr: true (NVR subsystem must stay enabled)'
  '^api: true$|api: true (control API must stay enabled)'
  '^playback: true$|playback: true (playback server must stay enabled)'
  '^logLevel: debug$|logLevel: debug (required for NVR troubleshooting)'
  '^nvrJWTSecret: .+$|nvrJWTSecret: <non-empty> (encrypts DB keys — NEVER clear)'
)

FAIL=0
for entry in "${REQUIRED[@]}"; do
  regex="${entry%%|*}"
  desc="${entry#*|}"
  if ! grep -qE "$regex" "$CONFIG_FILE"; then
    echo "ERROR: missing or modified required setting in $CONFIG_FILE:" >&2
    echo "       expected: $desc" >&2
    echo "       see CLAUDE.md -> 'Critical: Do NOT modify mediamtx.yml runtime settings'" >&2
    echo "       fix the TEST (e.g. TestSampleConfFile), not the config." >&2
    FAIL=1
  fi
done

if [[ "$FAIL" -ne 0 ]]; then
  echo "" >&2
  echo "mediamtx.yml NVR guard: FAIL" >&2
  exit 1
fi

echo "mediamtx.yml NVR guard: PASS"
exit 0
