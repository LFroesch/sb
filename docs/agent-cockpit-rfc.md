# RFC: Agent Dashboard and Foreman Mode in `sb`

## Before Implementation

We need to get into the nitty gritty before implementation.

This concept is strong, but it will get sloppy fast if the hard edges stay vague. Before building, lock the authority model, state boundaries, job lifecycle, safety model, and defaulting behavior.

## Summary

`sb` should evolve from a markdown work dashboard into a dispatch + supervision layer for agent work across many projects.

This is not a launcher-only feature. The point is to turn `sb` context into well-configured agent jobs quickly, keep those jobs visible, preserve history, and let the same system grow into unattended queue running later.

The first release should be a small horizontal slice:

- real enough to use daily
- narrow enough to ship without every advanced feature
- built so day mode and night/foreman mode share the same core model

## Product Stance

`sb` is already the second-brain layer:

- project discovery
- markdown task context
- routing context
- prioritization surface

The new agent dashboard becomes the third-hand layer:

- dispatch tasks into jobs fast
- apply reusable roles, workflows, policies, and executor presets
- supervise queued/running/paused work
- preserve logs, summaries, and resume state
- surface approvals and review gates
- sync meaningful outcomes back into markdown

Core ideas:

- `task -> launch preset -> job`
- modular underneath, frictionless on top

## Hidden Requirements

These are the easy-to-miss requirements that need concrete decisions before implementation.

### Authority model

- user vs master session vs foreman vs worker authority
- who can launch, amend, approve, stop, apply, queue, or auto-continue work
- which actions always require explicit human approval

### State model

- what lives in markdown
- what lives in runtime state
- what lives in PTY transcripts
- what gets summarized and persisted
- what is disposable

### Job lifecycle

- exact meaning of launch, queue, run, pause, resume, retry, fork, handoff, complete, fail
- what makes a job `needs_review` vs `completed`
- when a result is only a proposal vs safe to apply/sync back

### Safety model

- permission presets
- approval gates
- repo isolation and dirty-worktree rules
- multi-job conflicts in the same repo
- unattended/night-mode limits

### Defaulting model

- how tasks map to default launch presets
- how projects/work item types influence role/workflow/policy/executor
- when the app should auto-suggest vs auto-run

### Artifact model

- patches, diffs, plans, notes, logs, summaries, generated files
- where artifacts live
- how they attach to jobs and reviews

### Context packaging

- how `sb` context becomes a launch brief
- what task/project/history context gets passed to workers
- what the master session knows vs what worker sessions inherit

### Failure detection

- loop detection
- low-value progress detection
- blocked vs risky vs idle vs waiting-for-review states
- escalation rules

### Concurrency and scheduling

- per-repo limits
- per-executor/provider/account limits
- queue priority rules
- night-window scheduling and usage thresholds

### Recovery and observability

- what must survive app restart
- how active sessions are rehydrated
- what the dashboard must show at a glance
- how history/search should work across jobs, tasks, roles, and outcomes

## Control Plane

The app should have one control-plane layer over many worker sessions.

Pieces:

- `sb` remains the system of record and main UI shell
- worker jobs run as PTY-backed terminal sessions
- a master session or master console can inspect and manage the job system
- the master session is not the only source of truth

The master/control layer should be able to:

- summarize active work
- show what needs approval
- pause, resume, stop, retry, or amend jobs
- spawn new jobs from tasks or briefs
- queue work for unattended execution later

This makes the system feel like one hub without turning a single long chat into the whole architecture.

## V1

### Goal

Make it easy to knock out many small coding tasks across many repos from one place, with the beginnings of unattended queue execution built in.

### Must-have outcomes

- launch jobs from existing `sb` tasks/projects in a few steps
- use reusable launch presets instead of retyping prompts/settings every time
- see queued, running, paused, completed, blocked, and review-needed jobs in one dashboard
- drill into underlying terminal/session history when needed
- pause, retry, resume, stop, and approve from the cockpit
- sync important outcomes back into the source markdown task system
- support a basic night queue for approved unattended work
- support a master console/session for text-driven summaries and job control

### Primary workload

The main V1 workload is many small one-off coding tasks spread across many repos.

Examples:

- small fixes
- bug investigation
- scaffold work
- docs/readme cleanup
- local low-risk night jobs

### UI shape

Keep the existing `sb` dashboard and add one `Agent Dashboard` tab/page.

The page should be organized around jobs, not raw terminals.

Top-to-bottom priority:

1. needs attention
2. queued/running work
3. launchable suggestions / dispatch queue
4. recent history

Jobs are the main row type. The underlying terminal/session is drill-in detail.

The agent dashboard can later grow a master console pane, but V1 should already assume the control-plane concept exists.

### Core objects

#### Task

A markdown-backed work item discovered by `sb`.

#### Role

Who the agent is acting as.

Examples:

- `senior_dev`
- `project_manager`
- `cto`
- `marketing`
- `researcher`
- `docs_editor`

#### Workflow

How the work should be done.

Examples:

- `small-fix`
- `bug-investigation`
- `scaffold`
- `readme-refresh`
- `night-local-ollama`

#### Policy

What the job is allowed to do.

Examples:

- permission preset
- autonomy level
- retry behavior
- review rules
- night-queue eligibility

#### Executor preset

Where and how the job runs.

Examples:

- `codex-safe`
- `claude-wide`
- `ollama-local`
- `cheap-night-runner`

#### Launch preset

A saved combination of role, workflow, policy, and executor defaults.

Fields may include:

- role default
- prompt/system framing
- workflow hooks
- policy preset
- executor preset
- review behavior
- sync-back behavior

This should be the main frictionless user-facing object for daily launches.

#### Job

One launched unit of agent work tied to a task or ad hoc brief.

Jobs should track:

- source task/project
- role/workflow/policy/executor selection
- launch preset used
- status
- summary
- logs/history
- resume metadata
- approvals/review state
- resulting markdown sync-back

#### Session

The underlying PTY-backed process behind a live job.

Sessions should support:

- launch in a repo/context
- output capture
- input injection
- attach/detach
- stop/kill
- transcript persistence

#### Campaign

A grouped or iterative sequence of jobs. V1 can keep this lightweight, but the concept should exist from the start.

#### Event

Append-only state update for a job or campaign.

### V1 workflow

Day mode:

- open `sb`
- review items that need attention
- launch a task with a default or chosen launch preset
- monitor progress from the agent dashboard
- inspect terminal/log history only when needed
- approve, retry, pause, resume, or stop
- write meaningful result back to markdown
- optionally use the master console/session to manage jobs by text

Night mode:

- run the same job model under stricter unattended policies
- start from an approved queue
- support local/low-risk jobs first

### Launch UX

The design rule is:

composition in the model, defaults in the UX.

Fast path:

- select task
- launch with the default preset

Override path:

- select task
- choose a preset
- launch

Advanced compose path:

- role
- workflow
- policy
- executor
- hooks
- night-queue eligibility

The user should not have to assemble these pieces manually for most launches.

### Architecture boundary

- `sb` is the first cockpit UI
- runtime orchestration state must live outside the Bubble Tea process
- markdown remains the human planning surface
- markdown is not the only runtime store
- sync-back writes important human-facing outcomes into markdown
- Codex CLI and Claude CLI are the first serious executors
- existing `sb` LLM/provider support remains useful for routing, planning, summarization, and small helper work
- live worker jobs should be PTY-backed so the hub can launch, inspect, attach, and interact with terminal-native tools
- the master session/control layer operates over structured app state, not over hidden chat memory alone

### Status model

V1 should support at least:

- `queued`
- `running`
- `paused`
- `needs_review`
- `blocked`
- `completed`
- `failed`
- `deferred_to_night_queue`

### Non-goals

- full remote/mobile control
- broad autonomous task-picking across everything
- full non-code workload automation
- advanced account/provider balancing
- a new standalone TUI
- a giant profile/plugin ecosystem before the core loop works

## V2 Notes

Keep this section short and directional.

### Foreman expansion

- real night/foreman mode
- approved queue runner first
- later bounded autonomous work-picking
- policy knobs for usage thresholds, days until reset, concurrency, provider/account preference, retry limits, and review requirements
- richer master-session behavior over the same job/session store

### Remote/away mode

- remote summaries
- remote approvals
- away-from-PC control
- notifications across phone/email/chat surfaces

### Broader workload classes

Use the same `task/profile/job/policy` model for:

- ideas
- standards/versioning
- readmes/docs
- knowledge-bank upkeep
- app ideas and scaffolding
- competitor/market research
- job-search/admin pipelines
- new-tech exploration

### Smarter utilization

- continuous stream of useful queued work
- idle-capacity consumption
- token/account-aware scheduling
- bounded autonomous iteration

## Acceptance Criteria

V1 is successful if it can:

- launch a small task into a job quickly
- show all active and queued jobs in one dashboard
- preserve enough history to understand and resume work later
- surface review-needed work clearly
- keep jobs linked to their source tasks/projects
- sync meaningful outcomes back into markdown
- run a basic unattended night queue on the same core model
- survive `sb` restarts without losing runtime state
- manage PTY-backed sessions from a single control hub
- support text-driven control through a master console/session without making that chat the system of record

## Decision

Build this as a real 80/20 horizontal slice:

- broad enough to cover the full daily loop
- narrow enough to avoid v2 sprawl
- shaped around small-task dispatch first
- ready to grow into a broader work operating system later
