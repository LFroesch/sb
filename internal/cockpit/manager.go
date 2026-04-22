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
	"strings"
	"sync"
	"time"
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

	mu       sync.Mutex
	active   map[JobID]context.CancelFunc // cancel hook for the in-flight turn
	stopping map[JobID]bool               // set when user explicitly requested stop

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
		Paths:    paths,
		Registry: r,
		active:   map[JobID]context.CancelFunc{},
		stopping: map[JobID]bool{},
		subs:     map[int]chan Event{},
	}
	m.tmux = newTmuxRunner(paths, r, m.emit)
	// Reconcile any tmux-backed jobs that were mid-run across a daemon
	// restart. Silently no-ops if tmux is unavailable.
	if HasTmux() {
		m.tmux.Rehydrate(r.List())
	}
	return m, nil
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
	Preset   LaunchPreset
	Sources  []SourceTask
	Repo     string
	Freeform string
	Provider *ExecutorSpec
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
	brief := ComposeBrief(req.Preset, req.Sources, req.Freeform)
	executor := req.Preset.Executor
	if req.Provider != nil {
		executor = *req.Provider
	}
	runner := resolveRunner(executor)
	j := Job{
		PresetID:      req.Preset.ID,
		Sources:       req.Sources,
		Brief:         brief,
		Repo:          req.Repo,
		Executor:      executor,
		Hooks:         req.Preset.Hooks,
		Permissions:   req.Preset.Permissions,
		Status:        StatusQueued,
		SyncBackState: SyncBackPending,
		Runner:        runner,
	}
	if providerSupportsResume(executor) && runner == RunnerExec {
		j.SessionID = newUUIDv4()
	}
	jp, err := m.Registry.Create(j)
	if err != nil {
		return Job{}, err
	}
	if runner == RunnerTmux {
		// Pre-shell hooks still fire so post-hooks have symmetry, but we
		// run them inline on the caller goroutine so any failure flips
		// the job to blocked before the tmux window is created.
		if err := m.runPreHooks(jp); err != nil {
			return *jp, nil // job is persisted with StatusBlocked; UI shows it
		}
		if startErr := m.tmux.StartJob(*jp); startErr != nil {
			_ = m.Registry.Update(jp.ID, func(jj *Job) {
				jj.Status = StatusFailed
				jj.Note = "tmux start: " + startErr.Error()
				jj.FinishedAt = time.Now()
			})
			m.emit(Event{TS: time.Now(), JobID: jp.ID, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusFailed), "note": startErr.Error()}})
			final, _ := m.Registry.Get(jp.ID)
			return final, nil
		}
		final, _ := m.Registry.Get(jp.ID)
		return final, nil
	}
	go m.runFirstTurn(*jp)
	return *jp, nil
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

	if ok := m.startTurn(j.ID, j.Brief); !ok {
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
	m.mu.Lock()
	m.active[id] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.active, id)
		delete(m.stopping, id)
		m.mu.Unlock()
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
		bin := spec.Cmd
		if bin == "" {
			bin = "claude"
		}
		args := []string{"-p"}
		extraArgs := normalizedClaudeArgs(spec.Args)
		// First assistant turn: pin the session id. Subsequent: resume it.
		if countAssistantTurns(&j) == 0 {
			if j.SessionID != "" {
				args = append(args, "--session-id", j.SessionID)
			}
		} else if j.SessionID != "" {
			args = append(args, "--resume", j.SessionID)
		}
		args = append(args, extraArgs...)
		args = append(args, userInput)
		return exec.CommandContext(ctx, bin, args...), "", nil
	case "codex":
		bin := spec.Cmd
		if bin == "" {
			bin = "codex"
		}
		extraArgs := normalizedCodexArgs(spec.Args)
		if j.SessionID != "" {
			args := append([]string{"exec", "resume", "--json"}, extraArgs...)
			args = append(args, j.SessionID, userInput)
			return exec.CommandContext(ctx, bin, args...), "", nil
		}
		args := append([]string{"exec", "--json"}, extraArgs...)
		args = append(args, userInput)
		return exec.CommandContext(ctx, bin, args...), "", nil
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

func normalizedCodexArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "exec", "resume", "--json":
			continue
		default:
			out = append(out, arg)
		}
	}
	return out
}

// renderHistoryReplay turns the turn history into a plain-text prompt
// for providers without a native resume flag. The newest user turn is
// already on j.Turns at call time.
func renderHistoryReplay(j Job) string {
	var b strings.Builder
	for _, t := range j.Turns {
		switch t.Role {
		case TurnUser:
			b.WriteString("User: ")
		case TurnAssistant:
			b.WriteString("Assistant: ")
		case TurnHook:
			continue // hooks don't feed the model
		}
		b.WriteString(t.Content)
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

// StopJob cancels an in-flight turn. For tmux-backed jobs it kills the
// tmux window (after a brief C-c). No-op if nothing is active.
func (m *Manager) StopJob(id JobID) error {
	if j, ok := m.Registry.Get(id); ok && j.Runner == RunnerTmux {
		if err := m.tmux.StopJob(j); err != nil {
			return err
		}
		// scan() will flip status when the window actually dies; emit
		// the stop event immediately so UI feels snappy.
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusNeedsReview), "note": "stopped"}})
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

	// Post-shell hooks fire on approve, not per turn. A non-zero exit
	// flips sync-back into skipped state and notes the reason; sync-back
	// still applies, since by approving the user has chosen to accept.
	for _, h := range j.Hooks.PostShell {
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventHookStarted, Payload: map[string]any{"phase": "post", "name": h.Name, "cmd": h.Cmd}})
		res := RunShellHook(context.Background(), h, j.Repo, os.Environ())
		_ = appendTranscriptLine(j.TranscriptPath, fmt.Sprintf("\n$ [post-hook %s]\n%s\n", h.Name, res.Output))
		m.emit(Event{TS: time.Now(), JobID: id, Kind: EventHookFinished, Payload: map[string]any{"phase": "post", "name": h.Name, "exit": res.ExitCode, "output": res.Output}})
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
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventSyncedBack, Payload: map[string]any{"ok": true, "files": touched}})
	m.emit(Event{TS: time.Now(), JobID: id, Kind: EventStatusChanged, Payload: map[string]any{"status": string(StatusCompleted)}})
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
	return m.LaunchJob(LaunchRequest{
		Preset:  preset,
		Sources: j.Sources,
		Repo:    j.Repo,
	})
}

// DeleteJob stops any in-flight turn, then removes the job record and
// on-disk directory.
func (m *Manager) DeleteJob(id JobID) error {
	if j, ok := m.Registry.Get(id); ok && j.Runner == RunnerTmux && j.TmuxTarget != "" {
		_ = KillWindow(j.TmuxTarget)
	}
	_ = m.StopJob(id)
	m.mu.Lock()
	_, running := m.active[id]
	m.mu.Unlock()
	if running {
		time.Sleep(50 * time.Millisecond)
	}
	return m.Registry.Delete(id)
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

// openEventLog kept for registry/rehydrate paths that still reference it.
func openEventLog(path string) (*json.Encoder, func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	enc := json.NewEncoder(f)
	return enc, f.Close, nil
}
