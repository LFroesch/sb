package cockpit

import (
	"context"
	"fmt"
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

func TestBuildTurnCmdCodexForemanUsesExplicitSandboxAndNoApproval(t *testing.T) {
	t.Parallel()

	repo := "/tmp/sb-demo"
	cmd, stdinBody, err := buildTurnCmd(context.Background(), Job{
		Repo:           repo,
		Permissions:    "scoped-write",
		ForemanManaged: true,
		Executor:       ExecutorSpec{Type: "codex"},
	}, "follow-up")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	want := []string{"codex", "--sandbox", "workspace-write", "--cd", repo, "--ask-for-approval", "never", "exec", "--json", "follow-up"}
	assertArgsEqual(t, cmd.Args, want)
	if stdinBody != "" {
		t.Fatal("expected codex prompt as argv, not stdin replay")
	}
}

func TestBuildTurnCmdClaudeWideOpenUsesBypassPermissions(t *testing.T) {
	t.Parallel()

	cmd, stdinBody, err := buildTurnCmd(context.Background(), Job{
		Permissions: "wide-open",
		Executor:    ExecutorSpec{Type: "claude"},
	}, "follow-up")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	want := []string{"claude", "-p", "--permission-mode", "bypassPermissions", "follow-up"}
	assertArgsEqual(t, cmd.Args, want)
	if stdinBody != "" {
		t.Fatal("expected claude prompt as argv")
	}
}

func TestBuildTurnCmdClaudeHonorsExecutorModel(t *testing.T) {
	t.Parallel()

	cmd, _, err := buildTurnCmd(context.Background(), Job{
		Executor: ExecutorSpec{Type: "claude", Model: "claude-sonnet-4-6"},
	}, "do thing")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	want := []string{"claude", "-p", "--model", "claude-sonnet-4-6", "do thing"}
	assertArgsEqual(t, cmd.Args, want)
}

func TestBuildTurnCmdClaudeArgsModelOverridesExecutorModel(t *testing.T) {
	t.Parallel()

	cmd, _, err := buildTurnCmd(context.Background(), Job{
		Executor: ExecutorSpec{Type: "claude", Model: "claude-opus-4-7", Args: []string{"--model", "sonnet"}},
	}, "do thing")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	want := []string{"claude", "-p", "--model", "sonnet", "do thing"}
	assertArgsEqual(t, cmd.Args, want)
}

func TestBuildTurnCmdCodexHonorsExecutorModel(t *testing.T) {
	t.Parallel()

	cmd, _, err := buildTurnCmd(context.Background(), Job{
		Executor: ExecutorSpec{Type: "codex", Model: "gpt-5"},
	}, "do thing")
	if err != nil {
		t.Fatalf("buildTurnCmd: %v", err)
	}
	want := []string{"codex", "--model", "gpt-5", "exec", "--json", "do thing"}
	assertArgsEqual(t, cmd.Args, want)
}

func TestTakeOverJobSupersedesForemanTmuxJob(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "tmux-shim.sh")
	logPath := filepath.Join(dir, "tmux.log")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
printf '%s\n' "$*" >> "` + logPath + `"
case "$1" in
  list-panes)
    printf '0\n'
    ;;
  capture-pane)
    printf '$ go test ./...\nopened app/main.go\n'
    ;;
  kill-window)
    ;;
esac
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(shim): %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	sourceFile := filepath.Join(dir, "WORK.md")
	if err := os.WriteFile(sourceFile, []byte("- task\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(source): %v", err)
	}
	old, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Task:           "ship it",
		Brief:          "ship it",
		Prompt:         "ship it",
		Freeform:       "echo ship-it",
		Repo:           dir,
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Status:         StatusIdle,
		Runner:         RunnerTmux,
		TmuxTarget:     "sb-cockpit:@3",
		LogPath:        filepath.Join(dir, "old.log"),
		TranscriptPath: filepath.Join(dir, "old.txt"),
		ArtifactsDir:   filepath.Join(dir, "artifacts"),
		Sources:        []SourceTask{{File: sourceFile, Line: 1, Text: "ship it"}},
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create(old): %v", err)
	}
	if err := os.MkdirAll(old.ArtifactsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(artifacts): %v", err)
	}
	if err := os.WriteFile(old.LogPath, []byte("$ go test ./...\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(log): %v", err)
	}
	if err := os.WriteFile(old.TranscriptPath, []byte("opened app/main.go\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript): %v", err)
	}

	replacement, err := mgr.TakeOverJob(old.ID, []LaunchPreset{{
		ID:          "shell",
		Name:        "Shell",
		Permissions: "scoped-write",
		Executor:    ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:       HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
	}})
	if err != nil {
		t.Fatalf("TakeOverJob: %v", err)
	}
	waitForJobTerminalState(t, mgr, replacement.ID)

	updatedOld, ok := mgr.GetJob(old.ID)
	if !ok {
		t.Fatalf("old job missing")
	}
	if updatedOld.SupersededBy != replacement.ID {
		t.Fatalf("old.SupersededBy = %q, want %q", updatedOld.SupersededBy, replacement.ID)
	}
	if updatedOld.Status != StatusCompleted {
		t.Fatalf("old.Status = %s, want completed", updatedOld.Status)
	}
	updatedNew, ok := mgr.GetJob(replacement.ID)
	if !ok {
		t.Fatalf("replacement job missing")
	}
	if updatedNew.TakeoverOf != old.ID {
		t.Fatalf("new.TakeoverOf = %q, want %q", updatedNew.TakeoverOf, old.ID)
	}
	if updatedNew.ForemanManaged || updatedNew.WaitForForeman {
		t.Fatalf("replacement should not be Foreman-managed: %+v", updatedNew)
	}
	if len(updatedNew.Sources) != 1 || updatedNew.Sources[0].Text != "ship it" {
		t.Fatalf("replacement sources changed: %+v", updatedNew.Sources)
	}
	if !strings.Contains(updatedNew.Prompt, "## Manual Takeover Handoff") {
		t.Fatalf("replacement prompt missing handoff block:\n%s", updatedNew.Prompt)
	}
}

func TestTakeOverJobRejectsNonForemanJobs(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	job, err := mgr.Registry.Create(Job{
		PresetID: "shell",
		Repo:     dir,
		Status:   StatusIdle,
		Runner:   RunnerTmux,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := mgr.TakeOverJob(job.ID, nil); err == nil || !strings.Contains(err.Error(), "Foreman-managed") {
		t.Fatalf("expected Foreman-managed error, got %v", err)
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
	if !ok || status != StatusAwaitingHuman || note != "waiting for human input" {
		t.Fatalf("waiting marker = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("hello\nSB_STATUS:READY_REVIEW\n")
	if !ok || status != StatusNeedsReview || note != "ready for review" {
		t.Fatalf("review marker = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("hello\nConversation interrupted\n")
	if !ok || status != StatusIdle || note != "conversation interrupted" {
		t.Fatalf("interrupt fallback = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("hello\nUsage limit reached, try again at 3am\n")
	if !ok || status != StatusIdle || note != "provider limit reached" {
		t.Fatalf("limit fallback = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("done with the patch\nWould you like me to also add the migration?\n")
	if !ok || status != StatusAwaitingHuman || note != "awaiting operator input" {
		t.Fatalf("question fallback = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("tests are green\nIf you'd like, I can also clean up the helper naming.\n")
	if !ok || status != StatusAwaitingHuman || note != "awaiting operator input" {
		t.Fatalf("follow-up fallback = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("I fixed the root issue.\nThe next obvious cleanup, if you'd like me to keep going, is to pull the tmux waiting heuristics into a structured matcher instead of the current hardcoded string block.\n")
	if !ok || status != StatusAwaitingHuman || note != "awaiting operator input" {
		t.Fatalf("soft offer fallback = (%v, %q, %v)", status, note, ok)
	}

	status, note, ok = supervisorStateFromPane("Done.\nChoose one: keep current behavior, broaden detection, or add a config knob.\n")
	if !ok || status != StatusAwaitingHuman || note != "awaiting operator input" {
		t.Fatalf("choice fallback = (%v, %q, %v)", status, note, ok)
	}

	_, _, ok = supervisorStateFromPane("hello\nno markers\n")
	if ok {
		t.Fatal("unexpected marker detection")
	}
}

func TestSupervisorQuietLongEnough(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	if err := os.WriteFile(logPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(logPath, now, now); err != nil {
		t.Fatalf("Chtimes(now): %v", err)
	}
	if supervisorQuietLongEnough(logPath, now.Add(5*time.Second)) {
		t.Fatal("quiet gate triggered too early")
	}
	if err := os.Chtimes(logPath, now.Add(-11*time.Second), now.Add(-11*time.Second)); err != nil {
		t.Fatalf("Chtimes(old): %v", err)
	}
	if !supervisorQuietLongEnough(logPath, now) {
		t.Fatal("quiet gate did not trigger after quiet period")
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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

func TestBuildTmuxCommandCodexForemanUsesExplicitRuntimePolicy(t *testing.T) {
	t.Parallel()

	repo := "/tmp/sb-demo"
	cmd, err := buildTmuxCommand(Job{
		Repo:           repo,
		Brief:          "scaffold tests",
		Permissions:    "wide-open",
		ForemanManaged: true,
		Executor:       ExecutorSpec{Type: "codex", Args: []string{"--model", "gpt-5"}},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"codex", "--sandbox", "danger-full-access", "--cd", repo, "--ask-for-approval", "never", "--model", "gpt-5", "scaffold tests"}
	assertArgsEqual(t, cmd, want)
}

func TestBuildTmuxCommandClaudeForemanUsesDontAsk(t *testing.T) {
	t.Parallel()

	cmd, err := buildTmuxCommand(Job{
		Brief:          "fix the bug",
		Permissions:    "scoped-write",
		ForemanManaged: true,
		Executor:       ExecutorSpec{Type: "claude", Args: []string{"--model", "sonnet"}},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"claude", "--permission-mode", "dontAsk", "--model", "sonnet", "fix the bug"}
	assertArgsEqual(t, cmd, want)
}

func TestBuildTmuxCommandClaudeAttendedScopedWriteUsesAcceptEdits(t *testing.T) {
	t.Parallel()

	// Attended scoped-write maps to --permission-mode acceptEdits so edits
	// are auto-approved but bash still gates interactively.
	cmd, err := buildTmuxCommand(Job{
		Brief:       "fix the bug",
		Permissions: "scoped-write",
		Executor:    ExecutorSpec{Type: "claude", Args: []string{"--model", "sonnet"}},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"claude", "--permission-mode", "acceptEdits", "--model", "sonnet", "fix the bug"}
	assertArgsEqual(t, cmd, want)
}

func TestBuildTmuxCommandClaudeAttendedReadOnlyUsesPlan(t *testing.T) {
	t.Parallel()

	cmd, err := buildTmuxCommand(Job{
		Brief:       "explain this",
		Permissions: "read-only",
		Executor:    ExecutorSpec{Type: "claude"},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"claude", "--permission-mode", "plan", "explain this"}
	assertArgsEqual(t, cmd, want)
}

func TestBuildTmuxCommandCodexAttendedScopedWriteOmitsAskNever(t *testing.T) {
	t.Parallel()

	repo := "/tmp/sb-demo"
	cmd, err := buildTmuxCommand(Job{
		Repo:        repo,
		Brief:       "scaffold tests",
		Permissions: "scoped-write",
		Executor:    ExecutorSpec{Type: "codex", Args: []string{"--model", "gpt-5"}},
	})
	if err != nil {
		t.Fatalf("buildTmuxCommand: %v", err)
	}
	want := []string{"codex", "--sandbox", "workspace-write", "--cd", repo, "--model", "gpt-5", "scaffold tests"}
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
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
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
		if ok && (j.Status == StatusIdle || j.Status == StatusAwaitingHuman || j.Status == StatusFailed || j.Status == StatusCompleted || j.Status == StatusBlocked || j.Status == StatusNeedsReview) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := mgr.GetJob(id)
	t.Fatalf("job did not settle: status=%s note=%q", j.Status, j.Note)
}

// stubProviderLimitPct overrides the package var for the duration of t.
func stubProviderLimitPct(t *testing.T, fn func(provider string) (int, bool)) {
	t.Helper()
	prev := providerLimitPct
	providerLimitPct = fn
	t.Cleanup(func() { providerLimitPct = prev })
}

func newSchedulerTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	paths := Paths{
		StateDir:     filepath.Join(dir, "state"),
		JobsDir:      filepath.Join(dir, "state", "jobs"),
		CampaignDir:  filepath.Join(dir, "state", "campaigns"),
		ForemanFile:  filepath.Join(dir, "state", "foreman.json"),
		PresetsDir:   filepath.Join(dir, "config", "presets"),
		ProvidersDir: filepath.Join(dir, "config", "providers"),
		PromptsDir:   filepath.Join(dir, "config", "prompts"),
		HooksDir:     filepath.Join(dir, "config", "hooks"),
		Socket:       filepath.Join(dir, "state", "foreman.sock"),
		PIDFile:      filepath.Join(dir, "state", "foreman.pid"),
		LogFile:      filepath.Join(dir, "state", "foreman.log"),
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr, dir
}

func waitForEligibilityReason(t *testing.T, mgr *Manager, id JobID, want string) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := mgr.GetJob(id)
		if ok && j.EligibilityReason == want {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := mgr.GetJob(id)
	t.Fatalf("eligibility reason = %q, want %q", j.EligibilityReason, want)
	return j
}

func waitForJobStatus(t *testing.T, mgr *Manager, id JobID, want Status) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := mgr.GetJob(id)
		if ok && j.Status == want {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := mgr.GetJob(id)
	t.Fatalf("job %s status = %s, want %s (note=%q)", id, j.Status, want, j.Note)
	return Job{}
}

func TestForemanOffMarksParkedJobsWaitingForForeman(t *testing.T) {
	mgr, dir := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(string) (int, bool) { return 0, false })

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
	waitForEligibilityReason(t, mgr, job.ID, "waiting for foreman")

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled(true): %v", err)
	}
	waitForJobTerminalState(t, mgr, job.ID)
	final, _ := mgr.GetJob(job.ID)
	if final.EligibilityReason != "" {
		t.Fatalf("EligibilityReason after start = %q, want empty", final.EligibilityReason)
	}
}

func TestForemanStartsEligibleJobsAcrossReposInParallel(t *testing.T) {
	mgr, _ := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(string) (int, bool) { return 0, false })

	jobA, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "sleep 2",
		Repo:           t.TempDir(),
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		WaitForForeman: true,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create jobA: %v", err)
	}
	jobB, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "sleep 2",
		Repo:           t.TempDir(),
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		WaitForForeman: true,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create jobB: %v", err)
	}

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled(true): %v", err)
	}

	startedA := waitForJobStatus(t, mgr, jobA.ID, StatusRunning)
	startedB := waitForJobStatus(t, mgr, jobB.ID, StatusRunning)
	if startedA.WaitForForeman || startedB.WaitForForeman {
		t.Fatalf("jobs still marked waiting for foreman: A=%v B=%v", startedA.WaitForForeman, startedB.WaitForForeman)
	}

	waitForJobTerminalState(t, mgr, jobA.ID)
	waitForJobTerminalState(t, mgr, jobB.ID)
}

func TestForemanConcurrencyCapDefersExtraJobs(t *testing.T) {
	mgr, _ := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(string) (int, bool) { return 0, false })

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled: %v", err)
	}

	// Pre-populate three foreman-managed running jobs across distinct
	// repos (so repo lock doesn't masquerade as the cap).
	for i := 0; i < 3; i++ {
		repo := filepath.Join(t.TempDir(), fmt.Sprintf("r%d", i))
		if _, err := mgr.Registry.Create(Job{
			PresetID:       "shell",
			Brief:          "running",
			Repo:           repo,
			Permissions:    "scoped-write",
			Status:         StatusRunning,
			ForemanManaged: true,
		}); err != nil {
			t.Fatalf("Create active job: %v", err)
		}
	}

	queuedRepo := t.TempDir()
	queued, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "fourth",
		Repo:           queuedRepo,
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create queued: %v", err)
	}

	mgr.tickScheduler()
	waitForEligibilityReason(t, mgr, queued.ID, "foreman concurrency cap (3/3)")
}

func TestAwaitingHumanReleasesForemanConcurrencyButKeepsRepoLock(t *testing.T) {
	mgr, dir := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(string) (int, bool) { return 0, false })

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled: %v", err)
	}

	holder, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "holder",
		Repo:           dir,
		Permissions:    "scoped-write",
		Status:         StatusAwaitingHuman,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create holder: %v", err)
	}

	sameRepo, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "echo same-repo",
		Repo:           dir,
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create sameRepo: %v", err)
	}

	otherRepo := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(otherRepo, 0o755); err != nil {
		t.Fatalf("MkdirAll(otherRepo): %v", err)
	}
	other, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "echo other-repo",
		Repo:           otherRepo,
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}

	mgr.tickScheduler()
	waitForEligibilityReason(t, mgr, sameRepo.ID, "repo busy: "+shortDisplayJobID(holder.ID))
	waitForJobTerminalState(t, mgr, other.ID)
	finalOther, _ := mgr.GetJob(other.ID)
	if finalOther.Status != StatusIdle {
		t.Fatalf("other repo job status = %s, want idle after concurrency slot released", finalOther.Status)
	}
}

func TestForemanLimitGuardDefersClaudeWhenNearLimit(t *testing.T) {
	mgr, dir := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(provider string) (int, bool) {
		if provider == "claude" {
			return 95, true
		}
		return 0, false
	})

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled: %v", err)
	}

	queued, err := mgr.Registry.Create(Job{
		PresetID:       "claude",
		Brief:          "claude run",
		Repo:           dir,
		Executor:       ExecutorSpec{Type: "claude"},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mgr.tickScheduler()
	waitForEligibilityReason(t, mgr, queued.ID, "claude near 5h limit (95%)")
}

func TestForemanLimitGuardSkipsLocalProviders(t *testing.T) {
	mgr, dir := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(string) (int, bool) { return 99, true })

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled: %v", err)
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
	waitForJobTerminalState(t, mgr, job.ID)
	final, _ := mgr.GetJob(job.ID)
	if final.EligibilityReason != "" {
		t.Fatalf("shell job blocked by limit guard: reason=%q", final.EligibilityReason)
	}
}

func TestRepoBusyReasonNamesBlockingJob(t *testing.T) {
	mgr, dir := newSchedulerTestManager(t)
	stubProviderLimitPct(t, func(string) (int, bool) { return 0, false })

	if _, err := mgr.SetForemanEnabled(true); err != nil {
		t.Fatalf("SetForemanEnabled: %v", err)
	}

	holder, err := mgr.Registry.Create(Job{
		PresetID:    "shell",
		Brief:       "holder",
		Repo:        dir,
		Permissions: "scoped-write",
		Status:      StatusNeedsReview,
	})
	if err != nil {
		t.Fatalf("Create holder: %v", err)
	}

	queued, err := mgr.Registry.Create(Job{
		PresetID:       "shell",
		Brief:          "blocked",
		Repo:           dir,
		Executor:       ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:          HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
		Permissions:    "scoped-write",
		Status:         StatusQueued,
		ForemanManaged: true,
	})
	if err != nil {
		t.Fatalf("Create queued: %v", err)
	}

	mgr.tickScheduler()
	got := waitForEligibilityReason(t, mgr, queued.ID, "repo busy: "+shortDisplayJobID(holder.ID))
	if !strings.Contains(got.EligibilityReason, "repo busy") {
		t.Fatalf("reason missing repo busy prefix: %q", got.EligibilityReason)
	}
}
