package statusbar

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTmuxBlockShowsResetMarkerWhenAvailable(t *testing.T) {
	fiveReset := time.Date(2026, time.April, 28, 15, 0, 0, 0, time.Local)
	sevenReset := time.Date(2026, time.May, 5, 9, 0, 0, 0, time.Local)
	out := renderTmuxBlock(Usage{
		Source: "codex",
		FiveHour: Window{
			PctUsed:   5,
			ResetAt:   fiveReset,
			Available: true,
		},
		SevenDay: Window{
			PctUsed:   44,
			ResetAt:   sevenReset,
			Available: true,
		},
	})
	if !strings.Contains(out, " @3pm") {
		t.Fatalf("renderTmuxBlock 5h reset marker missing: %q", out)
	}
	if !strings.Contains(out, "7d:") {
		t.Fatalf("renderTmuxBlock missing seven-day segment: %q", out)
	}
	if !strings.Contains(out, " @5/5") {
		t.Fatalf("renderTmuxBlock 7d reset marker missing: %q", out)
	}
}

func TestRenderTmuxBlockUsesProviderColor(t *testing.T) {
	claude := renderTmuxBlock(Usage{
		Source:   "claude",
		FiveHour: Window{PctUsed: 10, Available: true},
	})
	codex := renderTmuxBlock(Usage{
		Source:   "codex",
		FiveHour: Window{PctUsed: 10, Available: true},
	})
	if !strings.HasPrefix(claude, "#[fg=#0099ff]claude") {
		t.Fatalf("claude block wrong color: %q", claude)
	}
	if !strings.HasPrefix(codex, "#[fg=#10a37f]codex") {
		t.Fatalf("codex block wrong color: %q", codex)
	}
}

func TestTmuxShortDateMonthDay(t *testing.T) {
	d := time.Date(2026, time.May, 5, 9, 0, 0, 0, time.Local)
	if got := tmuxShortDate(d); got != "5/5" {
		t.Fatalf("tmuxShortDate = %q, want 5/5", got)
	}
}

func TestTmuxShortTimeOmitsMinutesOnHour(t *testing.T) {
	onHour := time.Date(2026, time.April, 28, 15, 0, 0, 0, time.Local)
	withMinutes := time.Date(2026, time.April, 28, 15, 4, 0, 0, time.Local)
	if got := tmuxShortTime(onHour); got != "3pm" {
		t.Fatalf("tmuxShortTime(onHour) = %q, want 3pm", got)
	}
	if got := tmuxShortTime(withMinutes); got != "3:04pm" {
		t.Fatalf("tmuxShortTime(withMinutes) = %q, want 3:04pm", got)
	}
}
