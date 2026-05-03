#!/bin/bash
# Common preamble sourced by all project-journal hook scripts.
# Sets up: PJ_HOOK_LOG, log_err(), reads HOOK_INPUT from stdin,
# extracts HOOK_CWD, cd's into it, exports PJ_TASK (current task id).
#
# Hard rules:
#   * Never crash a Claude session: every error path must `exit 0`.
#   * Stay silent unless the calling hook deliberately writes to stdout.
#   * Skip silently if `pj` or `jq` is missing, or if no current task.

set -u

PJ_HOOK_LOG="${HOME}/.project-journal-hook.log"

log_info() {
  { echo "[$(date -u +%FT%TZ)] [INFO] [$(basename "${0:-hook}")] $*" >> "$PJ_HOOK_LOG"; } 2>/dev/null || true
}

log_err() {
  # Best-effort: never let a logging failure propagate.
  { echo "[$(date -u +%FT%TZ)] [$(basename "${0:-hook}")] $*" >> "$PJ_HOOK_LOG"; } 2>/dev/null || true
}

# Skip if pj not available.
if ! command -v pj >/dev/null 2>&1; then
  exit 0
fi

# Skip if jq not available (all hooks parse JSON input).
if ! command -v jq >/dev/null 2>&1; then
  log_err "jq not available, skipping hook"
  exit 0
fi

# Read stdin once. Default to empty object so jq calls below don't fail.
HOOK_INPUT=$(cat 2>/dev/null || true)
if [ -z "$HOOK_INPUT" ]; then
  HOOK_INPUT="{}"
fi

# Extract cwd from input and switch into it. Hooks run in the Claude
# Code process cwd, which is not necessarily the project root, so prefer
# the cwd reported by the hook payload.
HOOK_CWD=$(printf '%s' "$HOOK_INPUT" | jq -r '.cwd // empty' 2>/dev/null || true)
if [ -n "$HOOK_CWD" ] && [ -d "$HOOK_CWD" ]; then
  cd "$HOOK_CWD" || exit 0
fi

# Resolve current task.
# Exit codes from `pj current`:
#   0 = task ID printed to stdout
#   1 = no active task (not an error; journal may not be initialized)
#   2 = real error (FS failure, corrupt layout, etc.)
# Hooks that require a task call `pj_require_task` after sourcing.
PJ_TASK=""
pj current --quiet >"${PJ_HOOK_LOG}.curtmp" 2>/dev/null
_pj_current_exit=$?
if [ $_pj_current_exit -eq 0 ]; then
  PJ_TASK=$(cat "${PJ_HOOK_LOG}.curtmp")
elif [ $_pj_current_exit -eq 2 ]; then
  log_err "common: pj current returned error (exit 2); skipping hook"
  rm -f "${PJ_HOOK_LOG}.curtmp"
  exit 0
fi
rm -f "${PJ_HOOK_LOG}.curtmp"

pj_require_task() {
  if [ -z "$PJ_TASK" ]; then
    exit 0
  fi
}

# True (exit 0) iff a `.project-journal` marker exists in the current
# working directory or any ancestor. We can't rely on `pj status` because
# it falls back to ~/.project-journal/ when no marker is found.
pj_is_initialized() {
  local d
  d=$(pwd)
  while [ -n "$d" ] && [ "$d" != "/" ]; do
    if [ -e "$d/.project-journal" ]; then
      return 0
    fi
    d=$(dirname "$d")
  done
  return 1
}

export PJ_HOOK_LOG HOOK_INPUT HOOK_CWD PJ_TASK
