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

const (
	LaunchModeSingleJob         = "single_job"
	LaunchModeTaskQueueSequence = "task_queue_sequence"
)

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
	StatusRunning     Status = "running" // a turn is in flight
	StatusIdle        Status = "idle"    // waiting for next user turn
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
	Kind      string `json:"kind"`                // "literal" | "file"
	Placement string `json:"placement,omitempty"` // "before" | "after" (default "after")
	Label     string `json:"label,omitempty"`     // heading rendered before the block
	Body      string `json:"body,omitempty"`
	BodyRef   string `json:"body_ref,omitempty"` // path (~ expanded) for Kind=file
}

// ShellHook is a pre-launch or post-run shell step. Cwd defaults to the
// job's Repo. A non-zero exit from a post-shell hook gates the job into
// needs_review; V0 only wires needs_review from the basic exit code.
//
// PreviewCmd / PreviewSafe drive pre-approve preview rendering: if a
// post-hook's effective preview command (PreviewCmd if set, else Cmd)
// looks side-effect-free (or PreviewSafe explicitly opts in), we run it
// at review time so the operator sees ✓/✗ before pressing approve.
type ShellHook struct {
	Name        string        `json:"name,omitempty"`
	Cmd         string        `json:"cmd"`
	Cwd         string        `json:"cwd,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	PreviewCmd  string        `json:"preview_cmd,omitempty"`
	PreviewSafe bool          `json:"preview_safe,omitempty"`
}

// IterationPolicy is reserved for V1+. V0 only exercises IterationOneShot.
type IterationPolicy struct {
	Mode   string `json:"mode"`              // "one_shot" | "loop_n" | "until_signal"
	N      int    `json:"n,omitempty"`       // loop_n count
	Signal string `json:"signal,omitempty"`  // until_signal sentinel substring
	OnFile string `json:"on_file,omitempty"` // until_signal file marker
}

// HookSpec is the full hook bundle attached to a job (composed from the
// preset at launch time, with room for per-launch overrides later).
type HookSpec struct {
	Prompt    []PromptHook    `json:"prompt,omitempty"`
	PreShell  []ShellHook     `json:"pre_shell,omitempty"`
	PostShell []ShellHook     `json:"post_shell,omitempty"`
	Iteration IterationPolicy `json:"iteration,omitempty"`
}

// PromptTemplate is a named, reusable system prompt body. Stored
// one-per-file under prompts_dir. A LaunchPreset references one
// PromptTemplate by ID.
type PromptTemplate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Body string `json:"body"`
}

// HookBundle is a named, reusable hook set: prompt-injection hooks +
// pre/post shell steps + iteration policy. Stored one-per-file under
// hooks_dir. A LaunchPreset references one HookBundle by ID.
type HookBundle struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Prompt    []PromptHook    `json:"prompt,omitempty"`
	PreShell  []ShellHook     `json:"pre_shell,omitempty"`
	PostShell []ShellHook     `json:"post_shell,omitempty"`
	Iteration IterationPolicy `json:"iteration,omitempty"`
}

// LaunchPreset is the user-editable master recipe. On disk it stores
// only refs into the prompts / hooks / providers libraries plus the
// inline permissions enum and launch mode. At load time the refs are
// resolved into the runtime fields (SystemPrompt, Hooks, Executor) so
// downstream code can read a fully-populated preset.
//
// Saved files contain only ref fields plus identity/permissions/
// launch_mode — the runtime fields are zeroed before marshal.
type LaunchPreset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LaunchMode  string `json:"launch_mode,omitempty"` // single_job | task_queue_sequence
	Permissions string `json:"permissions,omitempty"` // "read-only"|"scoped-write"|"wide-open"

	// Library refs — source of truth on disk.
	PromptID     string `json:"prompt_id,omitempty"`
	HookBundleID string `json:"hook_bundle_id,omitempty"`
	EngineID     string `json:"engine_id,omitempty"`

	// Runtime fields populated by LoadPresets from the libraries. Saved
	// files have these zeroed; downstream consumers (TUI, manager, hooks
	// runtime) read them post-load.
	SystemPrompt string       `json:"system_prompt,omitempty"`
	Executor     ExecutorSpec `json:"executor,omitempty"`
	Hooks        HookSpec     `json:"hooks,omitempty"`
}

type ForemanState struct {
	Enabled       bool      `json:"enabled"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
	MaxConcurrent int       `json:"max_concurrent,omitempty"` // 0 = unlimited; defaults to 3 when zero on read
	LimitGuardPct int       `json:"limit_guard_pct,omitempty"` // 0 = use default 90
}

// ForemanMaxConcurrentDefault caps the number of foreman-managed jobs that
// may run concurrently when the persisted state has not been customized.
const ForemanMaxConcurrentDefault = 3

// ForemanLimitGuardPctDefault is the percent-used threshold above which
// foreman defers a claude/codex start to wait for a quota reset.
const ForemanLimitGuardPctDefault = 90

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
	ID                 JobID         `json:"id"`
	CampaignID         CampaignID    `json:"campaign_id,omitempty"`
	PresetID           string        `json:"preset_id"`
	Task               string        `json:"task,omitempty"`
	Sources            []SourceTask  `json:"sources,omitempty"`
	Brief              string        `json:"brief"`
	Prompt             string        `json:"prompt,omitempty"`
	Freeform           string        `json:"freeform,omitempty"`
	RepoStatusAtLaunch []string      `json:"repo_status_at_launch,omitempty"`
	Repo               string        `json:"repo"`
	Executor           ExecutorSpec  `json:"executor"`
	Hooks              HookSpec      `json:"hooks"`
	Permissions        string        `json:"permissions,omitempty"`
	Status             Status        `json:"status"`
	SessionID          string        `json:"session_id,omitempty"` // provider-native resume id (claude)
	Turns              []Turn        `json:"turns,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	StartedAt          time.Time     `json:"started_at,omitempty"`
	FinishedAt         time.Time     `json:"finished_at,omitempty"`
	ExitCode           int           `json:"exit_code"`
	TranscriptPath     string        `json:"transcript_path"`
	EventLogPath       string        `json:"event_log_path"`
	ArtifactsDir       string        `json:"artifacts_dir"`
	SyncBackState      SyncBackState `json:"sync_back_state"`
	Note               string        `json:"note,omitempty"` // last status message (e.g. hook failure reason)
	QueueIndex         int           `json:"queue_index,omitempty"`
	QueueTotal         int           `json:"queue_total,omitempty"`
	WaitForForeman     bool          `json:"wait_for_foreman,omitempty"`
	ForemanManaged     bool          `json:"foreman_managed,omitempty"`

	// EligibilityReason explains why the scheduler last passed over this
	// job (repo busy, foreman concurrency cap, provider near limit, etc).
	// Empty when the job has been dispatched or has not been gated yet.
	EligibilityReason    string    `json:"eligibility_reason,omitempty"`
	EligibilityCheckedAt time.Time `json:"eligibility_checked_at,omitempty"`

	// Tmux runner fields. Zero values are backwards-compatible with
	// pre-v2 persisted jobs: empty Runner → treat as exec.
	Runner     Runner `json:"runner,omitempty"`      // exec | tmux
	TmuxTarget string `json:"tmux_target,omitempty"` // "sb-cockpit:@3"
	LogPath    string `json:"log_path,omitempty"`    // jobs/<id>/tmux.log (pipe-pane sink)
}

type HookEventSummary struct {
	Phase      string    `json:"phase"`
	Name       string    `json:"name"`
	Cmd        string    `json:"cmd,omitempty"`
	ExitCode   int       `json:"exit_code,omitempty"`
	Output     string    `json:"output,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	TS         time.Time `json:"ts,omitempty"`
}

type ReviewArtifact struct {
	GeneratedAt      time.Time          `json:"generated_at"`
	Status           Status             `json:"status"`
	ChangedFiles     []string           `json:"changed_files,omitempty"`
	PreexistingDirty []string           `json:"preexisting_dirty,omitempty"`
	DiffStat         []string           `json:"diff_stat,omitempty"`
	HookEvents       []HookEventSummary `json:"hook_events,omitempty"`
	PendingPostHooks []string           `json:"pending_post_hooks,omitempty"`
}

// HookPreviewStatus enumerates dry-run outcomes for a post-hook.
type HookPreviewStatus string

const (
	HookPreviewOK        HookPreviewStatus = "ok"          // exit 0
	HookPreviewWouldFail HookPreviewStatus = "would_fail"  // exit non-zero
	HookPreviewSkipped   HookPreviewStatus = "skipped"     // mutating cmd, no preview run
	HookPreviewError     HookPreviewStatus = "error"       // failed to run (timeout, missing binary, etc)
)

// HookPreview is a single post-hook's dry-run result, captured before
// approve so the review pane can show ✓ / ✗ / skipped instead of waiting
// for approve to discover that a hook would fail.
type HookPreview struct {
	Name        string            `json:"name"`
	Cmd         string            `json:"cmd"`
	Status      HookPreviewStatus `json:"status"`
	ExitCode    int               `json:"exit_code"`
	Output      string            `json:"output,omitempty"`
	DurationMS  int64             `json:"duration_ms,omitempty"`
	SkipReason  string            `json:"skip_reason,omitempty"`
	GeneratedAt time.Time         `json:"generated_at"`
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
	EventForemanState  EventKind = "foreman_state_changed"
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
