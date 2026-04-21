package workmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SpecialTarget mirrors the routing-time concept of a non-project target
// (catch-all bucket, ideas bucket). Used purely for the index file's display.
type SpecialTarget struct {
	Name        string
	Path        string
	Description string
}

// WriteIndex renders the routing-context index file. The file is purely an
// inspection artifact — sb reads descriptions live during Discover() and never
// reads back from this file. Hand-edits get overwritten on the next startup.
func WriteIndex(path string, projects []Project, targets []SpecialTarget) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# sb index\n")
	b.WriteString("# Auto-generated. Do not edit — regenerated on every `sb` startup.\n")
	b.WriteString("# Edit any discovered markdown title line to change label/description: `# TYPE - label | description`\n\n")

	b.WriteString("## Projects\n\n")
	if len(projects) == 0 {
		b.WriteString("- (none discovered)\n")
	}
	for _, p := range projects {
		if p.Description == "" {
			fmt.Fprintf(&b, "- %s\n", p.Name)
		} else {
			fmt.Fprintf(&b, "- %s — %s\n", p.Name, p.Description)
		}
	}

	if len(targets) > 0 {
		b.WriteString("\n## Special Targets\n\n")
		for _, t := range targets {
			if t.Description == "" {
				fmt.Fprintf(&b, "- %s → %s\n", t.Name, t.Path)
			} else {
				fmt.Fprintf(&b, "- %s → %s — %s\n", t.Name, t.Path, t.Description)
			}
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}
