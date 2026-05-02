---
name: journal-bootstrap
description: Use PROACTIVELY at the start of any task-oriented coding session. Checks whether project-journal is initialized in the current working directory and whether a task is currently active (`pj current`). If the project has no `.project-journal/` directory, offer to init it. If no task is active, propose creating and starting one based on the user's stated intent. Triggers when the user begins describing work to do — phrases like "let's build", "I want to implement", "add feature X", "fix bug Y", "refactor Z", "create the API for", or any imperative task request — AND no journal task is currently active. Do NOT trigger if the user's request is a question, code review, or read-only inspection.
allowed-tools: Bash(pj init*), Bash(pj current*), Bash(pj task add*), Bash(pj start*), Bash(pj tree*), Bash(test *)
---

The user is starting coding work. Before doing anything else, ensure the project journal is set up and a task is active so the work gets recorded.

## Step 1 — Check journal state

Run `pj current --quiet` to check if a task is active.

- **If exit 0 with a task ID** → a task is already active. Run `pj show <id>` and confirm with the user: "Active task is **<id>** (<title>). Continue with this task or start a new one?" Then proceed accordingly. STOP this skill if user wants to continue.
- **If exit 1 (no active task)** → continue to Step 2.

## Step 2 — Check journal initialization

Run `test -d .project-journal && echo yes || echo no`.

- **If "no"** → tell user: "No project-journal found in this directory. Initialize one to start tracking tasks?" Wait for confirmation. If yes, run `pj init`.
- **If "yes"** → continue.

## Step 3 — Propose a task

Based on the user's stated intent, propose a short task title and a kebab-cased ID (or external ID if user mentioned one like `JIRA-123`, `T1`, `NHD-24`). Examples:
- User: "Let's add JWT auth" → propose ID `auth-jwt` or ask for external ID if project uses one
- User: "Fix the login bug NHD-45" → use ID `NHD-45`

Show the user:
```
Proposed task:
  ID:    <id>
  Title: <title>
  Phase: <phase if mentioned, else none>

Create and start? [Y/n]
```

If yes:
1. `pj task add <id> "<title>" [--phase <p>] [--depends-on <ids>]`
2. `pj start <id>`
3. The `pj start` output IS the briefing. Present it to the user.

## Constraints

- DO NOT auto-init or auto-create without confirmation. Always ask.
- DO NOT trigger this skill on read-only requests, questions, or "what does X do?" queries.
- If user declines initialization, just proceed with the work — don't repeat the offer.
- If `pj` binary is not on PATH (`command -v pj` fails), silently skip — the journal is optional.
