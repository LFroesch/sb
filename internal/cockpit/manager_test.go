package cockpit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildTurnCmdCodexInitialTurnUsesJSONExec(t *testing.T) {
	t.Parallel()

	cmd, stdinBody, err := buildTurnCmd(context.Background(), Job{
		Executor: ExecutorSpec{Type: "codex"},
	}, "follow-up")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	if got, want := cmd.Args, []string{"codex", "exec", "--json", "follow-up"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if stdinBody != "" {
		t.Fatal("expected codex prompt as argv, not stdin replay")
	}
}

func TestBuildTurnCmdCodexResumeUsesThreadID(t *testing.T) {
	t.Parallel()

	cmd, stdinBody, err := buildTurnCmd(context.Background(), Job{
		Executor:  ExecutorSpec{Type: "codex", Args: []string{"exec", "--json"}},
		SessionID: "thread-123",
	}, "follow-up")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	if got, want := cmd.Args, []string{"codex", "exec", "resume", "--json", "thread-123", "follow-up"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] || got[4] != want[4] || got[5] != want[5] {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if stdinBody != "" {
		t.Fatal("expected codex resume prompt as argv, not stdin replay")
	}
}

func TestBuildTurnCmdCodexRejectsPositionalExtraArgs(t *testing.T) {
	t.Parallel()

	_, _, err := buildTurnCmd(context.Background(), Job{
		Executor: ExecutorSpec{Type: "codex", Args: []string{"--model", "gpt-5", "extra-positional"}},
	}, "follow-up")
	if err == nil || !strings.Contains(err.Error(), "positional") {
		t.Fatalf("expected positional-args error, got %v", err)
	}
}

func TestRenderHistoryReplayCompactsLaunchPromptOnFollowUps(t *testing.T) {
	t.Parallel()

	bigPrompt := "You are concise.\n\n### Style\n\nlong style block\n\n### Tasks\n\n- fix the bug\n\nplease include a regression test\n"
	job := Job{
		Task:   "fix the bug",
		Prompt: bigPrompt,
		Turns: []Turn{
			{Role: TurnUser, Content: bigPrompt},
		},
	}

	first := renderHistoryReplay(job)
	if !strings.Contains(first, "### Style") {
		t.Fatalf("first turn replay should ship full launch prompt:\n%s", first)
	}

	job.Turns = append(job.Turns,
		Turn{Role: TurnAssistant, Content: "patched main.go and added a test."},
		Turn{Role: TurnUser, Content: "now also run go vet."},
	)

	second := renderHistoryReplay(job)
	if strings.Contains(second, "### Style") || strings.Contains(second, "### Tasks") {
		t.Fatalf("replay (turn 2+) leaked the full launch prompt:\n%s", second)
	}
	if !strings.Contains(second, "User: fix the bug\n\n") {
		t.Fatalf("replay should compact turn 0 to Task summary:\n%s", second)
	}
	if !strings.Contains(second, "Assistant: patched main.go and added a test.") {
		t.Fatalf("replay missing assistant turn:\n%s", second)
	}
	if !strings.Contains(second, "User: now also run go vet.") {
		t.Fatalf("replay missing latest user turn:\n%s", second)
	}
	if strings.Count(second, "long style block") != 0 {
		t.Fatalf("replay should not re-ship hook bodies:\n%s", second)
	}
}

func TestBuildTurnCmdClaudeNormalizesLegacyPrintArg(t *testing.T) {
	t.Parallel()

	cmd, stdinBody, err := buildTurnCmd(context.Background(), Job{
		Executor:  ExecutorSpec{Type: "claude", Args: []string{"--print"}},
		SessionID: "11111111-1111-4111-8111-111111111111",
	}, "follow-up")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	if got, want := cmd.Args, []string{"claude", "-p", "--session-id", "11111111-1111-4111-8111-111111111111", "follow-up"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] || got[4] != want[4] {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if stdinBody != "" {
		t.Fatal("expected claude prompt as argv")
	}
}

func TestLaunchJobFallsBackToCurrentWorkingDir(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir tempdir: %v", err)
	}

	job, err := mgr.LaunchJob(LaunchRequest{
		Preset: LaunchPreset{
			ID:       "shell",
			Name:     "shell",
			Executor: ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		},
		Freeform: "pwd",
	})
	if err != nil {
		t.Fatalf("LaunchJob: %v", err)
	}
	if job.Repo != dir {
		t.Fatalf("Repo = %q, want %q", job.Repo, dir)
	}
	waitForJobTerminalState(t, mgr, job.ID)
}

func TestLaunchJobStoresTaskSummarySeparatelyFromPrompt(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	job, err := mgr.createQueuedJob(LaunchRequest{
		Repo: dir,
		Preset: LaunchPreset{
			ID:           "senior-dev",
			Name:         "Senior dev",
			SystemPrompt: "You are concise.",
			Executor:     ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		},
		Freeform: "include a regression test",
	}, []SourceTask{{File: filepath.Join(dir, "WORK.md"), Line: 1, Text: "fix the bug"}}, "", 0, 1)
	if err != nil {
		t.Fatalf("createQueuedJob: %v", err)
	}
	if got := job.Task; got != "fix the bug\ninclude a regression test" {
		t.Fatalf("Task = %q", got)
	}
	if got := job.Brief; got != job.Task {
		t.Fatalf("Brief = %q, want operator task summary %q", got, job.Task)
	}
	if !strings.Contains(job.Prompt, "You are concise.") || !strings.Contains(job.Prompt, "### Tasks") {
		t.Fatalf("Prompt missing composed context:\n%s", job.Prompt)
	}
}

func TestStopJobMarksTurnStoppedAndReturnsIdle(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	job, err := mgr.LaunchJob(LaunchRequest{
		Repo: dir,
		Preset: LaunchPreset{
			ID:       "shell",
			Name:     "shell",
			Executor: ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		},
		Freeform: "sleep 5",
	})
	if err != nil {
		t.Fatalf("LaunchJob: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := mgr.GetJob(job.ID)
		if ok && j.Status == StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := mgr.StopJob(job.ID); err != nil {
		t.Fatalf("StopJob: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := mgr.GetJob(job.ID)
		if ok && j.Status == StatusIdle {
			if j.Note != "stopped" {
				t.Fatalf("Note = %q, want stopped", j.Note)
			}
			if len(j.Turns) == 0 || j.Turns[len(j.Turns)-1].Role != TurnAssistant {
				t.Fatalf("expected assistant turn appended after stop, got %+v", j.Turns)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	j, _ := mgr.GetJob(job.ID)
	t.Fatalf("job did not return to idle after stop: status=%s note=%q", j.Status, j.Note)
}

func TestDeleteJobWaitsForCanceledTurnToExit(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	job, err := mgr.LaunchJob(LaunchRequest{
		Repo: dir,
		Preset: LaunchPreset{
			ID:       "shell",
			Name:     "shell",
			Executor: ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		},
		Freeform: "sleep 5",
	})
	if err != nil {
		t.Fatalf("LaunchJob: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := mgr.GetJob(job.ID)
		if ok && j.Status == StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := mgr.DeleteJob(job.ID); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if _, ok := mgr.GetJob(job.ID); ok {
		t.Fatalf("job %s still present after delete", job.ID)
	}
}

func TestLaunchJobTaskQueueSequenceCreatesQueuedSiblingJobs(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Existing active job holds the repo lock, so the new sequence should
	// be created fully queued without trying to start yet.
	if _, err := mgr.Registry.Create(Job{
		PresetID:    "busy",
		Brief:       "busy",
		Repo:        dir,
		Permissions: "scoped-write",
		Status:      StatusNeedsReview,
	}); err != nil {
		t.Fatalf("Create busy job: %v", err)
	}

	first, err := mgr.LaunchJob(LaunchRequest{
		Repo: dir,
		Preset: LaunchPreset{
			ID:          "senior-dev",
			Name:        "Senior dev",
			LaunchMode:  LaunchModeTaskQueueSequence,
			Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
			Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
			Permissions: "scoped-write",
		},
		Sources: []SourceTask{
			{File: filepath.Join(dir, "WORK.md"), Line: 1, Text: "task one"},
			{File: filepath.Join(dir, "WORK.md"), Line: 2, Text: "task two"},
		},
	})
	if err != nil {
		t.Fatalf("LaunchJob: %v", err)
	}
	if first.Status != StatusQueued {
		t.Fatalf("first status = %s, want queued", first.Status)
	}

	var queued []Job
	for _, j := range mgr.ListJobs() {
		if j.CampaignID == first.CampaignID {
			queued = append(queued, j)
		}
	}
	if len(queued) != 2 {
		t.Fatalf("campaign jobs = %d, want 2", len(queued))
	}
	for _, j := range queued {
		if j.Status != StatusQueued {
			t.Fatalf("job %s status = %s, want queued", j.ID, j.Status)
		}
		if j.QueueTotal != 2 {
			t.Fatalf("job %s queue total = %d, want 2", j.ID, j.QueueTotal)
		}
	}
}

func TestQueuedJobsAdvanceSeriallyPerRepo(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	campaign := NewCampaignID()
	job1, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo first",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  0,
		QueueTotal:  2,
	})
	if err != nil {
		t.Fatalf("Create job1: %v", err)
	}
	job2, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo second",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  1,
		QueueTotal:  2,
	})
	if err != nil {
		t.Fatalf("Create job2: %v", err)
	}

	mgr.maybeStartQueuedJobs()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j1, _ := mgr.GetJob(job1.ID)
		j2, _ := mgr.GetJob(job2.ID)
		if j1.Status == StatusIdle && j2.Status == StatusQueued {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	j1, _ := mgr.GetJob(job1.ID)
	j2, _ := mgr.GetJob(job2.ID)
	if j1.Status != StatusIdle {
		t.Fatalf("job1 status = %s, want idle after first run", j1.Status)
	}
	if j2.Status != StatusQueued {
		t.Fatalf("job2 status = %s, want queued while repo lock held", j2.Status)
	}

	if err := mgr.ApproveJob(job1.ID, ""); err != nil {
		t.Fatalf("ApproveJob(job1): %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j2, _ = mgr.GetJob(job2.ID)
		if j2.Status == StatusIdle {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job2 did not start after approving job1: status=%s", j2.Status)
}

func TestSkipJobPreservesRecordAndAdvancesQueuedSibling(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	campaign := NewCampaignID()
	job1, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo first",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  0,
		QueueTotal:  2,
	})
	if err != nil {
		t.Fatalf("Create job1: %v", err)
	}
	job2, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo second",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  1,
		QueueTotal:  2,
	})
	if err != nil {
		t.Fatalf("Create job2: %v", err)
	}

	mgr.maybeStartQueuedJobs()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j1, _ := mgr.GetJob(job1.ID)
		j2, _ := mgr.GetJob(job2.ID)
		if j1.Status == StatusIdle && j2.Status == StatusQueued {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := mgr.SkipJob(job1.ID); err != nil {
		t.Fatalf("SkipJob(job1): %v", err)
	}
	skipped, ok := mgr.GetJob(job1.ID)
	if !ok {
		t.Fatalf("skipped job missing; skip should preserve record")
	}
	if skipped.Status != StatusCompleted {
		t.Fatalf("skipped job status = %s, want completed", skipped.Status)
	}
	if skipped.SyncBackState != SyncBackSkipped {
		t.Fatalf("skipped sync state = %s, want skipped", skipped.SyncBackState)
	}
	if skipped.Note != "skipped by operator" {
		t.Fatalf("skipped note = %q, want skipped by operator", skipped.Note)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j2, _ := mgr.GetJob(job2.ID)
		if j2.Status == StatusIdle {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	j2, _ := mgr.GetJob(job2.ID)
	t.Fatalf("job2 did not start after skipping job1: status=%s", j2.Status)
}

func TestSkipCampaignSkipsCurrentAndQueuedTail(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	campaign := NewCampaignID()
	job1, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo first",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  0,
		QueueTotal:  3,
	})
	if err != nil {
		t.Fatalf("Create job1: %v", err)
	}
	job2, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo second",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  1,
		QueueTotal:  3,
	})
	if err != nil {
		t.Fatalf("Create job2: %v", err)
	}
	job3, err := mgr.Registry.Create(Job{
		CampaignID:  campaign,
		PresetID:    "shell",
		Brief:       "echo third",
		Repo:        dir,
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions: "scoped-write",
		Status:      StatusQueued,
		QueueIndex:  2,
		QueueTotal:  3,
	})
	if err != nil {
		t.Fatalf("Create job3: %v", err)
	}

	mgr.maybeStartQueuedJobs()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j1, _ := mgr.GetJob(job1.ID)
		if j1.Status == StatusIdle {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := mgr.SkipCampaign(job1.ID); err != nil {
		t.Fatalf("SkipCampaign(job1): %v", err)
	}
	for _, id := range []JobID{job1.ID, job2.ID, job3.ID} {
		j, ok := mgr.GetJob(id)
		if !ok {
			t.Fatalf("job %s missing after SkipCampaign", id)
		}
		if j.Status != StatusCompleted {
			t.Fatalf("job %s status = %s, want completed", id, j.Status)
		}
		if j.SyncBackState != SyncBackSkipped {
			t.Fatalf("job %s sync state = %s, want skipped", id, j.SyncBackState)
		}
	}
	j1State, _ := mgr.GetJob(job1.ID)
	if j1State.Note != "skipped by operator" {
		t.Fatalf("job1 note = %q, want skipped by operator", j1State.Note)
	}
	for _, id := range []JobID{job2.ID, job3.ID} {
		j, _ := mgr.GetJob(id)
		if j.Note != "skipped by campaign abort" {
			t.Fatalf("job %s note = %q, want skipped by campaign abort", id, j.Note)
		}
	}
}

func TestQueueOnlyLaunchWaitsForForeman(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		ForemanFile:  filepath.Join(dir, "state", "foreman.json"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	job, err := mgr.LaunchJob(LaunchRequest{
		Repo: dir,
		Preset: LaunchPreset{
			ID:          "shell",
			Name:        "Shell",
			Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
			Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
			Permissions: "scoped-write",
		},
		Freeform:  "echo hello",
		QueueOnly: true,
	})
	if err != nil {
		t.Fatalf("LaunchJob: %v", err)
	}
	if job.Status != StatusQueued {
		t.Fatalf("job status = %s, want queued", job.Status)
	}
	if !job.WaitForForeman {
		t.Fatalf("job.WaitForForeman = false, want true")
	}
	if !job.ForemanManaged {
		t.Fatalf("job.ForemanManaged = false, want true")
	}

	time.Sleep(150 * time.Millisecond)
	queued, _ := mgr.GetJob(job.ID)
	if queued.Status != StatusQueued {
		t.Fatalf("job status while foreman off = %s, want queued", queued.Status)
	}

	state, err := mgr.SetForemanEnabled(true)
	if err != nil {
		t.Fatalf("SetForemanEnabled(true): %v", err)
	}
	if !state.Enabled {
		t.Fatalf("state.Enabled = false, want true")
	}

	waitForJobTerminalState(t, mgr, job.ID)
	started, _ := mgr.GetJob(job.ID)
	if started.Status != StatusIdle {
		t.Fatalf("job status after foreman on = %s, want idle", started.Status)
	}
	if started.WaitForForeman {
		t.Fatalf("job.WaitForForeman after start = true, want false")
	}
}

func TestStartJobPromotesWaitingForemanJobWithoutEnablingForeman(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		ForemanFile:  filepath.Join(dir, "state", "foreman.json"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	job, err := mgr.LaunchJob(LaunchRequest{
		Repo: dir,
		Preset: LaunchPreset{
			ID:          "shell",
			Name:        "Shell",
			Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
			Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
			Permissions: "scoped-write",
		},
		Freeform:  "echo hello",
		QueueOnly: true,
	})
	if err != nil {
		t.Fatalf("LaunchJob: %v", err)
	}

	started, err := mgr.StartJob(job.ID)
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	if started.ForemanManaged {
		t.Fatalf("started.ForemanManaged = true, want false")
	}
	if started.WaitForForeman {
		t.Fatalf("started.WaitForForeman = true, want false")
	}

	waitForJobTerminalState(t, mgr, job.ID)
	final, _ := mgr.GetJob(job.ID)
	if final.Status != StatusIdle {
		t.Fatalf("job status after manual start = %s, want idle", final.Status)
	}
}

func TestSupervisorStateFromPane(t *testing.T) {
	t.Parallel()

	status, note, ok := supervisorStateFromPane("hello\nSB_STATUS:WAITING_HUMAN\n")
	if !ok || status != StatusIdle || note != "waiting for human input" {
		t.Fatalf("waiting marker = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("hello\nSB_STATUS:READY_REVIEW\n")
	if !ok || status != StatusNeedsReview || note != "ready for review" {
		t.Fatalf("review marker = (%v, %q, %v)", status, note, ok)
	}

	_, _, ok = supervisorStateFromPane("hello\nno markers\n")
	if ok {
		t.Fatal("unexpected marker detection")
	}
}

func TestForemanStatePersistsAcrossManagerRestart(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		ForemanFile:  filepath.Join(dir, "state", "foreman.json"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled(true): %v", err)
	}

	mgr2, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager restart: %v", err)
	}
	if !mgr2.GetForemanState().Enabled {
		t.Fatalf("foreman state after restart = off, want on")
	}
}

func TestBuildTmuxCommandClaudeNormalizesPrintArg(t *testing.T) {
	t.Parallel()

	cmd, err := buildTmuxCommand(Job{
		Brief:    "fix the bug",
		Executor: ExecutorSpec{Type: "claude", Args: []string{"--print", "--model", "sonnet"}},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"claude", "--model", "sonnet", "fix the bug"}
	assertArgsEqual(t, cmd, want)
}

func TestBuildTmuxCommandCodexNormalizesExecJSONArgs(t *testing.T) {
	t.Parallel()

	cmd, err := buildTmuxCommand(Job{
		Brief:    "scaffold tests",
		Executor: ExecutorSpec{Type: "codex", Args: []string{"exec", "--json", "--model", "gpt-5"}},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"codex", "--model", "gpt-5", "scaffold tests"}
	assertArgsEqual(t, cmd, want)
}

func TestRegistryRehydrateKeepsRunningTmuxJobs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	reg := NewRegistry(paths)
	job, err := reg.Create(Job{
		PresetID: "claude",
		Brief:    "hello",
		Repo:     dir,
		Executor: ExecutorSpec{Type: "claude"},
		Status:   StatusRunning,
		Runner:   RunnerTmux,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := reg.Save(job.ID); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reg2 := NewRegistry(paths)
	if err := reg2.Rehydrate(); err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	got, ok := reg2.Get(job.ID)
	if !ok {
		t.Fatalf("missing job %s", job.ID)
	}
	if got.Status != StatusRunning {
		t.Fatalf("status = %s, want %s", got.Status, StatusRunning)
	}
}

func assertArgsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d (%q vs %q)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full=%q)", i, got[i], want[i], got)
		}
	}
}

func waitForJobTerminalState(t *testing.T, mgr *Manager, id JobID) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := mgr.GetJob(id)
		if ok && (j.Status == StatusIdle || j.Status == StatusFailed || j.Status == StatusCompleted || j.Status == StatusBlocked || j.Status == StatusNeedsReview) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := mgr.GetJob(id)
	t.Fatalf("job did not settle: status=%s note=%q", j.Status, j.Note)
}
