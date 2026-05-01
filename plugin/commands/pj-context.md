---
description: Render the briefing for the current (or specified) task
argument-hint: "[task_id]"
allowed-tools: Bash(pj context*)
---

!`if [ -n "$ARGUMENTS" ]; then pj context --for "$ARGUMENTS"; else pj context; fi`
