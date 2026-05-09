# sb

`sb` is a terminal control plane for `WORK.md`-style project management. It is built around task-file cleanup, routing brain dumps into the right project, and launching agent-backed work from that task data.

## Install

Recommended:

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/sb/main/install.sh | bash
```

Or install with Go:

```bash
go install github.com/LFroesch/sb@latest
go install github.com/LFroesch/sb/cmd/foreman@latest
```

Run:

```bash
sb
sb --version
```

## Main Jobs

| Area | Purpose |
|------|---------|
| Dashboard | Browse discovered task files, clean them up, and jump into other flows |
| Dump | Turn a rough brain dump into routed task bullets |
| Agents | Start task-backed or freeform coding-agent runs |

If `tmux` is available, `sb` uses a shared cockpit session for the richer agent workflow. Without `tmux`, the TUI still works, but the agent flow is more limited.

## Canonical Task File Shape

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

The important rules are simple:

- task files are for active work only
- shipped history belongs in `DEVLOG.md`
- cleanup rewrites files into the canonical shape instead of preserving ad hoc sections

## Useful Commands

```bash
sb
sb tmux-status
sb audit-taskfiles
sb account list
sb account show
sb account save claude work
sb account use codex personal
```

## Config

Config is read from `~/.config/sb/config.json`.

Common fields:

- `scan_roots`
- `file_patterns`
- `explicit_paths`
- `idea_dirs`
- `catchall_target`
- `ideas_target`
- `provider` and `providers`

## Notes

- saved account snapshots live under `~/.config/sb/accounts/`
- logs are written under your user data dir, usually `~/.local/share/sb/logs/`
- repo-local workflow rules for a checkout can live in `AGENTS.md`

## License

[AGPL-3.0](LICENSE)
