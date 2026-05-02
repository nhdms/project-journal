---
name: journal-checkpoint
description: Use after significant milestones or before risky operations during a journaled coding session. Logs a `checkpoint` trajectory event with a brief description so the eventual `pj finish --auto` LLM summary captures the moment. Trigger when: tests pass after non-trivial changes, a feature reaches "first working version", before destructive operations (`rm -rf`, `git push --force`, dropping tables, deleting branches), after a key architectural decision is made, or when the user explicitly says "this is working", "major progress", "milestone", "checkpoint". Skip if no journal task is active or for trivial events (single-line edits, read operations).
allowed-tools: Bash(pj current*), Bash(pj log*)
---

Log a checkpoint to the active task's trajectory.

## Step 1 — Get current task

```
TASK=$(pj current --quiet)
```

If empty → STOP. No active task.

## Step 2 — Compose checkpoint description

Write a 1-2 sentence summary of the milestone in past tense:
- "All 12 auth tests passing after JWT signature fix."
- "First working POST /api/business/register with Clerk webhook activation."
- "Decided to use httpOnly cookie over localStorage for token storage (XSS protection)."
- "About to run db migration that drops legacy_users table."

Avoid generic checkpoints like "did some work" or "made progress" — be specific about WHAT happened.

## Step 3 — Log it

```
pj log "$TASK" --type assistant_text --content "[CHECKPOINT] <your summary>"
```

The `[CHECKPOINT]` prefix makes it easy for the induce LLM to weight these events when summarizing.

## Constraints

- DO NOT spam checkpoints. Aim for 1-3 per task max.
- DO NOT log routine events (file edits, test runs) — the PostToolUse hook already captures those.
- DO NOT mention the checkpoint to the user unless they explicitly asked — silent logging is the goal.
- If `pj log` fails, swallow the error silently. Never block work.
