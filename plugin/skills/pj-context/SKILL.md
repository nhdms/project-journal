---
name: pj-context
description: Render markdown briefing for the current (or specified) task — phase context, dependencies, relevant past tasks
argument-hint: "[task_id]"
allowed-tools: Bash(pj context*)
---

Render the briefing for a project-journal task. If the user supplies a task ID as `$ARGUMENTS`, brief that one; otherwise brief the current active task.

The briefing includes: current task ID/title/phase, phase goal, hard dependencies (from DependsOn), and top-5 relevant past tasks (blended scoring: cosine similarity + dependency boost + same-phase boost + recency).

!`if [ -n "$ARGUMENTS" ]; then pj context --for "$ARGUMENTS"; else pj context; fi`
