# Foreman / Night Mode

## Purpose

Foreman is the unattended execution layer of `sb`.

During the day, the user manages projects, captures brain dumps, and runs agents interactively.
At night or while away, the user should be able to hand `sb` a prepared batch of jobs and trust it to run whatever is eligible according to explicit rules.

This is not optional polish. It is part of `sb` V1.

## Core User Story

The user works on tasks all day inside `sb`.

At the end of the day, the user:

1. picks 10+ tasks across one or more repos
2. decides how those tasks should run
3. turns Foreman on
4. walks away

Overnight, `sb` should:

- run the prepared work according to the chosen setup
- avoid conflicting repo writes
- respect runtime/model/policy/budget constraints
- keep clear records of what happened
- stop cleanly when the queue finishes or a limit is hit

In the morning, the user should:

1. open `sb`
2. see what ran, what finished, what failed, and what was skipped
3. review each result
4. accept, reject, retry, or revise
5. turn Foreman off and go back to normal interactive use

That is the product.

## Product Stance

Foreman is not "just queued runs".

Foreman is:

- run preparation
- unattended execution
- explicit on/off lifecycle
- guardrails
- later review in the normal Agents surfaces

The current queueing/campaign machinery is only part of the implementation.

## Required Mental Model

The user should only need to understand:

- `foreman jobs`: runs explicitly sent to Foreman instead of started immediately
- `foreman off`: queue exists but nothing auto-advances
- `foreman on`: unattended execution is allowed
- `night mode`: just using Foreman while away or asleep
- `review`: check back in on the resulting jobs later

The user should not have to think about:

- campaign ids
- queue indexes
- daemon/socket semantics
- tmux plumbing

## Main Flows

### 1. Prepare Foreman Jobs

The user should be able to prepare jobs for Foreman from selected tasks.

Each Foreman-managed job needs:

- source task
- repo
- template
- runtime/model
- permissions policy
- iteration policy
- optional note/instructions

Foreman-level constraints should also exist:

- per-repo serialization when writes would conflict
- token/budget guardrails
- optional stop conditions
- optional max concurrency later

### 2. Turn Foreman On

The user needs an explicit action to enable unattended advancement.

Turning Foreman on should:

- mark unattended execution as live
- start eligible prepared jobs
- clearly show that unattended mode is active
- record start time and active rules

This should feel like flipping the system from interactive supervision into away-mode.

### 3. Overnight Execution

While Foreman is on, `sb` should:

- launch eligible Foreman-managed jobs automatically
- allow different repos to run in parallel when safe
- keep at most one write-capable run active per repo by default
- respect runtime/budget guards before starting another run
- record why a run was skipped, deferred, blocked, or failed
- leave completed runs waiting for human review rather than auto-accepting them

### 4. Turn Foreman Off

The user needs an explicit way to disable unattended advancement.

Turning Foreman off should:

- stop starting new Foreman-managed work
- leave active runs either:
- allowed to finish normally
- or explicitly stopped, depending on the chosen operator action
- preserve the queue state for later resumption

The default safe behavior should be:

- do not start anything new
- do not kill active work unless the user explicitly requests that

### 5. Review Later

The return flow should use the normal Agents surfaces, not a separate special inbox.

The user should be able to review:

- finished runs
- failed runs
- skipped/deferred runs
- why each item ended up in that state
- changed files / diff stat / hook results / source task context

The morning actions should be:

- accept
- retry
- revise and rerun
- skip/archive

## Execution Rules

Foreman must enforce these rules by default:

### Repo Safety

- one write-capable run per repo at a time
- later tasks for the same repo wait until the current write-capable run is no longer blocking the repo

### Review Gate

- completed unattended runs do not auto-accept into `WORK.md`
- they land in review for a human decision

### Budget Guards

Before starting a new unattended run, Foreman should be able to check:

- token budget
- provider/rate-limit state
- optional time window

If a guard blocks execution, the reason must be recorded clearly.

### Permission Respect

Foreman should not silently escalate permissions.

The queued item's policy must determine whether the run is allowed.

### Deterministic Advancement

The operator should be able to tell why the next run was chosen.

If the queue is not simple FIFO, the scheduling rule must be visible.

## V1 Foreman Scope

Foreman V1 should include:

- prepare Foreman-managed jobs from selected tasks
- parallel unattended execution across different repos when safe
- serial per-repo unattended advancement when writes would conflict
- explicit Foreman on/off control
- queue persistence across `sb` restarts
- unattended results visible in the normal Agents review flow
- skip/defer/fail reasons recorded clearly
- basic budget/rate-limit guardrails

Foreman V1 does not need:

- mobile/remote control
- autonomous task picking
- complex parallel swarms
- cross-machine execution
- fully automatic acceptance into project files

## Required UI Surfaces

`sb` needs a real Foreman surface, not just queue hints inside Agents.

Minimum V1 surfaces:

- queue-prep view
- Foreman status panel (`off` / `on` / blocked / draining / finished)
- Foreman-managed job list with per-item state
- explicit on/off controls

The user should be able to tell at a glance:

- is Foreman enabled right now?
- what is running?
- what is pending?
- why is something blocked?
- what needs review?

## Existing Code That Can Be Reused

The current codebase already provides useful building blocks:

- persisted daemon-backed job state
- queued serial task sequences
- repo lock behavior for queued write-capable jobs
- skip-one and skip-rest-of-queue controls
- tmux-backed and exec-backed run handling
- review artifacts and sync-back previews

These are building blocks, not the finished product.

## Gaps That Still Need To Be Built

Foreman/night mode still needs:

- explicit Foreman on/off state
- real unattended scheduler behavior
- queue-prep workflow as a first-class UX
- budget/limit checks before starting the next run
- blocked/deferred reason tracking as a user-facing concept
- clearer Foreman status for pending/running/blocked/draining/finished states
- tests for on/off lifecycle and unattended advancement rules

## Required Tests

Foreman/night mode is not done without tests for:

- turning Foreman on starts eligible queued work
- turning Foreman off stops new starts without corrupting queue state
- cross-repo parallel starts when jobs are otherwise eligible
- repo serialization across multiple repos and multiple queued items
- budget/rate-limit guard blocking
- resuming after `sb` / daemon restart
- unattended results remain visible in normal Agents review states
- skip/defer/fail reason visibility

## Acceptance Criteria

Foreman/night mode is working when this scenario succeeds:

1. user prepares 10+ Foreman-managed jobs across multiple repos
2. user sets per-run template/runtime/policy choices
3. user turns Foreman on
4. `sb` starts as many jobs as it safely can, without overlapping unsafe repo writes
5. stopped/deferred items have clear reasons
6. completed work is waiting in the normal Agents review flow later
7. user can accept/retry/revise from that review flow
8. user turns Foreman off and returns to normal interactive use

If that scenario is not solid, Foreman/night mode is not done.
