#!/bin/bash
# PreCompact hook: drop a compact_marker into the trajectory so post-hoc
# analysis can correlate behavior shifts with context compaction events.

# shellcheck disable=SC1091
. "$(dirname "$0")/_common.sh"
pj_require_task

TS=$(date -u +%FT%TZ)
TRIGGER=$(printf '%s' "$HOOK_INPUT" | jq -r '.trigger // "unknown"' 2>/dev/null || echo "unknown")

if ! pj log "$PJ_TASK" \
      --type compact_marker \
      --content "Context compacted at $TS (trigger=$TRIGGER)" \
      >>"$PJ_HOOK_LOG" 2>&1; then
  log_err "pre-compact: pj log failed for task $PJ_TASK"
fi

exit 0
