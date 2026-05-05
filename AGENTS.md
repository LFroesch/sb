# Local Codex Instructions

## WORK.md Format
- `WORK.md` is for active work only. Do not use it as a changelog, release log, or narrative scratchpad.
- Keep `WORK.md` concise and cleanly formatted markdown.
- Default shape is:
  - H1 title: `# WORK - <name>`
  - one-line plain-text summary
  - `## Current Phase`
  - `## Current Tasks`
  - `## Backlog / Future Features`
- Keep exactly those three `##` sections unless the user explicitly wants another section for a real feature area.
- Do not add “Done”, “Completed”, “Recent Changes”, dated notes, test logs, or devlog-style writeups to `WORK.md`.
- When work finishes, remove it from current tasks or rewrite the remaining follow-up. Put shipped details in `DEVLOG.md`.
- Repo workflow/agent instructions belong in [`CLAUDE.md`](CLAUDE.md), not in `WORK.md`.

## Reading Docs
- Do not pre-read `WORK.md`, `README.md`, or other project docs by reflex.
- Read project docs only when the task needs project context, status, architecture, workflow, or ambiguity resolution.
- For one-shot edits, focused code changes, or direct questions, start from the relevant files instead of sweeping docs first.

## Doc Separation
- `WORK.md` tracks active tasks and backlog.
- `DEVLOG.md` holds dated implementation history.
- `README.md` explains user-facing behavior and usage.
