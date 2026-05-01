# sb

Second Brain control plane. `sb` is a terminal app for managing `WORK.md`-style project backlogs, sorting brain dumps, launching/supervising coding agents against real tasks, and running unattended Foreman batches when you are away.

For the intended product shape, see [docs/product-definition.md](docs/product-definition.md). For the lower-level Agent architecture, see [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md).

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

Project structure:

- `main.go` is the small CLI entrypoint and subcommand dispatch layer.
- `internal/tui/` contains the Bubble Tea application (`tui.Run()` plus the `model`/`update`/`view` split).
- `cmd/foreman/` contains the `sb-foreman` daemon used by the Agents workflow.

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

Press `,` inside sb to open the main `sb` config directory in your editor.

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
export VISUAL=code      # GUI editor (checked first)
```

The header now shows the active LLM profile and model, for example `llm=openai:gpt-4.1-mini`. If the selected provider is incomplete, sb shows a warning like `llm disabled (missing OPENAI_API_KEY)` instead of leaving the active backend ambiguous.

### Project labels and descriptions

Any discovered markdown file can define both its dashboard label and routing description from the top preamble:

```markdown
# WORK - sb
Second Brain TUI for managing backlog markdown files across projects

# ROADMAP - toolkit
v1 polish and follow-up milestones
```

The typed H1 (`# WORK - ...`, `# ROADMAP - ...`, etc.) sets the label. The first meaningful plain-text line below it becomes the short description used for routing and the index. Legacy inline metadata like `# WORK - sb | description` still works, but the preferred format is typed H1 plus the next-line summary.

For local Ollama use, sb keeps routing context compact: each project contributes only its label, summary, optional current phase, and at most a couple of active-task preview bullets instead of the full markdown body.

If there is no usable title label, sb falls back to the shortest useful relative path within the scan root, expanding only when needed to resolve collisions.

If two different roots still collide after path expansion, sb prefixes the fallback label with the root name, e.g. `work/api` and `client/api`.

### Index file

On every startup sb writes `~/.config/sb/index.md` — a human-readable inventory of discovered projects, summaries, phase/task hints, and the active discovery config (`scan_roots`, `file_patterns`, `idea_dirs`), plus the configured special targets. It's a **read-only artifact** for inspecting routing context. Edits get overwritten on the next startup; update the source markdown file instead.

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

Lightly tidy a WORK.md file via the active LLM provider — removes obvious duplicates, fixes malformed bullets/tables, and preserves your existing headers/layout unless a move is clearly correct.

- `c` — clean up the currently selected/viewed project
- `C` — chain cleanup: runs on all selected projects (use `space` to select, or all if none selected)

After the model runs, you see a diff. Press `y` to accept, `n` to reject, or `f` to give feedback and regenerate.

### Daily Plan (`P`)

Select projects with `space`, then press `P`. The active model reads their tasks and generates a prioritized daily plan.

### Next Todo (`t`)

Press `t` on any project — the active model reads the WORK.md and tells you the single best thing to work on next.

### Fix non-list lines (`-`)

Scans your project .md files for lines that aren't proper list items and fixes them in-place. Useful after messy manual edits.

### Agents (Agents tab)

Launch coding agents against `- ` items from your `WORK.md` files. The core flow is: choose a task, start a run, monitor it, review the result, and accept it back into your task system. See [docs/product-definition.md](docs/product-definition.md) for the product model and [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) for the lower-level architecture.

From the dashboard, switch to the **Agents** tab:

1. `n` opens the picker. Row 0 is `★ New run without task source` — select it to start without attaching task lines (lands on the Repo tab so you pick where to run). Selecting any project below it goes through the normal task-picker flow.
   From the dashboard, `A` jumps straight into the current project's task picker.
2. Step 1: pick a file with `enter` (or pick the freeform sentinel)
3. Step 2 (sourced only): `space` toggles task items, `enter` continues when at least one is selected, `b` or `esc` returns to the file list with a clean selection state
4. The new-run composer stays as small as possible. Task-backed runs cycle through **Role**, **Engine**, **Note**, and **Review** because the repo already comes from the selected task. Runs without a task source add an explicit **Repo** step where you can pick a discovered project, the cwd, or `(custom path…)` to type any absolute path. The repo list stays in a stable order while you scroll; `(custom path…)` is always row 1, but the initial cursor starts on row 2 so the normal repo remains the default and one `↑` opens the custom-path route. `↑/↓` moves within the focused list or review pane, `enter` on **Repo** confirms the selected repo and advances to **Note** (or opens the custom-path editor), `enter` on the other non-note tabs launches, and `alt+enter` launches from the note editor.
   While the custom-path editor is open, the field is kept visible ahead of the repo list and its width stays clamped to the pane so you can still see what you are typing on shorter or narrower terminals.
   While a textarea is focused, typing `?` stays in that textarea instead of toggling help.
   Press `F` in the composer to switch between **start now** and **send to Foreman**.
   On shorter terminals, the composer now stays within the visible body area instead of pushing the global header/footer off-screen; longer role/review content scrolls inside the composer.
   Scroll positions are also clamped to the last real screenful now, so paging past the end of review/setup/detail panes should not leave you stuck "below" the visible content.
5. New runs default to the `senior-dev` role with the `codex` engine when those profiles exist. Multi-task launches can stay bundled or become a queued run sequence, depending on the selected role.
6. The Agents tab is the main supervision surface: the left pane shows repo/task/queue/status/role, and the right pane shows a live peek into the selected transcript or tmux log.
   The list uses stable columns and compacts multiline task text to a single line so scanning multiple runs stays readable.
   Queued runs also show explicit progress (`solo`, `1/2 active`, `2/2 next`, `1/3 review`, etc.) without forcing you into the detail pane first.
   `pgup/pgdn` scroll that right-side peek directly from the jobs list.
   Local control copy now stays in the footer instead of being repeated inside each body pane, so picker / new-run / attached / setup content areas stay focused on the actual run state and content.
   That right-hand detail pane is now intentionally compact: task, repo/session state, review risk, queue-next context, and the most useful output tail first, rather than a full inline review transcript.
7. `f` or `tab` cycles the run filters (`all/live/running/attention/foreman/done`). The header also shows provider/model session mix, Foreman on/off state, and Claude/Codex limit rows.
8. Claude and Codex runs use real `tmux` windows under the shared `sb-cockpit` session. Launching one auto-attaches into the native CLI instead of trying to fake it inside Bubble Tea.
9. `enter` or `i` on a live tmux-backed run switches the client into that run's window. Use `F1`, `Ctrl+g`, `F12`, or `Ctrl+C` to jump back to the shared `sb` `main` window.
10. Finished tmux runs open an in-app log/review view. The peek prefers tmux pane snapshots and falls back to captured transcript/log output only when needed.
11. Ollama and shell runs stay on the exec-per-turn path and use the attached chat view inside `sb`, including the sessions rail and `[` / `]` quick switching.
12. `q` from the dashboard detaches the current `tmux` client instead of tearing down the cockpit session. Relaunching `sb` reattaches to the same session, and a second `sb` reuses the same shared cockpit/foreman.
13. `F` from the Agents list toggles **Foreman** on or off. Inside the New Run composer, `ctrl+t` toggles whether the run starts immediately or gets sent to Foreman, so the Note textarea can still accept a literal uppercase `F`. When Foreman is off, runs explicitly sent to Foreman stay parked as `waiting for Foreman`, show up in the `foreman` filter, and do not auto-start. Turning Foreman on lets eligible Foreman runs launch unattended in their own tmux sessions, while same-repo write-capable work stays serialized.
14. Runs explicitly sent to Foreman now also get an extra `FOREMAN PROTOCOL` block appended to the composed prompt, telling the agent to iterate until complete without permission prompts unless it hits the dirty-repo plan-only case.
    For Claude/Codex providers, `sb` also now translates the role permission policy into the actual provider CLI flags at launch time, so unattended runs do not silently fall back to the provider's default interactive approval prompts.
15. Session controls are now literal. `s` sends `Esc` to a tmux-backed session as a soft stop/back action; it does not put the run into a separate paused state. `S` sends `Ctrl+C` as a hard interrupt. `c` literally sends `continue`. For exec runs, `S` still cancels the in-flight turn, while `c` sends a normal follow-up turn with `continue`.
16. `a` accepts the selected reviewed run. For sourced runs this syncs back into `WORK.md` plus `DEVLOG.md`; for runs without a task source it marks the run complete without editing task files.
   Review surfaces preview the task removals, `DEVLOG.md` additions, changed files, diff stat, hook activity, and preexisting dirty files before you accept.
   Accept will refuse sync-back when the target `WORK.md` or `DEVLOG.md` already has uncommitted changes.
17. `R` starts a waiting Foreman job immediately if it is still queued, or opens the selected existing session if it is already live/stopped.
18. `K` skips the current queued item and keeps it in history. `C` skips the current item plus the rest of that queued run sequence, again preserving history.
19. `m` opens **Agent Setup**, the role/engine wizard. The right pane shows one group at a time (Identity → Prompting → Suggested Engine → Iteration); `tab`/`shift+tab` cycles groups, `j/k` moves within the visible group, and `enter` edits a field — except `Permissions` and `Iteration mode` cycle in place between their fixed options. `pgup/pgdn` also jumps groups. `a` toggles the advanced groups (`Hooks` JSON, `Advanced` overrides). `ctrl+s` saves, `esc` cancels. Saved edits now refresh in place immediately, `enter`-to-cycle fields refresh immediately too, changing an item ID behaves like a rename rather than leaving the old JSON file behind as a duplicate, and preset summaries show the current prompt / hook bundle / engine refs so composition edits are visible right away.
   `n` creates a new role/engine and drops you into the wizard with `Name` already focused; saving each field auto-advances to the next group.
   `D` duplicates and `d` deletes the highlighted item.
   The picker, setup, list, and attached-session views share the same terminal-height budget, so local scrolling should not hide the app chrome on short terminals.

tmux-backed jobs now also carry an explicit supervisor protocol inside the launch prompt. When a session needs the user to respond, it should print `SB_STATUS:WAITING_HUMAN`. When it is done and ready for review, it should print `SB_STATUS:READY_REVIEW`. `sb` watches the pane for those markers so normal jobs visibly flip into `waiting on you` / `needs review`, and Foreman can treat that as a real yield signal instead of guessing from whether the tmux window is still open.
As a fallback, `sb` also treats obvious interruption / provider-limit messages in the live pane (for example `conversation interrupted` or `usage limit reached`) as a yield back to the operator, and it now also catches broader handoff endings when the model forgets to print `SB_STATUS`: direct questions, soft `if you'd like me to keep going...` offers, `Choose one` / `Select an option` prompts, `y/n` confirmations, and similar GUI-style follow-up requests. Those fallback detections only fire after the tmux session log has been quiet for 10 seconds, so an in-progress answer is less likely to false-trigger midway through a turn.
Foreman handoffs now split cleanly: `waiting on you` and `needs review` no longer consume a Foreman concurrency slot, but write-capable jobs still keep their repo lock until you resolve them so later same-repo queued work does not pile onto a dirty tree by accident.

**Roles** describe reusable launch behavior: role/persona, launch mode, system prompt, hooks, iteration, policies, and a suggested engine. **Engines** describe the concrete executor/runtime (Claude CLI, Codex CLI, Ollama model). A role can be launched with any loaded engine.

Seed **roles** currently materialise in `~/.config/sb/presets/` on first run: `senior-dev`, `bug-fixer`, `test-writer`, `refactor`, `code-analyzer`, `explainer`, `docs-writer`, `scaffold`.

Seed **engines** materialise in `~/.config/sb/providers/` on first run: `claude`, `codex`, `ollama-qwen`, `ollama-llama`, `ollama-gemma`.

Edit any `*.json` in those dirs to customise. The on-disk schema still uses `presets` and `providers` for compatibility, even though the UI now frames them as roles and engines. Older utility roles still load if you already have them; they now sort below the core coding roles instead of crowding the top of the picker.
From the Agents page, `m` opens in-app Agent Setup.

### tmux status bar and scrolling

The bar at the bottom of live Claude/Codex sessions is the `tmux` status bar for the isolated `sb-cockpit` session. `sb` now gives that session its own styling, mouse support, higher scrollback, and no-prefix wheel/page scrolling without touching your personal tmux server or config.

- The tmux bar refreshes on a moderate interval, and Claude usage snapshots are cached for a few minutes so the status command does not hammer the usage API.
- The right side now shows only Claude/Codex limits; and any available 5h reset time is shown with the same `@3pm` marker for both providers.
- `mouse` is enabled for the cockpit session, and wheel-up; `Esc` or `q` exits tmux copy-mode.
- If you want the normal `sb` transcript/log view instead of the live native CLI pane, use `F1` / `Ctrl+g` / `F12` to return to `sb`.
- Finished tmux jobs are easier to review inside `sb` itself, where `j/k`, `pgup/pgdn`, and the sessions rail are handled by the TUI instead of the native CLI.

### Mouse wheel and long-file editing

- The main `sb` surfaces now respond to the mouse wheel: dashboard preview, project view, help overlay, Agent jobs list, Agent settings, and attached transcript/log review.
- On the dashboard, `pgup/pgdn` page the project list itself, while preview paging uses `ctrl+b` / `ctrl+f`.
- The top header nav is mouse-clickable too: left-click `Dashboard`, `Dump`, or `Agent` to switch pages directly.
- Inline `.md` edit mode supports `ctrl+home` / `ctrl+end` to jump to the top or bottom of long files.
- Agent list, attached chat, launch flow, and library views now clamp themselves to short terminals instead of rendering past the footer; when space is tight, the viewport shrinks first and long summaries wrap to the available pane width before falling back to truncation.

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
| `pgup/pgdn` | Page through the project list |
| `home/end` | Jump to the first / last project |
| `J/K` | Scroll the WORK.md preview |
| `ctrl+b` / `ctrl+f` | Page the WORK.md preview |
| `enter` | Open project |
| `e` | Edit WORK.md inline |
| `d` | Brain dump |
| `a` | Agents |
| `c` / `C` | Cleanup (single / chain) |
| `P` | Daily plan |
| `t` | Next todo |
| `/` | Search across all WORK.md files |
| `f` | Pin/unpin project |
| `space` | Toggle project selection |
| `o` | Open project dir in editor |
| `r` | Refresh |
| `,` | Open the `sb` config directory in editor |
| `?` | Help |

Agents tab:

| Key | Action |
|-----|--------|
| `n` | New run picker (row 0 is `★ New run without task source`, then projects) |
| `F` | List: toggle Foreman on/off |
| `ctrl+t` | New run: toggle immediate launch vs send to Foreman |
| `f` | List: cycle job filters |
| `tab` | List: cycle filters · New run: cycle role → engine → repo → note → review · Agent Setup: cycle wizard groups · Attached exec-chat: swap transcript ↔ input |
| `space` | Toggle task in picker |
| `i` | List: open selected job (`tmux` attach while live, input focus for exec-chat jobs) · Attached exec-chat: focus input |
| `pgup/pgdn` | List: page the right-side peek · Attached/review: page the transcript/log |
| `enter` | Launch (from role/engine/review tabs) · open selected job (`tmux` attach while live, log review when finished, chat for exec jobs) · send when input-focused |
| `alt+enter` | Launch from note |
| `R` | Start waiting job now, or open the selected session |
| `s` | Send `Esc` to the selected tmux-backed session |
| `S` | Send `Ctrl+C` to the selected session / hard interrupt the active turn |
| `c` | Send literal `continue` |
| `ctrl+g` | Live tmux session: jump back to the shared `sb` main window |
| `j/k` | Scroll transcript or tmux log in the attached view |
| `[` / `]` | Attached view: previous / next job |
| `K` | Skip queued/reviewed job and keep it in history (confirm) |
| `C` | Skip current item and the rest of its queued run sequence (confirm) |
| `d` | Delete job (confirm) |
| `q` | Detach cockpit client when running inside `sb-cockpit`; otherwise quit/go back |
| `esc` | Back (or leave input focus when typing) |

Full keybind reference is available in-app with `?`.

---

## Design Docs

- [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) — product direction for turning `sb` into the first cockpit UI for coding-agent orchestration, with a separable foreman/runtime layer behind it

## License

[AGPL-3.0](LICENSE)
