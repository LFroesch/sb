package cockpit

import (
	"os"
	"path/filepath"
)

// CleanLegacyConfig rewrites old cockpit config into the current shape:
// remove legacy interactive provider files and strip redundant Claude/Codex
// executor args that were carried by older seeds.
func CleanLegacyConfig(paths Paths) error {
	if err := cleanProviders(paths.ProvidersDir); err != nil {
		return err
	}
	if err := cleanPresets(paths.PresetsDir); err != nil {
		return err
	}
	return nil
}

func cleanProviders(dir string) error {
	for _, id := range []string{"claude-interactive", "codex-interactive"} {
		_ = os.Remove(filepath.Join(dir, id+".json"))
	}

	providers, err := LoadProviders(dir)
	if err != nil {
		return err
	}
	for _, p := range providers {
		p.Executor.Args = cleanExecutorArgs(p.Executor.Type, p.Executor.Args)
		if err := SaveProvider(dir, p); err != nil {
			return err
		}
	}
	return nil
}

func cleanPresets(dir string) error {
	presets, err := LoadPresets(dir)
	if err != nil {
		return err
	}
	for _, p := range presets {
		p.Executor.Args = cleanExecutorArgs(p.Executor.Type, p.Executor.Args)
		if err := SavePreset(dir, p); err != nil {
			return err
		}
	}
	return nil
}

func cleanExecutorArgs(kind string, args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch kind {
		case "claude":
			if arg == "-p" || arg == "--print" {
				continue
			}
		case "codex":
			if arg == "exec" || arg == "resume" || arg == "--json" {
				continue
			}
		}
		out = append(out, arg)
	}
	return out
}
