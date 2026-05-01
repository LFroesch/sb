package cockpit

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths resolves the on-disk layout described in the RFC. Everything is
// derived from the user's XDG dirs so tests can point it at a temp dir
// by setting HOME + XDG_* before calling DefaultPaths.
type Paths struct {
	StateDir    string // ~/.local/state/sb
	JobsDir     string // <state>/jobs
	CampaignDir string // <state>/campaigns
	ForemanFile string // <state>/foreman.json
	Socket      string // <state>/foreman.sock
	PIDFile     string // <state>/foreman.pid
	LogFile      string // ~/.local/share/sb/logs/foreman.log
	PresetsDir   string // ~/.config/sb/presets
	ProvidersDir string // ~/.config/sb/providers
	PromptsDir   string // ~/.config/sb/prompts
	HooksDir     string // ~/.config/sb/hooks
}

// DefaultPaths returns the standard layout. Directories are *not* created
// here — callers that need them should call EnsureDirs first.
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	state := xdgStateHome(home)
	data := xdgDataHome(home)
	config := xdgConfigHome(home)
	sbState := filepath.Join(state, "sb")
	sbData := filepath.Join(data, "sb")
	sbConfig := filepath.Join(config, "sb")
	return Paths{
		StateDir:    sbState,
		JobsDir:     filepath.Join(sbState, "jobs"),
		CampaignDir: filepath.Join(sbState, "campaigns"),
		ForemanFile: filepath.Join(sbState, "foreman.json"),
		Socket:      filepath.Join(sbState, "foreman.sock"),
		PIDFile:     filepath.Join(sbState, "foreman.pid"),
		LogFile:      filepath.Join(sbData, "logs", "foreman.log"),
		PresetsDir:   filepath.Join(sbConfig, "presets"),
		ProvidersDir: filepath.Join(sbConfig, "providers"),
		PromptsDir:   filepath.Join(sbConfig, "prompts"),
		HooksDir:     filepath.Join(sbConfig, "hooks"),
	}
}

// EnsureDirs makes sure every referenced directory exists.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.StateDir, p.JobsDir, p.CampaignDir, p.PresetsDir, p.ProvidersDir, p.PromptsDir, p.HooksDir, filepath.Dir(p.LogFile)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// JobDir returns <JobsDir>/<id> without creating it.
func (p Paths) JobDir(id JobID) string { return filepath.Join(p.JobsDir, string(id)) }

func xdgStateHome(home string) string {
	if v := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); v != "" {
		return v
	}
	return filepath.Join(home, ".local", "state")
}

func xdgDataHome(home string) string {
	if v := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); v != "" {
		return v
	}
	return filepath.Join(home, ".local", "share")
}

func xdgConfigHome(home string) string {
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return v
	}
	return filepath.Join(home, ".config")
}

// ExpandHome replaces a leading ~ with $HOME.
func ExpandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
