#!/bin/bash
# UserPromptSubmit hook: log the FIRST user prompt as the task intent.
# Subsequent prompts are intentionally ignored (--first-only on pj log).
# No stdout output: the user's prompt should not be modified.

# shellcheck disable=SC1091
. "$(dirname "$0")/_common.sh"
pj_require_task

PROMPT=$(printf '%s' "$HOOK_INPUT" | jq -r '.prompt // empty' 2>/dev/null || true)
if [ -z "$PROMPT" ]; then
  exit 0
fi

if ! pj log "$PJ_TASK" \
      --type user_prompt \
      --content "$PROMPT" \
      --first-only \
      >>"$PJ_HOOK_LOG" 2>&1; then
  log_err "user-prompt-submit: pj log failed for task $PJ_TASK"
fi

exit 0
