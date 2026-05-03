#!/bin/bash
# SessionStart hook: inject the current task's briefing as additionalContext.
# If project-journal is initialized but no task is active, inject a short
# nudge so Claude offers to create a first task instead of staying silent.
#
# Stdout contract (from docs):
#   { "hookSpecificOutput": { "hookEventName": "SessionStart",
#                             "additionalContext": "<markdown>" } }

# shellcheck disable=SC1091
. "$(dirname "$0")/_common.sh"

emit_context() {
  jq -n --arg ctx "$1" '{
    hookSpecificOutput: {
      hookEventName: "SessionStart",
      additionalContext: $ctx
    }
  }' 2>>"$PJ_HOOK_LOG" || {
    log_err "session-start: jq failed to build output"
    return 1
  }
}

if [ -n "$PJ_TASK" ]; then
  # PJ_NO_LLM=1: disable live embedding queries on the session-start critical
  # path. Cached embeddings are still used for cosine ranking; no network call.
  BRIEFING=$(PJ_NO_LLM=1 pj context --for "$PJ_TASK" 2>/dev/null || true)
  if [ -z "$BRIEFING" ]; then
    log_info "session-start: no briefing for task $PJ_TASK"
    exit 0
  fi
  emit_context "$BRIEFING"
  exit 0
fi

# No active task. Only nudge if the project is actually initialized —
# otherwise stay silent (this might not be a project-journal repo at all).
if ! pj_is_initialized; then
  exit 0
fi

NUDGE=$'project-journal is initialized in this repo, but no task is active.\n\nBefore starting work, ask the user once: "I notice there\'s no active task in project-journal. Want me to create your first task for what you\'re about to work on?" If they agree, infer a short title from their next message and run:\n\n    pj phase add "<phase title>"     # only if no phases exist yet\n    pj task add P1 "<task title>"\n    pj start P1.T1\n\nThen continue with the work. If they decline, do not ask again this session.'

emit_context "$NUDGE"
exit 0
