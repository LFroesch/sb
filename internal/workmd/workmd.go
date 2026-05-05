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
	Section string // "current" or "backlog"
	Done    bool
}

type Project struct {
	Name          string
	Description   string // one-line summary from preamble or legacy H1 metadata
	Path          string // absolute path to WORK.md
	Dir           string // directory containing WORK.md
	RootName      string
	RelativePath  string
	Content       string
	Title         string
	FileName      string
	Phase         string
	ActivePreview []string
	Tasks         []Task
	TaskCount     int // non-done tasks
	CurrentCount  int
	BacklogCount  int
	NonListCount  int // plain-text lines in sections (not list items)
	ModTime       time.Time
	Hydrated      bool
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

// Discover finds markdown files under scan roots matching filePatterns, plus
// explicit task-file paths, plus all .md files in ideaDirs (flat, non-recursive).
// It fully hydrates every discovered file.
func Discover(scanRoots []config.ScanRoot, filePatterns, explicitPaths, ideaDirs []string, cfg *config.Config) []Project {
	projects := DiscoverCandidates(scanRoots, filePatterns, explicitPaths, ideaDirs, cfg)
	for i := range projects {
		hydrated, ok := HydrateProject(projects[i], cfg)
		if !ok {
			continue
		}
		projects[i] = hydrated
	}
	resolveProjectNames(projects, cfg)
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ModTime.After(projects[j].ModTime)
	})
	return projects
}

// DiscoverCandidates returns lightweight project entries quickly so the TUI can
// render before full file hydration completes.
func DiscoverCandidates(scanRoots []config.ScanRoot, filePatterns, explicitPaths, ideaDirs []string, cfg *config.Config) []Project {
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
				if p, ok := candidateProject(path, root); ok {
					projects[entry.idx] = p
					seen[resolved] = seenEntry{idx: entry.idx, depth: depth}
				}
			}
			return
		}
		if p, ok := candidateProject(path, root); ok {
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
				if line == "" {
					continue
				}
				if cfg.IsScanPathBlocked(line) {
					continue
				}
				addOrReplace(line, dr)
			}
		}
	}

	for _, path := range explicitPaths {
		path = strings.TrimSpace(path)
		if path == "" || cfg.IsScanPathBlocked(path) {
			continue
		}
		addOrReplace(path, discoverRoot{
			Name: filepath.Base(filepath.Dir(path)),
			Path: filepath.Dir(path),
			Flat: true,
		})
	}

	// Flat idea dirs — load all .md files directly (non-recursive)
	for _, dir := range ideaDirs {
		discoverFlatDir(dir, discoverRoot{
			Name: filepath.Base(strings.TrimRight(dir, string(filepath.Separator))),
			Path: dir,
			Flat: true,
		}, func(path string, root discoverRoot) {
			if cfg.IsScanPathBlocked(path) {
				return
			}
			addOrReplace(path, root)
		})
	}

	resolveProjectNames(projects, cfg)
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ModTime.After(projects[j].ModTime)
	})
	return projects
}

func candidateProject(path string, root discoverRoot) (Project, bool) {
	rel := relativeProjectPath(path, root)
	fallback := fallbackProjectName(rel, root.Name)
	var modTime time.Time
	if info, err := os.Stat(path); err == nil {
		modTime = info.ModTime()
	}
	return Project{
		Name:         fallback,
		Path:         path,
		Dir:          filepath.Dir(path),
		RootName:     root.Name,
		RelativePath: rel,
		FileName:     filepath.Base(path),
		ModTime:      modTime,
	}, true
}

// HydrateProject reads a markdown file and fills in metadata, content, and
// counts while preserving the candidate discovery identity fields.
func HydrateProject(project Project, cfg *config.Config) (Project, bool) {
	if strings.TrimSpace(project.Path) == "" {
		return Project{}, false
	}
	_ = cfg
	content, err := os.ReadFile(project.Path)
	if err != nil {
		return Project{}, false
	}
	text := string(content)
	meta := extractProjectMetadata(text)
	tasks := extractTasks(text)
	phase := extractPhase(text)
	activePreview := activeTaskPreview(tasks, 2)

	var cur, backlog int
	for _, t := range tasks {
		if t.Done {
			continue
		}
		switch t.Section {
		case "current":
			cur++
		case "backlog":
			backlog++
		}
	}

	project.Name = meta.Label
	project.Description = meta.Description
	project.Content = text
	project.Title = meta.Title
	project.Phase = phase
	project.ActivePreview = activePreview
	project.Tasks = tasks
	project.TaskCount = cur + backlog
	project.CurrentCount = cur
	project.BacklogCount = backlog
	project.NonListCount = countNonListLines(text)
	project.Hydrated = true
	return project, true
}

type titleMetadata struct {
	Title       string
	Label       string
	Description string
}

// extractProjectMetadata expects a typed H1 such as "# WORK - sb" followed by a
// short plain-text summary line below it.
func extractProjectMetadata(content string) titleMetadata {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			if !strings.Contains(title, " - ") {
				return titleMetadata{}
			}
			beforeDash, afterDash, ok := strings.Cut(title, " - ")
			if !ok || strings.TrimSpace(beforeDash) == "" || strings.TrimSpace(afterDash) == "" {
				return titleMetadata{}
			}
			return titleMetadata{
				Title:       title,
				Label:       strings.TrimSpace(afterDash),
				Description: extractPreambleSummary(lines[i+1:]),
			}
		}
	}
	return titleMetadata{}
}

func extractPreambleSummary(lines []string) string {
	inCode := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode || trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			break
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "> ") || strings.HasPrefix(trimmed, "|") {
			continue
		}
		return trimmed
	}
	return ""
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

// AppendToSection appends text to a named canonical task section.
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
	case "", "current", "current tasks":
		return "## Current Tasks"
	case "backlog":
		return "## Backlog / Future Features"
	default:
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

func (p Project) RelativePathOrFile() string {
	if strings.TrimSpace(p.RelativePath) != "" {
		return p.RelativePath
	}
	if strings.TrimSpace(p.FileName) != "" {
		return p.FileName
	}
	return filepath.Base(p.Path)
}

func extractPhase(content string) string {
	lines := strings.Split(content, "\n")
	inPhase := false
	var parts []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), "Current Phase") {
				inPhase = true
				parts = nil
				continue
			}
			if inPhase {
				break
			}
		}
		if !inPhase {
			continue
		}
		if trimmed == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			trimmed = strings.TrimSpace(trimmed[2:])
		}
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
		if len(parts) >= 1 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func activeTaskPreview(tasks []Task, limit int) []string {
	if limit <= 0 {
		return nil
	}
	sections := []string{"current", "backlog"}
	var out []string
	for _, section := range sections {
		for _, task := range tasks {
			if task.Done || task.Section != section {
				continue
			}
			name := strings.TrimSpace(task.Name)
			if name == "" {
				continue
			}
			out = append(out, name)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
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
		if projects[i].Title != "" {
			candidates[i] = nameCandidate{
				base:     projects[i].Name,
				explicit: true,
			}
		} else {
			fallback := fallbackProjectName(projects[i].RelativePath, projects[i].RootName)
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

// ResolveProjectNamesForTUI exposes the shared collision-aware label resolver to
// the TUI's staged hydration path without duplicating naming logic there.
func ResolveProjectNamesForTUI(projects []Project, cfg *config.Config) {
	resolveProjectNames(projects, cfg)
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
		var name string
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

func fallbackProjectName(rel, rootName string) string {
	fallback := fixedSuffix(relativeParts(rel), 2)
	if fallback == "" {
		fallback = rootName
	}
	return fallback
}

// sectionType maps a heading string to a canonical section name.
// Returns "" for sections we don't care about.
func sectionType(heading string) string {
	h := strings.ToLower(heading)
	switch {
	case strings.Contains(h, "current task") || h == "tasks" || h == "todo":
		return "current"
	case strings.Contains(h, "backlog") || strings.Contains(h, "future feature"):
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
