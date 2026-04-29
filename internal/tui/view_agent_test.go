package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/config"
	xansi "github.com/charmbracelet/x/ansi"
)

type stubCockpitClient struct {
	jobs map[cockpit.JobID]cockpit.Job
}

func (s stubCockpitClient) ListJobs() []cockpit.Job {
	out := make([]cockpit.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, job)
	}
	return out
}

func (s stubCockpitClient) GetJob(id cockpit.JobID) (cockpit.Job, bool) {
	job, ok := s.jobs[id]
	return job, ok
}

func (s stubCockpitClient) GetForemanState() cockpit.ForemanState { return cockpit.ForemanState{} }

func (s stubCockpitClient) SetForemanEnabled(enabled bool) (cockpit.ForemanState, error) {
	return cockpit.ForemanState{Enabled: enabled}, nil
}

func (s stubCockpitClient) LaunchJob(cockpit.LaunchRequest) (cockpit.Job, error) {
	return cockpit.Job{}, nil
}

func (s stubCockpitClient) StartJob(id cockpit.JobID) (cockpit.Job, error) {
	job := s.jobs[id]
	job.Status = cockpit.StatusRunning
	job.WaitForForeman = false
	s.jobs[id] = job
	return job, nil
}

func (s stubCockpitClient) SoftStopJob(cockpit.JobID) error { return nil }

func (s stubCockpitClient) ContinueJob(cockpit.JobID) error { return nil }

func (s stubCockpitClient) StopJob(cockpit.JobID) error { return nil }

func (s stubCockpitClient) SkipJob(cockpit.JobID) error { return nil }

func (s stubCockpitClient) SkipCampaign(cockpit.JobID) error { return nil }

func (s stubCockpitClient) DeleteJob(cockpit.JobID) error { return nil }

func (s stubCockpitClient) ApproveJob(cockpit.JobID, string) error { return nil }

func (s stubCockpitClient) RetryJob(cockpit.JobID, []cockpit.LaunchPreset) (cockpit.Job, error) {
	return cockpit.Job{}, nil
}

func (s stubCockpitClient) SendInput(cockpit.JobID, []byte) error { return nil }

func (s stubCockpitClient) ReadTranscript(cockpit.JobID) (string, error) { return "", nil }

func (s stubCockpitClient) AttachTmux(cockpit.JobID) error { return nil }

func (s stubCockpitClient) Subscribe() (<-chan cockpit.Event, func()) {
	ch := make(chan cockpit.Event)
	return ch, func() { close(ch) }
}

func (s stubCockpitClient) Close() error { return nil }

func renderedLineCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(strings.TrimRight(xansi.Strip(s), "\n"), "\n"))
}

func TestFormatTurnDurationDropsSecondsAfterMinute(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "sub-second", in: 200 * time.Millisecond, want: "0s"},
		{name: "seconds", in: 59 * time.Second, want: "59s"},
		{name: "minutes", in: 61 * time.Second, want: "1m"},
		{name: "hours", in: 65 * time.Minute, want: "1h5m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatTurnDuration(tc.in); got != tc.want {
				t.Fatalf("formatTurnDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestJobOperatorStatusUsesWaitingForTmuxWithStaleLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	if err := os.WriteFile(logPath, []byte("old output\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	old := time.Now().Add(-30 * time.Second)
	if err := os.Chtimes(logPath, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got, _ := jobOperatorStatus(cockpit.Job{
		Status:  cockpit.StatusRunning,
		Runner:  cockpit.RunnerTmux,
		LogPath: logPath,
	})
	if got != "waiting for input" {
		t.Fatalf("jobOperatorStatus(tmux stale) = %q, want waiting for input", got)
	}
}

func TestJobOperatorStatusUsesStoppedForInterruptedTmuxJob(t *testing.T) {
	got, _ := jobOperatorStatus(cockpit.Job{
		Status: cockpit.StatusIdle,
		Runner: cockpit.RunnerTmux,
		Note:   "interrupted",
	})
	if got != "stopped" {
		t.Fatalf("jobOperatorStatus(interrupted tmux) = %q, want stopped", got)
	}
}

func TestJobOperatorStatusUsesClosedForClosedTmuxJob(t *testing.T) {
	got, _ := jobOperatorStatus(cockpit.Job{
		Status: cockpit.StatusIdle,
		Runner: cockpit.RunnerTmux,
		Note:   "tmux window closed",
	})
	if got != "closed" {
		t.Fatalf("jobOperatorStatus(closed tmux) = %q, want closed", got)
	}
}

func TestJobOperatorStatusUsesWorkingForExecTurn(t *testing.T) {
	now := time.Now()
	got, _ := jobOperatorStatus(cockpit.Job{
		Status:    cockpit.StatusRunning,
		CreatedAt: now.Add(-3 * time.Minute),
		Turns: []cockpit.Turn{{
			Role:      cockpit.TurnUser,
			StartedAt: now.Add(-95 * time.Second),
		}},
	})
	if got != "working" {
		t.Fatalf("jobOperatorStatus(exec running) = %q, want working", got)
	}
}

func TestOrderAgentJobsPrioritizesWorkingThenWaiting(t *testing.T) {
	now := time.Now()
	jobs := []cockpit.Job{
		{ID: "done", Status: cockpit.StatusCompleted, CreatedAt: now.Add(-1 * time.Minute)},
		{ID: "wait", Status: cockpit.StatusIdle, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "work", Status: cockpit.StatusRunning, CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "review", Status: cockpit.StatusNeedsReview, CreatedAt: now.Add(-4 * time.Minute)},
	}

	got := orderAgentJobs(jobs)
	want := []cockpit.JobID{"work", "wait", "review", "done"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("orderAgentJobs()[%d] = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestRenderTmuxLogConversationSanitizesRawPaneBytes(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	raw := "draft\rfinal\x1b[32m line\x1b[0m\n"
	if err := os.WriteFile(logPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := renderTmuxLogConversation(cockpit.Job{LogPath: logPath}, 80)
	if strings.Contains(out, "\x1b") {
		t.Fatalf("renderTmuxLogConversation leaked ANSI escape: %q", out)
	}
	if !strings.Contains(out, "final line") {
		t.Fatalf("renderTmuxLogConversation missing sanitized output: %q", out)
	}
	if strings.Contains(out, "draft") {
		t.Fatalf("renderTmuxLogConversation kept overwritten text: %q", out)
	}
	if strings.Contains(out, "tmux-backed session") || strings.Contains(out, "log:") {
		t.Fatalf("renderTmuxLogConversation kept session wrapper metadata: %q", out)
	}
}

func TestRenderAgentPeekShowsTaskLine(t *testing.T) {
	m := newModel(nil)
	out := m.renderAgentPeek(cockpit.Job{
		ID:        "job-123456",
		PresetID:  "senior-dev",
		CreatedAt: time.Now().Add(-2 * time.Minute),
		Sources: []cockpit.SourceTask{
			{Text: "first task"},
			{Text: "second task"},
		},
	}, 80, 20, 0)
	if !strings.Contains(out, "task") {
		t.Fatalf("renderAgentPeek missing task field: %q", out)
	}
	if !strings.Contains(out, "first task · second task") {
		t.Fatalf("renderAgentPeek missing combined task text: %q", out)
	}
}

func TestJobTaskTextPrefersRawFreeformOverComposedBrief(t *testing.T) {
	j := cockpit.Job{
		Brief:    "system prompt\n\n### hook\n\ncontext\n\nfix the real thing\n",
		Freeform: "fix the real thing",
	}
	if got := jobTaskText(j); got != "fix the real thing" {
		t.Fatalf("jobTaskText() = %q", got)
	}
}

func TestRenderAgentPeekShowsSyncBackReviewPreview(t *testing.T) {
	dir := t.TempDir()
	workPath := filepath.Join(dir, "WORK.md")
	if err := os.WriteFile(workPath, []byte("# WORK - demo\n\n## Current Tasks\n\n- keep\n- delete me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := newModel(nil)
	out := m.renderAgentPeek(cockpit.Job{
		ID:        "job-123456",
		PresetID:  "senior-dev",
		Status:    cockpit.StatusNeedsReview,
		CreatedAt: time.Now().Add(-2 * time.Minute),
		Sources: []cockpit.SourceTask{
			{File: workPath, Line: 6, Text: "delete me"},
		},
		Repo: dir,
	}, 90, 24, 0)
	if !strings.Contains(out, "accept will remove 1 task lines") {
		t.Fatalf("renderAgentPeek missing review summary: %q", out)
	}
	if !strings.Contains(out, "- delete me") {
		t.Fatalf("renderAgentPeek missing task removal preview: %q", out)
	}
	if !strings.Contains(out, "DEVLOG.md") {
		t.Fatalf("renderAgentPeek missing devlog preview target: %q", out)
	}
}

func TestJobAdvanceStateShowsQueuedCampaignProgress(t *testing.T) {
	got, _ := jobAdvanceState(cockpit.Job{
		Status:     cockpit.StatusQueued,
		QueueIndex: 1,
		QueueTotal: 3,
	})
	if got != "2/3 next" {
		t.Fatalf("jobAdvanceState(queued campaign) = %q, want %q", got, "2/3 next")
	}
}

func TestJobOperatorStatusShowsWaitingForForeman(t *testing.T) {
	got, _ := jobOperatorStatus(cockpit.Job{
		Status:         cockpit.StatusQueued,
		WaitForForeman: true,
	})
	if got != "waiting for foreman" {
		t.Fatalf("jobOperatorStatus(waiting foreman) = %q, want waiting for foreman", got)
	}
}

func TestRenderAgentPeekShowsQueueControlsAndNextUp(t *testing.T) {
	now := time.Now()
	current := cockpit.Job{
		ID:         "job-1",
		CampaignID: "c-123",
		PresetID:   "senior-dev",
		Status:     cockpit.StatusNeedsReview,
		CreatedAt:  now.Add(-2 * time.Minute),
		QueueIndex: 0,
		QueueTotal: 2,
		Sources: []cockpit.SourceTask{
			{Text: "first item"},
		},
		Repo: "/tmp/demo",
	}
	next := cockpit.Job{
		ID:         "job-2",
		CampaignID: "c-123",
		PresetID:   "senior-dev",
		Status:     cockpit.StatusQueued,
		CreatedAt:  now.Add(-1 * time.Minute),
		QueueIndex: 1,
		QueueTotal: 2,
		Sources: []cockpit.SourceTask{
			{Text: "second item"},
		},
		Repo: "/tmp/demo",
	}

	m := newModel(nil)
	m.cockpitJobs = []cockpit.Job{next, current}
	out := m.renderAgentPeek(current, 90, 28, 0)
	if !strings.Contains(out, "queue controls") {
		t.Fatalf("renderAgentPeek missing queue controls: %q", out)
	}
	if !strings.Contains(out, "2/2 second item") {
		t.Fatalf("renderAgentPeek missing next queued item: %q", out)
	}
}

func TestRenderAgentManageUsesLibraryLanguage(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentManage
	m.width = 120
	m.height = 40
	m.agentManageKind = "preset"
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:   "senior-dev",
		Name: "Senior dev",
		Role: "senior-dev",
		Executor: cockpit.ExecutorSpec{
			Type: "codex",
		},
		Hooks: cockpit.HookSpec{
			Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot},
		},
	}}

	out := m.renderAgentManage()
	if !strings.Contains(out, "Agent Setup") {
		t.Fatalf("renderAgentManage missing Agent Setup title: %q", out)
	}
	if !strings.Contains(out, "Templates") {
		t.Fatalf("renderAgentManage missing Templates label: %q", out)
	}
	if !strings.Contains(out, "Editable Fields") {
		t.Fatalf("renderAgentManage missing grouped field section: %q", out)
	}
}

func TestRenderAgentLaunchShowsReviewComposer(t *testing.T) {
	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{{Project: "demo", File: "/tmp/demo/WORK.md", Line: 10, Text: "ship the thing"}}
	m.launchFocus = m.launchReviewFocus()
	m.launchRepo = "/tmp/demo"
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:          "senior-dev",
		Name:        "Senior dev",
		Role:        "senior-dev",
		Executor:    cockpit.ExecutorSpec{Type: "codex"},
		Hooks:       cockpit.HookSpec{Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot}},
		Permissions: "scoped-write",
	}}
	m.cockpitProviders = []cockpit.ProviderProfile{{
		ID:       "codex",
		Name:     "Codex",
		Executor: cockpit.ExecutorSpec{Type: "codex", Runner: "tmux"},
	}}

	out := m.renderAgentLaunch()
	if !strings.Contains(out, "New Run") {
		t.Fatalf("renderAgentLaunch missing New Run title: %q", out)
	}
	if !strings.Contains(out, "Review Run") {
		t.Fatalf("renderAgentLaunch missing Review Run panel: %q", out)
	}
	if !strings.Contains(out, "Role") {
		t.Fatalf("renderAgentLaunch missing role-first wording: %q", out)
	}
	if strings.Contains(out, "▸ Repo") || strings.Contains(out, "  Repo") {
		t.Fatalf("renderAgentLaunch unexpectedly showed Repo tab for sourced run: %q", out)
	}
	if !strings.Contains(out, "Source Preview") {
		t.Fatalf("renderAgentLaunch missing source preview: %q", out)
	}
}

func TestRenderAgentLaunchShowsRepoTabForFreeform(t *testing.T) {
	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentLaunch
	m.launchRepo = "/tmp/demo"
	m.launchFocus = 2
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}

	out := m.renderAgentLaunch()
	if !strings.Contains(out, "Repo") {
		t.Fatalf("renderAgentLaunch missing Repo tab for freeform run: %q", out)
	}
}

func TestRenderAgentPickerShowsSelectionCount(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentPicker
	m.width = 100
	m.height = 30
	m.pickerFile = "/tmp/demo/WORK.md"
	m.pickerItems = []cockpit.PickerItem{
		{Text: "first task"},
		{Text: "second task"},
	}
	m.pickerSelected = map[int]bool{0: true}

	out := m.renderAgentPicker()
	if !strings.Contains(out, "1 selected") {
		t.Fatalf("renderAgentPicker missing selection count: %q", out)
	}
}

func TestRenderAgentAttachedShowsMessageLabel(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Status:    cockpit.StatusRunning,
		CreatedAt: now.Add(-2 * time.Minute),
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "first"},
		},
	}

	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentAttached
	m.attachedJobID = job.ID
	m.attachedFocus = 1
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	out := m.renderAgentAttached()
	if !strings.Contains(out, "message") {
		t.Fatalf("renderAgentAttached missing message label: %q", out)
	}
}

func TestAgentLaunchViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.launchRepo = "/tmp/demo"
	m.launchFocus = m.launchReviewFocus()
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:          "senior-dev",
		Name:        "Senior dev",
		Executor:    cockpit.ExecutorSpec{Type: "codex"},
		Hooks:       cockpit.HookSpec{Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot}},
		Permissions: "scoped-write",
	}}
	m.cockpitProviders = []cockpit.ProviderProfile{{
		ID:       "codex",
		Name:     "Codex",
		Executor: cockpit.ExecutorSpec{Type: "codex", Runner: "tmux"},
	}}

	out := m.View()
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("agent launch rendered %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
}

func TestAgentPickerViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentPicker
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.pickerFile = "/tmp/demo/WORK.md"
	m.pickerItems = []cockpit.PickerItem{
		{Text: "first task"},
		{Text: "second task"},
		{Text: "third task"},
		{Text: "fourth task"},
	}
	m.pickerSelected = map[int]bool{0: true}

	out := m.View()
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("agent picker rendered %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
}

func TestAgentManageViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentManage
	m.width = 80
	m.height = 14
	m.cfg = &config.Config{}
	m.agentManageKind = "preset"
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:          "senior-dev",
		Name:        "Senior dev",
		Executor:    cockpit.ExecutorSpec{Type: "codex"},
		Hooks:       cockpit.HookSpec{Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot}},
		Permissions: "scoped-write",
	}}

	out := m.View()
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("agent manage rendered %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
}

func TestAgentListViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentList
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.cockpitClient = stubCockpitClient{}
	m.cockpitJobs = []cockpit.Job{{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Brief:     "fix clipping",
		Status:    cockpit.StatusRunning,
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}}

	out := m.View()
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("agent list rendered %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
}

func TestAgentAttachedViewFitsShortTerminal(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Status:    cockpit.StatusRunning,
		CreatedAt: now.Add(-2 * time.Minute),
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "first"},
			{Role: cockpit.TurnAssistant, Content: "second"},
		},
	}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	out := m.View()
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("agent attached rendered %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
}
