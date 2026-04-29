package cockpit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureReviewArtifactIncludesGitAndHookContext(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("before\nafter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tracked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile new: %v", err)
	}

	artifactsDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll artifacts: %v", err)
	}
	eventLog := filepath.Join(dir, "events.jsonl")
	events := []string{
		`{"ts":"2026-04-27T12:00:00Z","job_id":"j-1","kind":"hook_started","payload":{"phase":"pre","name":"lint","cmd":"make lint"}}`,
		`{"ts":"2026-04-27T12:00:01Z","job_id":"j-1","kind":"hook_finished","payload":{"phase":"pre","name":"lint","exit":0,"duration_ms":42,"output":"ok"}}`,
	}
	if err := os.WriteFile(eventLog, []byte(strings.Join(events, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile event log: %v", err)
	}

	// Stub git with deterministic output so the test does not depend on a real repo.
	gitStubDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(gitStubDir, 0o755); err != nil {
		t.Fatalf("MkdirAll git stub: %v", err)
	}
	gitStub := filepath.Join(gitStubDir, "git")
	script := `#!/bin/sh
if [ "$3" = "status" ]; then
  printf ' M tracked.txt\n?? new.txt\n'
  exit 0
fi
if [ "$3" = "diff" ]; then
  printf ' tracked.txt | 1 +\n 1 file changed, 1 insertion(+)\n'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(gitStub, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile git stub: %v", err)
	}
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", gitStubDir+string(os.PathListSeparator)+origPath)

	job := Job{
		ID:           "j-1",
		Repo:         repo,
		ArtifactsDir: artifactsDir,
		EventLogPath: eventLog,
		Status:       StatusNeedsReview,
		Hooks: HookSpec{
			PostShell: []ShellHook{{Name: "git diff --stat", Cmd: "git diff --stat"}},
		},
	}
	if err := CaptureReviewArtifact(job); err != nil {
		t.Fatalf("CaptureReviewArtifact: %v", err)
	}
	artifact, ok := LoadReviewArtifact(job)
	if !ok {
		t.Fatal("LoadReviewArtifact returned !ok")
	}
	if len(artifact.ChangedFiles) != 2 || artifact.ChangedFiles[0] != " M tracked.txt" {
		t.Fatalf("ChangedFiles = %#v", artifact.ChangedFiles)
	}
	if len(artifact.PreexistingDirty) != 0 {
		t.Fatalf("PreexistingDirty = %#v, want empty", artifact.PreexistingDirty)
	}
	if len(artifact.DiffStat) == 0 || !strings.Contains(artifact.DiffStat[0], "tracked.txt") {
		t.Fatalf("DiffStat = %#v", artifact.DiffStat)
	}
	if len(artifact.HookEvents) == 0 || artifact.HookEvents[0].Name != "lint" {
		t.Fatalf("HookEvents = %#v", artifact.HookEvents)
	}
	if len(artifact.PendingPostHooks) != 1 || artifact.PendingPostHooks[0] != "git diff --stat" {
		t.Fatalf("PendingPostHooks = %#v", artifact.PendingPostHooks)
	}
	if artifact.GeneratedAt.IsZero() || artifact.GeneratedAt.Before(time.Now().Add(-1*time.Minute)) {
		t.Fatalf("GeneratedAt looks wrong: %v", artifact.GeneratedAt)
	}
}

func TestCaptureReviewArtifactSeparatesPreexistingDirtyFromNewChanges(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	artifactsDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll artifacts: %v", err)
	}

	gitStubDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(gitStubDir, 0o755); err != nil {
		t.Fatalf("MkdirAll git stub: %v", err)
	}
	gitStub := filepath.Join(gitStubDir, "git")
	script := `#!/bin/sh
if [ "$3" = "status" ]; then
  printf ' M WORK.md\n?? new.txt\n'
  exit 0
fi
if [ "$3" = "diff" ]; then
  printf ' new.txt | 1 +\n 1 file changed, 1 insertion(+)\n'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(gitStub, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile git stub: %v", err)
	}
	t.Setenv("PATH", gitStubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	job := Job{
		Repo:               repo,
		ArtifactsDir:       artifactsDir,
		Status:             StatusNeedsReview,
		RepoStatusAtLaunch: []string{" M WORK.md"},
	}
	if err := CaptureReviewArtifact(job); err != nil {
		t.Fatalf("CaptureReviewArtifact: %v", err)
	}
	artifact, ok := LoadReviewArtifact(job)
	if !ok {
		t.Fatal("LoadReviewArtifact returned !ok")
	}
	if len(artifact.ChangedFiles) != 1 || artifact.ChangedFiles[0] != "?? new.txt" {
		t.Fatalf("ChangedFiles = %#v", artifact.ChangedFiles)
	}
	if len(artifact.PreexistingDirty) != 1 || artifact.PreexistingDirty[0] != " M WORK.md" {
		t.Fatalf("PreexistingDirty = %#v", artifact.PreexistingDirty)
	}
}

func TestEnsureSyncBackTargetsCleanRefusesDirtySourceOrDevlog(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	workPath := filepath.Join(repo, "WORK.md")
	devlogPath := filepath.Join(repo, "DEVLOG.md")

	gitStubDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(gitStubDir, 0o755); err != nil {
		t.Fatalf("MkdirAll git stub: %v", err)
	}
	gitStub := filepath.Join(gitStubDir, "git")
	script := `#!/bin/sh
if [ "$3" = "status" ]; then
  printf ' M WORK.md\n M DEVLOG.md\n'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(gitStub, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile git stub: %v", err)
	}
	t.Setenv("PATH", gitStubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := ensureSyncBackTargetsClean(Job{
		Repo: repo,
		Sources: []SourceTask{
			{File: workPath, Line: 1, Text: "fix me"},
		},
	}, devlogPath)
	if err == nil || !strings.Contains(err.Error(), "sync-back refused") || !strings.Contains(err.Error(), "WORK.md") {
		t.Fatalf("ensureSyncBackTargetsClean error = %v", err)
	}
}

func TestRefreshPostHookPreviewMarksOKAndWouldFail(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	artifactsDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll artifacts: %v", err)
	}
	job := Job{
		ID:           "j-preview",
		Repo:         repo,
		ArtifactsDir: artifactsDir,
		Hooks: HookSpec{
			PostShell: []ShellHook{
				{Name: "always-ok", Cmd: "true"},
				{Name: "always-fail", Cmd: "false"},
			},
		},
	}
	previews := RefreshPostHookPreview(job)
	if len(previews) != 2 {
		t.Fatalf("len(previews) = %d, want 2", len(previews))
	}
	if previews[0].Status != HookPreviewOK {
		t.Fatalf("preview[0].Status = %q, want ok", previews[0].Status)
	}
	if previews[1].Status != HookPreviewWouldFail || previews[1].ExitCode == 0 {
		t.Fatalf("preview[1] = %+v, want would_fail with non-zero exit", previews[1])
	}
	cached, ok := LoadPostHookPreviews(job)
	if !ok || len(cached) != 2 || cached[1].Status != HookPreviewWouldFail {
		t.Fatalf("LoadPostHookPreviews = %v, %v", cached, ok)
	}
}

func TestRefreshPostHookPreviewSkipsMutatingCmds(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	artifactsDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll artifacts: %v", err)
	}
	job := Job{
		ID:           "j-skip",
		Repo:         repo,
		ArtifactsDir: artifactsDir,
		Hooks: HookSpec{
			PostShell: []ShellHook{
				{Name: "push", Cmd: "git push origin main"},
				{Name: "log-redirect", Cmd: "echo hi > /tmp/log.txt"},
				{Name: "force-safe", Cmd: "git push origin main", PreviewSafe: true, PreviewCmd: "true"},
			},
		},
	}
	previews := RefreshPostHookPreview(job)
	if len(previews) != 3 {
		t.Fatalf("len(previews) = %d", len(previews))
	}
	if previews[0].Status != HookPreviewSkipped || previews[0].SkipReason == "" {
		t.Fatalf("git push preview = %+v, want skipped", previews[0])
	}
	if previews[1].Status != HookPreviewSkipped {
		t.Fatalf("redirection preview = %+v, want skipped", previews[1])
	}
	if previews[2].Status != HookPreviewOK {
		t.Fatalf("force-safe preview = %+v, want ok via PreviewCmd override", previews[2])
	}
}

func TestPreviewPostHooksReturnsCachedWhenFresh(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	artifactsDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll artifacts: %v", err)
	}
	job := Job{
		ID:           "j-cache",
		Repo:         repo,
		ArtifactsDir: artifactsDir,
		Hooks:        HookSpec{PostShell: []ShellHook{{Name: "ok", Cmd: "true"}}},
	}
	first := PreviewPostHooks(job)
	if len(first) != 1 || first[0].Status != HookPreviewOK {
		t.Fatalf("first PreviewPostHooks = %+v", first)
	}
	second := PreviewPostHooks(job)
	if len(second) != 1 {
		t.Fatalf("second PreviewPostHooks len = %d", len(second))
	}
	if !second[0].GeneratedAt.Equal(first[0].GeneratedAt) {
		t.Fatalf("expected cached result, got fresh refresh: %v vs %v", first[0].GeneratedAt, second[0].GeneratedAt)
	}
}

func TestGitReviewHelpersIgnoreCommandFailures(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Fatalf("sh not found: %v", err)
	}
	if got := gitShortStatus(repo); got != nil {
		t.Fatalf("gitShortStatus = %#v, want nil outside git repo", got)
	}
	if got := gitDiffStat(repo); got != nil {
		t.Fatalf("gitDiffStat = %#v, want nil outside git repo", got)
	}
}
