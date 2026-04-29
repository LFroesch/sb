package cockpit

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type SyncBackPreview struct {
	Path   string
	Before string
	After  string
}

// ApplySyncBack is the V0 sync-back: for each source task, delete its
// exact line from the file, then append a dated DevLog entry summarising
// the completed work. Idempotent on re-run of the same approved job
// only in the sense that re-deletion is a no-op when the line is gone;
// the devlog entry is appended every time the caller invokes this.
//
// Returns the list of files it modified so the TUI can refresh state.
func ApplySyncBack(job Job, devlogPath string) ([]string, error) {
	previews, err := PreviewSyncBack(job, devlogPath)
	if err != nil {
		return nil, err
	}
	var touched []string
	for _, preview := range previews {
		if err := os.WriteFile(preview.Path, []byte(preview.After), 0o644); err != nil {
			return touched, err
		}
		touched = append(touched, preview.Path)
	}
	return touched, nil
}

func PreviewSyncBack(job Job, devlogPath string) ([]SyncBackPreview, error) {
	if len(job.Sources) == 0 {
		return nil, nil
	}

	// Group deletions by file so we rewrite each file once.
	bySource := map[string][]SourceTask{}
	for _, s := range job.Sources {
		if s.File == "" || s.Line <= 0 {
			continue
		}
		bySource[s.File] = append(bySource[s.File], s)
	}

	var previews []SyncBackPreview
	for file, sources := range bySource {
		raw, err := os.ReadFile(file)
		if err != nil {
			return previews, fmt.Errorf("sync-back %s: %w", file, err)
		}
		next, err := deleteLinesContent(string(raw), file, sources)
		if err != nil {
			return previews, fmt.Errorf("sync-back %s: %w", file, err)
		}
		previews = append(previews, SyncBackPreview{
			Path:   file,
			Before: string(raw),
			After:  next,
		})
	}

	if devlogPath != "" {
		raw, err := os.ReadFile(devlogPath)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return previews, fmt.Errorf("sync-back devlog %s: %w", devlogPath, err)
		}
		next, err := appendDevlogContent(string(raw), exists, job)
		if err != nil {
			return previews, fmt.Errorf("sync-back devlog %s: %w", devlogPath, err)
		}
		previews = append(previews, SyncBackPreview{
			Path:   devlogPath,
			Before: string(raw),
			After:  next,
		})
	}
	return previews, nil
}

// deleteLinesContent removes lines from content whose (1-indexed) line
// number matches one of sources *and* whose content matches the recorded
// Raw text. The content check keeps us safe when the file has been
// edited since the job launched: a mismatch aborts with an error.
func deleteLinesContent(content, path string, sources []SourceTask) (string, error) {
	lines := strings.Split(content, "\n")

	// Build a map of line -> expected text, and sort sources so the
	// error message points at the first divergence.
	want := map[int]string{}
	for _, s := range sources {
		want[s.Line] = "- " + s.Text
	}

	// Verify every target still looks right.
	keys := make([]int, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, ln := range keys {
		if ln-1 >= len(lines) {
			return "", fmt.Errorf("line %d missing in %s (file shrunk since launch)", ln, path)
		}
		got := strings.TrimSpace(lines[ln-1])
		expect := strings.TrimSpace(want[ln])
		if got != expect {
			return "", fmt.Errorf("line %d in %s changed since launch: %q vs %q", ln, path, got, expect)
		}
	}

	// Drop the matched lines, preserving indices by iterating in reverse.
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	for _, ln := range keys {
		lines = append(lines[:ln-1], lines[ln:]...)
	}

	return strings.Join(lines, "\n"), nil
}

// appendDevlogContent inserts a dated entry under `## DevLog`, creating
// the section if the file has none. The entry lists every source line
// that was just removed.
func appendDevlogContent(content string, exists bool, job Job) (string, error) {
	date := time.Now().Format("2006-01-02")
	title := fmt.Sprintf("### %s — Agent: %s", date, job.PresetID)
	var body strings.Builder
	body.WriteString(title)
	body.WriteString("\n")
	for _, s := range job.Sources {
		body.WriteString("- ")
		body.WriteString(s.Text)
		body.WriteString("\n")
	}
	entry := body.String()
	if !strings.HasSuffix(entry, "\n") {
		entry += "\n"
	}
	if !exists {
		return "# DEVLOG\n\n## DevLog\n\n" + entry + "\n", nil
	}
	if idx := strings.Index(content, "## DevLog"); idx >= 0 {
		// Insert right after the `## DevLog` header + its blank line.
		lines := strings.Split(content, "\n")
		out := make([]string, 0, len(lines)+8)
		inserted := false
		for i, l := range lines {
			out = append(out, l)
			if !inserted && strings.TrimSpace(l) == "## DevLog" {
				// Skip any existing blank line so we don't introduce a second.
				j := i + 1
				if j < len(lines) && strings.TrimSpace(lines[j]) == "" {
					out = append(out, lines[j])
				} else {
					out = append(out, "")
				}
				out = append(out, entry)
				// Consume the blank line we appended above if it was from source.
				if j < len(lines) && strings.TrimSpace(lines[j]) == "" {
					lines[j] = "__CONSUMED__"
				}
				inserted = true
			}
		}
		// Strip sentinel lines introduced above.
		final := make([]string, 0, len(out))
		for _, l := range out {
			if l == "__CONSUMED__" {
				continue
			}
			final = append(final, l)
		}
		return strings.Join(final, "\n"), nil
	}

	// No `## DevLog` section yet — append one at the end.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n## DevLog\n\n" + entry + "\n"
	return content, nil
}
