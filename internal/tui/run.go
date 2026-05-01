package tui

import (
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/logs"
)

func Run() error {
	_ = config.WriteDefaults() // create ~/.config/sb/config.json on first run
	cfg := config.Load()
	slog.SetDefault(logs.Open("sb", cfg.LogLevel))

	// Bootstrap: re-exec ourselves inside the cockpit tmux session so
	// that window 0 of sb-cockpit is sb itself. No-op (and ExecFallback
	// is set) if tmux is missing or the user opted out via SB_NO_TMUX.
	if _, fb, bootErr := cockpit.MaybeReExecIntoTmux(); bootErr != nil && fb {
		slog.Info("cockpit: tmux bootstrap skipped", "err", bootErr)
	}

	m := newModel(cfg)
	m.cockpitDetachQuit = cockpit.ShouldDetachOnQuit()

	// Cockpit: seed presets + providers, then connect to the manager.
	// Preferred path is dial sb-foreman over the unix socket so jobs
	// survive sb quit; if that fails (no binary, perm issue, etc.) we
	// fall back to an in-proc Manager so the TUI still works.
	paths := cockpit.DefaultPaths()
	m.cockpitPaths = paths
	if _, err := cockpit.WriteDefaultPrompts(paths.PromptsDir); err != nil {
		slog.Warn("cockpit: prompt seed failed", "err", err)
	}
	if _, err := cockpit.WriteDefaultHookBundles(paths.HooksDir); err != nil {
		slog.Warn("cockpit: hook bundle seed failed", "err", err)
	}
	if _, err := cockpit.WriteDefaultProviders(paths.ProvidersDir); err != nil {
		slog.Warn("cockpit: provider seed failed", "err", err)
	}
	if _, err := cockpit.WriteDefaultPresets(paths.PresetsDir); err != nil {
		slog.Warn("cockpit: preset seed failed", "err", err)
	}
	if prompts, err := cockpit.LoadPrompts(paths.PromptsDir); err != nil {
		slog.Warn("cockpit: load prompts", "err", err)
	} else {
		m.cockpitPrompts = prompts
	}
	if bundles, err := cockpit.LoadHookBundles(paths.HooksDir); err != nil {
		slog.Warn("cockpit: load hook bundles", "err", err)
	} else {
		m.cockpitHookBundles = bundles
	}
	if providers, err := cockpit.LoadProviders(paths.ProvidersDir); err != nil {
		slog.Warn("cockpit: load providers", "err", err)
	} else {
		m.cockpitProviders = providers
	}
	if presets, err := cockpit.LoadPresets(paths.PresetsDir, m.cockpitPrompts, m.cockpitHookBundles, m.cockpitProviders); err != nil {
		m.cockpitErr = "load presets: " + err.Error()
	} else {
		m.cockpitPresets = presets
	}

	var client cockpit.Client
	if cfg.UseCockpitDaemon() {
		sc, err := cockpit.EnsureDaemon(paths, cfg.CockpitForemanBin)
		if err != nil {
			slog.Warn("cockpit: daemon unavailable, falling back to in-proc", "err", err)
		} else {
			client = sc
			m.cockpitMode = "daemon"
		}
	}
	if client == nil {
		mgr, err := cockpit.NewManager(paths)
		if err != nil {
			m.cockpitErr = "manager: " + err.Error()
		} else {
			client = mgr
			m.cockpitMode = "in-proc"
		}
	}
	if client != nil {
		m.cockpitClient = client
		ch, _ := client.Subscribe()
		m.cockpitEvents = ch
		m.cockpitJobs = client.ListJobs()
		m.cockpitForeman = client.GetForemanState()
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}
	if m.cockpitClient != nil {
		_ = m.cockpitClient.Close()
	}
	return nil
}
