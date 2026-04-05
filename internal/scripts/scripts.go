package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Script struct {
	Name        string
	Description string
	Path        string
}

type DoneMsg struct {
	Name   string
	Output string
	Err    error
}

var brainDir = filepath.Join(os.Getenv("HOME"), "projects/active/daily_use/SECOND_BRAIN")

// Available returns all maintenance scripts sb knows about.
func Available() []Script {
	return []Script{
		{
			Name:        "knowledge-index",
			Description: "Regenerate knowledge/INDEX.md",
			Path:        filepath.Join(brainDir, "claude/scripts/knowledge-index.sh"),
		},
		// {
		// 	Name:        "obsidian-sync",
		// 	Description: "Sync to Obsidian vault",
		// 	Path:        filepath.Join(brainDir, "claude/scripts/obsidian-sync.sh"),
		// },
		{
			Name:        "workmd-audit --fix",
			Description: "Check / Fix WORKmd ↔ project hardlinks",
			Path:        filepath.Join(brainDir, "claude/scripts/workmd-audit.sh"),
		},
		{
			Name:        "daily-git-digest",
			Description: "Today's commits across all repos NOTE: work on this",
			Path:        filepath.Join(brainDir, "claude/scripts/daily-git-digest.sh"),
		},
	}
	// Note: per-project cleanup is done with 'c' in project view (uses ollama inline)
}

// RunCmd returns a tea.Cmd that executes the script.
func RunCmd(s Script) tea.Cmd {
	return func() tea.Msg {
		parts := strings.Fields(s.Name)
		var cmd *exec.Cmd
		if len(parts) > 1 && parts[0] != s.Path {
			// Has args in name like "workmd-audit --fix"
			cmd = exec.Command("bash", append([]string{s.Path}, parts[1:]...)...)
		} else {
			cmd = exec.Command("bash", s.Path)
		}
		cmd.Dir = brainDir

		out, err := cmd.CombinedOutput()
		return DoneMsg{
			Name:   s.Name,
			Output: string(out),
			Err:    err,
		}
	}
}
