package cockpit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ReviewArtifactPath(j Job) string {
	if strings.TrimSpace(j.ArtifactsDir) == "" {
		return ""
	}
	return filepath.Join(j.ArtifactsDir, "review.json")
}

func HookPreviewPath(j Job) string {
	if strings.TrimSpace(j.ArtifactsDir) == "" {
		return ""
	}
	return filepath.Join(j.ArtifactsDir, "post_hook_preview.json")
}

// hookPreviewMaxAge bounds how long a cached preview is considered fresh.
// Refreshes happen automatically when a turn finishes (callers explicitly
// invoke RefreshPostHookPreview) or when this TTL expires from open.
const hookPreviewMaxAge = 5 * time.Minute

// LoadPostHookPreviews reads the cached preview for a job, if any.
// Returns (nil, false) when the cache is missing or unparseable.
func LoadPostHookPreviews(j Job) ([]HookPreview, bool) {
	path := HookPreviewPath(j)
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var out []HookPreview
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	return out, true
}

// RefreshPostHookPreview re-runs every post-hook in dry-run mode and
// caches the results to <artifacts>/post_hook_preview.json. Returns the
// newly captured slice. Hooks whose cmd looks side-effect-bearing are
// skipped with HookPreviewSkipped + a SkipReason rather than executed.
func RefreshPostHookPreview(j Job) []HookPreview {
	out := make([]HookPreview, 0, len(j.Hooks.PostShell))
	for _, h := range j.Hooks.PostShell {
		out = append(out, runHookPreview(h, j.Repo))
	}
	if path := HookPreviewPath(j); path != "" {
		if b, err := json.MarshalIndent(out, "", "  "); err == nil {
			_ = os.WriteFile(path, append(b, '\n'), 0o644)
		}
	}
	return out
}

// PreviewPostHooks returns cached previews when fresh enough, otherwise
// triggers a refresh. Always safe to call from the UI thread; refresh
// runs synchronously bounded by per-hook timeout.
func PreviewPostHooks(j Job) []HookPreview {
	if cached, ok := LoadPostHookPreviews(j); ok && len(cached) == len(j.Hooks.PostShell) {
		fresh := true
		for _, p := range cached {
			if time.Since(p.GeneratedAt) > hookPreviewMaxAge {
				fresh = false
				break
			}
		}
		if fresh {
			return cached
		}
	}
	return RefreshPostHookPreview(j)
}

// runHookPreview dry-runs a single hook. Refuses to run hooks whose
// effective command contains shell redirections or known mutating verbs
// unless PreviewSafe is true. Output is truncated to keep the cache and
// review pane sane.
func runHookPreview(h ShellHook, fallbackCwd string) HookPreview {
	preview := HookPreview{
		Name:        strings.TrimSpace(h.Name),
		GeneratedAt: time.Now(),
	}
	cmd := strings.TrimSpace(h.PreviewCmd)
	if cmd == "" {
		cmd = strings.TrimSpace(h.Cmd)
	}
	preview.Cmd = cmd
	if preview.Name == "" {
		preview.Name = cmd
	}
	if cmd == "" {
		preview.Status = HookPreviewSkipped
		preview.SkipReason = "no cmd"
		return preview
	}
	if !h.PreviewSafe {
		if reason := mutatingHookReason(cmd); reason != "" {
			preview.Status = HookPreviewSkipped
			preview.SkipReason = reason
			return preview
		}
	}

	timeout := h.Timeout
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cwd := h.Cwd
	if cwd == "" {
		cwd = fallbackCwd
	}
	cwd = ExpandHome(cwd)

	start := time.Now()
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = cwd
	c.Env = append(os.Environ(), "GIT_PAGER=cat", "PAGER=cat", "NO_COLOR=1", "SB_HOOK_PREVIEW=1")
	out, err := c.CombinedOutput()
	preview.DurationMS = time.Since(start).Milliseconds()
	preview.Output = truncateHookOutput(string(out))

	if ctx.Err() == context.DeadlineExceeded {
		preview.Status = HookPreviewError
		preview.SkipReason = "timed out"
		preview.ExitCode = -1
		return preview
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			preview.ExitCode = ee.ExitCode()
			preview.Status = HookPreviewWouldFail
			return preview
		}
		preview.Status = HookPreviewError
		preview.SkipReason = err.Error()
		preview.ExitCode = -1
		return preview
	}
	preview.Status = HookPreviewOK
	return preview
}

// mutatingHookReason returns a non-empty reason if the cmd contains shell
// redirection or a known mutating verb. Keeps the dry-run safe-by-default.
// Heuristic, not authoritative — users can opt out per-hook with
// PreviewSafe: true (or supply a side-effect-free PreviewCmd).
func mutatingHookReason(cmd string) string {
	lower := strings.ToLower(cmd)
	for _, sym := range []string{">>", ">", "|tee", "| tee"} {
		if strings.Contains(lower, sym) {
			return "redirection — preview disabled"
		}
	}
	mutators := []string{
		"git commit", "git push", "git reset", "git checkout",
		"git rebase", "git merge", "git apply", "git stash",
		"git clean", "git rm", "git mv", "git tag",
		"npm publish", "npm install", "npm uninstall",
		"yarn add", "yarn remove", "yarn install", "pnpm add", "pnpm install",
		"go install", "cargo install", "pip install", "pip uninstall",
		"docker push", "docker pull", "docker rm", "docker rmi",
		"rm ", "rm\t", "rm -", "mv ", "cp -",
		"sed -i", "perl -i",
	}
	for _, verb := range mutators {
		if strings.Contains(lower, verb) {
			return "mutating cmd — preview disabled"
		}
	}
	return ""
}

func truncateHookOutput(s string) string {
	const maxBytes = 4096
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "\n…(truncated)"
}

func CaptureReviewArtifact(j Job) error {
	path := ReviewArtifactPath(j)
	if path == "" {
		return nil
	}
	artifact := ReviewArtifact{
		GeneratedAt:      time.Now(),
		Status:           j.Status,
		HookEvents:       readHookEventSummaries(j.EventLogPath),
		PendingPostHooks: pendingPostHookLabels(j.Hooks.PostShell),
	}
	if repo := strings.TrimSpace(j.Repo); repo != "" {
		current := gitStatusSnapshot(repo)
		artifact.ChangedFiles = diffStatusSinceLaunch(j.RepoStatusAtLaunch, current)
		artifact.PreexistingDirty = intersectStatusSnapshots(j.RepoStatusAtLaunch, current)
		if len(artifact.ChangedFiles) == 0 {
			artifact.ChangedFiles = current
		}
		artifact.DiffStat = gitDiffStat(repo)
	}
	b, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func LoadReviewArtifact(j Job) (ReviewArtifact, bool) {
	path := ReviewArtifactPath(j)
	if path == "" {
		return ReviewArtifact{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ReviewArtifact{}, false
	}
	var out ReviewArtifact
	if err := json.Unmarshal(b, &out); err != nil {
		return ReviewArtifact{}, false
	}
	return out, true
}

func pendingPostHookLabels(hooks []ShellHook) []string {
	out := make([]string, 0, len(hooks))
	for _, h := range hooks {
		label := strings.TrimSpace(h.Name)
		if label == "" {
			label = strings.TrimSpace(h.Cmd)
		}
		if label == "" {
			continue
		}
		out = append(out, label)
	}
	return out
}

func gitShortStatus(repo string) []string {
	return runGitLines(repo, "status", "--short", "--untracked-files=all")
}

func gitStatusSnapshot(repo string) []string {
	return gitShortStatus(repo)
}

func gitDiffStat(repo string) []string {
	return runGitLines(repo, "diff", "--stat", "--no-ext-diff", "--submodule=short")
}

func runGitLines(repo string, args ...string) []string {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func readHookEventSummaries(path string) []HookEventSummary {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []HookEventSummary
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var ev Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Kind != EventHookStarted && ev.Kind != EventHookFinished {
			continue
		}
		payload, ok := ev.Payload.(map[string]any)
		if !ok {
			continue
		}
		summary := HookEventSummary{
			TS:     ev.TS,
			Phase:  stringValue(payload["phase"]),
			Name:   stringValue(payload["name"]),
			Cmd:    stringValue(payload["cmd"]),
			Output: strings.TrimSpace(stringValue(payload["output"])),
		}
		summary.ExitCode = intValue(payload["exit"])
		summary.DurationMS = int64(intValue(payload["duration_ms"]))
		out = append(out, summary)
	}
	return out
}

func ensureSyncBackTargetsClean(j Job, devlogPath string) error {
	repo := strings.TrimSpace(j.Repo)
	if repo == "" {
		return nil
	}
	status := statusMapByPath(gitStatusSnapshot(repo))
	if len(status) == 0 {
		return nil
	}
	targets := map[string]string{}
	for _, src := range j.Sources {
		if path := strings.TrimSpace(src.File); path != "" {
			targets[path] = "task source"
		}
	}
	if path := strings.TrimSpace(devlogPath); path != "" {
		targets[path] = "devlog target"
	}
	if len(targets) == 0 {
		return nil
	}
	var conflicts []string
	for path, kind := range targets {
		rel := repoRelativePath(repo, path)
		if rel == "" {
			rel = filepath.Base(path)
		}
		if state, ok := status[rel]; ok {
			conflicts = append(conflicts, fmt.Sprintf("%s (%s %s)", rel, kind, state))
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	if len(conflicts) > 3 {
		conflicts = append(conflicts[:3], fmt.Sprintf("+%d more", len(conflicts)-3))
	}
	return fmt.Errorf("sync-back refused: target files are already dirty: %s", strings.Join(conflicts, ", "))
}

func diffStatusSinceLaunch(before, after []string) []string {
	beforeSet := make(map[string]struct{}, len(before))
	for _, line := range before {
		beforeSet[line] = struct{}{}
	}
	var out []string
	for _, line := range after {
		if _, ok := beforeSet[line]; ok {
			continue
		}
		out = append(out, line)
	}
	return out
}

func intersectStatusSnapshots(before, after []string) []string {
	beforeSet := make(map[string]struct{}, len(before))
	for _, line := range before {
		beforeSet[line] = struct{}{}
	}
	var out []string
	for _, line := range after {
		if _, ok := beforeSet[line]; ok {
			out = append(out, line)
		}
	}
	return out
}

func statusMapByPath(lines []string) map[string]string {
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		out[path] = strings.TrimSpace(line[:3])
	}
	return out
}

func repoRelativePath(repo, path string) string {
	repo = filepath.Clean(repo)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	default:
		return 0
	}
}
