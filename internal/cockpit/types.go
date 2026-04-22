// Package cockpit implements the agent orchestration runtime for sb:
// job model + registry, PTY-backed executors, pre/post hooks, preset
// config, and sync-back of approved work into WORK.md / DEVLOG.md.
//
// See sb/docs/agent-cockpit-rfc.md for the full design. The package is
// organised around a single Manager that the TUI (or the sb-foreman
// daemon) drives via a small synchronous API plus an event channel.
package cockpit

import "time"

type JobID string
type CampaignID string

// SourceTask identifies a single `- ` bullet in a WORK.md-style file that
// feeds a job. Jobs launched from the task picker have len(Sources)>=1;
// freeform launches carry a zero-value SourceTask list.
type SourceTask struct {
	File    string `json:"file"`              // absolute path
	Line    int    `json:"line"`              // 1-indexed; 0 for freeform
	Text    string `json:"text"`              // the `- ` item text, verbatim (no leading "- ")
	Project string `json:"project,omitempty"` // sb project Name
	Repo    string `json:"repo,omitempty"`    // resolved repo path
}

// Status is the job lifecycle FSM. Transitions are owned by Manager.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"     // a turn is in flight
	StatusIdle        Status = "idle"        // waiting for next user turn
	StatusPaused      Status = "paused"      // unused in V1; reserved
	StatusNeedsReview Status = "needs_review"
	StatusBlocked     Status = "blocked"
	StatusCompleted   Status = "completed" // user marked done (approve) or shell oneshot exit
	StatusFailed      Status = "failed"
)

type SyncBackState string

const (
	SyncBackPending SyncBackState = "pending"
	SyncBackApplied SyncBackState = "applied"
	SyncBackSkipped SyncBackState = "skipped"
	SyncBackFailed  SyncBackState = "failed"
)

// ExecutorSpec describes which external tool drives a job. V0 supports:
//
//	claude     — Claude Code CLI, brief piped via prompt arg
//	codex      — Codex CLI, brief piped via prompt arg
//	ollama     — local llm via ollama run
//	shell      — generic shell escape hatch; Cmd runs with brief on stdin
type ExecutorSpec struct {
	Type   string   `json:"type"`             // claude|codex|ollama|shell
	Model  string   `json:"model,omitempty"`  // provider model id
	Cmd    string   `json:"cmd,omitempty"`    // override binary (shell) or executable path
	Args   []string `json:"args,omitempty"`   // extra CLI args appended after the default set
	Runner string   `json:"runner,omitempty"` // "tmux"|"exec"|"" (infer by Type)
}

// Runner identifies the in-Manager code path used to drive a job.
// Persisted on the job so the poller knows which path owns a record.
type Runner string

const (
	RunnerExec Runner = "exec" // legacy per-turn exec.Cmd
	RunnerTmux Runner = "tmux" // window-per-job in the cockpit tmux session
)

// PromptHook injects extra context into the final brief before the
// executor sees it. Kind selects the source: "file" reads BodyRef from
// disk; "literal" uses Body as-is. Placement controls where in the brief
// the block lands.
type PromptHook struct {
	Kind      string `json:"kind"` // "literal" | "file"
	Placement string `json:"placement,omitempty"` // "before" | "after" (default "after")
	Label     string `json:"label,omitempty"`     // heading rendered before the block
	Body      string `json:"body,omitempty"`
	BodyRef   string `json:"body_ref,omitempty"` // path (~ expanded) for Kind=file
}

// ShellHook is a pre-launch or post-run shell step. Cwd defaults to the
// job's Repo. A non-zero exit from a post-shell hook gates the job into
// needs_review; V0 only wires needs_review from the basic exit code.
type ShellHook struct {
	Name    string        `json:"name,omitempty"`
	Cmd     string        `json:"cmd"`
	Cwd     string        `json:"cwd,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty"`
}

// IterationPolicy is reserved for V1+. V0 only exercises IterationOneShot.
type IterationPolicy struct {
	Mode    string `json:"mode"`              // "one_shot" | "loop_n" | "until_signal"
	N       int    `json:"n,omitempty"`       // loop_n count
	Signal  string `json:"signal,omitempty"`  // until_signal sentinel substring
	OnFile  string `json:"on_file,omitempty"` // until_signal file marker
}

// HookSpec is the full hook bundle attached to a job (composed from the
// preset at launch time, with room for per-launch overrides later).
type HookSpec struct {
	Prompt    []PromptHook    `json:"prompt,omitempty"`
	PreShell  []ShellHook     `json:"pre_shell,omitempty"`
	PostShell []ShellHook     `json:"post_shell,omitempty"`
	Iteration IterationPolicy `json:"iteration,omitempty"`
}

// LaunchPreset is the user-editable recipe: persona + executor + hooks +
// iteration + permissions. Stored one-per-file under presets_dir.
type LaunchPreset struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Role          string          `json:"role,omitempty"`
	SystemPrompt  string          `json:"system_prompt,omitempty"`
	Executor      ExecutorSpec    `json:"executor"`
	Hooks         HookSpec        `json:"hooks,omitempty"`
	Permissions   string          `json:"permissions,omitempty"`    // "read-only"|"scoped-write"|"wide-open"
	NightEligible bool            `json:"night_eligible,omitempty"` // V2
}

// TurnRole tags who authored a single turn in the job's conversation.
type TurnRole string

const (
	TurnUser      TurnRole = "user"
	TurnAssistant TurnRole = "assistant"
	TurnHook      TurnRole = "hook" // pre/post-shell hook output captured inline
)

// Turn is one entry in a job's conversation history. The first user turn
// is the composed brief; each subsequent user turn is a follow-up message
// typed by the user in the attached view.
type Turn struct {
	Role       TurnRole  `json:"role"`
	Content    string    `json:"content"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ExitCode   int       `json:"exit_code,omitempty"`
	Note       string    `json:"note,omitempty"`
}

// Job is one conversation with a provider (Claude Code, Codex, Ollama,
// shell). Each job has a growing Turns slice; the provider is re-invoked
// per turn with --resume (claude) or history replay (codex, ollama) or
// fresh exec (shell). Persisted to <state>/jobs/<id>/job.json.
type Job struct {
	ID             JobID         `json:"id"`
	CampaignID     CampaignID    `json:"campaign_id,omitempty"`
	PresetID       string        `json:"preset_id"`
	Sources        []SourceTask  `json:"sources,omitempty"`
	Brief          string        `json:"brief"`
	Repo           string        `json:"repo"`
	Executor       ExecutorSpec  `json:"executor"`
	Hooks          HookSpec      `json:"hooks"`
	Permissions    string        `json:"permissions,omitempty"`
	Status         Status        `json:"status"`
	SessionID      string        `json:"session_id,omitempty"` // provider-native resume id (claude)
	Turns          []Turn        `json:"turns,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	StartedAt      time.Time     `json:"started_at,omitempty"`
	FinishedAt     time.Time     `json:"finished_at,omitempty"`
	ExitCode       int           `json:"exit_code"`
	TranscriptPath string        `json:"transcript_path"`
	EventLogPath   string        `json:"event_log_path"`
	ArtifactsDir   string        `json:"artifacts_dir"`
	SyncBackState  SyncBackState `json:"sync_back_state"`
	Note           string        `json:"note,omitempty"` // last status message (e.g. hook failure reason)

	// Tmux runner fields. Zero values are backwards-compatible with
	// pre-v2 persisted jobs: empty Runner → treat as exec.
	Runner      Runner `json:"runner,omitempty"`       // exec | tmux
	TmuxTarget  string `json:"tmux_target,omitempty"`  // "sb-cockpit:@3"
	LogPath     string `json:"log_path,omitempty"`     // jobs/<id>/tmux.log (pipe-pane sink)
}

// Campaign wraps multiple jobs spawned from a shared goal. V0 only
// produces "solo" campaigns (one job). Directory exists from day one.
type Campaign struct {
	ID        CampaignID `json:"id"`
	Goal      string     `json:"goal"`
	JobIDs    []JobID    `json:"job_ids"`
	Strategy  string     `json:"strategy"` // "solo"|"parallel"|"sequence"|"compare"
	CreatedAt time.Time  `json:"created_at"`
}

// EventKind enumerates what the append-only event log records.
type EventKind string

const (
	EventStatusChanged EventKind = "status_changed"
	EventStdout        EventKind = "stdout"
	EventStderr        EventKind = "stderr"
	EventHookStarted   EventKind = "hook_started"
	EventHookFinished  EventKind = "hook_finished"
	EventReviewSet     EventKind = "review_set"
	EventSyncedBack    EventKind = "synced_back"
	EventTurnStarted   EventKind = "turn_started"
	EventTurnFinished  EventKind = "turn_finished"
)

// Event is one line in <state>/jobs/<id>/events.jsonl.
type Event struct {
	TS      time.Time   `json:"ts"`
	JobID   JobID       `json:"job_id,omitempty"`
	Kind    EventKind   `json:"kind"`
	Payload interface{} `json:"payload,omitempty"`
}

// IterationOneShot is the V0 default policy.
const IterationOneShot = "one_shot"
