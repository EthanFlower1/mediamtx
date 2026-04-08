#!/usr/bin/env bash
# proto-lock.sh — coordinate proto schema changes across autonomous agents.
#
# See docs/proto-lock.md for the full protocol.
#
# Subcommands:
#   status                                        Print current lock state
#   acquire <kai-id> <agent> "<reason>" [protos]  Acquire the lock for a KAI ticket
#   release <kai-id>                              Release the lock (must match holder)
#   check   <kai-id>                              Verify current holder matches (CI use)
#   cleanup                                       Release any expired lock (scheduled cleanup)
#
# Environment:
#   PROTO_LOCK_TTL_HOURS    Hours the lock is held before expiry (default: 2)
#   PROTO_LOCK_NO_PR        If set, update the file locally without opening a PR (for CI/cleanup)
#   PROTO_LOCK_DRY_RUN      If set, print what would happen without mutating anything
#
# Dependencies: bash, jq, git, gh (GitHub CLI) for acquire/release.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
LOCK_FILE="$REPO_ROOT/.proto-lock.json"
TTL_HOURS="${PROTO_LOCK_TTL_HOURS:-2}"

die() { echo "proto-lock: $*" >&2; exit 1; }
log() { echo "proto-lock: $*" >&2; }

require_jq() { command -v jq >/dev/null || die "jq is required"; }
require_gh() { command -v gh >/dev/null || die "gh (GitHub CLI) is required"; }

iso_now() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }
iso_plus_hours() {
  local hours="$1"
  if date -u -v+"${hours}"H +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null; then
    return
  fi
  date -u -d "+${hours} hours" +"%Y-%m-%dT%H:%M:%SZ"
}
iso_to_epoch() {
  local iso="$1"
  if date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$iso" +%s 2>/dev/null; then
    return
  fi
  date -u -d "$iso" +%s
}

lock_is_free() {
  require_jq
  local holder expires now exp_epoch now_epoch
  holder="$(jq -r '.holder' "$LOCK_FILE")"
  if [[ "$holder" == "null" || -z "$holder" ]]; then
    return 0
  fi
  expires="$(jq -r '.expires_at' "$LOCK_FILE")"
  [[ "$expires" == "null" || -z "$expires" ]] && return 1
  exp_epoch="$(iso_to_epoch "$expires")"
  now_epoch="$(iso_to_epoch "$(iso_now)")"
  [[ "$now_epoch" -gt "$exp_epoch" ]]
}

cmd_status() {
  require_jq
  local holder kai agent reason expires protos
  holder=$(jq -r '.holder // "null"' "$LOCK_FILE")
  if [[ "$holder" == "null" ]]; then
    echo "FREE"
    return 0
  fi
  kai=$(jq -r '.kai_id // "?"' "$LOCK_FILE")
  agent=$(jq -r '.agent // "?"' "$LOCK_FILE")
  reason=$(jq -r '.reason // "?"' "$LOCK_FILE")
  expires=$(jq -r '.expires_at // "?"' "$LOCK_FILE")
  protos=$(jq -r '.protos_touched | join(", ")' "$LOCK_FILE")
  printf 'HELD by %s (%s) — agent: %s — expires: %s\nReason: %s\nProtos touched: %s\n' \
    "$holder" "$kai" "$agent" "$expires" "$reason" "$protos"
}

cmd_acquire() {
  require_jq
  local kai="${1:-}" agent="${2:-}" reason="${3:-}"; shift 3 || true
  [[ -z "$kai" || -z "$agent" || -z "$reason" ]] && die "usage: acquire <kai-id> <agent> \"<reason>\" [protos...]"

  local mode="pr"
  [[ -n "${PROTO_LOCK_NO_PR:-}" ]] && mode="inplace"
  [[ -n "${PROTO_LOCK_DRY_RUN:-}" ]] && mode="dryrun"

  # For PR mode, refuse to run on a dirty tree — we're about to checkout a new branch.
  if [[ "$mode" == "pr" ]]; then
    if ! git diff --quiet || ! git diff --cached --quiet; then
      die "working tree is dirty; commit or stash before acquiring the lock in PR mode"
    fi
    git fetch origin main --quiet || die "unable to fetch origin/main"
  fi

  # Determine the authoritative current lock state.
  # In PR mode we use origin/main's version (canonical source of truth).
  # In inplace/dryrun mode we use the working tree (caller knows what they're doing).
  local base_file
  base_file="$(mktemp)"
  if [[ "$mode" == "pr" ]]; then
    git show "origin/main:.proto-lock.json" > "$base_file" 2>/dev/null || cp "$LOCK_FILE" "$base_file"
  else
    cp "$LOCK_FILE" "$base_file"
  fi

  # Check whether the base lock is free (or expired).
  local base_holder base_expires base_exp_epoch now_epoch base_free=0
  base_holder=$(jq -r '.holder // "null"' "$base_file")
  if [[ "$base_holder" == "null" ]]; then
    base_free=1
  else
    base_expires=$(jq -r '.expires_at // empty' "$base_file")
    if [[ -n "$base_expires" ]]; then
      base_exp_epoch=$(iso_to_epoch "$base_expires" 2>/dev/null || echo 0)
      now_epoch=$(iso_to_epoch "$(iso_now)")
      [[ "$now_epoch" -gt "$base_exp_epoch" ]] && base_free=1
    fi
  fi

  if [[ "$base_free" -ne 1 ]]; then
    log "lock is HELD on main — refusing to acquire"
    jq '.' "$base_file" >&2
    rm -f "$base_file"
    exit 2
  fi

  local protos_json='[]'
  if [[ $# -gt 0 ]]; then
    protos_json="$(printf '%s\n' "$@" | jq -R . | jq -s .)"
  fi

  local now expires new_file
  now="$(iso_now)"
  expires="$(iso_plus_hours "$TTL_HOURS")"
  new_file="$(mktemp)"
  jq \
    --arg holder "$kai" \
    --arg kai "$kai" \
    --arg agent "$agent" \
    --arg reason "$reason" \
    --arg now "$now" \
    --arg expires "$expires" \
    --argjson protos "$protos_json" \
    '.holder = $holder | .kai_id = $kai | .agent = $agent | .reason = $reason | .acquired_at = $now | .expires_at = $expires | .protos_touched = $protos' \
    "$base_file" > "$new_file"
  rm -f "$base_file"

  case "$mode" in
    dryrun)
      log "DRY RUN — would write:"
      cat "$new_file"
      rm -f "$new_file"
      return 0
      ;;
    inplace)
      mv "$new_file" "$LOCK_FILE"
      log "updated lock file in place; caller handles commit"
      return 0
      ;;
  esac

  # PR mode: create a branch from origin/main, write the new lock, commit, push, open PR.
  require_gh
  local branch="proto-lock/acquire-$(echo "$kai" | tr '[:upper:]' '[:lower:]')"
  if git show-ref --verify --quiet "refs/heads/${branch}"; then
    git branch -D "$branch"
  fi
  git checkout -b "$branch" origin/main
  mv "$new_file" "$LOCK_FILE"
  git add .proto-lock.json
  git commit -m "proto-lock: acquire ${kai}

${reason}

Proto-Lock-Holder: ${kai}
Proto-Lock-Agent: ${agent}"
  git push -u origin "$branch"

  gh pr create \
    --base main \
    --head "$branch" \
    --title "proto-lock: acquire ${kai}" \
    --body "Automated proto-lock acquisition for **${kai}**.

**Agent:** \`${agent}\`
**Reason:** ${reason}
**Protos to touch:** ${protos_json}
**Expires:** ${expires}

This PR only modifies \`.proto-lock.json\`. Merge queue should serialize it against other lock PRs. Once merged, the holder can open their real proto PR with a \`Proto-Lock-Holder: ${kai}\` trailer.

See \`docs/proto-lock.md\`." \
    --label proto-lock-acquire

  gh pr merge --auto --squash || log "auto-merge not available — the acquire PR will wait for manual merge"

  log "acquire PR opened for ${kai} on branch ${branch}"
}

cmd_release() {
  require_jq
  local kai="${1:-}"
  [[ -z "$kai" ]] && die "usage: release <kai-id>"

  local current
  current="$(jq -r '.holder' "$LOCK_FILE")"
  if [[ "$current" != "$kai" ]]; then
    die "lock is not held by ${kai} (current holder: ${current})"
  fi

  local tmp; tmp="$(mktemp)"
  jq '.holder = null | .kai_id = null | .agent = null | .reason = null | .acquired_at = null | .expires_at = null | .protos_touched = []' \
    "$LOCK_FILE" > "$tmp"
  mv "$tmp" "$LOCK_FILE"

  if [[ -n "${PROTO_LOCK_NO_PR:-}" || -n "${PROTO_LOCK_DRY_RUN:-}" ]]; then
    log "released (in-place). Caller must commit the lock file."
    return 0
  fi

  git add .proto-lock.json
  git commit -m "proto-lock: release ${kai}

Proto-Lock-Release: ${kai}" || log "nothing staged to commit"

  log "released ${kai}. Remember to push the commit on your feature branch."
}

cmd_check() {
  require_jq
  local kai="${1:-}"
  [[ -z "$kai" ]] && die "usage: check <kai-id>"
  local current; current="$(jq -r '.holder' "$LOCK_FILE")"
  if [[ "$current" != "$kai" ]]; then
    die "lock holder mismatch: expected=${kai} current=${current}"
  fi
  log "ok: ${kai} holds the lock"
}

cmd_cleanup() {
  require_jq
  local current expires
  current="$(jq -r '.holder // "null"' "$LOCK_FILE")"
  if [[ "$current" == "null" ]]; then
    log "nothing to clean up (lock is already free)"
    return 0
  fi
  # Holder is set — is it expired?
  expires="$(jq -r '.expires_at // empty' "$LOCK_FILE")"
  if [[ -n "$expires" ]]; then
    local exp_epoch now_epoch
    exp_epoch="$(iso_to_epoch "$expires" 2>/dev/null || echo 0)"
    now_epoch="$(iso_to_epoch "$(iso_now)")"
    if [[ "$now_epoch" -le "$exp_epoch" ]]; then
      log "lock held by ${current} but not expired (expires ${expires}) — skipping"
      return 0
    fi
  fi
  log "EXPIRED lock detected: holder=${current} expired_at=${expires}"

  local tmp; tmp="$(mktemp)"
  jq '.holder = null | .kai_id = null | .agent = null | .reason = null | .acquired_at = null | .expires_at = null | .protos_touched = []' \
    "$LOCK_FILE" > "$tmp"
  mv "$tmp" "$LOCK_FILE"

  if [[ -n "${PROTO_LOCK_NO_PR:-}" || -n "${PROTO_LOCK_DRY_RUN:-}" ]]; then
    log "released stale lock (in-place). Caller must commit the lock file."
    return 0
  fi

  require_gh
  local branch="proto-lock/cleanup-$(date -u +%Y%m%d-%H%M%S)"
  git checkout -b "$branch" "origin/main"
  git add .proto-lock.json
  git commit -m "proto-lock: expire stale lock held by ${current}

The lock exceeded its TTL (${TTL_HOURS}h, expired at ${expires}) and was
force-released by the proto-lock cleanup job. If the original holder is
still working, they must re-acquire the lock and rebase.

Proto-Lock-Cleanup: ${current}"
  git push -u origin "$branch"
  gh pr create \
    --base main \
    --head "$branch" \
    --title "proto-lock: expire stale ${current}" \
    --body "Automated stale-lock cleanup. Held by ${current}, expired at ${expires}." \
    --label proto-lock-cleanup
  gh pr merge --auto --squash || log "auto-merge not available"
}

main() {
  local sub="${1:-}"; shift || true
  case "$sub" in
    status)  cmd_status  "$@" ;;
    acquire) cmd_acquire "$@" ;;
    release) cmd_release "$@" ;;
    check)   cmd_check   "$@" ;;
    cleanup) cmd_cleanup "$@" ;;
    ""|-h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) die "unknown subcommand: $sub (try --help)" ;;
  esac
}

main "$@"
