#!/bin/bash
# PostToolUse hook: log a compact tool_use event for the trajectory.
#
# Hook-config matcher already filters to Edit|Write|MultiEdit|Bash|NotebookEdit,
# but we re-check here as defense in depth (the matcher is configurable
# at install time and a future edit could broaden it).
#
# Per docs the result field is `tool_result` (string or object). Older or
# alternate harnesses sometimes use `tool_response`; we accept either.

# shellcheck disable=SC1091
. "$(dirname "$0")/_common.sh"

TOOL_NAME=$(printf '%s' "$HOOK_INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)
if [ -z "$TOOL_NAME" ]; then
  exit 0
fi

case "$TOOL_NAME" in
  Edit|Write|MultiEdit|Bash|NotebookEdit) ;;
  *) exit 0 ;;
esac

# Build a small, human-readable summary of the tool input, picking the
# most informative field per tool. Fall back to compact JSON.
INPUT_SUMMARY=$(
  printf '%s' "$HOOK_INPUT" | jq -r --arg t "$TOOL_NAME" '
    .tool_input as $i |
    if $t == "Bash" then
      ($i.command // "") + (if ($i.description // "") != "" then "  # " + $i.description else "" end)
    elif $t == "Edit" or $t == "Write" or $t == "MultiEdit" then
      ($i.file_path // ($i.path // ""))
    elif $t == "NotebookEdit" then
      ($i.notebook_path // ($i.file_path // ""))
    else
      ($i | tostring)
    end' 2>/dev/null || true
)

# Output may live under tool_result (per docs) or tool_response. Coerce
# objects to compact JSON; strings pass through unchanged.
OUTPUT_SUMMARY=$(
  printf '%s' "$HOOK_INPUT" | jq -r '
    (.tool_result // .tool_response // "") |
    if type == "string" then .
    elif type == "object" or type == "array" then (. | tostring)
    else (. | tostring) end' 2>/dev/null || true
)

# Truncate both to 500 chars to keep the session log lean.
truncate500() {
  awk -v s="$1" 'BEGIN{ if (length(s) > 500) print substr(s,1,500) "...[truncated]"; else print s }'
}
INPUT_SUMMARY=$(truncate500 "$INPUT_SUMMARY")
OUTPUT_SUMMARY=$(truncate500 "$OUTPUT_SUMMARY")

if ! pj log "$PJ_TASK" \
      --type tool_use \
      --tool "$TOOL_NAME" \
      --input-summary "$INPUT_SUMMARY" \
      --output-summary "$OUTPUT_SUMMARY" \
      >>"$PJ_HOOK_LOG" 2>&1; then
  log_err "post-tool-use: pj log failed (tool=$TOOL_NAME task=$PJ_TASK)"
fi

exit 0
