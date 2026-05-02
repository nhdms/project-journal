---
name: pj-review
description: Review the LLM-induced summary of a finished task and suggest improvements — use when summary feels weak or you want a second-pass quality check
argument-hint: "<task_id>"
allowed-tools: Task, Bash(pj show*), Bash(pj tree*)
---

The user wants a quality check on a project-journal task's induced summary. Delegate to the `journal-validator` agent.

1. If `$ARGUMENTS` is empty, ask the user which task ID to review (offer `pj tree` output as context).
2. Otherwise, invoke the `journal-validator` agent with the task ID.

The agent will:
- Read the current task JSON via `pj show <id>` (or directly from `.project-journal/tasks.jsonl`)
- Read the raw trajectory from `.project-journal/sessions/<id>.jsonl`
- Compare summary against trajectory for completeness, accuracy, missing TODOs, missing files
- Output a quality score and concrete suggested edits
- If user agrees, apply edits via `pj edit <id>` (which opens $EDITOR with JSON)
