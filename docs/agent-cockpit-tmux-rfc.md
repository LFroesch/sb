# RFC: sb cockpit v2 — tmux runner + global-config editor

> **Status:** refined 2026-04-21, implementation in progress. Sibling doc to `agent-cockpit-rfc.md`.
> Covers the next iteration of the cockpit after v0.5 landed.
>
> Jump to [Implementation plan (2026-04-21)](#implementation-plan-2026-04-21) for the locked
> attack plan + design decisions. The sections above it are retained as the
> problem framing / rationale that fed into the plan.

## Context

`sb`'s agent cockpit (v0.5, landed 2026-04-21) runs `claude` / `codex` / `ollama` / `shell` as one `exec.Cmd` per turn, parses their stdout (raw for claude, `--json` for codex), and re-implements the chat UI in Bubble Tea (transcript buffer, input textarea, optimistic local echo, turn merge logic). Two problems:

1. **It never catches up to the real CLI.** Slash commands, model switching, approve/deny, interrupts, tool-use rendering, and every future Claude Code / Codex feature live in the upstream TUIs. Rebuilding them inside sb is a treadmill.
2. **Global config surface (CLAUDE.md, settings.json, `commands/`, `agents/`, `hooks/`) has no home.** The cockpit knows about presets and providers but not about the files the CLIs themselves read on startup.

Industry direction (Conductor-style multi-agent tooling, aider farms, etc.) is **dispatch, not embed** — the orchestrator launches real CLIs in real terminals (usually tmux) and manages them from outside. This RFC switches sb to that model for interactive providers while keeping the existing `exec.Cmd` path for truly headless jobs, and adds a data-driven global-config editor.

Intended outcome: the "attached" experience *is* Claude Code / Codex — because it literally is. sb owns launching, multi-session awareness, sync-back, hooks, queues, and config editing. Upstream owns the interactive UX.

## Shape of the change

### Split runners by interaction model, not by provider type

- **Interactive runner (new, tmux-backed):** `claude`, `codex`. One tmux window per job under a dedicated `sb-cockpit` session. sb launches the real CLI with the brief as the opening prompt, pipes the pane to a log, and flips job status based on pane lifetime. User iterates by *switching tmux client into the window* — they're talking to the real CLI with full parity.
- **Headless runner (kept as-is):** `ollama`, `shell` (and any future stream-only provider). The current `exec.Cmd` + history replay path is fine for queued classify/summarize/docs-tidy/lint/test/build jobs — they don't need a TUI.

Runner selection is a per-`ExecutorSpec` property derived from `Type`, overridable via a new `ExecutorSpec.Runner` field (`"tmux"` | `"exec"`) so users can force one or the other.

### Tmux as substrate, not spawned terminal windows

Spawning a new terminal emulator window (`$TERMINAL -e claude …`) depends on WM, fails over SSH/WSL, and gives no handle back to sb. Tmux is portable across Linux/macOS/WSL/SSH, gives programmatic control (`list-panes`, `pipe-pane`, `switch-client`, `send-keys`, `kill-pane`), and composes with the user's existing tmux setup if they have one.

If `tmux` is not on PATH: cockpit auto-falls-back to current exec-per-turn mode with a visible `[exec-fallback]` badge in the header. No auto-install, no silent failure, no prompt — decided.

### Pane lifecycle detection: poll `tmux list-panes` at ~1s

Dead simple, portable, negligible overhead even at ~20 concurrent jobs. One daemon goroutine iterates all tmux-backed jobs, diffs the alive-set, fires status transitions. No `wait-for` / event plumbing — decided.

### Global-config editor is data-driven on ProviderProfile

Add `GlobalFiles []GlobalFileSpec` to `ProviderProfile`. A `GlobalFileSpec` is `{Label, Path, Kind}` where `Kind ∈ {"file","dir"}`. Seed the claude provider with `~/.claude/CLAUDE.md`, `~/.claude/settings.json`, `~/.claude/commands`, `~/.claude/agents`, `~/.claude/hooks`, `~/.claude/keybindings.json`. Seed codex with whatever its real paths are (verify at implementation time — `~/.codex/config.toml`, `~/.codex/AGENTS.md`, etc.). A new modal (key `g` from Agent page) lists providers → files → opens in `$EDITOR` via a suspend-and-exec helper; for dir kinds, opens the dir in `$EDITOR` too (most editors handle directories).

## Files to change

### New files

- `internal/cockpit/tmux.go` — thin wrapper around the `tmux` CLI: `HasTmux()`, `EnsureSession(name)`, `NewWindow(session, windowName string, cmd []string, env []string, cwd string) (target string, err)`, `PipePane(target, logPath)`, `SwitchClient(target)`, `KillWindow(target)`, `SendKeys(target string, keys string)`, `ListWindows(session) ([]WindowInfo, error)`, `WindowAlive(target) bool`. No daemon — each call shells out. Add a fake-tmux shim for tests via `SB_TMUX_BIN` env.
- `internal/cockpit/runner_tmux.go` — implements a new `interactiveRunner` interface used by Manager when a job's runner is tmux. Creates the window, starts the pipe-pane log, flips status to `StatusRunning`, spawns a poller goroutine that `WindowAlive`-polls (~1s) and on exit flips status to `needs_review` (sourced jobs) or `idle` (freeform, still attached). Post-shell hooks run on approve as today.
- `internal/cockpit/global_files.go` — `GlobalFileSpec` type, resolution helpers (`~` expansion, existence check), default seeds per provider.
- `update_agent_globalcfg.go`, `view_agent_globalcfg.go` — new "global config" mode: provider picker → file list with existence badges → open in `$EDITOR`. Use `tea.ExecProcess` (Bubble Tea native) so the TUI suspends cleanly while the editor runs.

### Files to modify (meaningfully)

- `internal/cockpit/types.go` — `ExecutorSpec.Runner string` ("tmux"|"exec", empty = infer from type). Add `Job.TmuxTarget string` for persistence. Add `Job.LogPath string` for the pipe-pane output (distinct from `TranscriptPath`; keep Transcript for headless jobs' turn log).
- `internal/cockpit/manager.go` — branch `LaunchJob` on runner. Interactive path: delegate to `runner_tmux.StartJob`. Headless path: keep `runFirstTurn` / `runTurn` / `buildTurnCmd` unchanged. Rip `startTurn`/`finishTurn`/`SendTurn`/`SendInput` behavior for tmux-backed jobs (SendInput on a tmux job is an error: "send input in the tmux window"). `StopJob` on a tmux job sends `C-c` then kills the window.
- `internal/cockpit/providers.go` — add `GlobalFiles` to `ProviderProfile`; update `defaultProviders` seeds with known claude/codex paths.
- `internal/cockpit/iface.go` & `protocol.go` & `client.go` & `server.go` — add `AttachTmux(id) error` method that does `SwitchClient` on the server side (server must do it since tmux targets live in the user's tmux server; this is fine because the cockpit daemon runs as the same user). Add matching NDJSON method. `SendInput` keeps current semantics for headless jobs; returns a clear error for tmux jobs.
- `update_agent.go` — when the selected job is tmux-backed, `a` (attach) calls `AttachTmux` instead of entering `modeAgentAttached`. The existing chat view stays but only shows for headless jobs. New keybind `g` enters global-config mode. Simplify: delete the 300+ lines of `mergeAttachedTurns` / `refreshAttachedViewport` / optimistic-echo / textarea handling only for the tmux branch (headless keeps them).
- `view_agent.go` — job row shows a runner badge (`tmux` / `exec`) and, for tmux jobs, the tmux target (`sb-cockpit:3`). List pane shows tail of pipe-pane log for the selected tmux job instead of the synthesized transcript.
- `sb` config — new fields: `cockpit_runner_default` ("auto"|"tmux"|"exec", default "auto"), `tmux_session_name` (default "sb-cockpit"). Document in README.
- `docs/agent-cockpit-rfc.md` — append a "2026-XX-XX — tmux runner" update block near the top noting the split-runner model.
- `WORK.md` — remove the obsolete "agent cockpit" tasks that this makes moot, add a single "cockpit v2: tmux runner + global-config editor" current task.

### Files / code to delete

**Deferred.** Do not delete the per-turn exec code, codex JSON parser, or attached-chat UI in this PR — they still drive headless jobs. A follow-up can prune anything that ends up only ever touched by shell/ollama jobs once the tmux path is proven in daily use.

## Reusing existing code

- `internal/cockpit/syncback.go::ApplySyncBack` — unchanged. Fires on approve regardless of runner. This is the whole point of keeping cockpit in charge.
- `internal/cockpit/hooks.go::RunShellHook` — unchanged. Pre-shell fires before tmux window create; post-shell fires on approve.
- `internal/cockpit/registry.go` — unchanged. Job persistence is runner-agnostic.
- `internal/cockpit/presets.go` — unchanged shape; presets still describe persona + provider + hooks.
- `cmd/foreman` daemon — unchanged transport. Tmux commands run on the daemon side (same user, same tmux server).
- `tea.ExecProcess` (Bubble Tea built-in) — already the standard way to suspend and exec `$EDITOR`. Don't hand-roll stty save/restore.

## What the user's flow looks like after this

1. Launch sb, tab to Agent page, pick a task, pick the `senior-dev` preset + `claude` provider, press Enter.
2. sb creates window `sb-cockpit:42-senior-dev` (or similar), starts `claude -p "<composed brief>"` in it, sets up `pipe-pane`, `switch-client`s into it.
3. User is now in real Claude Code. `/model sonnet-4-6`, approve/deny, `/commands`, interrupts, tool use — all work natively.
4. Exits claude (or hits `prefix-d` to detach and come back later). sb's poller sees the pane die, flips job to `needs_review` (sourced) or `idle` (freeform).
5. Back in sb, `a` on the job runs post-hooks + sync-back (deletes WORK.md line, appends DevLog). Or `r` retries with same brief, `d` deletes.
6. `g` from Agent page → "Claude Code" → "CLAUDE.md" → opens `~/.claude/CLAUDE.md` in `$EDITOR`. Save, exit, back in sb.

## Verification

- `go build ./...` and `go vet ./...` clean.
- `go test ./internal/cockpit/...` — existing tests stay green; new tests:
  - `tmux_test.go`: fake-tmux binary via `SB_TMUX_BIN` — asserts window creation, pipe-pane, switch-client, kill sequences.
  - `runner_tmux_test.go`: launch → poller detects "pane dead" (simulated) → status flips to `needs_review` → `ApproveJob` runs post-hooks + sync-back.
  - `runner_fallback_test.go`: with `HasTmux()` false, an interactive preset launches via exec runner and the header badge reflects that.
- Manual smoke:
  - Inside `tmux new -s outer`, run sb, launch claude job, verify window in `sb-cockpit` session, `/model`, `/help`, approve/deny work.
  - Outside tmux: sb prints "cockpit needs tmux for interactive runner — falling back to exec" badge; existing claude/codex exec path still works.
  - Without claude on PATH: job fails with a clear error in the pane (the CLI's own error), window stays up, user sees it, sb flips to `failed`.
  - `g` opens each seeded global file in `$EDITOR`; nonexistent files show "(missing — create?)" and create-on-save.
  - Kill sb mid-job → relaunch → job rehydrates; tmux window still alive → status polling resumes; if tmux server died, job marked `failed` on rehydrate.

## Risks + exit ramps

- **Tmux as a dependency.** Hard on macOS/Linux/WSL, awkward-but-possible on bare Windows (we don't ship there anyway). Mitigation: auto-fallback to exec runner + visible badge. Lucas can test this by `PATH= sb` once.
- **`switch-client` only works if the user has a tmux client attached.** If sb is launched outside tmux, the daemon can create the session but cannot pull the user in. Mitigation: on first tmux launch, print instructions in the status line: "sb-cockpit session ready — run `tmux attach -t sb-cockpit`" or offer a `tea.ExecProcess` that does exactly that.
- **Log tailing is the only live signal.** Some tools render with TUI escape codes that look ugly in a plain tail. The cockpit list pane uses a last-N-lines preview only; the real view is the tmux window. Acceptable.
- **This doesn't automatically make approve/deny *mid-turn* available in sb.** It's handled by the CLI inside tmux. That's the point — but worth being explicit.
- **Existing in-flight jobs won't migrate.** An `exec`-runner claude session from before the change keeps working under its old code path. New launches get the new runner. Clean cut.

## Open threads (refine before implementation)

- **Launched outside tmux — what exactly happens?** Spawn a detached `sb-cockpit` session and print an attach hint? Refuse interactive launches until the user is inside tmux? Offer a `tea.ExecProcess` that runs `tmux attach -t sb-cockpit` for the user?
- **Freeform chat-with-ollama** — stay in the in-sb chat UI (current behavior) or also get a tmux window? Argument for tmux: consistency. Argument for keeping it in-sb: ollama is a stream, there's no interactive CLI to embed — a tmux window would just be a text stream with no input affordance.
- **Pre-shell hook output** — currently captured into sb's transcript. Under tmux runner, does it go into the tmux pane's scrollback (so the user sees it when they attach), into the sb-side log only, or both?

## Phasing

Single PR — decided. Tmux runner + global-config editor touch the same Agent-page UI surface and the same `ProviderProfile` struct; splitting would double the UI-layout churn for little review benefit.

## Implementation plan (2026-04-21)

Locked after design review. This section is the source of truth for the
build; the sections above remain for context/rationale. Deviations need
an explicit note added to the postscript at the bottom of this doc.

### Locked design decisions

1. **Isolated tmux server via `-L sb`.** Every tmux call the cockpit makes uses
   `tmux -L sb …`. That gives us our own socket under `$TMUX_TMPDIR` without
   ever touching the user's default server or their real tmux config
   (`~/.tmux.conf` is not loaded by default on a fresh socket — we apply the
   bindings we need explicitly). Implication: `switch-client` and attach must
   also go through `-L sb`, and the bootstrap must `exec tmux -L sb attach`
   rather than expecting the user's outer tmux to cooperate.
2. **Bootstrap self-exec into tmux.** When `sb` starts on a terminal and tmux
   is present, `sb` re-execs itself inside `tmux -L sb new-session -A -s
   sb-cockpit -n sb` so that window 0 of that session *is* the sb TUI. If tmux
   is missing, or `SB_NO_TMUX=1`, or `SB_IN_COCKPIT=1` is already set (we're
   the re-exec child), skip bootstrap and run the TUI directly. The re-exec
   child sets `SB_IN_COCKPIT=1` so downstream invocations don't loop.
3. **Session/window layout.**
   - Server: `-L sb` (our own tmux server socket).
   - Session: `sb-cockpit` (one, shared across all cockpit UIs on this host).
   - Window 0: `sb` itself (the TUI). Always present while sb runs.
   - Windows 1..N: one per job. Window name encodes job id prefix + preset
     slug, e.g. `42-senior-dev`. Pane lifetime == job lifetime.
   - Key binding (set by bootstrap): `F1` → `select-window -t sb-cockpit:0`
     so the user can always jump back to sb from a job. `prefix` left at tmux
     default (`C-b`) so nothing clashes with the user's muscle memory in
     window 0 — sb does its own key handling, tmux prefix goes to the job
     windows.
4. **Quit semantics.** `q` inside sb detaches the tmux client (`tmux -L sb
   detach-client`), it does **not** kill the sb-cockpit session. Jobs and sb
   keep running in the background; re-running `sb` reattaches to the same
   session. Explicit shutdown (`Q` uppercase or a dedicated menu item) is
   deferred — v2 ships attach/detach only, not a global kill path. Individual
   job windows can be closed by the user via `kill-window` inside tmux or by
   `d` (delete) on the sb job list.
5. **Fallback when tmux is absent.** `HasTmux()` false → skip bootstrap, show
   an `[exec-fallback]` badge in the Agent tab header, and route claude/codex
   launches through the existing `exec.Cmd` runner. No auto-install, no
   prompt, no silent failure. The user sees the badge and knows what's up.
   Shell/ollama jobs use the exec runner regardless.
6. **Pane lifecycle detection.** One daemon goroutine polls
   `tmux -L sb list-panes -t sb-cockpit -F '#{window_id} #{pane_pid}
   #{pane_dead}'` every second, diffs against the tracked set of alive
   targets, and fires `StatusRunning` → `StatusNeedsReview` (sourced job) /
   `StatusIdle` (freeform) transitions on death. No `wait-for`, no per-window
   goroutine, no event plumbing.
7. **Log capture via `pipe-pane`.** Each new window immediately gets
   `tmux -L sb pipe-pane -t <target> -o 'cat >> <log_path>'`. The log path
   is `jobs/<id>/tmux.log` — distinct from `transcript.log` which is still
   used by the exec runner for headless jobs. List-pane preview in sb
   shows the tail of `tmux.log`; the real UX is `a` → attach into the
   window.
8. **Attach = `switch-client` inside the cockpit session.** sb itself runs
   as window 0 of `sb-cockpit`. Pressing `a` on a tmux job issues
   `tmux -L sb select-window -t sb-cockpit:<window_id>` (we're already
   inside the session, so no `attach` dance). User lands inside the real
   claude/codex CLI. To come back: `F1` (select window 0) or `prefix-w`
   (native tmux window picker).
9. **SendInput on a tmux job is an error.** The cockpit does not forward
   text into tmux panes programmatically. If the user types in sb's
   in-list input while a tmux job is selected, we return a clear error:
   *"send input in the tmux window — press 'a' to attach"*. Simpler than
   shipping a second input model and keeps tmux as the single source of
   truth for in-flight user↔CLI text.
10. **Runner selection.** Per-`ExecutorSpec` `Runner string` field.
    Values: `"tmux"`, `"exec"`, `""` (= infer). Inference: `claude`/`codex`
    → tmux when `HasTmux()` else `exec`; `ollama`/`shell` → always `exec`.
    Users can force either by setting `Runner` explicitly on the provider
    JSON. `ExecutorSpec.Runner` also carries through `LaunchRequest` so
    a per-launch override is possible without editing a provider file.

### Attack plan (file-by-file)

1. **`internal/cockpit/tmux.go`** — thin shim over the `tmux` CLI with
   every call pinned to `-L sb`. Public surface:

   ```go
   type WindowInfo struct {
       Target   string // "sb-cockpit:@3"
       WindowID string // "@3"
       Name     string
       PaneID   string // "%7"
       PanePID  int
       Dead     bool
   }
   func HasTmux() bool
   func InsideTmux() bool                                // $TMUX set
   func Bin() string                                     // SB_TMUX_BIN or "tmux"
   func EnsureSession(name string) error                 // -L sb new-session -A -s name -d
   func NewWindow(session, name string, cmd []string, env []string, cwd string) (WindowInfo, error)
   func PipePane(target, logPath string) error
   func SelectWindow(target string) error                // -L sb select-window -t target
   func SwitchClient(target string) error                // -L sb switch-client -t target (only useful if a client is attached)
   func DetachClient() error                             // -L sb detach-client
   func KillWindow(target string) error
   func WindowAlive(target string) (bool, error)
   func SendKeys(target, keys string) error              // reserved for a later SendInput path
   func ListWindows(session string) ([]WindowInfo, error)
   func BindKey(key, tmuxCmd string) error               // -L sb bind-key -T root <key> <cmd>
   func UnbindKey(key string) error
   ```

   All commands shell out; no persistent tmux control-mode session. A
   `SB_TMUX_BIN` env var overrides `tmux` so tests can inject a fake
   binary. Errors carry the stdout+stderr of the failing tmux call so
   failures are diagnosable.

2. **`internal/cockpit/types.go`** — extend:
   - `ExecutorSpec.Runner string` (`tmux|exec|""`).
   - `Job.TmuxTarget string` (e.g. `sb-cockpit:@3`).
   - `Job.LogPath string` (path to `jobs/<id>/tmux.log`).
   No new status values; `StatusRunning` / `StatusIdle` / `StatusNeedsReview`
   cover the tmux lifecycle.

3. **`internal/cockpit/runner_tmux.go`** — owns the tmux-backed job
   lifecycle. Top-level API:

   ```go
   type tmuxRunner struct {
       paths Paths
       reg   *Registry
       emit  func(Event)
       mu    sync.Mutex
       alive map[JobID]string // jobID -> tmux target
       stopCh chan struct{}
   }
   func newTmuxRunner(...) *tmuxRunner
   func (r *tmuxRunner) StartJob(j Job) error       // creates window, pipe-pane, flips Running
   func (r *tmuxRunner) StopJob(j Job) error        // send C-c, wait ~500ms, kill-window
   func (r *tmuxRunner) Rehydrate(jobs []Job)       // called on daemon start; if target dead, flip to Failed/NeedsReview
   func (r *tmuxRunner) pollLoop(ctx context.Context) // 1s tick
   ```

   Window command: `claude -p "<brief>"` (first turn) or `codex exec
   "<brief>"` — the brief comes from `ComposeBrief(preset, sources,
   freeform)` same as the exec path. After that first invocation exits,
   the user is dropped back to a shell in the pane (we wrap the call in
   `bash -c '<cmd>; exec $SHELL -i'` so the pane doesn't auto-close and
   the user can continue the conversation via the CLI's native resume
   flow if they want).

   Actually: revisit — `claude` (Claude Code CLI) without `-p` drops
   into interactive mode with the brief as the opening prompt. That's
   the whole point of the tmux runner. So the command is
   `bash -c 'claude "<brief>"; exec $SHELL -i'` (claude interactive,
   fall back to shell after exit). For codex: `codex "<brief>"` similarly.
   Pane exit → poller flips job status. If the user types `exit` from
   the trailing shell, the window dies → job flips.

4. **`internal/cockpit/manager.go`** — branch at `LaunchJob`:
   - If resolved runner == `"tmux"` and `HasTmux()`: build job record,
     delegate to `tmuxRunner.StartJob`. Skip `runFirstTurn`'s per-turn
     exec path.
   - Otherwise: keep existing `runFirstTurn` / `runTurn`.
   - `StopJob(id)`: if tmux-backed, delegate to `tmuxRunner.StopJob`.
     Otherwise, cancel the in-flight exec.
   - `SendInput(id, data)`: if tmux-backed, return
     `errors.New("send input in the tmux window — press 'a' to attach")`.
   - `ApproveJob(id, ...)`: runner-agnostic (post-shell hooks + sync-back
     run regardless). Keep as-is.
   - `DeleteJob(id)`: if tmux-backed and window alive, kill the window
     before removing the registry record.
   - New helper `resolveRunner(spec ExecutorSpec) string`:
     * explicit `spec.Runner` wins;
     * else `claude`/`codex` → `"tmux"` if `HasTmux()` else `"exec"`;
     * else `"exec"`.

5. **`internal/cockpit/iface.go` + `protocol.go` + `client.go` + `server.go`**:
   - New method on `Client`: `AttachTmux(id JobID) error` — server-side
     does `SelectWindow(job.TmuxTarget)`. If no client attached to the
     tmux session, this is a no-op-with-error we surface to the user.
   - Protocol method: `attach_tmux` with `{id}` params.
   - `Manager.AttachTmux` looks up the job, validates `TmuxTarget` is
     non-empty, calls `tmux.SelectWindow`. Returns error if the window
     no longer exists (poller will also mark the job dead shortly).
   - `DetachTmux()` is a TUI-local op (just `tmux.DetachClient()`),
     no need to go through the daemon. sb's `q` handler calls it
     directly.

6. **`internal/cockpit/bootstrap.go`** — new file, called from
   `main.go` before Bubble Tea starts:

   ```go
   func MaybeReExecIntoTmux() (reExeced bool, fallback bool, err error)
   ```

   Behavior:
   - If `SB_IN_COCKPIT=1` → return (false, false, nil) unchanged; we're
     the child.
   - If `SB_NO_TMUX=1` or `!HasTmux()` → return (false, true, nil);
     caller runs the TUI with the `[exec-fallback]` badge on.
   - Else: `tmux -L sb new-session -A -s sb-cockpit -n sb -x 200 -y 50
     -d bash -c 'SB_IN_COCKPIT=1 exec <abs path to sb> --no-bootstrap'`
     then `BindKey("F1", "select-window -t sb-cockpit:0")` then
     `exec tmux -L sb attach -t sb-cockpit:sb`. `syscall.Exec` so the
     current process is replaced; when tmux detaches, sb exits.
   - On any tmux error: log to stderr, fall through to fallback.

7. **`main.go`** — call `MaybeReExecIntoTmux` before loading the TUI.
   If reExeced: unreachable (exec replaced us). If fallback: set a
   package-level flag `cockpit.ExecFallback = true` so the Agent tab
   renders the badge.

8. **`update_agent.go` + `view_agent.go`**:
   - Job row gets a runner badge: `tmux` (green) / `exec` (dim). For
     tmux jobs also render the target (`sb-cockpit:@3`).
   - Selected tmux job: list-right pane shows tail of `LogPath`
     instead of the turn-synthesized transcript.
   - Keybinds:
     - `a` / `enter` on a tmux job → `Client.AttachTmux(id)`.
       Exec-runner jobs keep the existing "enter attached mode" flow.
     - `q` inside sb (any mode) → if `InsideTmux()` and session is
       `sb-cockpit`, call `tmux.DetachClient()`; else exit normally.
     - `d` on a tmux job → stop + delete (same as exec jobs, but the
       stop path kills the window).
   - Header: when `cockpit.ExecFallback` is true show
     `[exec-fallback — no tmux]` in the Agent page header.

9. **Tests**:
   - `tmux_test.go`: fake-tmux shim script written to a temp dir,
     `SB_TMUX_BIN` pointed at it. Asserts `HasTmux`, `EnsureSession`,
     `NewWindow`, `PipePane`, `SelectWindow`, `KillWindow`,
     `ListWindows`, `WindowAlive` each produce the expected argv.
   - `runner_tmux_test.go`: with the shim, `StartJob` creates a
     window, poller sees it go dead (shim echoes `pane_dead=1`), job
     flips to `needs_review`. `ApproveJob` runs post-hooks + sync-back
     as today.
   - `runner_fallback_test.go`: `SB_TMUX_BIN` pointed at `/bin/false`
     → `HasTmux()` false → launching a claude preset uses the exec
     runner and the fallback badge is set.
   - Existing tests stay green (the exec-runner path is untouched for
     claude/codex in unit tests; they hit fake binaries via `Cmd`).

10. **Docs + tracking**:
    - `DEVLOG.md` entry dated 2026-04-21 describing the cockpit v2 cut.
    - `WORK.md` — remove obsolete cockpit tasks that this makes moot;
      add `- [ ] cockpit v2: tmux runner + global-config editor` as the
      current task until ship, then swap for follow-ups.
    - `CLAUDE.md` — add section on the tmux runner's invariants
      (isolated server via `-L sb`, bootstrap self-exec, `-L sb` on
      every call, do not touch user's default server).
    - `README.md` — user-facing note: "sb now launches itself inside a
      dedicated tmux session (`sb-cockpit`). `F1` jumps back to sb
      from a job. Set `SB_NO_TMUX=1` to disable."
    - This RFC: append a `## Postscript` with any deviations from
      the plan discovered during implementation.

### What we explicitly are NOT doing in this cut

- Global-config editor (`g` modal for `CLAUDE.md` / `settings.json` /
  `commands/` / etc.). Keep the RFC section above for reference but
  defer to a follow-up PR — the tmux runner is the risk, the editor
  is CRUD.
- Tmux control-mode (`tmux -C`). Shelling out per call is fine and
  keeps this portable.
- Auto-install of tmux. Fallback badge + done.
- Migration of in-flight exec-runner claude jobs into tmux. Old jobs
  keep their old runner until they finish / are deleted.
- A `Q` / "kill everything" path. Deferred.
- Programmatic `SendInput` to tmux panes. Deferred — user types in
  the tmux window.

### Execution order (so the build stays green at each step)

1. Land `internal/cockpit/tmux.go` + `tmux_test.go`. Shim-only tests
   give us confidence the CLI wrapper is right before anything wires
   it up. No callers yet, so no regressions.
2. Add `ExecutorSpec.Runner`, `Job.TmuxTarget`, `Job.LogPath` to
   `types.go`. Zero-value is backwards-compatible with persisted jobs.
3. Add `runner_tmux.go` + `runner_tmux_test.go`. Not yet called by
   `Manager`.
4. Wire `Manager.LaunchJob` / `StopJob` / `SendInput` / `DeleteJob`
   branches. `Manager.AttachTmux` stub added.
5. Extend protocol: `attach_tmux` method. Update client + server.
   `Client` interface gets `AttachTmux`.
6. Add `bootstrap.go`. Gate with `SB_NO_TMUX`; default off-in-tests.
7. Call `MaybeReExecIntoTmux` from `main.go`.
8. TUI wiring: badges, key handlers, detach-on-`q`, fallback header.
9. Smoke test manually (see `## Verification` above). Fix whatever
   breaks. Write the postscript.
10. Update docs (DEVLOG, WORK, CLAUDE, README).

## Postscript (implementation notes)

_Populated as the implementation lands; see commits dated 2026-04-21._
