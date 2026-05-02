# project-journal (`pj`)

Per-task journaling tool that gives Claude Code (or any coding workflow) **cross-session memory**. Each task = 1 work session; `pj` records what happened so the next task starts with context, not a blank slate.

Ships with a Claude Code plugin that auto-injects briefings, logs trajectories, and induces summaries via LLM.

## Install

### Linux / VPS — one-liner

```sh
curl -fsSL https://raw.githubusercontent.com/nhdms/project-journal/main/scripts/install.sh | sh
```

Requirements (script will check):
- Go 1.22+ (`sudo apt install -y golang-go` on Ubuntu/Debian)
- `jq` (optional, only needed for the Claude Code plugin)

### Manual install (any OS with Go)

```sh
go install github.com/nhdms/project-journal/cmd/pj@latest

# Add Go bin to PATH if not already
export PATH="$(go env GOPATH)/bin:$PATH"
pj --help
```

### Build from source

```sh
git clone https://github.com/nhdms/project-journal.git
cd project-journal
go build -o pj ./cmd/pj
sudo mv pj /usr/local/bin/   # or any dir on PATH
```

## Quick start

```sh
cd ~/your-project
pj init
pj phase add NHD-6 "Onboarding"               # optional grouping
pj task add NHD-23 "Biz registration" --phase NHD-6 --depends-on NHD-22
pj start NHD-23                                # prints briefing, marks active
# ... do work, log events ...
pj finish NHD-23                               # interactive summary
pj tree
```

With `OPENAI_API_KEY` set, `pj finish` auto-induces summary + autoeval status:

```sh
export OPENAI_API_KEY="sk-proj-..."
pj finish NHD-23 --auto                        # LLM proposes everything
```

## Claude Code plugin

The `plugin/` subfolder is a Claude Code plugin that wires `pj` into Claude sessions automatically.

Install:

```
/plugin marketplace add nhdms/project-journal
/plugin install project-journal@project-journal-local
```

Then per project:

```sh
cd ~/your-project && pj init
pj task add T1 "First task"
pj start T1
claude   # SessionStart hook injects briefing; Stop hook auto-induces summary
```

See [`plugin/README.md`](plugin/README.md) for full plugin docs.

## Storage layout

`pj init` creates `.project-journal/` in cwd:

```
.project-journal/
├── config.json         # version + created_at
├── current             # active task ID
├── phases.jsonl        # one Phase per line
├── tasks.jsonl         # one Task per line
├── embeddings.jsonl    # OpenAI embeddings cache (Phase 2)
└── sessions/           # per-task trajectory logs
    └── {task_id}.jsonl
```

All commands except `init` walk up from cwd to find `.project-journal/`. Pass `--no-prompt` to fail instead of prompting.

## Commands

| Command | Purpose |
| --- | --- |
| `pj init` | Create `.project-journal/` in cwd. |
| `pj phase add <id> <title>` | Add a phase. |
| `pj task add <id> <title> [--phase <id>] [--depends-on a,b,c]` | Add a task with `status=todo`. |
| `pj start <id> [--title <t>] [--phase <id>]` | Mark `in_progress`, set as current, print briefing. Lazy-creates if missing. |
| `pj finish <id> [--auto]` | End task. With `OPENAI_API_KEY`, LLM proposes summary + autoeval status. Without key OR `--auto` without key: marks `needs_review`. |
| `pj induce <id>` | Re-run LLM induction on a finished task. |
| `pj reindex [--force]` | (Re)build embeddings for all finished tasks. |
| `pj log <id> --type <...> [flags]` | Append trajectory event. Used by plugin hooks. |
| `pj show <id>` | Print task/phase detail. |
| `pj tree` | ASCII tree with status icons. |
| `pj context [--for <id>]` | Markdown briefing (deps + relevant past tasks via embeddings). |
| `pj edit <id>` | Open task/phase in `$EDITOR`. |
| `pj status` | Stats overview. |
| `pj current [--quiet]` | Print active task ID. Exits 1 if none. |

### Status values

`todo` · `in_progress` · `completed` · `partial` · `blocked` · `needs_review`

### Briefing structure (`pj context`)

- 🎯 Current Task
- 📦 Phase Goal
- 🔗 Hard dependencies (with summaries)
- ✨ Relevant past tasks (top 5, blended scoring: cosine + dep boost + same-phase + recency)
- ⏭️ Coming next (sibling todos)

## Configuration

Environment variables (all optional except `OPENAI_API_KEY` for LLM features):

| Var | Default | Purpose |
|-----|---------|---------|
| `OPENAI_API_KEY` | — | Required for LLM induce, autoeval, embeddings. |
| `OPENAI_CHAT_MODEL` | `gpt-4o-mini` | Chat completion model. |
| `OPENAI_EMBED_MODEL` | `text-embedding-3-small` | Embedding model. |
| `OPENAI_TIMEOUT_SECONDS` | `60` | Per-call timeout. |
| `OPENAI_BASE_URL` | OpenAI default | Override for proxies / Azure. |

## Storage notes

- JSONL: line-delimited JSON. Append uses `O_APPEND`; rewrites use temp + atomic rename.
- Timestamps UTC, `time.RFC3339Nano`.
- `embeddings.jsonl` contains task vectors; safe to delete (regenerate via `pj reindex`).

## License

MIT
