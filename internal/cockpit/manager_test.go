package cockpit

import (
	"context"
	"os"
	"path/filepath"
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
