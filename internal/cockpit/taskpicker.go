package cockpit

import (
	"os"
	"strings"
)

// PickerItem is one selectable `- ` bullet discovered in a WORK.md file.
// Line is 1-indexed so it matches editor conventions.
type PickerItem struct {
	Line   int
	Text   string
	Raw    string // original line incl. leading "- "
	Indent int    // count of leading spaces
}

// ParseItems reads content and returns every `- ` bullet inside the
// canonical task sections only. Nested items remain selectable so the
// TUI can show tree structure, but bullets outside `## Current Tasks`
// and `## Backlog / Future Features` are intentionally ignored.
func ParseItems(content string) []PickerItem {
	var out []PickerItem
	inTaskSection := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		heading := strings.TrimSpace(line)
		if strings.HasPrefix(heading, "## ") {
			name := strings.TrimSpace(strings.TrimPrefix(heading, "## "))
			inTaskSection = name == "Current Tasks" || name == "Backlog / Future Features"
			continue
		}
		if strings.HasPrefix(heading, "# ") || strings.HasPrefix(heading, "### ") || strings.HasPrefix(heading, "#### ") {
			if !strings.HasPrefix(heading, "## ") {
				inTaskSection = false
			}
		}
		if !inTaskSection {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		indent := len(line) - len(trimmed)
		out = append(out, PickerItem{
			Line:   i + 1,
			Text:   strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")),
			Raw:    line,
			Indent: indent,
		})
	}
	return out
}

// ReadItems is a convenience wrapper that reads the file then parses it.
func ReadItems(path string) ([]PickerItem, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseItems(string(b)), nil
}
