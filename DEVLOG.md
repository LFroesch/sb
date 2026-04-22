# DEVLOG

## 2026-04-20

- Rewrote [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) into a tighter build RFC: sharp `v1` scope, shared day/night job model, PTY-backed worker sessions, master-session control, concise `v2` notes, and a clearer `task -> launch preset -> job` backbone.
- Added a top-of-RFC warning plus a hidden-requirements checklist so implementation starts by locking authority, state, lifecycle, safety, defaults, artifacts, and recovery behavior.
- Added provider-oriented LLM config with named profiles plus compatibility fallback for older top-level `model` / `ollama_host` settings.
- Replaced the Ollama-only client with `internal/llm`, keeping the existing prompts while adding OpenAI and Anthropic HTTP adapters.
- Switched UI/help copy from provider-specific Ollama wording to generic model wording and added config tests in `internal/config/config_test.go`.
- Added header-visible provider status so the active LLM profile/model is always visible and incomplete setups surface as `llm disabled (...)`.
- Replaced `scan_dirs`-only fallback naming with named `scan_roots` plus collision-aware relative-path labels for users with many same-named files like `WORK.md`.
- Kept title-driven labels for any discovered markdown file via `# TYPE - label | description`, and now disambiguate duplicate title labels by appending the shortest useful relative suffix.
- Replaced hard-coded `/tmp/sb-*.log` files with JSON `slog` output under `~/.local/share/sb/logs/sb.log` using built-in size rotation.
- Added naming tests in `internal/workmd/naming_test.go`.
- Added [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md), an RFC for using `sb` as the first cockpit UI for coding-agent orchestration while keeping foreman/runtime concerns outside the TUI process.

## DevLog

### 2026-04-22 — Tmux cockpit path made coherent
- Finished the first real tmux-backed Claude/Codex path instead of mixing it with the older embedded-chat model. `internal/cockpit/runner_tmux.go` now launches the native interactive CLIs directly in tmux windows, normalizes legacy Claude/Codex args for that mode, and lets windows close cleanly so job status can advance instead of hanging in `running`.
- Rewired the Agent page so tmux-backed jobs use `AttachTmux` from the job list and on launch, while Ollama/shell jobs continue using the existing attached exec-chat view. Updated cockpit title/detail/help text to make the split visible, including an `exec-fallback` badge when tmux bootstrap is unavailable.
- Tightened cockpit tests by waiting for launched jobs to settle before tempdir cleanup, adding tmux-command normalization coverage, and skipping the unix-socket roundtrip test in environments where unix sockets are blocked.
- Fixed two lifecycle holes in the tmux path: dashboard `q` now detaches the cockpit client instead of blindly quitting when running inside `sb-cockpit`, and registry rehydrate now preserves tmux-backed running jobs so the daemon can reconnect them to still-live tmux windows after a restart.
- Added a clearer operator surface on top of that lifecycle work: Agent list/detail/attached views now expose explicit detach (`x`), live tmux-window state, and a finished-session log/review pane for tmux-backed jobs instead of only supporting live attach.
- Hardened tmux startup for detached foreman usage: cockpit tmux commands now inject a default `TERM=xterm-256color` when the daemon was started without a terminal environment, which fixes launches that previously failed with `open terminal failed: not a terminal`. The Agent UI now also surfaces that real launch note instead of the vague `tmux window not recorded for this job`.
- Fixed the tmux session bootstrap primitive itself: `EnsureSession` no longer uses `new-session -A`, and instead does an explicit `has-session` check before detached session creation. That avoids tmux falling into an attach/reuse path when `sb-cockpit` already exists, which was another source of confusing non-terminal startup failures.
- Added more reliable "return to sb" tmux root bindings. `F1` is still bound to window 0, but the cockpit now also binds `Ctrl+g` and `F12` so VS Code / Cursor terminals that intercept function keys still have a clean way to return from an attached job without using `Ctrl+C` and accidentally interrupting the agent process.
- Tightened that further by making `Ctrl+C` context-sensitive at the tmux root table: on window 0 (`sb`) it still passes through normally, but from attached job windows it now returns to `sb` instead of sending SIGINT to the agent process.

### 2026-04-21 — Agent cockpit control-plane pass
- Reworked the Agent page toward a real multi-session cockpit instead of a single-chat view. The list now shows top-level operational counts plus session usage grouped by provider/model so it is easier to see where Claude/Codex/Ollama jobs are concentrated.
- Reworked attached chat into a split layout with a persistent sessions rail and `[` / `]` quick-switching, which makes moving between multiple active conversations much faster.
- Tightened cockpit height calculations so the list/detail and attached layouts stay inside the terminal chrome on shorter screens instead of growing too tall.
- Sending a follow-up message now appends the user turn into the local attached transcript immediately, so the UI shows what was sent before the provider reply round-trip completes.
- `internal/cockpit/manager.go` now tracks explicit stop intent, returning stopped jobs to `idle` with note `stopped` instead of surfacing them as ambiguous provider failures. Added a regression test in `internal/cockpit/manager_test.go`.
- Added more power-user list flow: filter buckets (`all/live/running/attention/done`), `tab`/`1-5` filter switching, `i` to attach directly into input-ready iteration, and attach/back behavior that preserves the current job instead of resetting the cursor.
- Fixed an attached-chat clipping bug caused by mismatched width/height math: transcript rendering now uses the actual attached-chat column width, attached resize events refresh the viewport, and the chat panel no longer line-caps content after the viewport has already handled scrolling.
- Fixed an attached-chat optimistic-echo race in `update_agent.go`: stale daemon snapshots from `GetJob()` could overwrite the just-sent local user turn during immediate event/status refreshes, making a message appear only after the next send. Attached sync now preserves the optimistic local suffix until server state catches up.
- Removed transient "message sent" status churn from successful attached sends, because it changed cockpit height at the exact moment the viewport auto-followed and could push the newest visible turn below the fold.
- Added an explicit attached-layout recalculation step before transcript refresh/auto-follow, so viewport sizing now uses the real rendered textarea height instead of stale assumptions about the input area.

### 2026-04-21 — Agent: pm
- better readme for sb, how to use brain dump / cleaning etc

### 2026-04-21 — Jobs-are-chats: per-turn exec, multi-turn sessions
- Removed the PTY-embedded executor path entirely (`internal/cockpit/pty.go` deleted, `creack/pty` dropped from go.mod). It corrupted the transcript with ANSI/alt-screen sequences and locked each job to a single turn.
- Reworked `Job` into a turn-based session: added `Turns []Turn`, `SessionID`, `StatusIdle`, and `TurnRole` so the same primitive covers sourced tasks *and* freeform chats — one list view, one attached view, one primitive.
- `Manager.SendInput`/`SendTurn` now spawns a fresh `exec.Cmd` per turn. Claude uses native `--session-id` / `--resume`; codex/ollama replay the history to stdin; shell runs one-shot. Job stays `StatusIdle` between turns instead of flipping straight to completed.
- Post-shell hooks moved from per-turn to approve-time so follow-up turns don't re-trigger them. `ApproveJob` is what ends a conversation.
- UI: attached view wraps in `panelStyle` chrome to match the rest of the app; `StatusIdle` is treated as input-ready; input label calls out "turn in flight — wait for reply" vs "(conversation ended — no more turns)".
- Added freeform-chat entry: `N` in the job list jumps straight to the launch modal with empty sources and the brief textarea focused.
- Removed the ad-hoc parallel chat system (types, protocol methods, client methods, server dispatch, model fields, modes) in favor of unified jobs. `claude-interactive` / `codex-interactive` seed providers dropped — all providers now drive the same per-turn loop.

### 2026-04-21 — Agent: bug-fixer
- quick notes integration

### 2026-04-21 — Agent cockpit UX fixes (typing, selection, delete)
- Fixed list cursor/selection drift: `renderAgentList` grouped jobs (needs-attn / running / recent) while `updateAgentList` indexed into the ungrouped slice, so enter/a/s/r/d acted on the wrong job. Added shared `orderAgentJobs` used by both sides.
- Rewrote attached-view key handling as a two-mode focus model: transcript focus (shortcuts + j/k scroll, default after attach) vs input focus (all letters type freely, only `alt+enter`/`esc`/`tab` intercepted). `tab` or `i` switches to input; `esc`/`tab` leaves it. Fixes "can't type a/s/r/j/k into chat".
- Added job delete path end-to-end: `Registry.Delete`, `Manager.DeleteJob` (stops PTY first), `MethodDeleteJob` protocol + dispatch, `SocketClient.DeleteJob`. UI key `d` (prompts y/n in list, immediate when attached).
- Attached view shows preset · status badge · executor · id · age · exit code · note · sources preview, plus a focus indicator line. Input panel hides when the job isn't running, with an explicit "no input accepted" hint.
- Footer + help overlay updated to match. Agent list now clamps cursor when jobs shrink and adds `g/G` jump.

### 2026-04-21 — Agent: bug-fixer
- sometimes i open sb and it spins forever/bugs

### 2026-04-21 — Agent cockpit launch/runtime/layout fixes
- Fixed freeform chat launches end-to-end: `N` now seeds the launch modal with the current project repo (falling back to cwd), and `Manager.LaunchJob` also defaults empty repos to `os.Getwd()` instead of rejecting the job outright.
- Fixed Codex-backed presets building invalid commands. The preset seed no longer includes a redundant `exec` arg, so runtime command assembly now produces `codex exec -` as intended.
- Reworked attached transcript sizing in `view_agent.go` to subtract actual header/input chrome rather than a fixed constant, which keeps the header, transcript, and input visible on shorter terminals.
- Added focused cockpit tests in `internal/cockpit/manager_test.go` for Codex turn command assembly and freeform repo fallback.
- Hardened cockpit actions so `a`/`d` now stage an explicit `y/n` confirmation before approve sync-back or job deletion, including from the attached transcript view.
- Replaced Codex follow-up history replay with native resume: first turn now runs `codex exec --json`, captures `thread.started.thread_id`, and subsequent turns use `codex exec resume --json <thread_id> <prompt>`.
- Cleaned up Claude/Codex executor arg handling: seed Claude presets no longer carry redundant `--print`, and runtime now strips legacy `--print` / `exec` / `resume` / `--json` args so existing preset files keep working.
- Smoothed cockpit UX: launch now auto-attaches into the new chat, idle live jobs open with input focus, attached chat sends on plain `enter`, and the list page now includes a selected-job detail pane with last-turn preview and actions.
- Reworked attached chat rendering: the viewport now renders from structured `Turns` plus live streamed assistant output instead of the raw transcript log, sent user messages echo immediately, and viewport updates only auto-follow when you're already at the bottom or actively typing.
- Tightened the agent dashboard layout: the jobs list and selected-job detail pane now share the same capped panel height, and the job list windows around the cursor instead of growing unbounded.
- Fixed shared-viewport startup bleed-through: `projectsLoadedMsg` now only repaints the markdown viewport when you're actually on Dashboard or Project, so opening Agent immediately after startup no longer shows the top discovered WORK.md content there.
- Split agent attach focus behavior: new launches can still open input-first, but attaching to an existing job now starts in transcript focus so scrolling and reading old messages don't get hijacked by the textarea.
- Reworked attached chat state management to keep a local snapshot of job turns plus pending user text and live assistant output, instead of rebuilding entirely from fresh `GetJob` calls on every refresh. This stabilizes immediate local echo and reduces scroll jitter.
- Tightened immediate local echo further by appending the just-sent user turn into the attached chat's local turn list as soon as `SendInput` succeeds, so the last message shows up immediately rather than waiting for daemon state to round-trip.
- Added minimal preset/provider management to the Agent page: `p` creates a preset JSON template (including hook examples) and opens it in the editor, `v` does the same for a provider, and `P` / `V` open the config directories directly.
- Filtered legacy provider IDs `claude-interactive` and `codex-interactive` out of `LoadProviders`, so users with older `~/.config/sb/providers/` seeds no longer see duplicate Claude/Codex entries in the cockpit.
- Added `CleanLegacyConfig` at startup to rewrite old cockpit config instead of papering over it at runtime: it deletes `claude-interactive.json` / `codex-interactive.json` and strips redundant Claude/Codex executor args from provider and preset JSON on disk.
