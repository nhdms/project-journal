---
name: journal-recall
description: Use PROACTIVELY before starting any implementation work in a project that has a `.project-journal/` directory. Retrieves the briefing for the current active task (which includes phase context, hard dependencies, and top-5 semantically relevant past tasks via embedding search). Ensures Claude has full historical context before writing code. Triggers when about to: write new code, modify existing files, design a feature, debug an issue, or make architectural decisions — and a journal task is active OR the user just confirmed creating one via journal-bootstrap. Skip if the request is purely informational (read/explain) or if there is no active task.
allowed-tools: Bash(pj current*), Bash(pj context*), Bash(pj tree*), Bash(test *)
---

The user is about to do real implementation work. Pull the journal context first so prior tasks' decisions, files, and TODOs inform the current work.

## Step 1 — Confirm there's an active task

```
pj current --quiet
```

- Exit 1 → no active task. STOP this skill (let `journal-bootstrap` handle it next time).
- Exit 0 → continue.

## Step 2 — Pull the briefing

```
pj context
```

This renders markdown with:
- 🎯 Current task ID, title, phase
- 📦 Phase goal (if part of a phase)
- 🔗 Hard dependencies — past tasks the current one explicitly depends on, with their summaries and exposed interfaces
- ✨ Relevant past tasks (top 5 by blended scoring — cosine similarity + dependency boost + same-phase boost + recency)
- ⏭️ Coming next (sibling tasks still in `todo`)

## Step 3 — Internalize, don't dump

DO NOT just paste the briefing back to the user. Instead:

1. **Read** every section.
2. Identify which past tasks/decisions/files are relevant to the user's current request.
3. **Acknowledge briefly** what context you've loaded — one or two sentences max:
   > "Context loaded: NHD-22 (auth) exposes `requireAuth()` middleware which I'll reuse. NHD-23 left an open TODO for refresh tokens — flagging for after this task."
4. Then proceed with the actual implementation work, applying the context naturally.

## Constraints

- ALWAYS use prior decisions and exposed interfaces. Do NOT re-invent things past tasks already built.
- If a hard dependency is `needs_review` or `blocked`, explicitly call this out before depending on it.
- If briefing is empty (first task in project), just note "no prior context" and proceed.
- If `pj` is not on PATH, silently skip.
