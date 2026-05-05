package workmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/LFroesch/sb/internal/config"
)

type AuditIssue struct {
	Path   string
	Issues []string
}

// AuditDiscoveredFiles checks canonical task-source files discovered from scan
// roots + file patterns against sb's canonical task-file schema.
// Supplemental discovery sources such as idea_dirs and explicit_paths stay
// available to the UI/router, but are not forced through WORK.md-style audit.
func AuditDiscoveredFiles(cfg *config.Config) ([]AuditIssue, error) {
	if cfg == nil {
		cfg = config.Load()
	}

	projects := DiscoverCandidates(
		cfg.ExpandedScanRoots(),
		cfg.FilePatterns,
		nil,
		nil,
		cfg,
	)

	out := make([]AuditIssue, 0)
	for _, project := range projects {
		issues, err := auditFile(project.Path)
		if err != nil {
			return out, err
		}
		if len(issues) == 0 {
			continue
		}
		out = append(out, AuditIssue{
			Path:   project.Path,
			Issues: issues,
		})
	}
	return out, nil
}

func auditFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	content := string(data)
	var issues []string

	meta := extractProjectMetadata(content)
	if meta.Title == "" {
		issues = append(issues, "missing typed H1 like `# WORK - <name>`")
	}
	if meta.Description == "" {
		issues = append(issues, "missing one-line summary below the H1")
	}

	headings := secondLevelHeadings(content)
	want := []string{"Current Phase", "Current Tasks", "Backlog / Future Features"}
	if len(headings) != len(want) {
		issues = append(issues, fmt.Sprintf("expected exactly 3 `##` sections (%s); found %d", strings.Join(want, ", "), len(headings)))
	} else {
		for i := range want {
			if headings[i] != want[i] {
				issues = append(issues, fmt.Sprintf("section %d should be `## %s`, found `## %s`", i+1, want[i], headings[i]))
			}
		}
	}

	if phaseIssue := auditPhaseSection(content); phaseIssue != "" {
		issues = append(issues, phaseIssue)
	}

	return issues, nil
}

func secondLevelHeadings(content string) []string {
	var headings []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			headings = append(headings, strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
		}
	}
	return headings
}

func auditPhaseSection(content string) string {
	lines := strings.Split(content, "\n")
	inPhase := false
	var body []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if heading == "Current Phase" {
				inPhase = true
				body = nil
				continue
			}
			if inPhase {
				break
			}
		}
		if !inPhase || trimmed == "" {
			continue
		}
		body = append(body, trimmed)
	}

	if len(body) == 0 {
		return "Current Phase should contain one plain-text line"
	}
	if len(body) > 1 {
		return "Current Phase should contain only one plain-text line"
	}
	if strings.HasPrefix(body[0], "- ") || strings.HasPrefix(body[0], "* ") {
		return "Current Phase should be plain text, not a list item"
	}
	return ""
}
