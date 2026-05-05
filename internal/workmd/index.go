package workmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LFroesch/sb/internal/config"
)

// SpecialTarget mirrors the routing-time concept of a non-project target
// (catch-all bucket, ideas bucket). Used purely for the index file's display.
type SpecialTarget struct {
	Name        string
	Path        string
	Description string
}

type IndexOptions struct {
	ScanRoots     []config.ScanRoot
	FilePatterns  []string
	ExplicitPaths []string
	IdeaDirs      []string
}

// WriteIndex renders the routing-context index file. The file is purely an
// inspection artifact — sb reads descriptions live during Discover() and never
// reads back from this file. Hand-edits get overwritten on the next startup.
func WriteIndex(path string, projects []Project, targets []SpecialTarget, opts IndexOptions) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# sb index\n")
	b.WriteString("# Auto-generated. Do not edit — regenerated on every `sb` startup.\n")
	b.WriteString("# Canonical metadata format: typed H1 + first plain text line below it as the short summary.\n\n")

	b.WriteString("## Discovery Config\n\n")
	if len(opts.ScanRoots) == 0 {
		b.WriteString("- scan roots: (default)\n")
	} else {
		for _, root := range opts.ScanRoots {
			fmt.Fprintf(&b, "- scan root: %s → %s\n", root.Name, root.Path)
		}
	}
	if len(opts.FilePatterns) == 0 {
		b.WriteString("- file patterns: WORK.md\n")
	} else {
		fmt.Fprintf(&b, "- file patterns: %s\n", strings.Join(opts.FilePatterns, ", "))
	}
	if len(opts.ExplicitPaths) == 0 {
		b.WriteString("- explicit paths: (none)\n")
	} else {
		for _, path := range opts.ExplicitPaths {
			fmt.Fprintf(&b, "- explicit path: %s\n", path)
		}
	}
	if len(opts.IdeaDirs) == 0 {
		b.WriteString("- idea dirs: (none)\n")
	} else {
		fmt.Fprintf(&b, "- idea dirs: %s\n", strings.Join(opts.IdeaDirs, ", "))
	}

	b.WriteString("## Projects\n\n")
	if len(projects) == 0 {
		b.WriteString("- (none discovered)\n")
	}
	for _, p := range projects {
		fmt.Fprintf(&b, "### %s\n\n", p.Name)
		fmt.Fprintf(&b, "- source: `%s` (%s)\n", p.RelativePathOrFile(), p.FileName)
		if p.Description != "" {
			fmt.Fprintf(&b, "- summary: %s\n", p.Description)
		}
		if p.Phase != "" {
			fmt.Fprintf(&b, "- phase: %s\n", p.Phase)
		}
		if len(p.ActivePreview) > 0 {
			b.WriteString("- current preview:\n")
			for _, item := range p.ActivePreview {
				fmt.Fprintf(&b, "  - %s\n", item)
			}
		}
		b.WriteString("\n")
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
