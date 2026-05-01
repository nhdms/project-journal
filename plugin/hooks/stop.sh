#!/bin/bash
# Stop hook: kick off `pj finish --auto` in the background so the LLM
# induction step doesn't block Claude from returning control to the user.
# We never block the Stop event.

# shellcheck disable=SC1091
. "$(dirname "$0")/_common.sh"

# Background-spawn so a multi-second LLM call doesn't hang the session.
# `nohup ... &` plus `disown` detaches from this script's process group.
( nohup pj finish "$PJ_TASK" --auto >>"$PJ_HOOK_LOG" 2>&1 & ) || {
  log_err "stop: failed to spawn pj finish for task $PJ_TASK"
}

exit 0
