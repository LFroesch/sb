// Package cockpit runner_tmux.go: tmux-backed job lifecycle.
//
// One goroutine polls `list-windows -t sb-cockpit` every second; when a
// tracked window disappears or goes dead the job is flipped to the
// appropriate terminal status. Window creation, pipe-pane setup, and
// kill-on-stop all go through the `tmux.go` shim.

package cockpit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// tmuxRunner owns the tmux-backed job lifecycle. There is exactly one
// per Manager, started on first use.
type tmuxRunner struct {
	paths Paths
	reg   *Registry
	emit  func(Event)
	afterYield func()

	mu    sync.Mutex
	alive map[JobID]string // jobID -> tmux target

	startOnce sync.Once
	stop      chan struct{}
}

func newTmuxRunner(paths Paths, reg *Registry, emit func(Event), afterYield func()) *tmuxRunner {
	return &tmuxRunner{
		paths: paths,
		reg:   reg,
		emit:  emit,
		afterYield: afterYield,
		alive: map[JobID]string{},
		stop:  make(chan struct{}),
	}
}

// ensureLoop lazily starts the poller. Called from every path that
// might register a tracked window.
func (r *tmuxRunner) ensureLoop() {
	r.startOnce.Do(func() {
		go r.pollLoop()
	})
}

// StartJob creates the tmux window, attaches pipe-pane, and marks the
// job running. The brief is handed to the real CLI as its opening
// prompt; when it exits the pane falls back to an interactive shell so
// the user can continue the conversation using the CLI's native tools.
func (r *tmuxRunner) StartJob(j Job) error {
	if !HasTmux() {
		return fmt.Errorf("tmux not available")
	}
	if err := EnsureSession(CockpitSession); err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}

	logPath := filepath.Join(r.paths.JobDir(j.ID), "tmux.log")
	// touch the log so pipe-pane doesn't race with first write
	_ = appendTranscriptLine(logPath, "")

	winName := windowName(j)
	cmd, err := buildTmuxCommand(j)
	if err != nil {
		return err
	}

	info, err := NewWindow(CockpitSession, winName,
		cmd,
		[]string{"SB_JOB_ID=" + string(j.ID)},
		j.Repo,
	)
	if err != nil {
		return fmt.Errorf("new-window: %w", err)
	}

	if err := PipePane(info.Target, logPath); err != nil {
		// Non-fatal — the pane is running; we just lose log capture.
		r.emit(Event{TS: time.Now(), JobID: j.ID, Kind: EventStderr, Payload: "pipe-pane: " + err.Error()})
	}

	_ = r.reg.Update(j.ID, func(jj *Job) {
		jj.Runner = RunnerTmux
		jj.TmuxTarget = info.Target
		jj.LogPath = logPath
		jj.Status = StatusRunning
		if jj.StartedAt.IsZero() {
			jj.StartedAt = time.Now()
		}
		jj.Note = ""
	})
	r.emit(Event{TS: time.Now(), JobID: j.ID, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusRunning)}})

	r.mu.Lock()
	r.alive[j.ID] = info.Target
	r.mu.Unlock()
	r.ensureLoop()
	return nil
}

// StopJob sends C-c to the live CLI but keeps the tmux window open so
// the operator can re-enter the session immediately after interrupting
// the current turn.
func (r *tmuxRunner) StopJob(j Job) error {
	if j.TmuxTarget == "" {
		return nil
	}
	// Best-effort C-c so the CLI can stop the current turn gracefully.
	return SendKeys(j.TmuxTarget, "C-c")
}

func (r *tmuxRunner) SoftStopJob(j Job) error {
	if j.TmuxTarget == "" {
		return nil
	}
	return SendKeys(j.TmuxTarget, "Escape")
}

func (r *tmuxRunner) ContinueJob(j Job) error {
	if j.TmuxTarget == "" {
		return nil
	}
	_ = r.reg.Update(j.ID, func(jj *Job) {
		jj.Status = StatusRunning
		jj.Note = ""
	})
	r.mu.Lock()
	r.alive[j.ID] = j.TmuxTarget
	r.mu.Unlock()
	r.ensureLoop()
	return SendKeys(j.TmuxTarget, "continue", "Enter")
}

// Rehydrate is called once at startup. Any tmux-backed job whose target
// is gone gets flipped to Failed; live ones are re-tracked by the
// poller.
func (r *tmuxRunner) Rehydrate(jobs []Job) {
	any := false
	for _, j := range jobs {
		if j.Runner != RunnerTmux || j.TmuxTarget == "" {
			continue
		}
		alive, _ := WindowAlive(j.TmuxTarget)
		if alive {
			r.mu.Lock()
			r.alive[j.ID] = j.TmuxTarget
			r.mu.Unlock()
			any = true
			continue
		}
		// window gone — mark job appropriately
		_ = r.reg.Update(j.ID, func(jj *Job) {
			if jj.Status == StatusRunning {
				if len(jj.Sources) > 0 {
					jj.Status = StatusNeedsReview
				} else {
					jj.Status = StatusIdle
				}
				jj.FinishedAt = time.Now()
				jj.Note = "tmux window closed before rehydrate"
			}
		})
		if next, ok := r.reg.Get(j.ID); ok {
			_ = CaptureReviewArtifact(next)
		}
	}
	if any {
		r.ensureLoop()
	}
}

// pollLoop ticks every second and diffs the tracked alive-set against
// tmux's list-windows output.
func (r *tmuxRunner) pollLoop() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			r.scan()
		}
	}
}

func (r *tmuxRunner) scan() {
	r.mu.Lock()
	if len(r.alive) == 0 {
		r.mu.Unlock()
		return
	}
	// snapshot so we release the mutex quickly
	snapshot := make(map[JobID]string, len(r.alive))
	for k, v := range r.alive {
		snapshot[k] = v
	}
	r.mu.Unlock()

	windows, err := ListWindows(CockpitSession)
	if err != nil {
		return
	}
	live := make(map[string]bool, len(windows))
	for _, w := range windows {
		if !w.Dead {
			live[w.Target] = true
		}
	}
	for id, target := range snapshot {
		if live[target] {
			r.observeLiveJob(id, target)
			continue
		}
		// window is gone or dead → terminal transition
		j, ok := r.reg.Get(id)
		if !ok {
			r.mu.Lock()
			delete(r.alive, id)
			r.mu.Unlock()
			continue
		}
		if j.LogPath != "" {
			if snapshot, err := CapturePane(target); err == nil && strings.TrimSpace(snapshot) != "" {
				_ = os.WriteFile(j.LogPath, []byte(snapshot), 0o644)
			}
		}
		next := StatusIdle
		note := "tmux window closed"
		switch j.Status {
		case StatusRunning:
			if len(j.Sources) > 0 {
				next = StatusNeedsReview
			}
		case StatusIdle:
			next = StatusFailed
			note = "tmux session ended"
		default:
			next = j.Status
		}
		_ = r.reg.Update(id, func(jj *Job) {
			if jj.Status == StatusRunning || jj.Status == StatusIdle {
				jj.Status = next
				jj.FinishedAt = time.Now()
				jj.Note = note
			}
		})
		if updated, ok := r.reg.Get(id); ok && (updated.Status == StatusNeedsReview || updated.Status == StatusIdle || updated.Status == StatusCompleted) {
			_ = CaptureReviewArtifact(updated)
		}
		r.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(next), "note": note}})
		r.mu.Lock()
		delete(r.alive, id)
		r.mu.Unlock()
	}
}

func (r *tmuxRunner) observeLiveJob(id JobID, target string) {
	j, ok := r.reg.Get(id)
	if !ok || j.Status != StatusRunning {
		return
	}
	body, err := CapturePane(target)
	if err != nil {
		return
	}
	next, note, yielded := supervisorStateFromPane(body)
	if !yielded {
		return
	}
	_ = r.reg.Update(id, func(jj *Job) {
		jj.Status = next
		jj.Note = note
		if next == StatusNeedsReview {
			jj.FinishedAt = time.Now()
		}
	})
	if updated, ok := r.reg.Get(id); ok && (updated.Status == StatusNeedsReview || updated.Status == StatusIdle) {
		_ = CaptureReviewArtifact(updated)
	}
	r.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(next), "note": note}})
	r.mu.Lock()
	delete(r.alive, id)
	r.mu.Unlock()
	if r.afterYield != nil {
		r.afterYield()
	}
}

func supervisorStateFromPane(body string) (Status, string, bool) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		switch line {
		case SupervisorReadyReviewMarker:
			return StatusNeedsReview, "ready for review", true
		case SupervisorWaitingHumanMarker:
			return StatusIdle, "waiting for human input", true
		}
	}
	return "", "", false
}

// Shutdown stops the poll loop. Called from Manager.Close (reserved;
// Manager doesn't yet expose Close to the TUI, but tests use it).
func (r *tmuxRunner) Shutdown() {
	select {
	case <-r.stop:
		return
	default:
		close(r.stop)
	}
}

// windowName builds a short human-identifying window title from the
// job's repo basename. Falls back to preset slug / executor / id tail
// when the repo is unknown (e.g. freeform launches without a repo).
func windowName(j Job) string {
	if repo := strings.TrimSpace(j.Repo); repo != "" {
		if base := strings.TrimSpace(filepath.Base(repo)); base != "" && base != "." && base != "/" {
			return base
		}
	}
	slug := strings.ReplaceAll(strings.ToLower(j.PresetID), " ", "-")
	if slug == "" {
		slug = strings.ToLower(j.Executor.Type)
	}
	if slug == "" {
		slug = "job"
	}
	idTail := string(j.ID)
	if len(idTail) > 6 {
		idTail = idTail[len(idTail)-6:]
	}
	return idTail + "-" + slug
}

// buildTmuxCommand renders the argv the tmux window should run for an
// interactive job. Unlike the exec-per-turn path, this must launch the
// real upstream CLI and let it own the entire UX in the pane.
func buildTmuxCommand(j Job) ([]string, error) {
	brief := strings.TrimSpace(j.launchPrompt())
	spec := j.Executor
	switch strings.ToLower(spec.Type) {
	case "claude":
		bin := spec.Cmd
		if bin == "" {
			bin = "claude"
		}
		args := append([]string{}, normalizedClaudeArgs(spec.Args)...)
		if brief != "" {
			args = append(args, brief)
		}
		return append([]string{bin}, args...), nil
	case "codex":
		bin := spec.Cmd
		if bin == "" {
			bin = "codex"
		}
		args, positionalArgs := splitCodexArgs(spec.Args)
		if len(positionalArgs) > 0 {
			return nil, fmt.Errorf("codex executor args cannot include positional values: %q", positionalArgs)
		}
		if brief != "" {
			args = append(args, brief)
		}
		return append([]string{bin}, args...), nil
	default:
		return nil, fmt.Errorf("tmux runner does not support executor type %q", spec.Type)
	}
}
