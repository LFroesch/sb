package cockpit

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LFroesch/sb/internal/statusbar"
)

// Manager is the in-process orchestrator that owns job lifecycles.
// Exactly one Manager exists per sb or sb-foreman process. Each job is a
// growing conversation with a provider: the first turn uses the composed
// brief, follow-up turns are sent with SendInput. Providers are re-invoked
// per turn — claude uses --session-id/--resume; codex and ollama replay
// full history on stdin; shell is one-shot per turn.
type Manager struct {
	Paths    Paths
	Registry *Registry

	mu         sync.Mutex
	foreman    ForemanState
	active     map[JobID]context.CancelFunc // cancel hook for the in-flight turn
	activeDone map[JobID]chan struct{}      // closed when the in-flight turn fully exits
	stopping   map[JobID]bool               // set when user explicitly requested stop

	subsMu sync.RWMutex
	subs   map[int]chan Event
	nextID int

	tmux *tmuxRunner // lazy — nil until first tmux job
}

func NewManager(paths Paths) (*Manager, error) {
	if err := paths.EnsureDirs(); err != nil {
		return nil, err
	}
	r := NewRegistry(paths)
	if err := r.Rehydrate(); err != nil {
		return nil, err
	}
	m := &Manager{
		Paths:      paths,
		Registry:   r,
		foreman:    loadForemanState(paths),
		active:     map[JobID]context.CancelFunc{},
		activeDone: map[JobID]chan struct{}{},
		stopping:   map[JobID]bool{},
		subs:       map[int]chan Event{},
	}
	m.tmux = newTmuxRunner(paths, r, m.emit, m.maybeStartQueuedJobs)
	// Reconcile any tmux-backed jobs that were mid-run across a daemon
	// restart. Silently no-ops if tmux is unavailable.
	if HasTmux() {
		m.tmux.Rehydrate(r.List())
	}
	m.maybeStartQueuedJobs()
	go m.scheduleTicker()
	return m, nil
}

// scheduleTicker re-evaluates queued jobs at a low cadence so a quota
// reset or external state change can pick up parked work without an
// operator nudge. Runs forever; the manager has no shutdown hook.
func (m *Manager) scheduleTicker() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		m.tickScheduler()
	}
}

// Subscribe returns a buffered event channel and a cancel func the
// caller must invoke when done. Events from every job fan out to every
// subscriber; filtering is the caller's job.
func (m *Manager) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 128)
	m.subsMu.Lock()
	id := m.nextID
	m.nextID++
	m.subs[id] = ch
	m.subsMu.Unlock()
	return ch, func() {
		m.subsMu.Lock()
		if c, ok := m.subs[id]; ok {
			delete(m.subs, id)
			close(c)
		}
		m.subsMu.Unlock()
	}
}

func (m *Manager) emit(e Event) {
	m.persistEvent(e)
	m.subsMu.RLock()
	for _, ch := range m.subs {
		select {
		case ch <- e:
		default:
		}
	}
	m.subsMu.RUnlock()
}

// LaunchRequest is the data Manager needs to turn a preset + sources
// into a new job. Freeform is appended to the composed brief after the
// sources list. Provider, if non-nil, overrides the preset's default
// executor.
type LaunchRequest struct {
	Preset    LaunchPreset  `json:"preset"`
	Sources   []SourceTask  `json:"sources,omitempty"`
	Repo      string        `json:"repo"`
	Freeform  string        `json:"freeform,omitempty"`
	PromptAdd string        `json:"prompt_add,omitempty"`
	Provider  *ExecutorSpec `json:"provider,omitempty"`
	QueueOnly bool          `json:"queue_only,omitempty"`
}

// LaunchJob registers a job, runs pre-shell hooks, then launches the
// first turn in a background goroutine. Returns immediately with the
// created job.
func (m *Manager) LaunchJob(req LaunchRequest) (Job, error) {
	if req.Repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Job{}, fmt.Errorf("launch: repo is required")
		}
		req.Repo = cwd
	}
	if req.Preset.LaunchMode == "" {
		req.Preset.LaunchMode = LaunchModeSingleJob
	}
	if req.Preset.LaunchMode == LaunchModeTaskQueueSequence && len(req.Sources) > 1 {
		return m.launchQueuedSequence(req)
	}
	job, err := m.createQueuedJob(req, req.Sources, "", 0, 1)
	if err != nil {
		return Job{}, err
	}
	m.maybeStartQueuedJobs()
	final, _ := m.Registry.Get(job.ID)
	return final, nil
}

func (m *Manager) StartJob(id JobID) (Job, error) {
	j, ok := m.Registry.Get(id)
	if !ok {
		return Job{}, fmt.Errorf("unknown job %s", id)
	}
	if j.Status != StatusQueued {
		return Job{}, fmt.Errorf("job %s is %s, not queued", id, j.Status)
	}
	if repo := strings.TrimSpace(j.Repo); repo != "" {
		for _, other := range m.ListJobs() {
			if other.ID == id {
				continue
			}
			if strings.TrimSpace(other.Repo) == repo && jobLocksRepo(other) {
				return Job{}, fmt.Errorf("repo already has active work")
			}
		}
	}
	_ = m.Registry.Update(id, func(jj *Job) {
		if jj.WaitForForeman || jj.ForemanManaged {
			jj.WaitForForeman = false
			jj.ForemanManaged = false
			jj.Note = "started manually"
		}
	})
	if err := m.startQueuedJob(id); err != nil {
		return Job{}, err
	}
	final, _ := m.Registry.Get(id)
	return final, nil
}

func (m *Manager) launchQueuedSequence(req LaunchRequest) (Job, error) {
	campaignID := NewCampaignID()
	jobIDs := make([]JobID, 0, len(req.Sources))
	for i, src := range req.Sources {
		job, err := m.createQueuedJob(req, []SourceTask{src}, campaignID, i, len(req.Sources))
		if err != nil {
			return Job{}, err
		}
		jobIDs = append(jobIDs, job.ID)
	}
	campaign := Campaign{
		ID:        campaignID,
		Goal:      strings.TrimSpace(req.Freeform),
		JobIDs:    jobIDs,
		Strategy:  "sequence",
		CreatedAt: time.Now(),
	}
	if err := SaveCampaign(m.Paths.CampaignDir, campaign); err != nil {
		return Job{}, err
	}
	m.maybeStartQueuedJobs()
	first, _ := m.Registry.Get(jobIDs[0])
	return first, nil
}

func (m *Manager) createQueuedJob(req LaunchRequest, sources []SourceTask, campaignID CampaignID, queueIndex, queueTotal int) (Job, error) {
	brief := ComposeBrief(req.Preset, sources, req.Freeform, req.QueueOnly)
	if extra := strings.TrimSpace(req.PromptAdd); extra != "" {
		brief = strings.TrimRight(brief, "\n") + "\n\n" + extra + "\n"
	}
	task := SummarizeTask(sources, req.Freeform)
	repoStatusAtLaunch := gitStatusSnapshot(req.Repo)
	executor := req.Preset.Executor
	if req.Provider != nil {
		executor = *req.Provider
	}
	runner := resolveRunner(executor)
	j := Job{
		CampaignID:         campaignID,
		PresetID:           req.Preset.ID,
		Task:               task,
		Sources:            sources,
		Brief:              task,
		Prompt:             brief,
		Freeform:           strings.TrimSpace(req.Freeform),
		RepoStatusAtLaunch: repoStatusAtLaunch,
		Repo:               req.Repo,
		Executor:           executor,
		Hooks:              req.Preset.Hooks,
		Permissions:        req.Preset.Permissions,
		Status:             StatusQueued,
		SyncBackState:      SyncBackPending,
		Runner:             runner,
		QueueIndex:         queueIndex,
		QueueTotal:         queueTotal,
		WaitForForeman:     req.QueueOnly,
		ForemanManaged:     req.QueueOnly,
	}
	if req.QueueOnly {
		j.Note = "sent to Foreman"
	}
	if providerSupportsResume(executor) && runner == RunnerExec {
		j.SessionID = newUUIDv4()
	}
	jp, err := m.Registry.Create(j)
	if err != nil {
		return Job{}, err
	}
	return *jp, nil
}

func (m *Manager) startQueuedJob(id JobID) error {
	j, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	if j.Status != StatusQueued {
		return nil
	}
	if j.WaitForForeman {
		_ = m.Registry.Update(j.ID, func(jj *Job) {
			jj.WaitForForeman = false
			jj.Note = "started by Foreman"
		})
		j, _ = m.Registry.Get(j.ID)
	}
	if j.Runner == RunnerTmux {
		if err := m.runPreHooks(&j); err != nil {
			return nil
		}
		if startErr := m.tmux.StartJob(j); startErr != nil {
			_ = m.Registry.Update(j.ID, func(jj *Job) {
				jj.Status = StatusFailed
				jj.Note = "tmux start: " + startErr.Error()
				jj.FinishedAt = time.Now()
			})
			m.emit(Event{TS: time.Now(), JobID: j.ID, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusFailed), "note": startErr.Error()}})
			return nil
		}
		return nil
	}
	go m.runFirstTurn(j)
	return nil
}

// maybeStartQueuedJobs is the historical name for tickScheduler; kept so
// callers (tmux runner callback, post-approve / post-skip / post-delete
// triggers, tests) don't need to change.
func (m *Manager) maybeStartQueuedJobs() { m.tickScheduler() }

// tickScheduler walks queued jobs in priority order and either starts
// them or records why they were deferred (waiting for foreman, repo busy,
// concurrency cap, near rate limit). Idempotent — safe to call from
// event handlers and from the background ticker.
func (m *Manager) tickScheduler() {
	jobs := orderQueuedJobs(m.ListJobs())
	state := m.GetForemanState()
	maxConcurrent := state.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = ForemanMaxConcurrentDefault
	}
	limitGuard := state.LimitGuardPct
	if limitGuard <= 0 {
		limitGuard = ForemanLimitGuardPctDefault
	}

	lockedRepos := map[string]JobID{}
	activeForeman := 0
	for _, j := range jobs {
		if jobLocksRepo(j) {
			if _, ok := lockedRepos[j.Repo]; !ok {
				lockedRepos[j.Repo] = j.ID
			}
		}
		if j.ForemanManaged {
			switch j.Status {
			case StatusRunning, StatusIdle, StatusBlocked:
				activeForeman++
			}
		}
	}

	for _, j := range jobs {
		if j.Status != StatusQueued {
			continue
		}
		reason := m.eligibilityReason(j, state, lockedRepos, activeForeman, maxConcurrent, limitGuard)
		if reason != "" {
			m.recordEligibilityReason(j.ID, reason)
			continue
		}
		prevReason := j.EligibilityReason
		if err := m.startQueuedJob(j.ID); err != nil {
			continue
		}
		if prevReason != "" {
			m.clearEligibilityReason(j.ID)
		}
		if strings.TrimSpace(j.Repo) != "" {
			if _, ok := lockedRepos[j.Repo]; !ok {
				lockedRepos[j.Repo] = j.ID
			}
		}
		if j.ForemanManaged {
			activeForeman++
		}
	}
}

// eligibilityReason returns the first gate that blocks this queued job,
// or "" if it is dispatchable. Gate order (first match wins): foreman
// off, repo busy, concurrency cap, provider near rate limit.
func (m *Manager) eligibilityReason(j Job, state ForemanState, lockedRepos map[string]JobID, activeForeman, maxConcurrent, limitGuard int) string {
	if j.WaitForForeman && !state.Enabled {
		return "waiting for foreman"
	}
	if repo := strings.TrimSpace(j.Repo); repo != "" {
		if other, ok := lockedRepos[repo]; ok && other != j.ID {
			return "repo busy: " + shortDisplayJobID(other)
		}
	}
	if j.ForemanManaged && maxConcurrent > 0 && activeForeman >= maxConcurrent {
		return fmt.Sprintf("foreman concurrency cap (%d/%d)", activeForeman, maxConcurrent)
	}
	if j.ForemanManaged && providerNeedsLimitGuard(j.Executor) {
		provider := strings.ToLower(strings.TrimSpace(j.Executor.Type))
		if pct, ok := providerLimitPct(provider); ok && pct >= limitGuard {
			return fmt.Sprintf("%s near 5h limit (%d%%)", provider, pct)
		}
	}
	return ""
}

// recordEligibilityReason persists the deferral reason on the job and
// emits a status_changed event so the TUI surfaces it. No-op if the
// reason hasn't changed.
func (m *Manager) recordEligibilityReason(id JobID, reason string) {
	current, ok := m.Registry.Get(id)
	if !ok || current.EligibilityReason == reason {
		return
	}
	_ = m.Registry.Update(id, func(jj *Job) {
		jj.EligibilityReason = reason
		jj.EligibilityCheckedAt = time.Now()
	})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusQueued), "note": reason}})
}

func (m *Manager) clearEligibilityReason(id JobID) {
	_ = m.Registry.Update(id, func(jj *Job) {
		jj.EligibilityReason = ""
		jj.EligibilityCheckedAt = time.Now()
	})
}

func providerNeedsLimitGuard(e ExecutorSpec) bool {
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "claude", "codex":
		return true
	}
	return false
}

// providerLimitPct returns the highest 5h-window utilization for a
// provider. Tests override this to drive the limit-guard gate.
var providerLimitPct = func(provider string) (int, bool) {
	var u statusbar.Usage
	var ok bool
	switch provider {
	case "claude":
		u, ok = statusbar.FetchClaude()
	case "codex":
		u, ok = statusbar.FetchCodex()
	default:
		return 0, false
	}
	if !ok {
		return 0, false
	}
	if !u.FiveHour.Available {
		return 0, false
	}
	return u.FiveHour.PctUsed, true
}

func shortDisplayJobID(id JobID) string {
	s := string(id)
	if len(s) <= 6 {
		return s
	}
	return s[len(s)-6:]
}

func (m *Manager) SetForemanEnabled(enabled bool) (ForemanState, error) {
	m.mu.Lock()
	m.foreman.Enabled = enabled
	m.foreman.UpdatedAt = time.Now()
	state := m.foreman
	m.mu.Unlock()
	if err := saveForemanState(m.Paths, state); err != nil {
		return ForemanState{}, err
	}
	m.emit(Event{TS: time.Now(), Kind: EventForemanState, Payload: state})
	// Tick either way: turning on may dispatch parked jobs; turning off
	// updates "waiting for foreman" reasons on remaining queued jobs.
	m.maybeStartQueuedJobs()
	return state, nil
}

func jobLocksRepo(j Job) bool {
	if strings.TrimSpace(j.Repo) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(j.Permissions)) {
	case "", "scoped-write", "wide-open":
	default:
		return false
	}
	switch j.Status {
	case StatusRunning, StatusIdle, StatusAwaitingHuman, StatusNeedsReview, StatusBlocked:
		return true
	default:
		return false
	}
}

func orderQueuedJobs(jobs []Job) []Job {
	out := append([]Job(nil), jobs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CampaignID != "" && out[i].CampaignID == out[j].CampaignID && out[i].QueueIndex != out[j].QueueIndex {
			return out[i].QueueIndex < out[j].QueueIndex
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// resolveRunner picks between the exec and tmux runners for a spec.
// Explicit Runner wins; otherwise we infer (claude/codex → tmux iff
// tmux is available; everything else → exec).
func resolveRunner(spec ExecutorSpec) Runner {
	switch strings.ToLower(strings.TrimSpace(spec.Runner)) {
	case "tmux":
		if HasTmux() {
			return RunnerTmux
		}
		return RunnerExec
	case "exec":
		return RunnerExec
	}
	switch strings.ToLower(spec.Type) {
	case "claude", "codex":
		if HasTmux() {
			return RunnerTmux
		}
	}
	return RunnerExec
}

// runPreHooks runs any pre-shell hooks and returns an error if any hook
// exited non-zero. Used by the tmux path to gate window creation.
func (m *Manager) runPreHooks(jp *Job) error {
	for _, h := range jp.Hooks.PreShell {
		m.emit(Event{TS: time.Now(), JobID: jp.ID, Kind: EventHookStarted, Payload: map[string]any{"phase": "pre", "name": h.Name, "cmd": h.Cmd}})
		res := RunShellHook(context.Background(), h, jp.Repo, os.Environ())
		_ = appendTranscriptLine(jp.TranscriptPath, fmt.Sprintf("\n$ [pre-hook %s]\n%s\n", h.Name, res.Output))
		m.emit(Event{TS: time.Now(), JobID: jp.ID, Kind: EventHookFinished, Payload: map[string]any{"phase": "pre", "name": h.Name, "exit": res.ExitCode, "duration_ms": res.Duration.Milliseconds()}})
		_ = m.Registry.Update(jp.ID, func(jj *Job) {
			jj.Turns = append(jj.Turns, Turn{Role: TurnHook, Content: res.Output, StartedAt: time.Now(), FinishedAt: time.Now(), ExitCode: res.ExitCode, Note: "pre-hook: " + h.Name})
		})
		if res.ExitCode != 0 {
			m.setStatus(jp.ID, StatusBlocked, fmt.Sprintf("pre-hook %q failed (exit %d)", h.Name, res.ExitCode))
			return fmt.Errorf("pre-hook %q failed", h.Name)
		}
	}
	return nil
}

// runFirstTurn runs pre-shell hooks (once per job lifetime) and then
// kicks off the first provider turn using the composed brief.
func (m *Manager) runFirstTurn(j Job) {
	// Pre-shell hooks first. Non-zero exit blocks the job.
	for _, h := range j.Hooks.PreShell {
		m.emit(Event{TS: time.Now(), JobID: j.ID, Kind: EventHookStarted, Payload: map[string]any{"phase": "pre", "name": h.Name, "cmd": h.Cmd}})
		res := RunShellHook(context.Background(), h, j.Repo, os.Environ())
		_ = appendTranscriptLine(j.TranscriptPath, fmt.Sprintf("\n$ [pre-hook %s]\n%s\n", h.Name, res.Output))
		m.emit(Event{TS: time.Now(), JobID: j.ID, Kind: EventHookFinished, Payload: map[string]any{"phase": "pre", "name": h.Name, "exit": res.ExitCode, "duration_ms": res.Duration.Milliseconds()}})
		_ = m.Registry.Update(j.ID, func(jj *Job) {
			jj.Turns = append(jj.Turns, Turn{Role: TurnHook, Content: res.Output, StartedAt: time.Now(), FinishedAt: time.Now(), ExitCode: res.ExitCode, Note: "pre-hook: " + h.Name})
		})
		if res.ExitCode != 0 {
			m.setStatus(j.ID, StatusBlocked, fmt.Sprintf("pre-hook %q failed (exit %d)", h.Name, res.ExitCode))
			return
		}
	}

	// StartedAt is fixed on first turn.
	_ = m.Registry.Update(j.ID, func(jj *Job) {
		if jj.StartedAt.IsZero() {
			jj.StartedAt = time.Now()
		}
	})

	if ok := m.startTurn(j.ID, j.launchPrompt()); !ok {
		return
	}
	go m.runTurn(j.ID)
}

// SendInput queues a follow-up turn. For backwards compatibility with
// the PTY-era signature the payload is still bytes; we treat it as UTF-8
// text and trim a trailing newline. Tmux-backed jobs reject input: the
// user types in the tmux window directly.
func (m *Manager) SendInput(id JobID, data []byte) error {
	if j, ok := m.Registry.Get(id); ok && j.Runner == RunnerTmux {
		return fmt.Errorf("send input in the tmux window — press 'a' to attach")
	}
	return m.SendTurn(id, strings.TrimRight(string(data), "\n"))
}

// SendTurn appends a user turn to the conversation and launches the
// provider to produce the assistant reply. Errors if the previous turn
// hasn't finished yet.
func (m *Manager) SendTurn(id JobID, content string) error {
	j, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("empty message")
	}
	if j.Status == StatusRunning {
		return fmt.Errorf("turn already in flight")
	}
	if j.Status == StatusCompleted || j.Status == StatusFailed || j.Status == StatusBlocked {
		return fmt.Errorf("job is %s — retry instead", j.Status)
	}
	if ok := m.startTurn(id, content); !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	go m.runTurn(id)
	return nil
}

// startTurn persists the user turn, flips the job into running, emits the
// start events, and appends the user/assistant headers to the transcript.
// It runs synchronously so callers can rely on the latest turn existing
// before SendTurn returns.
func (m *Manager) startTurn(id JobID, userInput string) bool {
	j, ok := m.Registry.Get(id)
	if !ok {
		return false
	}
	_ = m.Registry.Update(id, func(jj *Job) {
		jj.Turns = append(jj.Turns, Turn{Role: TurnUser, Content: userInput, StartedAt: time.Now()})
		jj.Status = StatusRunning
		jj.Note = ""
	})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusRunning)}})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventTurnStarted})
	_ = appendTranscriptLine(j.TranscriptPath, fmt.Sprintf("\n\x1b[36m› you\x1b[0m\n%s\n\n\x1b[32m› assistant\x1b[0m\n", userInput))
	return true
}

// runTurn is the heart of the turn loop: exec provider, stream stdout into
// transcript + events, append assistant turn, set status back to idle (or
// failed on error). The user turn has already been persisted by startTurn.
func (m *Manager) runTurn(id JobID) {
	j, ok := m.Registry.Get(id)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	turnDone := make(chan struct{})
	m.mu.Lock()
	m.active[id] = cancel
	m.activeDone[id] = turnDone
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.active, id)
		delete(m.activeDone, id)
		delete(m.stopping, id)
		m.mu.Unlock()
		close(turnDone)
	}()

	lastUserInput := ""
	for i := len(j.Turns) - 1; i >= 0; i-- {
		if j.Turns[i].Role == TurnUser {
			lastUserInput = j.Turns[i].Content
			break
		}
	}
	cmd, stdinBody, err := buildTurnCmd(ctx, j, lastUserInput)
	if err != nil {
		m.finishTurn(id, "", -1, "build cmd: "+err.Error(), StatusFailed)
		return
	}
	cmd.Dir = j.Repo
	cmd.Env = append(os.Environ(), "SB_JOB_ID="+string(j.ID))
	if stdinBody != "" {
		cmd.Stdin = strings.NewReader(stdinBody)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.finishTurn(id, "", -1, "stdout pipe: "+err.Error(), StatusFailed)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.finishTurn(id, "", -1, "stderr pipe: "+err.Error(), StatusFailed)
		return
	}
	if err := cmd.Start(); err != nil {
		m.finishTurn(id, "", -1, "start: "+err.Error(), StatusFailed)
		return
	}

	var reply strings.Builder
	done := make(chan struct{})
	if strings.ToLower(j.Executor.Type) == "codex" {
		go m.readCodexStdout(id, j.TranscriptPath, stdout, &reply, done)
	} else {
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			for {
				n, rerr := stdout.Read(buf)
				if n > 0 {
					chunk := string(buf[:n])
					reply.WriteString(chunk)
					_ = appendTranscriptLine(j.TranscriptPath, chunk)
					m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStdout, Payload: chunk})
				}
				if rerr != nil {
					return
				}
			}
		}()
	}
	go func() {
		b, _ := io.ReadAll(stderr)
		if len(b) > 0 {
			_ = appendTranscriptLine(j.TranscriptPath, "\x1b[31m"+string(b)+"\x1b[0m")
		}
	}()

	waitErr := cmd.Wait()
	<-done
	_ = appendTranscriptLine(j.TranscriptPath, "\n")

	exit := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = -1
		}
	}
	note := ""
	status := StatusIdle
	if ctx.Err() == context.Canceled {
		m.mu.Lock()
		stopped := m.stopping[id]
		m.mu.Unlock()
		if stopped {
			note = "stopped"
			exit = 130
		}
	} else if ctx.Err() == context.DeadlineExceeded {
		note = "turn timed out"
		status = StatusFailed
	} else if exit != 0 {
		note = fmt.Sprintf("provider exit %d", exit)
	}
	m.finishTurn(id, reply.String(), exit, note, status)
}

type codexJSONEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id,omitempty"`
	Message  string `json:"message,omitempty"`
	Item     struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"item,omitempty"`
}

func (m *Manager) readCodexStdout(id JobID, transcriptPath string, stdout io.Reader, reply *strings.Builder, done chan struct{}) {
	defer close(done)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var ev codexJSONEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "thread.started":
			if ev.ThreadID != "" {
				_ = m.Registry.Update(id, func(j *Job) {
					if j.SessionID == "" {
						j.SessionID = ev.ThreadID
					}
				})
			}
		case "item.completed":
			if ev.Item.Type != "agent_message" || ev.Item.Text == "" {
				continue
			}
			if reply.Len() > 0 && !strings.HasSuffix(reply.String(), "\n") {
				reply.WriteString("\n")
			}
			reply.WriteString(ev.Item.Text)
			reply.WriteString("\n")
			_ = appendTranscriptLine(transcriptPath, ev.Item.Text+"\n")
			m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStdout, Payload: ev.Item.Text + "\n"})
		case "error":
			if ev.Message == "" {
				continue
			}
			_ = appendTranscriptLine(transcriptPath, "\x1b[31m"+ev.Message+"\x1b[0m\n")
			m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStderr, Payload: ev.Message})
		}
	}
}

// finishTurn appends the assistant turn, flips status back to idle (or
// failed if the very first turn had a fatal start error), and emits.
func (m *Manager) finishTurn(id JobID, reply string, exit int, note string, status Status) {
	_ = m.Registry.Update(id, func(j *Job) {
		j.Turns = append(j.Turns, Turn{
			Role:       TurnAssistant,
			Content:    reply,
			StartedAt:  time.Now(), // approximate; fine for display
			FinishedAt: time.Now(),
			ExitCode:   exit,
			Note:       note,
		})
		j.ExitCode = exit
		j.Note = note
		// Fatal failure on the very first assistant turn (no content, no
		// prior assistant turns) marks the job failed unless the turn was
		// explicitly stopped by the user. Otherwise, stay idle so the user
		// can retry or keep chatting.
		if status == StatusFailed || (exit != 0 && note != "stopped" && countAssistantTurns(j) <= 1 && reply == "") {
			j.Status = StatusFailed
			j.FinishedAt = time.Now()
		} else {
			j.Status = status
			if status == StatusCompleted || status == StatusBlocked {
				j.FinishedAt = time.Now()
			}
		}
	})
	final, _ := m.Registry.Get(id)
	if final.Status == StatusIdle || final.Status == StatusNeedsReview || final.Status == StatusCompleted {
		_ = CaptureReviewArtifact(final)
	}
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventTurnFinished, Payload: map[string]any{"exit": exit, "note": note}})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(final.Status), "note": note}})
}

func countAssistantTurns(j *Job) int {
	n := 0
	for _, t := range j.Turns {
		if t.Role == TurnAssistant {
			n++
		}
	}
	return n
}

// buildTurnCmd produces the exec.Cmd for one turn. Returns (cmd, stdinBody, err);
// when stdinBody is non-empty, the caller must feed it to cmd.Stdin.
func buildTurnCmd(ctx context.Context, j Job, userInput string) (*exec.Cmd, string, error) {
	spec := j.Executor
	switch strings.ToLower(spec.Type) {
	case "claude":
		args := []string{"-p"}
		args = append(args, claudeLaunchArgs(j)...)
		// First assistant turn: pin the session id. Subsequent: resume it.
		if countAssistantTurns(&j) == 0 {
			if j.SessionID != "" {
				args = append(args, "--session-id", j.SessionID)
			}
		} else if j.SessionID != "" {
			args = append(args, "--resume", j.SessionID)
		}
		args = append(args, userInput)
		return exec.CommandContext(ctx, claudeBin(spec), args...), "", nil
	case "codex":
		args, err := codexLaunchArgs(j)
		if err != nil {
			return nil, "", err
		}
		if j.SessionID != "" {
			args = append(args, "exec", "resume")
			args = append(args, "--json")
			args = append(args, j.SessionID, userInput)
			return exec.CommandContext(ctx, codexBin(spec), args...), "", nil
		}
		args = append(args, "exec")
		args = append(args, "--json")
		args = append(args, userInput)
		return exec.CommandContext(ctx, codexBin(spec), args...), "", nil
	case "ollama":
		bin := spec.Cmd
		if bin == "" {
			bin = "ollama"
		}
		model := spec.Model
		if model == "" {
			model = "qwen2.5:7b"
		}
		args := append([]string{"run", model}, spec.Args...)
		return exec.CommandContext(ctx, bin, args...), renderHistoryReplay(j), nil
	case "shell", "":
		bin := spec.Cmd
		if bin == "" {
			bin = "bash"
		}
		args := append([]string{}, spec.Args...)
		args = append(args, userInput)
		return exec.CommandContext(ctx, bin, args...), "", nil
	default:
		return nil, "", fmt.Errorf("unsupported executor type %q", spec.Type)
	}
}

func normalizedClaudeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "-p", "--print":
			continue
		default:
			out = append(out, arg)
		}
	}
	return out
}

func claudeBin(spec ExecutorSpec) string {
	if strings.TrimSpace(spec.Cmd) != "" {
		return spec.Cmd
	}
	return "claude"
}

func claudeLaunchArgs(j Job) []string {
	args := make([]string, 0, len(j.Executor.Args)+4)
	if mode := claudePermissionMode(j); mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	userArgs := normalizedClaudeArgs(j.Executor.Args)
	if model := strings.TrimSpace(j.Executor.Model); model != "" && !argsContainFlag(userArgs, "--model") {
		args = append(args, "--model", model)
	}
	args = append(args, userArgs...)
	return args
}

func argsContainFlag(args []string, flag string) bool {
	prefix := flag + "="
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

func splitCodexArgs(args []string) ([]string, []string) {
	opts := make([]string, 0, len(args))
	positionals := make([]string, 0)
	expectsValue := false
	for _, arg := range args {
		switch arg {
		case "exec", "resume", "--json":
			expectsValue = false
			continue
		default:
			if strings.HasPrefix(arg, "-") {
				opts = append(opts, arg)
				expectsValue = !strings.Contains(arg, "=")
				continue
			}
			if expectsValue {
				opts = append(opts, arg)
				expectsValue = false
				continue
			}
			positionals = append(positionals, arg)
		}
	}
	return opts, positionals
}

func codexBin(spec ExecutorSpec) string {
	if strings.TrimSpace(spec.Cmd) != "" {
		return spec.Cmd
	}
	return "codex"
}

func codexLaunchArgs(j Job) ([]string, error) {
	extraArgs, positionalArgs := splitCodexArgs(j.Executor.Args)
	if len(positionalArgs) > 0 {
		return nil, fmt.Errorf("codex executor args cannot include positional values: %q", positionalArgs)
	}
	args := append([]string{}, codexRuntimeArgs(j)...)
	if model := strings.TrimSpace(j.Executor.Model); model != "" && !argsContainFlag(extraArgs, "--model") && !argsContainFlag(extraArgs, "-m") {
		args = append(args, "--model", model)
	}
	args = append(args, extraArgs...)
	return args, nil
}

func claudePermissionMode(j Job) string {
	// Unattended foreman/queued runs always bypass interactive prompts;
	// the perms enum still constrains *what* the agent can do via
	// downstream tool gating, but we can't have it sit waiting for input.
	if jobRunsUnattended(j) {
		switch strings.ToLower(strings.TrimSpace(j.Permissions)) {
		case "wide-open":
			return "bypassPermissions"
		case "read-only":
			return "plan"
		default:
			return "dontAsk"
		}
	}
	switch strings.ToLower(strings.TrimSpace(j.Permissions)) {
	case "read-only":
		return "plan"
	case "scoped-write":
		return "acceptEdits"
	case "wide-open":
		return "bypassPermissions"
	}
	return ""
}

func codexRuntimeArgs(j Job) []string {
	args := make([]string, 0, 6)
	switch strings.ToLower(strings.TrimSpace(j.Permissions)) {
	case "read-only":
		args = append(args, "--sandbox", "read-only")
	case "scoped-write":
		args = append(args, "--sandbox", "workspace-write")
	case "wide-open":
		args = append(args, "--sandbox", "danger-full-access")
	}
	if strings.TrimSpace(j.Repo) != "" {
		args = append(args, "--cd", j.Repo)
	}
	if strings.EqualFold(strings.TrimSpace(j.Permissions), "wide-open") || jobRunsUnattended(j) {
		args = append(args, "--ask-for-approval", "never")
	}
	return args
}

func jobRunsUnattended(j Job) bool {
	return j.ForemanManaged || j.WaitForForeman
}

// renderHistoryReplay turns the turn history into a plain-text prompt
// for providers without a native resume flag. The newest user turn is
// already on j.Turns at call time.
//
// On the first turn (len(Turns)==1) we replay the full launch prompt
// verbatim — that's the model's only chance to see the system prompt,
// hooks, and source tasks. On replay (len(Turns)>1) we substitute that
// first user turn with the compact j.Task, since the assistant's reply
// has already absorbed the launch context. This keeps unattended/replay
// providers from re-shipping the full composed brief on every turn.
func renderHistoryReplay(j Job) string {
	var b strings.Builder
	replay := len(j.Turns) > 1
	compactTask := strings.TrimSpace(j.Task)
	for i, t := range j.Turns {
		switch t.Role {
		case TurnUser:
			b.WriteString("User: ")
		case TurnAssistant:
			b.WriteString("Assistant: ")
		case TurnHook:
			continue // hooks don't feed the model
		}
		content := t.Content
		if replay && i == 0 && t.Role == TurnUser && compactTask != "" {
			content = compactTask
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	b.WriteString("Assistant: ")
	return b.String()
}

// providerSupportsResume reports whether we delegate session continuity
// to the provider. Only claude exposes a stable `--resume <uuid>`.
func providerSupportsResume(spec ExecutorSpec) bool {
	return strings.ToLower(spec.Type) == "claude"
}

func (m *Manager) setStatus(id JobID, s Status, note string) {
	_ = m.Registry.Update(id, func(j *Job) {
		j.Status = s
		j.Note = note
		if s == StatusFailed || s == StatusCompleted || s == StatusBlocked {
			j.FinishedAt = time.Now()
		}
	})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(s), "note": note}})
}

// StopJob interrupts the current turn. For tmux-backed jobs it sends
// C-c but keeps the session/window alive so the operator can re-enter.
// No-op if nothing is active.
func (m *Manager) StopJob(id JobID) error {
	if j, ok := m.Registry.Get(id); ok && j.Runner == RunnerTmux {
		if err := m.tmux.StopJob(j); err != nil {
			return err
		}
		_ = m.Registry.Update(id, func(jj *Job) {
			jj.Status = StatusIdle
			jj.Note = "interrupted"
		})
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusIdle), "note": "interrupted"}})
		return nil
	}
	m.mu.Lock()
	cancel, ok := m.active[id]
	if ok {
		m.stopping[id] = true
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	cancel()
	return nil
}

func (m *Manager) SoftStopJob(id JobID) error {
	if j, ok := m.Registry.Get(id); ok && j.Runner == RunnerTmux {
		if err := m.tmux.SoftStopJob(j); err != nil {
			return err
		}
		_ = m.Registry.Update(id, func(jj *Job) {
			jj.Note = "sent Esc"
		})
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(j.Status), "note": "sent Esc"}})
		return nil
	}
	return m.StopJob(id)
}

func (m *Manager) ContinueJob(id JobID) error {
	if j, ok := m.Registry.Get(id); ok && j.Runner == RunnerTmux {
		if err := m.tmux.ContinueJob(j); err != nil {
			return err
		}
		_ = m.Registry.Update(id, func(jj *Job) {
			jj.Note = "sent continue"
		})
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(j.Status), "note": "sent continue"}})
		return nil
	}
	return m.SendTurn(id, "continue")
}

// AttachTmux switches the active tmux client (running inside sb-cockpit)
// to the job's window. Returns an error if the job is not tmux-backed
// or its window is gone.
func (m *Manager) AttachTmux(id JobID) error {
	j, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	if j.Runner != RunnerTmux || j.TmuxTarget == "" {
		return fmt.Errorf("job %s is not tmux-backed", id)
	}
	alive, _ := WindowAlive(j.TmuxTarget)
	if !alive {
		return fmt.Errorf("tmux window %s is gone", j.TmuxTarget)
	}
	return SelectWindow(j.TmuxTarget)
}

// ApproveJob runs post-shell hooks (for review gating) then applies the
// V0 sync-back: delete source lines, append devlog entry.
func (m *Manager) ApproveJob(id JobID, devlogPath string) error {
	j, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	if err := ensureSyncBackTargetsClean(j, devlogPath); err != nil {
		_ = m.Registry.Update(id, func(jj *Job) {
			jj.Note = err.Error()
		})
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventReviewSet, Payload: map[string]any{"ok": false, "error": err.Error()}})
		return err
	}

	// Post-shell hooks fire on approve, not per turn. A non-zero exit
	// flips sync-back into skipped state and notes the reason; sync-back
	// still applies, since by approving the user has chosen to accept.
	for _, h := range j.Hooks.PostShell {
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventHookStarted, Payload: map[string]any{"phase": "post", "name": h.Name, "cmd": h.Cmd}})
		res := RunShellHook(context.Background(), h, j.Repo, os.Environ())
		_ = appendTranscriptLine(j.TranscriptPath, fmt.Sprintf("\n$ [post-hook %s]\n%s\n", h.Name, res.Output))
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventHookFinished, Payload: map[string]any{"phase": "post", "name": h.Name, "exit": res.ExitCode, "output": res.Output}})
	}
	if latest, ok := m.Registry.Get(id); ok {
		_ = CaptureReviewArtifact(latest)
	}

	touched, err := ApplySyncBack(j, devlogPath)
	if err != nil {
		_ = m.Registry.Update(id, func(jj *Job) {
			jj.SyncBackState = SyncBackFailed
			jj.Note = err.Error()
		})
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventSyncedBack, Payload: map[string]any{"ok": false, "error": err.Error()}})
		return err
	}
	_ = m.Registry.Update(id, func(jj *Job) {
		jj.SyncBackState = SyncBackApplied
		jj.Status = StatusCompleted
		jj.FinishedAt = time.Now()
		jj.Note = ""
	})
	if latest, ok := m.Registry.Get(id); ok {
		_ = CaptureReviewArtifact(latest)
	}
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventSyncedBack, Payload: map[string]any{"ok": true, "files": touched}})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusCompleted)}})
	m.maybeStartQueuedJobs()
	return nil
}

// RetryJob re-launches a job's preset with the same sources and brief.
// The preset must still exist in the presets dir.
func (m *Manager) RetryJob(id JobID, presets []LaunchPreset) (Job, error) {
	j, ok := m.Registry.Get(id)
	if !ok {
		return Job{}, fmt.Errorf("unknown job %s", id)
	}
	var preset LaunchPreset
	for _, p := range presets {
		if p.ID == j.PresetID {
			preset = p
			break
		}
	}
	if preset.ID == "" {
		return Job{}, fmt.Errorf("preset %s not found", j.PresetID)
	}
	exec := j.Executor
	return m.LaunchJob(LaunchRequest{
		Preset:    preset,
		Sources:   j.Sources,
		Repo:      j.Repo,
		Freeform:  j.Freeform,
		QueueOnly: false,
		Provider:  &exec,
	})
}

func (m *Manager) TakeOverJob(id JobID, presets []LaunchPreset) (Job, error) {
	j, ok := m.Registry.Get(id)
	if !ok {
		return Job{}, fmt.Errorf("unknown job %s", id)
	}
	if err := validateTakeOverJob(j); err != nil {
		return Job{}, err
	}
	var preset LaunchPreset
	for _, p := range presets {
		if p.ID == j.PresetID {
			preset = p
			break
		}
	}
	if preset.ID == "" {
		return Job{}, fmt.Errorf("preset %s not found", j.PresetID)
	}

	handoff, err := BuildTakeoverPrompt(j)
	if err != nil {
		return Job{}, err
	}
	replacement, err := m.createQueuedJob(LaunchRequest{
		Preset:    preset,
		Sources:   j.Sources,
		Repo:      j.Repo,
		Freeform:  j.Freeform,
		PromptAdd: handoff,
	}, j.Sources, "", 0, 1)
	if err != nil {
		return Job{}, err
	}

	if err := m.closeTakeoverSource(j.ID); err != nil {
		return Job{}, err
	}
	_ = m.Registry.Update(j.ID, func(jj *Job) {
		jj.SupersededBy = replacement.ID
		jj.Status = StatusCompleted
		jj.SyncBackState = SyncBackSkipped
		jj.Note = "taken over by " + string(replacement.ID)
		jj.FinishedAt = time.Now()
		jj.ForemanManaged = false
		jj.WaitForForeman = false
	})
	_ = m.Registry.Update(replacement.ID, func(jj *Job) {
		jj.TakeoverOf = j.ID
		jj.Note = "manual takeover of " + string(j.ID)
	})
	if err := m.startQueuedJob(replacement.ID); err != nil {
		return Job{}, err
	}
	final, _ := m.Registry.Get(replacement.ID)
	m.emit(Event{TS: time.Now(), JobID: j.ID, Kind: EventStatusChanged, Payload: map[string]any{
		"status": string(StatusCompleted),
		"note":   "taken over by " + string(replacement.ID),
	}})
	return final, nil
}

// SkipJob marks a queued/reviewed item as intentionally skipped while
// preserving the job record for operator history, then advances any
// blocked queue for the repo/campaign.
func (m *Manager) SkipJob(id JobID) error {
	if err := m.skipOneJob(id, "skipped by operator"); err != nil {
		return err
	}
	m.maybeStartQueuedJobs()
	return nil
}

// SkipCampaign marks the selected job and every later queued job in the
// same campaign as skipped, preserving the records while aborting the
// rest of the serial sequence from this point forward.
func (m *Manager) SkipCampaign(id JobID) error {
	current, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	if current.CampaignID == "" || current.QueueTotal <= 1 {
		return fmt.Errorf("job %s is not part of a queued campaign", id)
	}
	for _, job := range orderQueuedJobs(m.ListJobs()) {
		if job.CampaignID != current.CampaignID || job.QueueIndex < current.QueueIndex {
			continue
		}
		if job.Status == StatusCompleted {
			continue
		}
		note := "skipped by operator"
		if job.ID != current.ID {
			note = "skipped by campaign abort"
		}
		if err := m.skipOneJob(job.ID, note); err != nil {
			return err
		}
	}
	m.maybeStartQueuedJobs()
	return nil
}

func (m *Manager) skipOneJob(id JobID, note string) error {
	j, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	if j.Runner == RunnerTmux && j.TmuxTarget != "" {
		_ = KillWindow(j.TmuxTarget)
	}
	m.mu.Lock()
	cancel, running := m.active[id]
	done := m.activeDone[id]
	if running {
		m.stopping[id] = true
	}
	m.mu.Unlock()
	if running {
		cancel()
		<-done
	}
	if err := m.Registry.Update(id, func(jj *Job) {
		jj.SyncBackState = SyncBackSkipped
		jj.Status = StatusCompleted
		jj.Note = note
		jj.FinishedAt = time.Now()
	}); err != nil {
		return err
	}
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{
		"status": string(StatusCompleted),
		"note":   note,
	}})
	return nil
}

func (m *Manager) closeTakeoverSource(id JobID) error {
	j, ok := m.Registry.Get(id)
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	if j.Runner == RunnerTmux && j.TmuxTarget != "" {
		if snapshot, err := CapturePane(j.TmuxTarget); err == nil && strings.TrimSpace(snapshot) != "" {
			_ = appendTranscriptLine(j.LogPath, "\n[takeover snapshot]\n"+snapshot+"\n")
		}
		if err := KillWindow(j.TmuxTarget); err != nil && !isTmuxMissingResourceError(err) {
			return err
		}
	}
	return nil
}

// DeleteJob stops any in-flight turn, then removes the job record and
// on-disk directory.
func (m *Manager) DeleteJob(id JobID) error {
	j, ok := m.Registry.Get(id)
	if ok && j.Runner == RunnerTmux && j.TmuxTarget != "" {
		_ = KillWindow(j.TmuxTarget)
	}
	m.mu.Lock()
	cancel, running := m.active[id]
	done := m.activeDone[id]
	if running {
		m.stopping[id] = true
	}
	m.mu.Unlock()
	if running {
		cancel()
		<-done
	}
	if err := m.Registry.Delete(id); err != nil {
		return err
	}
	m.maybeStartQueuedJobs()
	return nil
}

// ReadTranscript returns the on-disk transcript bytes.
func (m *Manager) ReadTranscript(id JobID) (string, error) {
	j, ok := m.Registry.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown job %s", id)
	}
	b, err := os.ReadFile(j.TranscriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// appendTranscriptLine opens+appends+closes. Boring but safe under
// concurrent emit goroutines.
func appendTranscriptLine(path, s string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(s)
	_ = f.Close()
	return werr
}

// newUUIDv4 returns a random v4 UUID for claude --session-id.
func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (m *Manager) persistEvent(e Event) {
	if e.JobID == "" {
		return
	}
	j, ok := m.Registry.Get(e.JobID)
	if !ok || strings.TrimSpace(j.EventLogPath) == "" {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	_ = appendTranscriptLine(j.EventLogPath, string(b)+"\n")
}

func (j Job) launchPrompt() string {
	if prompt := strings.TrimSpace(j.Prompt); prompt != "" {
		return prompt
	}
	return j.Brief
}
