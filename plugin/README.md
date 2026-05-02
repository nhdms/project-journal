# project-journal — Claude Code plugin

Per-task journaling for Claude Code. Wraps the `pj` CLI so every Claude
Code session automatically picks up where you left off, logs what
happened, and induces a summary at the end.

It is the missing **cross-session memory** layer for multi-task projects:
ship a task, walk away, come back two days later, and Claude resumes
with full briefing on phase goals, hard dependencies, and the top-5 most
relevant prior tasks.

---

## Overview

The plugin is a thin wrapper around the `pj` CLI. It registers five
hooks and ships five user-invokable skills plus one validation agent.

| Component | Type | What it does |
|-----------|------|--------------|
| `session-start.sh` | SessionStart hook | Injects current task briefing as `additionalContext` |
| `user-prompt-submit.sh` | UserPromptSubmit hook | Logs the FIRST user prompt as task intent |
| `post-tool-use.sh` | PostToolUse hook | Logs `Edit\|Write\|MultiEdit\|Bash\|NotebookEdit` calls |
| `stop.sh` | Stop hook | Backgrounds `pj finish --auto` for LLM induction |
| `pre-compact.sh` | PreCompact hook | Drops a `compact_marker` into trajectory |
| `pj-status` | Skill | Run `pj status` |
| `pj-tree` | Skill | Run `pj tree` |
| `pj-current` | Skill | Run `pj current` |
| `pj-context` | Skill | Run `pj context [task_id]` |
| `pj-review` | Skill | Quality-check a task summary via the validator agent |
| `journal-validator` | Agent | Reviews induced summary against raw trajectory |

All journal state lives in `.project-journal/` inside each project
(created by `pj init`). The plugin itself stores no state.

---

## Prerequisites

- **Go 1.21+** — to build the `pj` binary
- **`jq`** — used by hooks to parse JSON stdin payloads
- **`bash`** — hooks are pure POSIX-ish bash
- *(optional)* **`OPENAI_API_KEY`** — enables LLM summary induction on `pj finish --auto`. Without it, the Stop hook still runs but marks the task `needs_review` instead of auto-summarizing.

If either `pj` or `jq` is missing, every hook silently no-ops — Claude
sessions are unaffected.

---

## Installation

### 1. Build and install the `pj` CLI

```bash
git clone https://github.com/nhdms/project-journal.git
cd project-journal
go install ./cmd/pj
```

Make sure `$HOME/go/bin` is on your `PATH`:

```bash
# In ~/.zshrc or ~/.bashrc
export PATH="$HOME/go/bin:$PATH"
```

Verify:

```bash
pj --version
which pj
```

### 2. Install the plugin

**Via marketplace (recommended):**

```
/plugin marketplace add nhdms/project-journal
/plugin install project-journal@project-journal-local
```

**Via local marketplace (development):**

```
/plugin marketplace add /path/to/project-journal
/plugin install project-journal@project-journal-local
```

---

## Per-project setup

```bash
cd ~/your-project
pj init                            # creates .project-journal/
pj phase add NHD-6 "Onboarding"    # optional: organize tasks under phases
pj task add T1 "First task" --phase NHD-6
pj start T1                        # mark T1 in_progress and set as current
```

Now any `claude` session run from this directory automatically logs into
`T1` and starts with `T1`'s briefing pre-loaded.

---

## Workflow

```
┌─────────────────────────────────────────────────────────────┐
│  pj start T2                                                │
│           │                                                 │
│           ▼                                                 │
│  claude  ──── SessionStart ──► injects briefing for T2      │
│           │                                                 │
│           │   first prompt ──► user_prompt event            │
│           │                                                 │
│           │   Edit/Write/Bash ──► tool_use events           │
│           │                                                 │
│           │   /compact ──► compact_marker event             │
│           │                                                 │
│           │   exit ──► Stop ──► pj finish T2 --auto (bg)    │
│           ▼                                                 │
│  pj show T2                                                 │
│  /project-journal:pj-review T2  (optional quality check)    │
└─────────────────────────────────────────────────────────────┘
```

### Daily commands

```bash
# Switch tasks
pj start T2

# Open Claude — SessionStart hook injects T2's briefing
claude

# ... work happens, hooks log silently ...

# Stop hook spawns `pj finish T2 --auto` (LLM induction in background)

# Inspect
pj show T2
pj edit T2     # tweak summary / status as JSON
```

### Slash commands (skills)

| Command                       | What it does                                                     |
| :---------------------------- | :--------------------------------------------------------------- |
| `/project-journal:pj-status`  | High-level journal summary (counts, current task)                |
| `/project-journal:pj-tree`    | ASCII tree of phases + tasks with status icons                   |
| `/project-journal:pj-current` | Print the current active task ID                                 |
| `/project-journal:pj-context` | Render briefing for current task; pass an ID for a specific task |
| `/project-journal:pj-review`  | Score a finished task's summary and propose improvements         |
| `/project-journal:pj-init`    | Init journal in current dir, optionally create + start a task    |

## Auto-invoked skills (no slash needed)

These skills trigger automatically based on context — Claude Code activates them when their description matches what you're doing.

| Skill | Auto-triggers when... |
|-------|----------------------|
| `journal-bootstrap` | You start describing work ("let's build X") and no journal task is active. Offers to init/create a task. |
| `journal-recall` | You're about to implement code in a journaled project. Pulls briefing with deps + relevant past tasks. |
| `journal-checkpoint` | A milestone is reached or risky operation imminent. Silently logs a checkpoint event for the induce summary. |

You can disable any skill by removing its directory or by setting `disable-model-invocation: true` in its frontmatter.

---

## Configuration

Environment variables read by `pj`:

| Variable | Default | Purpose |
|----------|---------|---------|
| `OPENAI_API_KEY` | — | Enables LLM induction on `pj finish --auto` |
| `OPENAI_CHAT_MODEL` | `gpt-4o-mini` | Model used for summary induction |
| `OPENAI_EMBED_MODEL` | `text-embedding-3-small` | Model used to embed tasks for relevance retrieval |

---

## Logs and debugging

All hook errors are appended to `~/.project-journal-hook.log`. Hooks
*never* write to stderr (which would leak into the Claude UI), and
they always exit 0 — they cannot break a Claude session.

```bash
tail -f ~/.project-journal-hook.log
pj current --quiet            # what task are hooks logging into?
cat .project-journal/sessions/<TASK_ID>.jsonl | tail -20
```

Run a single hook manually for diagnosis:

```bash
echo '{"cwd":"'"$PWD"'","tool_name":"Bash","tool_input":{"command":"ls"},"tool_result":"ok"}' \
  | bash ${CLAUDE_PLUGIN_ROOT}/hooks/post-tool-use.sh
```

---

## Disabling

The plugin no-ops automatically when:

- `pj` is not on `PATH`, **or**
- `jq` is not on `PATH`, **or**
- the project has no current task (`pj current --quiet` returns empty),
  which is the case before `pj init` or before you `pj start` a task.

To disable globally:

```
/plugin disable project-journal
```

Or uninstall:

```
/plugin uninstall project-journal
```

---

## Cost

LLM induction on `pj finish --auto` runs once per task with `gpt-4o-mini`
by default. Typical cost is **~$0.001–0.005 per finished task** depending
on trajectory length. Embedding cost on `pj task add` is negligible
(~$0.00001 per task).

Without `OPENAI_API_KEY`, no LLM calls are made — the Stop hook just
marks the task `needs_review` and exits.

---

## File structure

```
plugin/
├── .claude-plugin/
│   └── plugin.json
├── README.md
├── hooks/
│   ├── hooks.json
│   ├── _common.sh
│   ├── session-start.sh
│   ├── user-prompt-submit.sh
│   ├── post-tool-use.sh
│   ├── stop.sh
│   └── pre-compact.sh
├── skills/
│   ├── pj-status/SKILL.md
│   ├── pj-tree/SKILL.md
│   ├── pj-current/SKILL.md
│   ├── pj-context/SKILL.md
│   └── pj-review/SKILL.md
└── agents/
    └── journal-validator.md
```

---

## Constraints / non-goals

- Bash only (no Python/Node) so the runtime dependency surface stays at
  `pj + jq + bash`.
- The plugin never modifies Phase 1/2 Go code in `cmd/` or `internal/`.
  It is a pure wrapper.
- No marketplace files inside this directory — distribution is via the
  top-level `.claude-plugin/marketplace.json`.
