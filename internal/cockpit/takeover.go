package cockpit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	pathTokenPattern     = regexp.MustCompile(`(?:[A-Za-z0-9._-]+/)+[A-Za-z0-9._-]+`)
	commandLinePattern   = regexp.MustCompile(`(?m)^\$ ([^\n]+)$`)
	takeoverTailMaxLines = 16
)

func validateTakeOverJob(j Job) error {
	if !j.ForemanManaged {
		return fmt.Errorf("job %s is not Foreman-managed", j.ID)
	}
	if j.Runner != RunnerTmux {
		return fmt.Errorf("job %s is not tmux-backed", j.ID)
	}
	if j.TmuxTarget == "" {
		return fmt.Errorf("job %s has no live tmux target", j.ID)
	}
	switch j.Status {
	case StatusRunning, StatusIdle, StatusAwaitingHuman:
	default:
		return fmt.Errorf("job %s is %s, not eligible for takeover", j.ID, j.Status)
	}
	alive, err := WindowAlive(j.TmuxTarget)
	if err != nil {
		return err
	}
	if !alive {
		return fmt.Errorf("job %s pane is already closed", j.ID)
	}
	return nil
}

func BuildTakeoverPrompt(j Job) (string, error) {
	artifact, _ := LoadReviewArtifact(j)
	if artifact.GeneratedAt.IsZero() {
		_ = CaptureReviewArtifact(j)
		artifact, _ = LoadReviewArtifact(j)
	}
	sessionText := tmuxOrTranscriptText(j)
	files := takeoverObservedFiles(j, artifact, sessionText)
	commands := takeoverObservedCommands(j, sessionText)
	tail := takeoverRecentTail(sessionText)

	var sb strings.Builder
	sb.WriteString("## Manual Takeover Handoff\n\n")
	sb.WriteString("This run was originally launched unattended through Foreman and is now being manually taken over.\n\n")
	sb.WriteString("### Original task\n\n")
	if task := strings.TrimSpace(j.Task); task != "" {
		sb.WriteString(task)
		sb.WriteString("\n\n")
	} else if prompt := strings.TrimSpace(j.Brief); prompt != "" {
		sb.WriteString(prompt)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("(no task summary recorded)\n\n")
	}
	sb.WriteString("### Takeover reason\n\n")
	sb.WriteString("Continue this job manually under attended mode. Use the observed context below as reference only; verify before relying on it.\n\n")
	writeBulletSection(&sb, "Observed files", files)
	writeBulletSection(&sb, "Observed commands", commands)
	writeBulletSection(&sb, "Repo changes since launch", artifact.ChangedFiles)
	sb.WriteString("### Last known status\n\n")
	statusLine := fmt.Sprintf("- status: %s\n- note: %s\n", j.Status, fallbackText(j.Note, "(none)"))
	sb.WriteString(statusLine)
	sb.WriteString("\n")
	sb.WriteString("### Recent raw tail\n\n")
	if tail == "" {
		sb.WriteString("(no recent session text captured)\n\n")
	} else {
		sb.WriteString("```text\n")
		sb.WriteString(tail)
		if !strings.HasSuffix(tail, "\n") {
			sb.WriteByte('\n')
		}
		sb.WriteString("```\n\n")
	}
	sb.WriteString("### Reference paths\n\n")
	sb.WriteString("- transcript: " + fallbackText(j.TranscriptPath, "(none)") + "\n")
	sb.WriteString("- log: " + fallbackText(j.LogPath, "(none)") + "\n")
	sb.WriteString("- review artifact: " + fallbackText(ReviewArtifactPath(j), "(none)") + "\n")
	return strings.TrimRight(sb.String(), "\n"), nil
}

func writeBulletSection(sb *strings.Builder, title string, items []string) {
	sb.WriteString("### " + title + "\n\n")
	if len(items) == 0 {
		sb.WriteString("(none)\n\n")
		return
	}
	for _, item := range items {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")
}

func takeoverObservedFiles(j Job, artifact ReviewArtifact, text string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, line := range artifact.ChangedFiles {
		add(strings.TrimSpace(line))
	}
	for _, m := range pathTokenPattern.FindAllString(text, -1) {
		add(strings.TrimPrefix(filepath.Clean(m), "./"))
		if len(out) >= 12 {
			break
		}
	}
	sort.Strings(out)
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func takeoverObservedCommands(j Job, text string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, m := range commandLinePattern.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			add(m[1])
		}
		if len(out) >= 8 {
			return out
		}
	}
	if j.EventLogPath != "" {
		if body, err := os.ReadFile(j.EventLogPath); err == nil {
			for _, m := range commandLinePattern.FindAllStringSubmatch(string(body), -1) {
				if len(m) > 1 {
					add(m[1])
				}
				if len(out) >= 8 {
					return out
				}
			}
		}
	}
	return out
}

func takeoverRecentTail(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > takeoverTailMaxLines {
		lines = lines[len(lines)-takeoverTailMaxLines:]
	}
	return strings.Join(lines, "\n")
}

func tmuxOrTranscriptText(j Job) string {
	if j.TmuxTarget != "" {
		if body, err := CapturePane(j.TmuxTarget); err == nil && strings.TrimSpace(body) != "" {
			return body
		}
	}
	if j.LogPath != "" {
		if body, err := os.ReadFile(j.LogPath); err == nil && strings.TrimSpace(string(body)) != "" {
			return string(body)
		}
	}
	if j.TranscriptPath != "" {
		if body, err := os.ReadFile(j.TranscriptPath); err == nil {
			return string(body)
		}
	}
	return ""
}

func fallbackText(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
