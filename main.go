package main

import (
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/logs"
)

func main() {
	_ = config.WriteDefaults() // create ~/.config/sb/config.json on first run
	cfg := config.Load()
	slog.SetDefault(logs.Open("sb", cfg.LogLevel))
	m := newModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sb: %v\n", err)
		os.Exit(1)
	}
}
