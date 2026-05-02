#!/bin/bash
# SessionStart hook: inject the current task's briefing as additionalContext.
#
# Stdout contract (from docs):
#   { "hookSpecificOutput": { "hookEventName": "SessionStart",
#                             "additionalContext": "<markdown>" } }
# Plain text on stdout is also accepted and added as context, but JSON is
# more explicit about intent.

# shellcheck disable=SC1091
. "$(dirname "$0")/_common.sh"

BRIEFING=$(pj context --for "$PJ_TASK" 2>/dev/null || true)
if [ -z "$BRIEFING" ]; then
  log_info "session-start: no briefing for task $PJ_TASK"
  exit 0
fi

# Build JSON safely: jq --arg handles quotes, newlines, and backslashes.
jq -n --arg ctx "$BRIEFING" '{
  hookSpecificOutput: {
    hookEventName: "SessionStart",
    additionalContext: $ctx
  }
}' 2>>"$PJ_HOOK_LOG" || {
  log_err "session-start: jq failed to build output"
  exit 0
}

exit 0
