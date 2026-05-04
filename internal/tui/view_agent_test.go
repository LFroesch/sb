package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/llm"
	"github.com/LFroesch/sb/internal/workmd"
	xansi "github.com/charmbracelet/x/ansi"
)

type stubCockpitClient struct {
	jobs        map[cockpit.JobID]cockpit.Job
	retryResult cockpit.Job
	retryErr    error
	retryCalls  *int
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
	if s.retryCalls != nil {
		*s.retryCalls = *s.retryCalls + 1
	}
	if s.retryErr != nil {
		return cockpit.Job{}, s.retryErr
	}
	return s.retryResult, nil
}

func (s stubCockpitClient) TakeOverJob(id cockpit.JobID, _ []cockpit.LaunchPreset) (cockpit.Job, error) {
	job := s.jobs[id]
	job.Status = cockpit.StatusRunning
	job.ForemanManaged = false
	job.WaitForForeman = false
	job.TakeoverOf = id
	return job, nil
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

func assertViewFitsHeight(t *testing.T, m model) {
	t.Helper()
	out := m.View()
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("view rendered %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
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

func TestStandardAgentFiltersKeepForemanResultsVisible(t *testing.T) {
	now := time.Now()
	client := stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{
		"review": {
			ID:             "review",
			Status:         cockpit.StatusNeedsReview,
			ForemanManaged: true,
			CreatedAt:      now.Add(-2 * time.Minute),
		},
		"done": {
			ID:             "done",
			Status:         cockpit.StatusCompleted,
			ForemanManaged: true,
			CreatedAt:      now.Add(-1 * time.Minute),
		},
	}}
	m := newModel(nil)
	m.cockpitClient = client
	m.cockpitJobs = client.ListJobs()

	m.agentFilter = "attention"
	attention := m.filteredAgentJobs()
	if len(attention) != 1 || attention[0].ID != "review" {
		t.Fatalf("attention filter = %+v, want needs-review foreman job", attention)
	}

	m.agentFilter = "done"
	done := m.filteredAgentJobs()
	if len(done) != 1 || done[0].ID != "done" {
		t.Fatalf("done filter = %+v, want completed foreman job", done)
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

func TestRenderAgentPeekUsesLatestActivityLabel(t *testing.T) {
	m := newModel(nil)
	out := m.renderAgentPeek(cockpit.Job{
		ID:        "job-123456",
		PresetID:  "senior-dev",
		Runner:    cockpit.RunnerTmux,
		CreatedAt: time.Now().Add(-2 * time.Minute),
		LogPath:   filepath.Join(t.TempDir(), "tmux.log"),
	}, 80, 20, 0)
	if !strings.Contains(out, "latest activity") {
		t.Fatalf("renderAgentPeek missing latest activity label: %q", out)
	}
	if strings.Contains(out, "session log") {
		t.Fatalf("renderAgentPeek kept old session log copy: %q", out)
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
	if !strings.Contains(out, "sync-back: remove 1 task lines") {
		t.Fatalf("renderAgentPeek missing review summary: %q", out)
	}
	if !strings.Contains(out, "update 2 file(s)") {
		t.Fatalf("renderAgentPeek missing sync-back target count: %q", out)
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

func TestJobOperatorStatusShowsDeferredWhenEligibilityReasonSet(t *testing.T) {
	got, _ := jobOperatorStatus(cockpit.Job{
		Status:            cockpit.StatusQueued,
		WaitForForeman:    true,
		ForemanManaged:    true,
		EligibilityReason: "claude near 5h limit (95%)",
	})
	if got != "deferred" {
		t.Fatalf("jobOperatorStatus(deferred) = %q, want deferred", got)
	}
}

func TestJobOperatorStatusShowsTakenOverForSupersededJob(t *testing.T) {
	got, _ := jobOperatorStatus(cockpit.Job{
		Status:       cockpit.StatusCompleted,
		SupersededBy: "job-new",
	})
	if got != "taken over" {
		t.Fatalf("jobOperatorStatus(taken over) = %q, want taken over", got)
	}
}

func TestRenderAgentJobsHeaderShowsForemanPool(t *testing.T) {
	jobs := []cockpit.Job{
		{Status: cockpit.StatusQueued, WaitForForeman: true, ForemanManaged: true},
		{Status: cockpit.StatusQueued, WaitForForeman: true, ForemanManaged: true},
		{Status: cockpit.StatusRunning, ForemanManaged: true},
		{Status: cockpit.StatusQueued, WaitForForeman: true, ForemanManaged: true, EligibilityReason: "foreman concurrency cap (3/3)"},
	}
	out := strings.Join(renderAgentJobsHeader(jobs, "all", cockpit.ForemanState{Enabled: true}, 200), "\n")
	for _, want := range []string{"All", "filter", "foreman", "ON"} {
		if !strings.Contains(out, want) {
			t.Fatalf("renderAgentJobsHeader missing %q:\n%s", want, out)
		}
	}
}

func TestRenderAgentPeekShowsDeferredReason(t *testing.T) {
	m := newModel(nil)
	out := m.renderAgentPeek(cockpit.Job{
		ID:                "job-deferred",
		PresetID:          "claude",
		Status:            cockpit.StatusQueued,
		WaitForForeman:    true,
		ForemanManaged:    true,
		EligibilityReason: "claude near 5h limit (95%)",
		CreatedAt:         time.Now().Add(-2 * time.Minute),
	}, 90, 24, 0)
	if !strings.Contains(out, "deferred") {
		t.Fatalf("renderAgentPeek missing deferred field: %q", out)
	}
	if !strings.Contains(out, "claude near 5h limit (95%)") {
		t.Fatalf("renderAgentPeek missing reason text: %q", out)
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
		Executor: cockpit.ExecutorSpec{
			Type: "codex",
		},
		Hooks: cockpit.HookSpec{
			Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot},
		},
	}}

	out := m.renderAgentManage()
	if !strings.Contains(out, "Advanced Setup") {
		t.Fatalf("renderAgentManage missing Advanced Setup title: %q", out)
	}
	if !strings.Contains(out, "Roles") {
		t.Fatalf("renderAgentManage missing Roles label: %q", out)
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
	if !strings.Contains(out, "[Start now]") {
		t.Fatalf("renderAgentLaunch missing start-now header badge: %q", out)
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

func TestRenderAgentLaunchShowsForemanQueueBadgeInHeader(t *testing.T) {
	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentLaunch
	m.launchQueueOnly = true
	m.launchRepo = "/tmp/demo"
	m.launchFocus = m.launchReviewFocus()
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}

	out := xansi.Strip(m.renderAgentLaunch())
	if !strings.Contains(out, "[Foreman queue]") {
		t.Fatalf("renderAgentLaunch missing Foreman queue header badge: %q", out)
	}
}

func TestRenderAgentLaunchShowsRepoTabForFreeform(t *testing.T) {
	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentLaunch
	m.launchRepo = "/tmp/demo"
	m.launchFocus = m.launchRepoFocus()
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}

	out := m.renderAgentLaunch()
	if !strings.Contains(out, "Repo") {
		t.Fatalf("renderAgentLaunch missing Repo tab for freeform run: %q", out)
	}
}

func TestRenderAgentLaunchShowsLongerRepoPaths(t *testing.T) {
	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentLaunch
	m.launchRepo = "/tmp/demo/projects/alpha/with/a/very/long/path/name"
	m.launchFocus = m.launchRepoFocus()
	m.projects = []workmd.Project{
		{Dir: "/tmp/demo/projects/alpha/with/a/very/long/path/name"},
		{Dir: "/tmp/demo/projects/beta"},
	}
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}

	out := xansi.Strip(m.renderAgentLaunch())
	if !strings.Contains(out, "/tmp/demo/projects/alpha/with/a/very/long/path/name") {
		t.Fatalf("renderAgentLaunch still clipped repo path too aggressively: %q", out)
	}
}

func TestRenderAgentLaunchKeepsCustomRepoEditorVisibleOnShorterTerminals(t *testing.T) {
	m := newModel(nil)
	m.width = 34
	m.height = 18
	m.mode = modeAgentLaunch
	m.launchFocus = m.launchRepoFocus()
	m.launchRepo = repoSentinelCustom
	m.launchRepoEditing = true
	m.launchRepoCustom.SetValue("/tmp/demo/custom/repo")
	m.projects = []workmd.Project{
		{Dir: "/tmp/demo/projects/alpha"},
		{Dir: "/tmp/demo/projects/beta"},
		{Dir: "/tmp/demo/projects/gamma"},
		{Dir: "/tmp/demo/projects/delta"},
	}
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}

	out := xansi.Strip(m.renderAgentLaunch())
	if !strings.Contains(out, "type repo path") {
		t.Fatalf("renderAgentLaunch hid the custom repo hint on a shorter terminal: %q", out)
	}
	if !strings.Contains(out, "/tmp/demo/custom/repo") {
		t.Fatalf("renderAgentLaunch hid the custom repo input value on a shorter terminal: %q", out)
	}
	if got := renderedLineCount(out); got > m.height {
		t.Fatalf("renderAgentLaunch overflowed terminal height: got %d lines in %d-line terminal:\n%s", got, m.height, out)
	}
}

func TestRenderAgentLaunchUsesAvailableWidthForSourceSummary(t *testing.T) {
	m := newModel(nil)
	m.width = 160
	m.height = 30
	m.mode = modeAgentLaunch
	m.launchRepo = "/tmp/demo"
	m.launchSources = []cockpit.SourceTask{{
		Text: "this source summary should use the available terminal width instead of cutting off after forty two columns",
	}}
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}

	out := m.renderAgentLaunch()
	if !strings.Contains(out, "instead of cutting off after forty two columns") {
		t.Fatalf("renderAgentLaunch still truncated source summary too early: %q", out)
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

func TestRenderAgentAttachedTmuxUsesActivityCopy(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	if err := os.WriteFile(logPath, []byte("draft\rfinal line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Runner:    cockpit.RunnerTmux,
		Status:    cockpit.StatusNeedsReview,
		CreatedAt: now.Add(-2 * time.Minute),
		LogPath:   logPath,
		Sources: []cockpit.SourceTask{{
			Text: "ship it",
		}},
	}

	m := newModel(nil)
	m.width = 120
	m.height = 40
	m.mode = modeAgentAttached
	m.attachedJobID = job.ID
	m.attachedFocus = 0
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.refreshAttachedViewport(true)

	out := m.renderAgentAttached()
	if !strings.Contains(out, "activity") {
		t.Fatalf("renderAgentAttached missing activity label: %q", out)
	}
	if !strings.Contains(out, "captured output") {
		t.Fatalf("renderAgentAttached missing capture source label: %q", out)
	}
	if strings.Contains(out, "session log") {
		t.Fatalf("renderAgentAttached kept old session log copy: %q", out)
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

func TestAgentManageEditingViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentManage
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.agentManageKind = "preset"
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:   "senior-dev",
		Name: "Senior dev",
	}}
	m.agentManageEditing = true
	m.agentManageField = 3
	m.agentManageEditor.SetWidth(40)
	m.agentManageEditor.SetHeight(8)
	m.agentManageEditor.SetValue(strings.Repeat("prompt line\n", 20))
	assertViewFitsHeight(t, m)
}

func TestRenderAgentManageSelectOverlayShowsChoices(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentManage
	m.width = 100
	m.height = 30
	m.agentManageKind = "preset"
	m.agentManageSelectEditing = true
	m.agentManageField = 3
	m.cockpitPrompts = []cockpit.PromptTemplate{
		{ID: "senior-dev", Name: "Senior dev"},
		{ID: "bug-fixer", Name: "Bug fixer"},
	}
	m.agentManageSelectInput.SetValue("senior-dev")

	out := xansi.Strip(m.renderAgentManage())
	if !strings.Contains(out, "Edit Prompt") {
		t.Fatalf("renderAgentManage missing overlay title: %q", out)
	}
	if !strings.Contains(out, "Choices") {
		t.Fatalf("renderAgentManage missing choices section: %q", out)
	}
	if !strings.Contains(out, "bug-fixer") {
		t.Fatalf("renderAgentManage missing prompt choice list: %q", out)
	}
}

func TestRenderAgentManageHookOverlayShowsStructuredEditor(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentManage
	m.width = 100
	m.height = 24
	m.agentManageKind = "hookbundle"
	m.agentManageHookEditing = true
	m.agentManageHookArrayKey = "pre_shell"
	m.agentManageShellDraft = []cockpit.ShellHook{{Name: "Git status", Cmd: "git status --short"}}

	out := xansi.Strip(m.renderAgentManage())
	if !strings.Contains(out, "hook editor") {
		t.Fatalf("renderAgentManage missing hook editor title: %q", out)
	}
	if !strings.Contains(out, "Git status") {
		t.Fatalf("renderAgentManage missing hook row label: %q", out)
	}
	if !strings.Contains(out, "Command") {
		t.Fatalf("renderAgentManage missing structured field list: %q", out)
	}
}

func TestAgentPickerKeepsCursorVisibleOnShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentPicker
	m.width = 80
	m.height = 12
	for i := 0; i < 20; i++ {
		m.projects = append(m.projects, workmd.Project{Name: "project-" + string(rune('a'+i)), Path: "/tmp/demo"})
	}
	m.agentCursor = 15

	out := m.renderAgentPicker()
	if !strings.Contains(out, "▸ project-") {
		t.Fatalf("renderAgentPicker did not render a visible cursor row: %q", out)
	}
}

func TestAgentLaunchKeepsSelectedRoleVisibleOnShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.width = 80
	m.height = 12
	m.launchFocus = 0
	for i := 0; i < 20; i++ {
		m.cockpitPresets = append(m.cockpitPresets, cockpit.LaunchPreset{ID: "id", Name: fmt.Sprintf("Role %d", i)})
	}
	m.launchPresetIdx = 15

	out := m.renderAgentLaunch()
	if !strings.Contains(out, "Role 15") {
		t.Fatalf("renderAgentLaunch did not keep selected role visible: %q", out)
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

func TestAgentAttachedExecViewFitsShortTerminal(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "ollama-qwen",
		Status:    cockpit.StatusIdle,
		CreatedAt: now.Add(-2 * time.Minute),
		Note:      "compact layout",
		Sources: []cockpit.SourceTask{{
			File: "/tmp/demo/WORK.md",
			Line: 12,
		}},
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "first"},
			{Role: cockpit.TurnAssistant, Content: "second"},
		},
	}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 72
	m.height = 12
	m.cfg = &config.Config{}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	assertViewFitsHeight(t, m)
}

func TestRenderAgentAttachedExecChatKeepsBottomVisibleAfterRefresh(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "ollama-qwen",
		Status:    cockpit.StatusIdle,
		CreatedAt: now.Add(-2 * time.Minute),
		Note:      "narrow layout repro",
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "show me the full reply"},
			{Role: cockpit.TurnAssistant, Content: strings.Join([]string{
				"line 01", "line 02", "line 03", "line 04", "line 05",
				"line 06", "line 07", "line 08", "line 09", "line 10",
				"line 11", "line 12", "line 13", "line 14", "BOTTOM MARKER",
			}, "\n")},
		},
	}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 72
	m.height = 14
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	out := m.renderAgentAttached()
	if !strings.Contains(out, "BOTTOM MARKER") {
		t.Fatalf("renderAgentAttached() lost transcript bottom after refresh:\n%s", out)
	}
}

func TestRefreshAttachedViewportKeepsExecScrollOffsetStable(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "ollama-qwen",
		Status:    cockpit.StatusIdle,
		CreatedAt: now.Add(-2 * time.Minute),
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "show me the full reply"},
			{Role: cockpit.TurnAssistant, Content: strings.Join([]string{
				"line 01", "line 02", "line 03", "line 04", "line 05",
				"line 06", "line 07", "line 08", "line 09", "line 10",
				"line 11", "line 12", "line 13", "line 14", "line 15",
			}, "\n")},
		},
	}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 72
	m.height = 14
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	m.viewport.LineUp(2)
	wantOffset := m.viewport.YOffset
	if wantOffset == 0 {
		t.Fatalf("expected viewport to scroll up from bottom; offset stayed 0")
	}

	m.refreshAttachedViewport(false)
	if got := m.viewport.YOffset; got != wantOffset {
		t.Fatalf("refreshAttachedViewport(false) offset = %d, want %d", got, wantOffset)
	}
}

func TestRefreshAttachedViewportDoesNotForceBottomJustBecauseInputIsFocused(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "ollama-qwen",
		Status:    cockpit.StatusIdle,
		CreatedAt: now.Add(-2 * time.Minute),
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "show me the full reply"},
			{Role: cockpit.TurnAssistant, Content: strings.Join([]string{
				"line 01", "line 02", "line 03", "line 04", "line 05",
				"line 06", "line 07", "line 08", "line 09", "line 10",
				"line 11", "line 12", "line 13", "line 14", "line 15",
			}, "\n")},
		},
	}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 72
	m.height = 14
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	m.attachedFocus = 1
	m.viewport.LineUp(2)
	wantOffset := m.viewport.YOffset
	if wantOffset == 0 {
		t.Fatalf("expected viewport to scroll up from bottom; offset stayed 0")
	}

	m.refreshAttachedViewport(false)
	if got := m.viewport.YOffset; got != wantOffset {
		t.Fatalf("focused refresh forced bottom/changed offset: got %d want %d", got, wantOffset)
	}
}

func TestDashboardViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageDashboard
	m.mode = modeNormal
	m.width = 80
	m.height = 12
	m.loading = false
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{
		Name:         "demo",
		Path:         "/tmp/demo/WORK.md",
		Content:      "# WORK - demo\n\n- one\n- two\n- three\n- four\n",
		CurrentCount: 4,
	}}
	m.viewport.SetContent("preview")
	assertViewFitsHeight(t, m)
}

func TestDashboardPlanResultFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageDashboard
	m.mode = modePlanResult
	m.width = 80
	m.height = 12
	m.loading = false
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{Name: "demo", Path: "/tmp/demo/WORK.md", Content: "# WORK - demo"}}
	m.viewport.SetContent(strings.Repeat("plan line\n", 30))
	assertViewFitsHeight(t, m)
}

func TestProjectViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageProject
	m.mode = modeNormal
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{Name: "demo", Path: "/tmp/demo/WORK.md", Content: "# WORK - demo\n\n- one\n- two"}}
	m.selected = 0
	m.viewport.SetContent(strings.Repeat("project line\n", 20))
	assertViewFitsHeight(t, m)
}

func TestProjectEditViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageProject
	m.mode = modeEdit
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.editArea.SetWidth(m.width - 4)
	m.editArea.SetHeight(m.contentViewportHeight(2))
	m.editArea.SetValue(strings.Repeat("edit line\n", 20))
	assertViewFitsHeight(t, m)
}

func TestCleanupReviewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageCleanup
	m.mode = modeNormal
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{Name: "demo"}}
	m.selected = 0
	m.viewport.SetContent(strings.Repeat("diff line\n", 30))
	assertViewFitsHeight(t, m)
}

func TestCleanupFeedbackFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageCleanup
	m.mode = modeCleanupFeedback
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{Name: "demo"}}
	m.selected = 0
	m.chainFeedback.SetWidth(m.width - 4)
	m.chainFeedback.SetHeight(3)
	m.chainFeedback.SetValue("fix this")
	m.viewport.SetContent(strings.Repeat("diff line\n", 30))
	assertViewFitsHeight(t, m)
}

func TestChainCleanupReviewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageCleanup
	m.mode = modeChainCleanupReview
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{Name: "demo"}}
	m.chainQueue = []int{0}
	m.viewport.SetContent(strings.Repeat("diff line\n", 30))
	assertViewFitsHeight(t, m)
}

func TestChainCleanupFeedbackFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageCleanup
	m.mode = modeChainCleanupFeedback
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.projects = []workmd.Project{{Name: "demo"}}
	m.chainQueue = []int{0}
	m.chainFeedback.SetWidth(m.width - 4)
	m.chainFeedback.SetHeight(3)
	m.chainFeedback.SetValue("fix this")
	m.viewport.SetContent(strings.Repeat("diff line\n", 30))
	assertViewFitsHeight(t, m)
}

func TestDumpInputFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageDump
	m.mode = modeDumpInput
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.dumpArea.SetWidth(m.width - 6)
	m.dumpArea.SetHeight(m.contentViewportHeight(2))
	m.dumpArea.SetValue(strings.Repeat("dump line\n", 20))
	assertViewFitsHeight(t, m)
}

func TestDumpClarifyFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageDump
	m.mode = modeDumpClarify
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.dumpItems = []llm.RouteItem{{Text: "clarify this"}}
	m.dumpClarifyArea.SetWidth(m.width - 6)
	m.dumpClarifyArea.SetHeight(3)
	m.dumpClarifyArea.SetValue("demo")
	assertViewFitsHeight(t, m)
}

func TestDumpSummaryFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageDump
	m.mode = modeDumpSummary
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	m.dumpAccepted = 3
	m.dumpSkipped = 1
	m.dumpItems = []llm.RouteItem{
		{Text: "one", Project: "demo", Section: "Current"},
		{Text: "two", Project: "demo", Section: "Current"},
		{Text: "three", Project: "demo", Section: "Current"},
	}
	m.dumpSkippedList = []llm.RouteItem{{Text: "skip", Project: "demo", Section: "Backlog"}}
	assertViewFitsHeight(t, m)
}

func TestHelpViewFitsShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.mode = modeHelp
	m.width = 80
	m.height = 12
	m.cfg = &config.Config{}
	assertViewFitsHeight(t, m)
}
