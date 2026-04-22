// Package cockpit runner_tmux.go: tmux-backed job lifecycle.
//
// One goroutine polls `list-windows -t sb-cockpit` every second; when a
// tracked window disappears or goes dead the job is flipped to the
// appropriate terminal status. Window creation, pipe-pane setup, and
// kill-on-stop all go through the `tmux.go` shim.

package cockpit

import (
	"fmt"
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

	mu    sync.Mutex
	alive map[JobID]string // jobID -> tmux target

	startOnce sync.Once
	stop      chan struct{}
}

func newTmuxRunner(paths Paths, reg *Registry, emit func(Event)) *tmuxRunner {
	return &tmuxRunner{
		paths: paths,
		reg:   reg,
		emit:  emit,
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

// StopJob sends C-c then kills the window. Used by both explicit stop
// and delete paths.
func (r *tmuxRunner) StopJob(j Job) error {
	if j.TmuxTarget == "" {
		return nil
	}
	// Best-effort C-c so the CLI can shut down gracefully.
	_, _ = runTmux("send-keys", "-t", j.TmuxTarget, "C-c")
	time.Sleep(200 * time.Millisecond)
	return KillWindow(j.TmuxTarget)
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
		next := StatusIdle
		if len(j.Sources) > 0 {
			next = StatusNeedsReview
		}
		_ = r.reg.Update(id, func(jj *Job) {
			if jj.Status == StatusRunning {
				jj.Status = next
				jj.FinishedAt = time.Now()
			}
		})
		r.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(next)}})
		r.mu.Lock()
		delete(r.alive, id)
		r.mu.Unlock()
	}
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
// job's id + preset. Example: "j-1713...-senior-dev".
func windowName(j Job) string {
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
	brief := strings.TrimSpace(j.Brief)
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
		args := append([]string{}, normalizedCodexArgs(spec.Args)...)
		if brief != "" {
			args = append(args, brief)
		}
		return append([]string{bin}, args...), nil
	default:
		return nil, fmt.Errorf("tmux runner does not support executor type %q", spec.Type)
	}
}
