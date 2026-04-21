# RFC: sb as the First Cockpit for Agent Orchestration

## Summary

`sb` should grow into the first operator cockpit for coding-agent orchestration, not just a task launcher. It already owns cross-project markdown context, which makes it the natural place to choose work, inspect state, and review outcomes. The orchestration runtime itself should remain separable from the Bubble Tea app so unattended execution, policies, and notifications are not trapped inside the TUI process.

Recommended boundary:

- `sb` is the first human-facing cockpit
- a separate orchestration core owns runtime state, policies, and events
- Codex CLI and Claude CLI and Ollama (Dwight?) are initial executors
- agent / hook / context/ prompt management / currently running dashboard etc / task starter / queueing / ralph wiggum iteration etc
- mobile updates and remote approvals are first-class design goals
- a separate TUI is deferred unless orchestration workflows outgrow `sb`

## Problem

Current agent workflows break down at the control layer:

- tasks exist across many markdown files and projects
- launching a coding assistant from a task is decently easy, but supervising many runs is not
- unattended work needs policy, stopping conditions, and review gates
- results need to flow back into markdown so the task system stays current
- when the user is away, updates and simple decisions should still be possible

A simple launcher inside `sb` would only solve the first 10 percent of the problem. The real opportunity is a foreman layer that can route work, supervise iteration, summarize progress, and stop when human judgment is required.

## Product Thesis

`sb` already acts as a second-brain control plane over `WORK.md` files. That makes it the right place to become a cockpit for coding assistants.

This is not just "open Codex or Claude from a task." The product direction is:

- choose work from existing markdown context
- launch and supervise multiple agent runs
- sequence related runs toward a larger outcome
- keep markdown plans and statuses up to date
- notify the user when progress or judgment matters
- allow simple remote responses while away

The missing product in most agent setups is not another model wrapper. It is durable operator control over many evolving runs.

## Recommended Product Boundary

Start `sb`-first, but do not make the orchestration model an implementation detail of the TUI.

### `sb` responsibilities

- discover projects and markdown task files
- let the user select tasks, projects, or groups of work
- show live run/campaign state in a cockpit view
- present review gates, summaries, and diffs
- write meaningful outcomes back into markdown

### orchestration core responsibilities

- own runs, campaigns, policies, and event history
- launch executors and track their lifecycle
- handle unattended iteration rules
- detect blocking, looping, risky, or review-worthy states
- emit notification events for local and remote surfaces

### executor responsibilities

- run Codex CLI, Claude CLI, and future backends behind a stable executor interface
- expose normalized output, exit state, and checkpoints to the orchestration core

### notification responsibilities

- consume structured events
- deliver desktop alerts, mobile updates, and simple remote action links or reply commands

This separation keeps the first UI inside `sb` while preserving a clean escape hatch if orchestration later deserves a dedicated interface.

## Why Not a New TUI First

Starting with a second TUI would duplicate the one thing `sb` already does well: organize work across markdown-backed projects.

Reasons to stay `sb`-first:

- `sb` already has project discovery and the operator mindset
- `WORK.md` context is already the user's planning system
- the hard unsolved problem is orchestration state and policy, not terminal navigation
- a second TUI now would mostly fork UX before the domain model is stable

When a new UI would be justified later:

- orchestration workflows dominate usage more than task management
- campaigns and approvals no longer map cleanly onto `sb`'s dashboard model
- remote/web/mobile surfaces become more important than the local markdown cockpit

## Core Concepts

### Task

A markdown task or work item discovered by `sb`. Tasks remain the human planning surface and the default source from which runs are created.

### Run

One execution attempt by an agent against a specific task or brief. A run has an executor, prompt/parameters, status, logs, outputs, and a final or intermediate outcome.

### Campaign

A managed sequence of runs toward a broader outcome. A campaign may include retries, follow-up runs, or work across multiple tasks or projects. "Ralph Wiggum mode" belongs here: it is a policy-driven campaign behavior, not a standalone feature toggle.

### Policy

Rules that decide when a run or campaign should continue, pause, retry, escalate, or stop for review.

### Review Gate

An explicit pause point that requires human input before more work happens. Review gates exist because many coding tasks are neither clean pass/fail nor safe to continue indefinitely.

### Event

A structured state update emitted by the orchestration core: started, checkpoint reached, tests failing, likely complete, blocked, needs review, canceled, and so on.

### Notification

A delivery of important events to a surface outside the main cockpit: desktop notifications, phone messages, or future chat integrations.

### Executor

An abstraction over runnable coding-agent backends such as Codex CLI and Claude CLI.

### Outcome

The orchestration core's normalized interpretation of what just happened: progressing, blocked, looping, risky, likely complete, needs human product judgment, or failed.

## Source of Truth

`WORK.md` should remain the source of truth for human planning. It should not be overloaded into the only store for runtime orchestration state.

Recommended split:

- markdown remains the human-readable plan and backlog surface
- runtime state lives in a dedicated orchestration store
- important orchestration outcomes sync back into markdown as status updates, notes, links, or summaries

Markdown alone is not sufficient for:

- append-only event history
- multiple attempts per task
- executor-specific logs and artifacts
- transient policy state
- remote approvals and notification bookkeeping

The design should therefore assume bidirectional sync instead of markdown-only runtime state.

## Intended Workflows

### Launch from tasks

The user selects one or more markdown tasks in `sb` and creates runs from them, choosing an executor and profile.

### Supervised iteration

The orchestration core allows safe continued work when failures are straightforward and bounded, but stops at review gates when judgment is needed.

### Campaign execution

The user groups related tasks into a higher-level effort and allows the foreman to sequence the work.

### Away mode

While the user is away, the system continues safe work, sends concise progress summaries, and pauses when approval or strategy is needed.

### Markdown sync-back

Meaningful outcomes are reflected back into the originating task system so the cockpit and the task files do not drift apart.

## Review and Stopping Logic

The system cannot treat coding work as binary pass/fail. It needs richer stop categories.

Runs or campaigns should be able to stop as:

- `progressing`
- `blocked`
- `looping`
- `risky_repo_state`
- `needs_human_judgment`
- `likely_ready_for_review`
- `hard_failed`

Examples of review-worthy states:

- tests mostly pass and the goal appears substantially reached
- repeated retries are no longer producing new progress
- the executor requests clarification
- the repo state looks risky for unattended continuation
- product or architectural tradeoffs appear instead of straightforward coding work

## Remote Interaction Model

Phone updates are a core goal, not an afterthought.

Remote updates should support:

- concise campaign or run summaries
- important state changes
- links or identifiers for the affected project/task
- simple remote decisions: continue, stop, retry, escalate, mark for review

The orchestration architecture should expose these as events and actions, not as a provider-specific SMS implementation baked into the core.

## Initial Executor Scope

The first executors should be:

- Codex CLI
- Claude CLI

The orchestration core should normalize enough state around them that additional backends can be added later without rewriting cockpit behavior.

## Constraints and Non-Goals

Non-goals for the initial direction:

- building a new standalone TUI before the orchestration model is proven
- stuffing all runtime state directly into markdown
- reducing the system to a simple task launcher
- locking the core to a single executor

Constraints to preserve:

- `sb` remains useful as a markdown control plane even before orchestration is fully built
- the first UI should feel native to `sb`
- architecture should not prevent later web/mobile/dedicated-client surfaces

## Decision

The recommended direction is:

1. Treat this as an orchestration/foreman product, not a launcher feature.
2. Use `sb` as the first cockpit UI.
3. Keep runtime orchestration state and policy outside the Bubble Tea process.
4. Preserve `WORK.md` as the human planning surface, with sync-back from runtime outcomes.
5. Design mobile updates and remote approvals into the event model from the start.
6. Revisit a standalone TUI only if the cockpit clearly outgrows `sb`.

## Evaluation Criteria

This direction is successful if it gives the project:

- a coherent answer for why this belongs in `sb` first
- a clear reason not to trap orchestration inside the TUI
- a shared vocabulary for tasks, runs, campaigns, policies, and review gates
- a place for "Ralph Wiggum mode" inside a broader system model
- a basis for future phased implementation without re-arguing the product boundary

## IMPORTANT

- The primary goal of this major feature implementation is to basically have sb have the ability to manage all current agents/etc
- Also to be able to easily "use tokens up" by just being able to batch through my checklists you know? or have them continue to start up new ones while I am away up to a certain limit etc
- I have both codex and claude accounts and ollama locally for small things / docs tuning / intent parsing? idk some way to help integrate it, or could use it as a way to continue certain wiggum loops etc idk down the line, this is a pretty intense update but I need it to really level up and continue developing other things while my bug lists / content generation / scaffolding / all types of work i need autonomously done are kinda "taken care of" if that makes sense, i think this could really be an insane app but it needs to be very carefully crafted
- end goal is to be able to manage and run all of my "sessions/jobs" from one place, see their status etc, all my usage limits etc, swap accounts maybe? idk launch in different providers / hook management / prompt management etc etc / foreman mode etc
- anything else we can think of in this, autonomous log checking / patches / notifier (email?) / parse my emails / parse job apps / do job apps? mine and create knowledge items, scaffold out ideas if they seem like good ideas / organize files or tidy readme.mds all sorts of shit

- i am envisioning a 3rd tab and maybe 4th maybe 5th tab of sb, that are devoted to this whole flow, lets really nail down this whole plan before we move on from this .md file