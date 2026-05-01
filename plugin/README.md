# project-journal — Claude Code plugin

Per-task journaling for Claude Code. Wraps the `pj` CLI so that every
Claude Code session automatically:

1. **SessionStart** — injects the current task's briefing as additional
   context, so Claude resumes with full task background.
2. **UserPromptSubmit** — captures the *first* user prompt of the
   session as the task's stated intent.
3. **PostToolUse** — logs `Edit | Write | MultiEdit | Bash |
   NotebookEdit` calls into the task's trajectory (filtered, summaries
   truncated to 500 chars).
4. **Stop** — fires `pj finish --auto` in the background, which runs
   LLM induction over the trajectory and proposes a summary + status.
5. **PreCompact** — drops a `compact_marker` event into the trajectory
   right before context compaction.

The plugin is purely a wrapper. All journal state lives in
`.project-journal/` inside each project (created by `pj init`).

---

## Requirements

- `pj` binary in `PATH` (build with `go install ./cmd/pj` from the
  project-journal repo).
- `jq` in `PATH` (used to parse hook JSON input).
- Optional: `OPENAI_API_KEY` for LLM-driven summary induction on
  `pj finish --auto`. Without it, the Stop hook still runs but marks
  the task `needs_review` instead of auto-summarizing.

If either `pj` or `jq` is missing, every hook silently no-ops and the
Claude session is unaffected.

---

## Installation

### Development install (recommended while iterating)

```bash
# 1. Build the pj binary and put it on PATH
cd /path/to/project-journal
go install ./cmd/pj
which pj   # verify it's on PATH

# 2. Load the plugin directly from disk
claude --plugin-dir /path/to/project-journal/plugin
```

### Marketplace install

Distribute via a Claude Code plugin marketplace (see Claude Code docs)
and have users run:

```bash
claude plugin install project-journal
```

---

## Per-project setup

Inside each project where you want a journal:

```bash
cd /path/to/your/project
pj init                       # creates .project-journal/
pj phase add P1 "MVP"         # optional: organize tasks under phases
pj task add T1 "Implement X"  # create a task
pj start T1                   # mark T1 in_progress and set as current
```

Now any `claude` session run from this directory will log into `T1`.

---

## Daily workflow

```bash
# Switch tasks
pj start T2

# Open a Claude Code session — SessionStart hook injects T2's briefing
claude

# ... work happens, hooks log silently ...

# When you're done (or close Claude), the Stop hook spawns
#   `pj finish T2 --auto`
# which runs LLM induction and auto-fills the summary.

# Inspect the result
pj show T2
pj edit T2     # tweak summary / status as JSON
```

### Slash commands

The plugin ships these slash commands (namespaced as
`/project-journal:<name>`):

| Command                       | What it does                                  |
| :---------------------------- | :-------------------------------------------- |
| `/project-journal:pj-status`  | Run `pj status` — high-level journal summary  |
| `/project-journal:pj-tree`    | Run `pj tree` — ASCII tree of phases + tasks  |
| `/project-journal:pj-current` | Print the current active task ID              |
| `/project-journal:pj-context` | Render briefing for current task; pass an ID to render a specific task |

---

## Logs and debugging

All hook errors are appended to `~/.project-journal-hook.log`. Hooks
*never* write to stderr (which would leak into the Claude UI), and
they always exit 0 — they cannot break a Claude session.

Useful debugging commands:

```bash
tail -f ~/.project-journal-hook.log
pj current --quiet            # what task are hooks logging into?
cat .project-journal/sessions/<TASK_ID>.jsonl | tail -20
```

If you suspect a hook is misfiring, run a single hook manually:

```bash
echo '{"cwd":"'"$PWD"'","tool_name":"Bash","tool_input":{"command":"ls"},"tool_result":"ok"}' \
  | bash /path/to/plugin/hooks/post-tool-use.sh
```

---

## Disabling

The plugin no-ops automatically when:

- `pj` is not on `PATH`, **or**
- `jq` is not on `PATH`, **or**
- the project has no current task (`pj current --quiet` returns empty),
  which is the case before `pj init` or before you `pj start` a task.

To disable globally, uninstall the plugin or remove the `--plugin-dir`
flag.

---

## Files

```
plugin/
├── .claude-plugin/
│   └── plugin.json              # plugin manifest
├── README.md                    # this file
├── hooks/
│   ├── hooks.json               # hook event registration
│   ├── _common.sh               # shared preamble (sourced by all hooks)
│   ├── session-start.sh         # SessionStart   → pj context
│   ├── user-prompt-submit.sh    # UserPromptSubmit → pj log --first-only
│   ├── post-tool-use.sh         # PostToolUse    → pj log --type tool_use
│   ├── stop.sh                  # Stop           → pj finish --auto (bg)
│   └── pre-compact.sh           # PreCompact     → pj log --type compact_marker
└── commands/
    ├── pj-status.md
    ├── pj-tree.md
    ├── pj-context.md
    └── pj-current.md
```

---

## Constraints / non-goals

- Bash only (no Python/Node) so the runtime dependency surface stays at
  `pj + jq + bash`.
- No marketplace files in this directory — distribution is via either
  `--plugin-dir` or a separate marketplace repo.
- The plugin never modifies Phase 1/2 Go code. It is a pure wrapper.
