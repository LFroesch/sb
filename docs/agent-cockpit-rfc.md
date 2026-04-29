# RFC: Agent Cockpit in `sb` (refined — ready for Codex review)

> **2026-04-21 update — jobs-are-chats.** The PTY-embedded executor path described below
> was replaced with a per-turn `exec.Cmd` model: each job is a multi-turn session
> (`Turns []Turn`, `SessionID`, `StatusIdle`), claude uses native `--session-id` / `--resume`,
> codex/ollama replay history to stdin, shell is one-shot per turn. `pty.go` and
> `creack/pty` are gone. Jobs and chats are the same primitive — freeform launches
> carry no `Sources`; sourced launches sync back on approve as before. References to
> "PTY pool" / `pty.go` below are historical.

## Context

`sb` already aggregates WORK.md-style files across many projects, routes brain dumps with an LLM, and understands per-project context. This RFC turns `sb` into a **cockpit** for coding-agent orchestration (Claude Code, Codex, local Ollama, generic shell) so that "checking off tasks" becomes frictionless: pick items from a discovered WORK.md, bundle them into a brief, launch an agent in the right repo, and sync the result back as a deleted line + devlog entry.

The previous revision of this doc was product stance + hidden-requirement checklists with no implementation shape. This revision locks the architecture and phases the build. V0 is the horizontal slice that proves the core model; V1/V2/V3 are sketched only enough to prove the architecture holds without schema churn.

## Locked decisions

- Cockpit lives inside `sb` as a new **Agent** tab, backed by a small **`sb-foreman`** daemon that owns PTYs + job state over a local unix socket. Jobs survive `sb` restart; the TUI is a thin client.
- Product stance: `sb` is a **work orchestration cockpit first**, with config/library authoring as a secondary workflow. Quick launch must stay task-first; deeper prompt/hook/provider authoring belongs in a separate library/editor flow.
- Architecture must anticipate **swarms** (N parallel jobs from one task), **campaigns**, **night queue**, **master console**, **approval gates** — but V0 ships none of that.
- Executors out of the box: Claude Code CLI, Codex CLI, local Ollama/llm scripts, and a generic shell brief escape hatch.
- V0 task picker: pick a discovered file, list its `- ` items as selectable, multi-select → **one bundled job**. Repo auto-inferred from the file's project. Freeform brief + manual repo also supported.
- Hooks: pre-launch shell, post-run shell, prompt-template injection, iteration policy (one-shot / loop-N / until-signal), plus named **role profiles** (code-analyzer, PM, senior-dev, etc.).
- V0 sync-back: on "accept result", **delete** the source `- ` line and **append** an entry to the project's DEVLOG.md.

## Big-picture information architecture

The cockpit should stop pretending one "preset settings" page can do every kind of agent authoring. The cleaner model is:

- **Providers**: concrete runtimes and executors (Claude/Codex/Ollama/shell, runner, model, args, auth/env expectations).
- **Components**: reusable prompt and behavior building blocks (system prompts, prompt snippets, hook packs, iteration policies, permission profiles). Some of these do not exist as first-class persisted objects yet, but the UI/terminology should move in this direction.
- **Recipes**: launchable compositions that bundle role/persona plus selected components and a suggested provider/runtime. Internally the current `LaunchPreset` struct continues to back this concept until the schema expands.

That split should drive the UI:

- **Quick launch** (`n`): task-first, choose a saved recipe, optionally override provider, add a brief note, review, launch.
- **Guided composition** (future): step through intent -> recipe/components -> provider -> review, with the option to save the assembled configuration as a recipe.
- **Agent Library** (`m`): authoring surface for reusable objects. Today that means recipes and providers; future component types should land here rather than being bolted into the launch flow.

## Architecture (forward-compatible, ships in slices)

```
 ┌──────────────────────────────────────────────────┐
 │ sb TUI (Bubble Tea)                              │
 │  ├─ existing tabs (Dashboard, Dump, Project)     │
 │  └─ NEW: Agent tab                               │
 │      ├─ job list (needs-attn / running / recent) │
 │      ├─ task picker (file → `- ` items)          │
 │      ├─ launch modal (preset + overrides)        │
 │      └─ attached session view (tail + input)     │
 └────────────────▲─────────────────────────────────┘
                  │ unix socket: ~/.local/state/sb/foreman.sock
                  │ newline-delimited JSON (request/response + event stream)
 ┌────────────────┴─────────────────────────────────┐
 │ sb-foreman daemon (Go, single binary)            │
 │  ├─ job registry + lifecycle FSM                 │
 │  ├─ PTY pool (creack/pty)                        │
 │  ├─ event log (append-only JSONL per job)        │
 │  ├─ preset + hook runner                         │
 │  ├─ sync-back worker (edits WORK.md / DEVLOG.md) │
 │  └─ future: scheduler, approval gates, swarms    │
 └──────────────────────────────────────────────────┘
        writes: ~/.local/state/sb/jobs/<id>/…
        reads:  ~/.config/sb/presets/*.json
```

### Data model (locked now so swarms/campaigns drop in later)

```go
// internal/cockpit/types.go
type JobID string
type CampaignID string

type SourceTask struct {
    File    string // absolute path to WORK.md-style file
    Line    int    // 1-indexed line of the `- ` item (0 = freeform/no source)
    Text    string // the `- ` item text, verbatim
    Project string // sb project Name (derived)
    Repo    string // resolved repo path
}

type Job struct {
    ID             JobID
    CampaignID     CampaignID // "" for solo jobs; set when job is one of a swarm
    PresetID       string
    Sources        []SourceTask // bundled items; len>=1 for task-picker launches, 0 for freeform
    Brief          string       // final composed brief handed to the executor
    Repo           string
    Executor       ExecutorSpec // type + model + extra flags
    Hooks          HookSpec     // pre, post, prompt-templates, iteration
    Permissions    string       // preset name: "read-only", "scoped-write", "wide-open"
    Status         Status       // queued|running|paused|needs_review|blocked|completed|failed
    CreatedAt      time.Time
    StartedAt      time.Time
    FinishedAt     time.Time
    ExitCode       int
    TranscriptPath string
    EventLogPath   string
    ArtifactsDir   string
    SyncBackState  SyncBack // pending|applied|skipped|failed
}

type Campaign struct {
    ID        CampaignID
    Goal      string
    JobIDs    []JobID
    Strategy  string   // "solo" (V0) | "parallel" | "sequence" | "compare"
    CreatedAt time.Time
}

type LaunchPreset struct {
    ID             string
    Name           string
    Role           string          // "senior-dev", "code-analyzer", "pm", ...
    SystemPrompt   string          // persona/framing injected into brief
    Executor       ExecutorSpec
    PromptHooks    []PromptHook    // file/string injection, ordered
    PreShell       []ShellHook     // run in repo before executor starts
    PostShell      []ShellHook     // run after executor exits; gates needs_review
    Iteration      IterationPolicy // one-shot | loop_n{N} | until_signal{sentinel|file}
    Permissions    string
}

type Event struct { // append-only JSONL per job
    TS      time.Time
    Kind    string      // status_changed | stdout | stderr | hook_started | hook_finished | review_set | synced_back
    Payload interface{}
}
```

**Why this shape holds for V1/V2/V3:** Campaigns already wrap jobs, so a swarm is just N jobs sharing a CampaignID. Foreman-managed jobs can still use the same `Status=queued` state plus explicit scheduler metadata without baking "night eligibility" into presets. Iteration is a policy on the job, not baked into the executor — the same executor implementation handles all three modes. Hooks are ordered lists, so prompt-template injection + repo-context fetch compose cleanly.

### Protocol (sb ↔ foreman)

Newline-delimited JSON over a unix socket. One connection per TUI; socket auto-starts the daemon if nothing is listening.

Requests: `launch_job`, `list_jobs`, `get_job`, `attach(job_id)` (stream), `send_input(job_id, data)`, `stop_job`, `approve_job`, `retry_job`, `list_presets`, `reload_presets`.

Events (push, when attached or subscribed): `job_status_changed`, `job_output`, `hook_finished`, `review_set`, `synced_back`.

Keep it JSON-RPC-ish but boring — one Go `encoding/json` Decoder per direction. No gRPC, no MCP in V0.

### Storage layout

Compatibility note: the current on-disk schema still uses `presets` / `LaunchPreset`. User-facing copy can call these **recipes** now, but migration of file names / type names can happen later once the broader component model settles.

```
~/.config/sb/presets/               # user-editable; seeded on first run
  claude-senior-dev.json
  codex-scaffold.json
  ollama-docs-tidy.json
  shell-escape.json                 # generic shell escape hatch
~/.local/state/sb/
  foreman.sock
  foreman.pid
  foreman.log
  jobs/
    <job_id>/
      job.json                      # persisted Job struct, updated in place
      transcript.log                # raw PTY dump (user sees this on attach)
      events.jsonl                  # append-only Event stream
      brief.md                      # composed brief, preserved for retry/audit
      artifacts/                    # diffs, patches, screenshots, exported summaries
  campaigns/<campaign_id>.json      # V1; directory exists from day one
```

## V0 — ships first

Goal: single daily-loop path; proves the core data model + daemon + TUI wiring.

**Scope (must):**

1. `sb-foreman` daemon binary (new `cmd/foreman/` under sb) — boots, listens on socket, handles `launch_job` / `list_jobs` / `attach` / `stop_job` / `approve_job`, persists state.
2. `internal/cockpit/` package in sb with types, socket client, and an in-proc fallback (so tests don't require the daemon).
3. New **Agent** tab in sb TUI:
   - job list (grouped: needs-attn, running, recent)
   - task-picker page: file list (reuses `workmd.Discover`) → `- ` items with checkboxes → multi-select → new-job composer
   - new-job composer: pick recipe, optionally override provider, edit brief, review, confirm
   - attached view: tail transcript, send input, stop
4. Four seed recipes (still stored as presets on disk) materialized in `~/.config/sb/presets/` on first run (claude-senior-dev, codex-scaffold, ollama-docs-tidy, shell-escape).
5. Hook execution: pre-shell + post-shell + prompt-template injection. Iteration policy wired but V0 only exercises `one-shot`.
6. Sync-back: on approve, delete source `- ` line in its file, append dated entry under `## DevLog` in the project's DEVLOG.md (or create the section if missing). Reuses `workmd` project-root logic.
7. Persistence: jobs survive sb quit and daemon restart (read `jobs/<id>/job.json` on startup, reconcile live PTYs).

**Explicitly NOT in V0:** night queue, master console, campaigns/swarms (data model ready, UI not), approval gates beyond `approve=delete+devlog`, remote control, autonomous task picking.

**New UI keys on Agent tab:**

- `n` new launch → task picker
- `enter` attach to job
- `s` stop, `a` approve (triggers sync-back), `r` retry, `p` pause
- `esc` back to job list

## V0.5 — shipped (socket daemon + preset/provider split)

V0 ran the cockpit in-proc inside sb; jobs died when sb quit. V0.5 closes that gap:

- `cmd/foreman` is now a real daemon. `cockpit.ListenUnix` + `cockpit.Serve` wrap `Manager` behind an NDJSON unix socket (`~/.local/state/sb/foreman.sock`). Protocol: `launch_job`, `list_jobs`, `get_job`, `stop_job`, `approve_job`, `retry_job`, `send_input`, `read_transcript`, `subscribe`, `ping`.
- `SocketClient` in `internal/cockpit/client.go` implements a new `Client` interface that `Manager` also satisfies. sb always holds a `Client`, never a `*Manager` directly.
- `EnsureDaemon(paths, binary)` dials the socket; if nothing answers it forks `sb-foreman -serve` (detached via `setsid` on unix), waits up to 3s for the socket, and re-dials. Failure falls back to in-proc `Manager` so the TUI still works offline.
- Presets were split from providers. In the current product language these presets are **recipes**: role-centric (`senior-dev`, `bug-fixer`, `explainer`, `pm`, …) launch definitions with a *suggested* executor. `ProviderProfile` describes an executor independently (`claude`, `ollama-qwen`, `shell`, …). `LaunchRequest.Provider` overrides the recipe's default at launch time, so any recipe can drive any provider. The new-job composer gets a dedicated provider picker.
- Config gains `cockpit_daemon` (default `true`) and `cockpit_foreman_bin` (optional). Header shows `[daemon]` / `[in-proc]`.
- Covered by `socket_test.go` (end-to-end: launch → stdout event → status=completed → transcript round-trip).

Open V0.5 follow-ups rolled into V1:

- Dirty-repo refusal for sync-back (prompt before destructive delete).
- Client reconnect on socket drop mid-session (today a daemon restart forces a sb restart to pick up the new socket).

## V1 — next slice (architecture already supports)

- Launch recipes/providers editable in-app as the first part of a broader Agent Library; prompt snippets / hook packs / policy profiles should become first-class library objects instead of more fields on one flat preset editor.
- Master console pane: a text box that drives `launch_job` / `list_jobs` / `approve_job` via the LLM.
- Post-hook gating: non-zero exit from post-shell → `needs_review` instead of `completed`.
- Iteration: `loop_n` and `until_signal` wired end-to-end.
- Simple review diff: `git diff` captured as an artifact at approve time.
- Reconnect-on-drop for the socket client.

## V2 — foreman mode

- Foreman mode runs queued work serially per enabled repo: at most one write-capable job may actively change a given repo at a time, while the foreman moves repo-by-repo through the queue.
- Queue policy must lock repo ownership explicitly before launch so overlapping changes are impossible by default. Read-only analysis jobs can be a separate policy later; the default operator mental model stays "one repo, one active change".
- Foreman-managed unattended execution builds on that same queue/lock model, but should launch all eligible jobs it safely can instead of pretending everything is one serial queue. Same-repo write-capable work stays serialized; different repos can run in parallel; skipped/deferred reasons stay visible.
- Swarms: campaign with `Strategy=parallel`, N jobs off one brief, compare view.
- Per-repo worktree isolation (git worktrees so two jobs don't clobber).
- Usage-threshold guards (token budget, days-until-reset).

## V3+

- Remote summaries + approvals (phone/email/chat bridges).
- Bounded autonomous work picking from `sb`'s dump/backlog.
- Broader workload classes (docs, research, job-search pipelines) reusing the same job model.

## Critical files

**New (V0):**

- `sb/cmd/foreman/main.go` — daemon entry
- `sb/internal/cockpit/types.go` — Job / Preset / Event / Campaign structs
- `sb/internal/cockpit/client.go` — sb-side socket client
- `sb/internal/cockpit/server.go` — daemon-side protocol handler
- `sb/internal/cockpit/registry.go` — job persistence + lifecycle FSM
- `sb/internal/cockpit/pty.go` — PTY spawn + IO pump (creack/pty)
- `sb/internal/cockpit/presets.go` — load/save/seed presets
- `sb/internal/cockpit/hooks.go` — pre/post shell + prompt-template runners
- `sb/internal/cockpit/syncback.go` — delete source line + append to DEVLOG.md
- `sb/internal/cockpit/taskpicker.go` — parse `- ` items out of a discovered file
- `sb/view_agent.go` — Agent tab rendering
- `sb/update_agent.go` — Agent tab key handling + messages
- `~/.config/sb/presets/*.json` — four seeds (materialized on first run by a `WriteDefaultPresets` helper)

**Modified (V0):**

- `sb/main.go` — start/ensure foreman daemon, wire cockpit client
- `sb/model.go` — add `pageAgent`, new modes (`modeAgentList`, `modeAgentPicker`, `modeAgentLaunch`, `modeAgentAttached`), cockpit client handle
- `sb/view.go` — route `pageAgent` to `renderAgent`, add Agent tab to header pages list (lines 68–75)
- `sb/update.go` — dispatch key handling for `pageAgent`
- `sb/internal/config/config.go` — add `presets_dir`, `foreman_socket`, `foreman_autostart` fields with sane defaults
- `sb/internal/workmd/workmd.go` — expose helper to resolve project repo path from a file path (may already be derivable from `Project.Path`)
- `sb/README.md` — new Agent tab section + presets config docs
- `sb/WORK.md` — move the agent-cockpit tasks into Current Tasks with V0/V1 grouping
- `sb/DEVLOG.md` — entry when V0 lands

### Existing sb code to reuse

- `workmd.Discover` + `Project` struct (`internal/workmd/workmd.go`) — multi-root scan, title/description parsing, label collision resolution. Task picker feeds off this.
- `llm.Client` (`internal/llm/llm.go`) — wraps ollama / openai / anthropic. Used by V1 master console + any prompt-template hook that wants an LLM to compose context.
- `config.Config` (`internal/config/config.go`) — provider profiles, env overrides, path expansion. Add cockpit fields here, don't fork config.
- `logs.Open` (`internal/logs/logs.go`) — structured JSON logs with rotation. Foreman writes to `~/.local/share/sb/logs/foreman.log` via the same helper.
- `markdown.Render` + `diff` packages — transcript preview + V1 review diffs.
- `page` / `mode` enum pattern in `model.go` (lines 33–71) — follow it verbatim for new Agent states; don't invent a new state-machine idiom.

## Verification

**Build + unit tests**

- `cd sb && go build ./...` — both `sb` and `cmd/foreman` must compile.
- `go test ./internal/cockpit/...` — covers: preset load/save roundtrip, brief composition with prompt-templates, sync-back (delete line + DEVLOG append) against golden files, job-lifecycle FSM transitions, registry rehydration after daemon restart.

**Manual end-to-end (V0 accept criteria)**

1. Start `sb` with a clean `~/.local/state/sb/`. Daemon auto-starts; socket appears; `ps` shows `sb-foreman`.
2. Switch to the new Agent page. Empty state visible.
3. `n` → pick `sb/WORK.md` → see its `- ` items listed, multi-select 2, confirm.
4. Launch modal shows `claude-senior-dev` preset pre-filled; confirm.
5. `claude` runs in a PTY in `~/projects/active/tui-suite/sb`; output streams in the attached view; status is `running`.
6. Send input via the attached view.
7. On exit, post-shell hook (`git diff --stat`) runs; job moves to `needs_review` if diff non-empty, else `completed`.
8. `a` → source `- ` lines are removed from `sb/WORK.md`, DEVLOG entry appended, `SyncBackState=applied`.
9. Quit `sb`. Re-launch. Previous completed jobs still listed with transcripts intact.
10. Kill daemon (`kill $(cat ~/.local/state/sb/foreman.pid)`). Launch `sb`. Daemon restarts; `list_jobs` returns the same history.
11. Repeat with `codex-scaffold` on a different file; with `ollama-docs-tidy` on a docs file; with `shell-escape` running `echo hi` to prove the escape hatch.

**Regression** — existing `sb` behavior (dashboard, dump, cleanup, plan, todo, search, favorites) unchanged; no config migration required for users who never open the Agent tab. Run `go test ./...` for the existing suite.

## Open items to revisit after Codex feedback

- Exact wire format (JSON-RPC 2.0 vs ad-hoc NDJSON). Leaning NDJSON for V0; revisit if Codex pushes back.
- Whether `sb-foreman` lives in the `sb` go module (`cmd/foreman`) or as a sibling app in `tui-suite/`. Default: `sb/cmd/foreman` to share `internal/cockpit` — split later only if release cadence diverges.
- V0 sync-back safety: should it refuse to run when the repo has uncommitted changes touching the `- ` line, to avoid clobbering in-progress edits?

## Codex review prompt

> You are reviewing `/home/lucas/projects/active/tui-suite/sb/docs/agent-cockpit-rfc.md`. Background: `sb` is a Go/Bubble Tea second-brain TUI that already discovers WORK.md files across projects and does LLM-assisted routing/cleanup. This RFC adds a cockpit for launching coding agents (Claude Code, Codex, Ollama, shell) against `- ` items from those files. V0 ships a daemon (`sb-foreman`) + new Agent tab in `sb`; V1/V2 add swarms, night queue, master console.
>
> Critique the RFC on:
> 1. **Daemon/TUI boundary** — is an NDJSON unix socket the right abstraction, or should it be JSON-RPC 2.0 / gRPC / MCP? Concrete tradeoffs only.
> 2. **Data model** — does `Job` + `Campaign` + `LaunchPreset` actually support parallel swarms and iteration without schema churn, or will V2 force migrations?
> 3. **PTY handling** for `claude` and `codex` CLIs specifically — raw mode, terminal resize, clean exit detection, handling interactive prompts vs. headless flags.
> 4. **Sync-back safety** — deleting source lines is destructive. What's missing for recovery (undo, soft-delete, dry run, dirty-repo refusal)?
> 5. **V0 cut** — is anything in V0 actually a V1 concern that's bloating the first slice? Is anything V1 that V0 secretly needs to be useful on day one?
> 6. **Hidden requirements** — name any item from the prior RFC's "Hidden Requirements" checklist (authority model, state boundaries, job lifecycle, safety model, defaulting, artifacts, context packaging, failure detection, concurrency, recovery) that this revision is still dodging.
>
> Return a short critique in the style of a senior-engineer code review: specific, cited to section or line in the RFC, no generic platitudes. If you think the architecture is wrong at the root, say so and propose a concrete alternative rather than a list of caveats.
