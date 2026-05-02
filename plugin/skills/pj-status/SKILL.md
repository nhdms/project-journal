---
name: pj-status
description: Show project-journal stats — total phases, tasks by status, current active task, last finished
allowed-tools: Bash(pj status*)
---

Run `pj status` and present the output to the user verbatim. The command shows phase count, task counts grouped by status, the currently active task ID, and the most recently finished task.

If `pj` is not installed or no `.project-journal/` exists in the current directory, the command exits with an error — relay it as-is.

!`pj status`
