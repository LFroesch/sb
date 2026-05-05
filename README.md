# sb

`sb` is a terminal control plane for task-file discovery, brain-dump routing, and agent-run supervision.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/sb/main/install.sh | bash
```

Or:

```bash
go install github.com/LFroesch/sb@latest
go install github.com/LFroesch/sb/cmd/foreman@latest
```

Run with:

```bash
sb
```

## Canonical Task File

`WORK.md` is the default task filename, but any configured task-source filename is allowed if it uses the same schema:

```md
# WORK - <name>
one-line summary

## Current Phase
single plain-text line

## Current Tasks
- active work only

## Backlog / Future Features
- not-now work
```

Rules:
- task files are for active work only
- no `Workflow Rules`, `Unsorted`, `Bugs + Blockers`, `Updates + Features`, or shipped-history sections
- completed implementation history belongs in `DEVLOG.md`
- typed H1 plus the next plain-text line is the only supported title/summary format

## Config

`~/.config/sb/config.json` controls discovery, providers, and Foreman behavior.

Important fields:
- `providers` and `provider`: named LLM profiles and the active profile
- `scan_roots`: recursive task-file roots
- `file_patterns`: allowed task-source basenames such as `WORK.md` or `ROADMAP.md`
- `explicit_paths`: one-off task files that should behave like normal task sources
- `idea_dirs`: flat directories of `.md` files to include directly
- `index_path`: read-only generated discovery index
- `catchall_target` and `ideas_target`: optional non-project routing buckets

`sb` keeps discovery intentionally narrow: only configured task-like markdown should be scanned.

## Behavior

### Discovery

- Startup renders from lightweight discovery first, then hydrates pinned files before the rest.
- The generated `index.md` is a human inspection artifact only. It is written asynchronously and is never used as runtime state.

### Brain Dump

- `d` opens brain dump.
- `ctrl+d` routes the dump through the active model.
- Project items can land only in `Current Tasks` or `Backlog / Future Features`.
- If the model cannot confidently choose a project, it uses `CLARIFY`.

### Cleanup

- `c` cleans the selected file.
- `C` chain-cleans the selected files, or all files when nothing is selected.
- Cleanup rewrites task files into the canonical schema above.
- Review the diff, then accept or reject it.

### Agents

- `a` opens Agents.
- `A` opens the current project directly in the task picker.
- Task-sourced runs remove accepted bullets from the source task file on approval and append shipped details to `DEVLOG.md`.
- `ctrl+t` in New Run toggles immediate start vs Foreman queue.
- Claude and Codex use tmux-backed runs; exec-style engines stay in-app.

## Key Dashboard Keys

| Key | Action |
|---|---|
| `enter` | Open selected task file |
| `e` | Edit selected task file |
| `c` / `C` | Cleanup selected / chain cleanup |
| `d` | Brain dump |
| `a` / `A` | Agents / open current project in task picker |
| `f` | Pin or unpin project |
| `r` | Refresh discovery |
| `,` | Open sb config directory |

## Notes

- `o` opens the selected project directory in your editor using `$VISUAL` or `$EDITOR`.
- Logs are written to `~/.local/share/sb/logs/sb.log`.
- Shared repo workflow rules for Claude and Codex live in [`CLAUDE.md`](CLAUDE.md).
