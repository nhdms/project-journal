---
name: pj-init
description: Initialize project-journal in the current directory. Optionally create and start a first task in one go. Use this when user explicitly invokes `/project-journal:pj-init` or asks to "set up project journal" / "init journal".
argument-hint: "[task_id task_title]"
allowed-tools: Bash(pj init*), Bash(pj task add*), Bash(pj start*)
---

Initialize project-journal in the current directory. If `$ARGUMENTS` is provided in the form `<task_id> <task_title...>`, also create and start that task.

```bash
!`pj init && if [ -n "$ARGUMENTS" ]; then \
   ID=$(echo "$ARGUMENTS" | awk '{print $1}'); \
   TITLE=$(echo "$ARGUMENTS" | cut -d' ' -f2-); \
   pj task add "$ID" "$TITLE" && pj start "$ID"; \
 fi`
```

After running, briefly confirm to the user:
- If just init: "Project journal initialized at `.project-journal/`. Run `pj task add` or use `/project-journal:journal-bootstrap` to create a task."
- If init + task: "Initialized + started task **<id>** — <title>. You can now begin work; the briefing has been printed above."
