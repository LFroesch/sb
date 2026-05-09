package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestEnsureColorProfilePromotesAsciiInTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("TERM", "screen")
	t.Setenv("COLORTERM", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")

	lipgloss.SetColorProfile(termenv.Ascii)
	ensureColorProfile()

	if got := lipgloss.ColorProfile(); got != termenv.ANSI256 {
		t.Fatalf("ColorProfile = %v, want ANSI256", got)
	}
}

func TestEnsureColorProfileHonorsNoColor(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("TERM", "screen-256color")
	t.Setenv("NO_COLOR", "1")

	lipgloss.SetColorProfile(termenv.Ascii)
	ensureColorProfile()

	if got := lipgloss.ColorProfile(); got != termenv.Ascii {
		t.Fatalf("ColorProfile = %v, want Ascii", got)
	}
}
