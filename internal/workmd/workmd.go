package workmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/LFroesch/sb/internal/config"
)

// fileKey returns a device:inode string that uniquely identifies a file,
// catching both symlinks and hard links pointing to the same data.
func fileKey(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return path // fallback to path if stat fails
	}
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return path
	}
	return fmt.Sprintf("%d:%d", sys.Dev, sys.Ino)
}

type Task struct {
	Name    string
	Status  string // raw status/priority text from table
	Section string // "current", "inbox", "backlog"
	Done    bool
}

type Project struct {
	Name          string
	Description   string // optional one-line description from "# WORK - name | description" title
	Path          string // absolute path to WORK.md
	Dir           string // directory containing WORK.md
	RootName      string
	RelativePath  string
	Content       string
	Phase         string
	Tasks         []Task
	TaskCount     int // non-done tasks
	CurrentCount  int
	BugsCount     int
	UnsortedCount int
	BacklogCount  int
	NonListCount  int // plain-text lines in sections (not list items)
	ModTime       time.Time
}

type discoverRoot struct {
	Name string
	Path string
	Flat bool
}

type nameCandidate struct {
	base     string
	explicit bool
}

// Discover finds markdown files under scanDirs matching filePatterns, plus
// all .md files in ideaDirs (flat, non-recursive).
// Pass nil/empty slices to use the legacy defaults.
func Discover(scanRoots []config.ScanRoot, filePatterns, ideaDirs []string, cfg *config.Config) []Project {
	home, _ := os.UserHomeDir()
	if len(scanRoots) == 0 {
		scanRoots = []config.ScanRoot{{Name: "projects", Path: filepath.Join(home, "projects")}}
	}
	if len(filePatterns) == 0 {
		filePatterns = []string{"WORK.md"}
	}

	// seenEntry tracks which projects slice index holds a given resolved path,
	// plus its path depth. When a shallower path for the same file is found,
	// we replace the existing entry.
	type seenEntry struct {
		idx   int
		depth int
	}
	seen := map[string]seenEntry{} // keyed by resolved (symlink-free) path
	var projects []Project

	addOrReplace := func(path string, root discoverRoot) {
		resolved := fileKey(path)
		depth := strings.Count(path, string(filepath.Separator))
		if entry, exists := seen[resolved]; exists {
			if depth < entry.depth {
				// Shallower path wins — replace the existing project in-place
				if p, ok := loadProject(path, root, cfg); ok {
					projects[entry.idx] = p
					seen[resolved] = seenEntry{idx: entry.idx, depth: depth}
				}
			}
			return
		}
		if p, ok := loadProject(path, root, cfg); ok {
			seen[resolved] = seenEntry{idx: len(projects), depth: depth}
			projects = append(projects, p)
		}
	}

	for _, root := range scanRoots {
		dr := discoverRoot{Name: root.Name, Path: root.Path}
		for _, pattern := range filePatterns {
			args := []string{root.Path,
				"-name", pattern,
				"-not", "-path", "*/node_modules/*",
				"-not", "-path", "*/.git/*",
				"-not", "-path", "*/vendor/*",
			}
			cmd := exec.Command("find", args...)
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line != "" {
					addOrReplace(line, dr)
				}
			}
		}
	}

	// Flat idea dirs — load all .md files directly (non-recursive)
	for _, dir := range ideaDirs {
		discoverFlatDir(dir, discoverRoot{
			Name: filepath.Base(strings.TrimRight(dir, string(filepath.Separator))),
			Path: dir,
			Flat: true,
		}, addOrReplace)
	}

	resolveProjectNames(projects, cfg)

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ModTime.After(projects[j].ModTime)
	})

	return projects
}

// loadProject reads a markdown file at path and builds a Project.
// root is used only for deriving the display name.
func loadProject(path string, root discoverRoot, cfg *config.Config) (Project, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Project{}, false
	}
	text := string(content)
	meta := extractTitleMetadata(text)
	rel := relativeProjectPath(path, root)
	tasks := extractTasks(text)

	var cur, bugs, unsorted, backlog int
	for _, t := range tasks {
		if t.Done {
			continue
		}
		switch t.Section {
		case "current":
			cur++
		case "bugs":
			bugs++
		case "unsorted":
			unsorted++
		case "backlog":
			backlog++
		}
	}

	var modTime time.Time
	if info, err := os.Stat(path); err == nil {
		modTime = info.ModTime()
	}

	return Project{
		Name:          meta.Label,
		Description:   meta.Description,
		Path:          path,
		Dir:           filepath.Dir(path),
		RootName:      root.Name,
		RelativePath:  rel,
		Content:       text,
		Tasks:         tasks,
		TaskCount:     cur + bugs + unsorted + backlog,
		CurrentCount:  cur,
		BugsCount:     bugs,
		UnsortedCount: unsorted,
		BacklogCount:  backlog,
		NonListCount:  countNonListLines(text),
		ModTime:       modTime,
	}, true
}

type titleMetadata struct {
	Label       string
	Description string
}

// extractTitleMetadata parses a title line like:
// "# WORK - sb | Second Brain TUI"
// "# ROADMAP - toolkit | v1 polish"
// "# WORK - sb"
func extractTitleMetadata(content string) titleMetadata {
	for _, line := range strings.SplitN(content, "\n", 5) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
		beforePipe, desc, hasDesc := strings.Cut(title, "|")
		label := ""
		if _, afterDash, ok := strings.Cut(strings.TrimSpace(beforePipe), " - "); ok {
			label = strings.TrimSpace(afterDash)
		}
		meta := titleMetadata{Label: label}
		if hasDesc {
			meta.Description = strings.TrimSpace(desc)
		}
		return meta
	}
	return titleMetadata{}
}

// discoverFlatDir loads all .md files from dir (non-recursive), deduplicating
// against already-found projects via addOrReplace.
func discoverFlatDir(dir string, root discoverRoot, addOrReplace func(path string, root discoverRoot)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		addOrReplace(filepath.Join(dir, e.Name()), root)
	}
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
			// Skip blank lines right after header to find first content line
			insert := i + 1
			for insert < len(lines) && strings.TrimSpace(lines[insert]) == "" {
				insert++
			}
			// Always emit: header, blank line, new entry, then remaining content
			result := make([]string, 0, len(lines)+2)
			result = append(result, lines[:i+1]...)
			result = append(result, "")
			result = append(result, entry)
			result = append(result, lines[insert:]...)
			return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0644)
		}
	}

	// Section not found — append it
	addition := "\n" + target + "\n\n" + entry + "\n"
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
	case "current tasks", "current":
		return "## Current Tasks"
	case "bugs blockers", "bugs", "blockers":
		return "## Bugs + Blockers"
	case "updates features", "updates", "features":
		return "## Updates + Features"
	case "backlog":
		return "## Backlog"
	case "unsorted", "inbox":
		return "## Unsorted"
	default:
		if section == "" {
			return "## Current Tasks"
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

func relativeProjectPath(path string, root discoverRoot) string {
	rel, err := filepath.Rel(root.Path, path)
	if err != nil {
		return fallbackStem(path)
	}
	return relativeStem(rel)
}

func relativeStem(rel string) string {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "/WORK.md") || rel == "WORK.md" {
		rel = strings.TrimSuffix(rel, "/WORK.md")
		rel = strings.TrimSuffix(rel, "WORK.md")
	} else if ext := filepath.Ext(rel); ext != "" {
		rel = strings.TrimSuffix(rel, ext)
	}
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return ""
	}
	return rel
}

func fallbackStem(path string) string {
	path = filepath.ToSlash(path)
	if strings.HasSuffix(path, "/WORK.md") || path == "WORK.md" {
		return filepath.Base(filepath.Dir(path))
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func resolveProjectNames(projects []Project, cfg *config.Config) {
	maxDepth := 2
	if cfg != nil && cfg.LabelMaxDepth > 0 {
		maxDepth = cfg.LabelMaxDepth
	}

	candidates := make([]nameCandidate, len(projects))
	groups := make(map[string][]int)
	for i := range projects {
		if projects[i].Name != "" {
			candidates[i] = nameCandidate{
				base:     projects[i].Name,
				explicit: true,
			}
		} else {
			fallback := fixedSuffix(relativeParts(projects[i].RelativePath), maxDepth)
			if fallback == "" {
				fallback = projects[i].RootName
			}
			candidates[i] = nameCandidate{
				base: fallback,
			}
			projects[i].Name = fallback
		}
		groups[candidates[i].base] = append(groups[candidates[i].base], i)
	}

	for _, idxs := range groups {
		if len(idxs) == 1 {
			if candidates[idxs[0]].explicit {
				projects[idxs[0]].Name = candidates[idxs[0]].base
			}
			continue
		}
		resolveCollisionGroup(projects, candidates, idxs, maxDepth)
	}
}

func resolveCollisionGroup(projects []Project, candidates []nameCandidate, idxs []int, maxDepth int) {
	suffixes := make(map[int]string, len(idxs))
	for _, idx := range idxs {
		minDepth := 1
		if !candidates[idx].explicit {
			minDepth = maxDepth
		}
		suffixes[idx] = shortestUniqueSuffix(projects, idxs, idx, minDepth)
		if suffixes[idx] == "" {
			suffixes[idx] = projects[idx].RootName
		}
	}

	nameCounts := make(map[string]int)
	for _, idx := range idxs {
		name := candidates[idx].base
		if candidates[idx].explicit {
			name = decorateExplicitName(candidates[idx].base, suffixes[idx])
		} else {
			name = suffixes[idx]
		}
		projects[idx].Name = name
		nameCounts[name]++
	}

	for _, idx := range idxs {
		if nameCounts[projects[idx].Name] == 1 {
			continue
		}
		rootPrefix := projects[idx].RootName
		if rootPrefix == "" {
			rootPrefix = "root"
		}
		if candidates[idx].explicit {
			extra := rootPrefix
			if suffixes[idx] != "" {
				extra = rootPrefix + "/" + suffixes[idx]
			}
			projects[idx].Name = decorateExplicitName(candidates[idx].base, extra)
			continue
		}
		if suffixes[idx] == "" {
			projects[idx].Name = rootPrefix
			continue
		}
		projects[idx].Name = rootPrefix + "/" + suffixes[idx]
	}
}

func decorateExplicitName(base, suffix string) string {
	if suffix == "" {
		return base
	}
	return fmt.Sprintf("%s (%s)", base, suffix)
}

func relativeParts(rel string) []string {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return nil
	}
	return strings.Split(rel, "/")
}

func fixedSuffix(parts []string, minDepth int) string {
	if len(parts) == 0 {
		return ""
	}
	if minDepth < 1 {
		minDepth = 1
	}
	if minDepth > len(parts) {
		minDepth = len(parts)
	}
	return strings.Join(parts[len(parts)-minDepth:], "/")
}

func shortestUniqueSuffix(projects []Project, idxs []int, targetIdx int, minDepth int) string {
	parts := relativeParts(projects[targetIdx].RelativePath)
	if len(parts) == 0 {
		return ""
	}
	if minDepth < 1 {
		minDepth = 1
	}
	if minDepth > len(parts) {
		minDepth = len(parts)
	}
	for depth := minDepth; depth <= len(parts); depth++ {
		candidate := fixedSuffix(parts, depth)
		unique := true
		for _, otherIdx := range idxs {
			if otherIdx == targetIdx {
				continue
			}
			otherParts := relativeParts(projects[otherIdx].RelativePath)
			if fixedSuffix(otherParts, depth) == candidate {
				unique = false
				break
			}
		}
		if unique {
			return candidate
		}
	}
	return strings.Join(parts, "/")
}

// sectionType maps a heading string to a canonical section name.
// Returns "" for sections we don't care about.
func sectionType(heading string) string {
	h := strings.ToLower(heading)
	switch {
	case strings.Contains(h, "current task") || h == "tasks" || h == "todo":
		return "current"
	case strings.Contains(h, "bugs") || strings.Contains(h, "blockers"):
		return "bugs"
	case strings.Contains(h, "inbox") || strings.Contains(h, "unsorted"):
		return "unsorted"
	case strings.Contains(h, "backlog") || strings.Contains(h, "feature") ||
		strings.Contains(h, "ideas") || strings.Contains(h, "someday") ||
		strings.Contains(h, "polish") ||
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

// skipPlaceholder returns true for common "None." placeholder lines.
func skipPlaceholder(s string) bool {
	switch strings.ToLower(strings.TrimRight(s, ".")) {
	case "none", "none identified", "none noted", "none known", "n/a", "tbd":
		return true
	}
	return false
}

// isContentLine returns true for lines that are not list/table/heading/blank.
func isContentLine(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "- ") ||
		strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "> ") ||
		strings.HasPrefix(trimmed, "|") ||
		strings.HasPrefix(trimmed, "**") {
		return false
	}
	return !skipPlaceholder(trimmed)
}

// countNonListLines returns the number of plain-text lines inside sections
// that are not list items (these should ideally be converted to list items).
func countNonListLines(content string) int {
	lines := strings.Split(content, "\n")
	count := 0
	inSection := false
	inCode := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") ||
			strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if isContentLine(trimmed) {
			count++
		}
	}
	return count
}

// FixNonListLines converts plain-text lines within sections to "- " list items.
func FixNonListLines(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inSection := false
	inCode := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			out = append(out, line)
			continue
		}
		if inCode {
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") ||
			strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
			inSection = true
			out = append(out, line)
			continue
		}
		if inSection && isContentLine(trimmed) {
			out = append(out, "- "+trimmed)
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
