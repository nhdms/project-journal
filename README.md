# project-journal (`pj`)

A small CLI for journaling tasks across Claude Code (or any single-task)
sessions. Phase 1 is purely manual: there is no LLM, no embeddings, and no
hooks. Each task corresponds to one work session; the tool helps you remember
what previous tasks did when you start new ones.

## Install

```sh
go install github.com/nhduc/project-journal/cmd/pj@latest
```

Or build locally from this directory:

```sh
go build -o pj ./cmd/pj
```

## Quick start

```sh
pj init
pj phase add NHD-6 "Business Onboarding"
pj task add NHD-23 "Business Registration" --phase NHD-6
pj start NHD-23
# ... do work ...
pj finish NHD-23
pj tree
```

## Storage layout

`pj init` creates `.project-journal/` in the current directory:

```
.project-journal/
├── config.json          # {"version": 1, "created_at": "..."}
├── current              # plain text: current task id (or empty)
├── phases.jsonl         # one Phase per line
├── tasks.jsonl          # one Task per line
└── sessions/            # per-task trajectory logs
    └── {task_id}.jsonl
```

All commands except `init` walk up from the cwd to find `.project-journal/`.
If none is found and stdin is interactive, you will be prompted to create one
in the cwd. Pass `--no-prompt` to fail instead of asking.

## Commands (Phase 1)

| Command | Purpose |
| --- | --- |
| `pj init` | Create `.project-journal/` in cwd. |
| `pj phase add <id> <title>` | Add a phase. |
| `pj task add <id> <title> [--phase <id>] [--depends-on a,b,c]` | Add a task with `status=todo`. |
| `pj start <id> [--title <t>] [--phase <id>]` | Mark a task `in_progress`, set it as current, print briefing. Will offer to create the task if missing. |
| `pj finish <id> [--auto]` | End an `in_progress` task. Without `--auto`, prompts for a multi-line summary and a status (`completed` / `partial` / `blocked`). With `--auto`, marks as `needs_review`. |
| `pj log <id> --type <...> [--tool ...] [--content ...] [--input-summary ...] [--output-summary ...] [--first-only]` | Append a trajectory event to `sessions/{id}.jsonl`. Reserved for future hook-driven flows. |
| `pj show <id>` | Print a task or phase. |
| `pj tree` | ASCII tree of phases and tasks with status icons. |
| `pj context [--for <id>]` | Markdown briefing for the current (or specified) task. |
| `pj edit <id>` | Open the task or phase as JSON in `$EDITOR` (default `vi`). |
| `pj status` | High-level summary (counts, current task, last finished). |
| `pj current [--quiet]` | Print the current active task ID. Exits 1 if none. |

### Status values

`todo` · `in_progress` · `completed` · `partial` · `blocked` · `needs_review`

### Briefing structure (`pj context`)

- 🎯 Current Task
- 📦 Phase Goal
- ✅ Completed in this phase (top 5 most-recent siblings)
- 🔗 Hard dependencies
- 🚧 Open TODOs aggregated from completed tasks
- ⏭️ Coming next (sibling todos)

## Notes

- JSONL files are line-delimited JSON. Append-style writes use `O_APPEND`;
  rewrites (e.g., on `finish` or `edit`) write to a temp file and `rename`.
- Timestamps are UTC, formatted with `time.RFC3339Nano`.
- LLM features, embeddings, and hooks are explicitly out of scope for Phase 1.
