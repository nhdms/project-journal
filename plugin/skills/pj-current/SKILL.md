---
name: pj-current
description: Show the currently active project-journal task ID (the one Claude Code hooks log to)
allowed-tools: Bash(pj current*)
---

Run `pj current`. If a task is active, prints its ID. If no task is active, exits silently with code 1 — tell the user there's no active task and suggest `pj start <task_id>`.

!`pj current`
