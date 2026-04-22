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

// ParseItems reads content and returns every top-level `- ` bullet.
// Nested list items (indent > 0) are included so the TUI can show tree
// structure, but sync-back only deletes exact Raw lines so indentation
// is preserved round-trip.
func ParseItems(content string) []PickerItem {
	var out []PickerItem
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
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
