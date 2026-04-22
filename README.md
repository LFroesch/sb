# sb

Second Brain control plane. TUI for managing WORK.md files across projects — brain dump ideas and clean up backlogs with a local LLM.

## Quick Install

Supported platforms: Linux and macOS. On Windows, use WSL.

Recommended (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/sb/main/install.sh | bash
```

Or download a binary from [GitHub Releases](https://github.com/LFroesch/sb/releases).

Or install with Go:

```bash
go install github.com/LFroesch/sb@latest
```

Or build from source:

```bash
make install
```

Command:

```bash
sb
```

## Config

`~/.config/sb/config.json` is created on first run:

```json
{
  "provider": "ollama",
  "providers": {
    "ollama": {
      "type": "ollama",
      "model": "qwen2.5:7b",
      "base_url": "http://localhost:11434"
    }
  },
  "scan_roots": [
    { "name": "projects", "path": "~/projects" }
  ],
  "file_patterns": ["WORK.md"],
  "idea_dirs": [],
  "label_max_depth": 2,
  "index_path": "~/.config/sb/index.md",
  "log_level": "info",
  "catchall_target": null,
  "ideas_target": null
}
```

| Field | Purpose |
|-------|---------|
| `provider` | Active provider profile name from the `providers` map. |
| `providers` | Named LLM profiles. Each entry supports `type`, `model`, `base_url`, `api_key`, `api_key_env`, and optional extra `headers`. |
| `scan_roots` | Recursive discovery roots. `name` is used only when sb needs extra label context. |
| `file_patterns` | Filenames sb treats as projects (e.g. add `ROADMAP.md`). |
| `idea_dirs` | Flat dirs whose `.md` files are loaded directly (no recursion). |
| `label_max_depth` | Fallback label depth: keep the last N path components when no title label is present. |
| `index_path` | Auto-regenerated routing-context cache (see below). |
| `log_level` | JSON log verbosity for `~/.local/share/sb/logs/sb.log` (`debug`, `info`, `warn`, `error`). |
| `catchall_target` | `{ "name": "...", "path": "..." }` — optional bucket for general notes that don't belong to a project. |
| `ideas_target` | Same shape — optional bucket for project-less ideas. |

Press `,` inside sb to open the config in your editor.

Use `api_key_env` when possible so secrets stay out of the config file. Existing top-level `model` and `ollama_host` fields are still honored as a compatibility shim for older configs.

Env overrides:

- `SB_PROVIDER` selects the active named profile.
- `SB_MODEL`, `SB_BASE_URL`, `SB_API_KEY`, `SB_API_KEY_ENV`, and `SB_PROVIDER_TYPE` override the active profile.
- `OLLAMA_HOST` still overrides the active profile's base URL when the provider type is `ollama`.

Example multi-provider setup:

```json
{
  "provider": "openai",
  "providers": {
    "ollama": {
      "type": "ollama",
      "model": "qwen2.5:7b",
      "base_url": "http://localhost:11434"
    },
    "openai": {
      "type": "openai",
      "model": "your-openai-model",
      "base_url": "https://api.openai.com/v1",
      "api_key_env": "OPENAI_API_KEY"
    },
    "anthropic": {
      "type": "anthropic",
      "model": "your-anthropic-model",
      "base_url": "https://api.anthropic.com",
      "api_key_env": "ANTHROPIC_API_KEY"
    }
  }
}
```

The `o` key opens project directories in your editor — set `$VISUAL` (GUI) or `$EDITOR` (terminal). Falls back to probing for cursor, code, nvim, vim, nano.

```bash
export EDITOR=nvim      # terminal editor
export VISUAL=cursor    # GUI editor (checked first)
```

The header now shows the active LLM profile and model, for example `llm=openai:gpt-4.1-mini`. If the selected provider is incomplete, sb shows a warning like `llm disabled (missing OPENAI_API_KEY)` instead of leaving the active backend ambiguous.

### Project labels and descriptions

Any discovered markdown file can define both its dashboard label and routing description from the first H1:

```markdown
# WORK - sb | Second Brain TUI for managing WORK.md files across projects
# ROADMAP - toolkit | v1 polish
```

`label` overrides the left-panel project name. `description` feeds the router and index. If there is no title label, sb falls back to the shortest useful relative path within the scan root, expanding only when needed to resolve collisions.

If two different roots still collide after path expansion, sb prefixes the fallback label with the root name, e.g. `work/api` and `client/api`.

### Index file

On every startup sb writes `~/.config/sb/index.md` — a human-readable list of every discovered project + its description, plus the configured special targets. It's a **read-only artifact** for inspecting what the active model sees during routing. Edits get overwritten on the next startup; change the markdown title line to update a label or description.

### Logging

sb now logs structured JSON to `~/.local/share/sb/logs/sb.log` and rotates at roughly 5 MiB, keeping 3 backups. The old `/tmp/sb-*.log` files are no longer used.

---

## Workflows

### Brain Dump (`d`)

Offload thoughts without deciding where they go. sb asks the active LLM provider to classify each item and route it to the right project's WORK.md.

1. Press `d` from the dashboard
2. Type anything — a task, idea, or note (multi-line ok)
3. `ctrl+d` to route — the active model splits and classifies each item
4. Step through items one by one:
   - `y`/`enter` — accept, write to target project
   - `n` — skip item
   - `r` — reroute: type a hint and the model re-classifies the item
   - `esc` — abort remaining items (already accepted ones are kept)
5. If the model is unsure about a project, a clarify prompt appears automatically — type a hint and press `enter` to reroute, or `esc` to skip

After stepping through all items, a summary shows accepted vs skipped.

### Cleanup (`c` / `C`)

Normalize and condense a WORK.md file via the active LLM provider — removes duplicates, fixes formatting, collapses redundant entries.

- `c` — clean up the currently selected/viewed project
- `C` — chain cleanup: runs on all selected projects (use `space` to select, or all if none selected)

After the model runs, you see a diff. Press `y` to accept, `n` to reject, or `f` to give feedback and regenerate.

### Daily Plan (`P`)

Select projects with `space`, then press `P`. The active model reads their tasks and generates a prioritized daily plan.

### Next Todo (`t`)

Press `t` on any project — the active model reads the WORK.md and tells you the single best thing to work on next.

### Fix non-list lines (`-`)

Scans WORK.md for lines that aren't proper list items and fixes them in-place. Useful after messy manual edits.

### Agent Cockpit (Agent tab)

Launch coding agents against `- ` items from your WORK.md files. See [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) for the full design.

From the dashboard, switch to the **Agent** tab:

1. `n` — new job sourced from a WORK.md task · `N` — freeform chat (no sources, defaults to the current project's repo or the current working directory)
2. Step 1 (sourced): pick a file (`enter`)
3. Step 2 (sourced): `space` toggles items, `enter` continues when at least one is selected
4. Launch modal: `tab` cycles focus between **preset**, **provider**, and the brief editor. `↑/↓` moves within the focused group. `enter` launches from either picker; when the brief is focused, `alt+enter` launches.
5. The jobs screen is now a real cockpit: the list shows total/live/running/attention counts plus session usage grouped by provider/model, so you can see at a glance which Claude/Codex/Ollama models are actually in use. `tab` cycles job filters, or press `1-5` for `all/live/running/attention/done`.
6. Claude and Codex jobs now run in real `tmux` windows under the isolated `sb-cockpit` session. Launching one auto-attaches into the native CLI instead of trying to emulate its UI inside Bubble Tea.
7. `enter` or `i` on a live tmux-backed job switches the client into that job's window. Use `F1`, `Ctrl+g`, `F12`, or `Ctrl+C` to jump back to the `sb` window. `Ctrl+C` is forwarded normally only when you're already on the `sb` window itself.
8. Finished tmux jobs open an in-app log/review view instead of trying to fake a live chat. The detail pane also shows whether the tmux window is still live or already closed.
9. Ollama and shell jobs stay on the exec-per-turn path. Those still use the attached chat view inside `sb`, including the sessions rail and `[` / `]` quick switching.
10. `q` from the dashboard detaches the current `tmux` client instead of tearing down the cockpit session. `x` is an explicit detach shortcut from the Agent UI. Jobs and the `sb-cockpit` session keep running; relaunching `sb` reattaches to the same session.
11. `s` stops the selected job. For tmux jobs that sends `C-c` and then closes the job window; exec jobs cancel the in-flight turn and return to `idle` with note `stopped`.
12. `a` asks for confirmation, then approves the conversation — the selected source lines are removed from their file and a dated entry is appended to the project's `DEVLOG.md`. Approve also runs post-shell hooks and ends the conversation.
13. `d` asks for confirmation before deleting a job.

**Presets** describe the *role* (persona, system prompt, hooks, iteration). **Providers** describe the *executor* (claude CLI, codex CLI, ollama model, shell). Each preset carries a suggested provider; the launch modal lets you override with any loaded provider — so you can drive the `senior-dev` role with Claude, Codex, or a local Ollama model interchangeably.

Seed **presets** materialise in `~/.config/sb/presets/` on first run: `senior-dev`, `bug-fixer`, `test-writer`, `refactor`, `code-analyzer`, `explainer`, `pm`, `docs-writer`, `scaffold`, `rfc`, `docs-tidy`, `classify`, `summarize`, plus shell-specific `shell-test`, `shell-lint`, `shell-build`, `shell-escape`.

Seed **providers** materialise in `~/.config/sb/providers/` on first run: `claude`, `codex`, `ollama-qwen`, `ollama-llama`, `ollama-gemma`, `shell`.

Edit any `*.json` in those dirs to customise. Each preset supports pre/post shell hooks, prompt-template injection, and role labels; see the RFC for the full schema.
From the Agent page, `p` creates a preset template and opens it in your editor, `v` does the same for a provider template, and `P` / `V` open the presets/providers directories directly.

Older preset files that still contain legacy executor args like Claude `--print` or Codex `exec` / `--json` are normalized at runtime, so they continue to work after the tmux split.

### Daemon (sb-foreman)

The cockpit runs in a small daemon (`sb-foreman`) that owns job state. sb dials it over a unix socket at `~/.local/state/sb/foreman.sock`, so running jobs survive sb quits and restarts. Claude/Codex jobs are tracked as `tmux` windows; Ollama/shell jobs stay on the short-lived `exec.Cmd` path.

When the daemon restarts, tmux-backed jobs are rehydrated from their persisted `TmuxTarget` and reconciled against the live `sb-cockpit` session instead of being marked failed immediately. That lets interactive Claude/Codex work keep running even if `sb` or `sb-foreman` was not up for a while.

- `go build ./cmd/foreman` to build the binary; put it on your `PATH` (or set `cockpit_foreman_bin` in `config.json`).
- sb auto-starts the daemon on launch if nothing is listening on the socket.
- Set `"cockpit_daemon": false` in `config.json` to force the pre-daemon in-process mode.

Job state is persisted under `~/.local/state/sb/jobs/<id>/`, rehydrated on every daemon start.

---

## Navigation

| Key | Action |
|-----|--------|
| `j/k` | Navigate |
| `enter` | Open project |
| `e` | Edit WORK.md inline |
| `d` | Brain dump |
| `a` | Agent cockpit |
| `c` / `C` | Cleanup (single / chain) |
| `P` | Daily plan |
| `t` | Next todo |
| `/` | Search across all WORK.md files |
| `f` | Pin/unpin project |
| `space` | Toggle project selection |
| `o` | Open project dir in editor |
| `r` | Refresh |
| `,` | Open `config.json` in editor |
| `?` | Help |

Agent tab:

| Key | Action |
|-----|--------|
| `n` | New launch (pick file → tasks → preset) |
| `N` | Freeform launch |
| `1-5` | Filter jobs by `all/live/running/attention/done` |
| `tab` | List: cycle filters · Launch modal: cycle preset → provider → brief · Attached exec-chat: swap transcript ↔ input |
| `p` / `v` | Create preset / provider template and open it |
| `P` / `V` | Open presets / providers directory |
| `space` | Toggle task in picker |
| `i` | List: open selected job (`tmux` attach while live, input focus for exec-chat jobs) · Attached exec-chat: focus input |
| `enter` | Launch (from preset/provider picker) · open selected job (`tmux` attach while live, log review when finished, chat for exec jobs) · send when input-focused |
| `alt+enter` | Launch from brief |
| `j/k` | Scroll transcript or tmux log in the attached view |
| `[` / `]` | Attached view: previous / next job |
| `a` | Approve (confirm, then sync-back) |
| `s` | Stop running job |
| `r` | Retry |
| `d` | Delete job (confirm) |
| `x` | Detach cockpit client immediately when running inside `sb-cockpit` |
| `q` | Detach cockpit client when running inside `sb-cockpit`; otherwise quit/go back |
| `esc` | Back (or leave input focus when typing) |

Full keybind reference is available in-app with `?`.

---

## Design Docs

- [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) — product direction for turning `sb` into the first cockpit UI for coding-agent orchestration, with a separable foreman/runtime layer behind it

## License

[AGPL-3.0](LICENSE)
