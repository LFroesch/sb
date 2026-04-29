# sb Product Definition

## Purpose

`sb` is a terminal control plane for personal project execution.

It has four connected jobs:

1. manage project tasklists stored in `WORK.md`-style files
2. capture messy thoughts and route them into the right project backlog
3. launch and supervise coding agents against concrete tasks
4. optionally run prepared unattended work in a more automated foreman/night mode

The important product rule is that these are **layers of one system**, not four unrelated apps forced into one binary.

## Core Promise

`sb` should let a user move through this loop without friction:

1. see what projects and tasks exist
2. dump or sort new thoughts into the right place
3. choose what to work on next
4. hand a task to an agent when useful
5. review the result
6. accept it back into the task system

If that loop is clear, the product works.
If the loop is obscured by internal machinery, the product feels overengineered even when the features are useful.

## Product Pillars

### 1. Dashboard

Use `sb` to browse, edit, clean up, and prioritize `WORK.md` files across many projects.

This includes:

- project discovery
- preview/edit
- cleanup and normalization
- next-task / daily-plan assistance

### 2. Dump

Use `sb` to capture brain dumps before they are organized.

This includes:

- freeform capture
- LLM-assisted routing
- per-item accept / skip / reroute

This flow is intentionally lightweight and should stay simpler than the Agent flow.

### 3. Agents

Use `sb` to turn a task into execution.

Default user story:

1. choose a task
2. launch an agent
3. monitor progress
4. review output
5. accept, retry, stop, or discard

This is the main execution layer of the app.

### 4. Foreman

Use `sb` to launch or supervise unattended work when the simple one-task-at-a-time loop is not enough.

This is an advanced layer built on top of Agents, not a separate product.

See [foreman-night-mode.md](foreman-night-mode.md) for the concrete unattended-execution workflow and acceptance criteria.

Foreman/night mode should feel like:

- "send these runs to Foreman and let them go"
- "run different repos in parallel when safe"
- "serialize same-repo writes automatically"
- "check back in on the same jobs later"

It should not require the user to learn internal queue jargon before the normal Agent flow makes sense.

## User-Facing Mental Model

The user should only need to learn these concepts:

- `project`: a discovered markdown-backed work area
- `task`: a concrete list item from a project file
- `dump`: unsorted input that needs routing
- `agent run`: an execution attempt against a task or brief
- `review`: decide whether the run should be accepted
- `foreman`: unattended execution for jobs explicitly sent there

That is enough for the product story.

## Concepts That Should Stay Advanced

These concepts are useful implementation/operator details, but they should not dominate the main UX:

- provider
- recipe
- tmux session layout
- daemon vs in-proc
- campaign
- queued/blocked advance states
- transcript transport details
- hook execution internals

They can exist, but the default experience should translate them into plain task/execution language.

## Language Rules

Prefer:

- `task`
- `agent run`
- `review`
- `queue`
- `accept`
- `retry`
- `stop`
- `resume`

Avoid leading with:

- `campaign`
- `executor`
- `launch preset`
- `socket client`
- `sync-back state`

Those terms are fine in docs for implementation or advanced settings, but they should not be the first thing a normal user has to parse.

## V1 Product Boundary

`sb` V1 is successful only if both of these loops are coherent and working:

- the interactive day loop:
- browse and manage `WORK.md` projects
- route brain dumps successfully
- launch an agent from selected tasks
- launch a freeform agent run when needed
- monitor the run
- review the result
- accept the result back into the task system
- resume or stop an in-progress run
- reattach to the shared cockpit state after reopening `sb`

- the unattended foreman/night loop:
- prepare jobs for away-mode execution
- choose template/runtime/policy/iteration setup for that queued work
- explicitly turn Foreman on
- let `sb` launch unattended work safely
- return to the normal Agents review flow later
- explicitly turn Foreman off and resume normal interactive use

V1 does **not** require:

- perfect library/settings architecture
- perfect tmux abstraction
- remote/mobile control
- autonomous task picking outside prepared Foreman jobs
- every advanced control to be polished

## Current Product Read

What is already strong:

- Dashboard + Dump are coherent enough to justify the app
- Agent launching, review, and tmux-backed supervision are real capabilities
- shared cockpit state and reattach behavior are directionally right

What currently hurts comprehension:

- too much advanced vocabulary leaks into the main story
- Agent and Foreman concepts are not layered cleanly enough
- settings/library concepts are more visible than they need to be for basic use
- some controls make sense only if you remember why they were added

## Keep / Simplify / Hide / Defer

### Keep

- project dashboard
- brain dump routing
- sourced agent launch
- freeform agent launch
- review and accept flow
- shared-cockpit reattach
- stop and resume

### Simplify

- Agent page language
- review terminology
- queue semantics shown in the default flow
- settings/library framing

### Hide

- raw provider/recipe complexity unless the user is customizing launches
- tmux details unless the user is attaching to a live native session
- campaign-specific controls outside queued/advanced workflows

### Defer

- richer library object model
- deeper foreman automation
- remote control
- broader unattended behaviors

## Implementation Direction

When making product decisions, follow this order:

1. make the basic Dashboard/Dump/Agent loop clearer
2. keep advanced power available without making it mandatory
3. let Foreman extend Agent behavior rather than redefining the app
4. cut or hide any feature that weakens the main mental model

This document is the source of truth for future cleanup work.
