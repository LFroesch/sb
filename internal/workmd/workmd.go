package workmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Task struct {
	Name    string
	Status  string // raw status/priority text from table
	Section string // "current", "inbox", "backlog"
	Done    bool
}

type Project struct {
	Name         string
	Path         string // absolute path to WORK.md
	Dir          string // directory containing WORK.md
	Content      string
	Phase        string
	Tasks        []Task
	TaskCount    int // non-done tasks
	CurrentCount int
	InboxCount   int
	BacklogCount int
	ModTime      time.Time
}

// Discover finds all WORK.md files under ~/projects.
func Discover() []Project {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, "projects")

	var projects []Project

	// Use find for speed — skip node_modules, .git, vendor
	cmd := exec.Command("find", root,
		"-name", "WORK.md",
		"-not", "-path", "*/node_modules/*",
		"-not", "-path", "*/.git/*",
		"-not", "-path", "*/vendor/*",
	)
	out, err := cmd.Output()
	if err != nil {
		return projects
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}

		content, err := os.ReadFile(line)
		if err != nil {
			continue
		}

		text := string(content)
		name := deriveName(line, root)
		tasks := extractTasks(text)

		var cur, inbox, backlog int
		for _, t := range tasks {
			if t.Done {
				continue
			}
			switch t.Section {
			case "current":
				cur++
			case "inbox":
				inbox++
			case "backlog":
				backlog++
			}
		}

		var modTime time.Time
		if info, err := os.Stat(line); err == nil {
			modTime = info.ModTime()
		}

		projects = append(projects, Project{
			Name:         name,
			Path:         line,
			Dir:          filepath.Dir(line),
			Content:      text,
			Tasks:        tasks,
			TaskCount:    cur + inbox + backlog,
			CurrentCount: cur,
			InboxCount:   inbox,
			BacklogCount: backlog,
			ModTime:      modTime,
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ModTime.After(projects[j].ModTime)
	})

	return projects
}

// Save writes content to a WORK.md file.
func Save(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// AppendToSection appends text to a named section in a WORK.md file.
// section is matched loosely (e.g. "inbox" → "## Inbox").
// If the section doesn't exist, it's created at the end of the file.
func AppendToSection(path, section, text string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(raw)
	lines := strings.Split(content, "\n")

	target := sectionHeader(section)
	entry := "- " + text

	for i, line := range lines {
		if strings.TrimSpace(line) == target {
			// Find insertion point: skip blank lines right after header
			insert := i + 1
			for insert < len(lines) && strings.TrimSpace(lines[insert]) == "" {
				insert++
			}
			result := make([]string, 0, len(lines)+1)
			result = append(result, lines[:insert]...)
			result = append(result, entry)
			result = append(result, lines[insert:]...)
			return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0644)
		}
	}

	// Section not found — append it
	addition := "\n" + target + "\n" + entry + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(addition)
	return err
}

// sectionHeader maps a loose section name to a ## heading.
func sectionHeader(section string) string {
	switch strings.ToLower(strings.ReplaceAll(section, "_", " ")) {
	case "inbox":
		return "## Inbox"
	case "backlog":
		return "## Backlog"
	case "current tasks":
		return "## Current Tasks"
	default:
		if section == "" {
			return "## Inbox"
		}
		return "## " + strings.ToUpper(section[:1]) + section[1:]
	}
}

// SplitDevlog extracts the DevLog section from content, returns (main, devlog).
func SplitDevlog(content string) (string, string) {
	lines := strings.Split(content, "\n")
	devlogStart := -1

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## DevLog") {
			devlogStart = i
			break
		}
	}

	if devlogStart == -1 {
		return content, ""
	}

	main := strings.Join(lines[:devlogStart], "\n")
	devlog := strings.Join(lines[devlogStart:], "\n")
	return strings.TrimRight(main, "\n") + "\n", devlog
}

// --- helpers ---

func deriveName(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(filepath.Dir(path))
	}
	// Remove /WORK.md suffix
	rel = strings.TrimSuffix(rel, "/WORK.md")

	// Shorten common prefixes
	rel = strings.TrimPrefix(rel, "active/daily_use/")
	rel = strings.TrimPrefix(rel, "tui-hub/apps/")

	// Use last component if it's still long
	parts := strings.Split(rel, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return rel
}

// sectionType maps a heading string to a canonical section name.
// Returns "" for sections we don't care about.
func sectionType(heading string) string {
	h := strings.ToLower(heading)
	switch {
	case strings.Contains(h, "current task") || h == "tasks" || h == "todo":
		return "current"
	case strings.Contains(h, "inbox"):
		return "inbox"
	case strings.Contains(h, "backlog") || strings.Contains(h, "feature") ||
		strings.Contains(h, "ideas") || strings.Contains(h, "someday") ||
		strings.Contains(h, "bugs") || strings.Contains(h, "polish") ||
		strings.Contains(h, "p1") || strings.Contains(h, "p2") ||
		strings.Contains(h, "high impact") || strings.Contains(h, "maybe"):
		return "backlog"
	default:
		return ""
	}
}

// tableColIndex finds the 0-based index of a column by name in a split table row.
// cols should already be trimmed (no leading/trailing |).
func tableColIndex(cols []string, names ...string) int {
	for i, c := range cols {
		lower := strings.ToLower(strings.TrimSpace(c))
		for _, n := range names {
			if lower == n {
				return i
			}
		}
	}
	return -1
}

// splitTableRow splits a pipe-delimited table row into trimmed column values,
// excluding the empty strings from leading/trailing pipes.
func splitTableRow(row string) []string {
	parts := strings.Split(row, "|")
	// drop first and last (empty from leading/trailing |)
	if len(parts) < 2 {
		return nil
	}
	cols := parts[1 : len(parts)-1]
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

func extractTasks(content string) []Task {
	lines := strings.Split(content, "\n")
	var tasks []Task

	// topSection is set by h1/h2 headings only (##/# level).
	// subSection is set by h3+ — inherits topSection for task matching.
	topSection := ""
	taskColIdx := -1
	statusColIdx := -1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Heading detection — h1/h2 set top section, h3+ stay in top section
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
			heading := strings.TrimLeft(trimmed, "# ")
			topSection = sectionType(heading)
			taskColIdx = -1
			statusColIdx = -1
			continue
		}
		if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
			// Sub-section: update sub-type in case it names a known section,
			// but fall back to topSection if unrecognized.
			heading := strings.TrimLeft(trimmed, "# ")
			if st := sectionType(heading); st != "" {
				topSection = st
			}
			taskColIdx = -1
			statusColIdx = -1
			continue
		}

		if topSection == "" {
			continue
		}

		// Table separator — skip, but also reset column detection for next header
		if strings.HasPrefix(trimmed, "|--") || strings.HasPrefix(trimmed, "|-") {
			continue
		}

		// Table rows
		if strings.HasPrefix(trimmed, "|") {
			cols := splitTableRow(trimmed)
			if len(cols) == 0 {
				continue
			}

			// Detect header row → record which columns are Task/Status/Priority
			if tableColIndex(cols, "task") >= 0 {
				taskColIdx = tableColIndex(cols, "task")
				statusColIdx = tableColIndex(cols, "status", "priority")
				continue
			}

			// Data row
			if taskColIdx < 0 || taskColIdx >= len(cols) {
				continue
			}
			name := cols[taskColIdx]
			if name == "" {
				continue
			}
			status := ""
			if statusColIdx >= 0 && statusColIdx < len(cols) {
				status = cols[statusColIdx]
			}
			isDone := strings.Contains(strings.ToLower(status), "done") ||
				strings.Contains(status, "✅")
			tasks = append(tasks, Task{
				Name:    name,
				Status:  status,
				Section: topSection,
				Done:    isDone,
			})
			continue
		}

		// Checkbox items: - [ ] or - [x]
		if strings.HasPrefix(trimmed, "- [") && len(trimmed) > 5 {
			done := trimmed[3] == 'x' || trimmed[3] == 'X'
			name := strings.TrimSpace(trimmed[5:])
			if name == "" {
				continue
			}
			tasks = append(tasks, Task{
				Name:    name,
				Status:  topSection,
				Section: topSection,
				Done:    done,
			})
			continue
		}

		// Plain dash items
		if strings.HasPrefix(trimmed, "- ") {
			name := strings.TrimSpace(trimmed[2:])
			if name == "" || strings.HasPrefix(name, "[") {
				continue
			}
			tasks = append(tasks, Task{
				Name:    name,
				Status:  topSection,
				Section: topSection,
				Done:    false,
			})
		}
	}

	return tasks
}
