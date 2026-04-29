package statusbar

import (
	"fmt"
	"strings"
	"time"
)

// RenderTmuxLine returns a tmux-format (#[fg=…]) one-line status showing
// Claude + Codex rolling usage in condensed form: e.g.
//
//	claude 5h 42% 7d 12% · codex 5h 5% 7d 44%
//
// Threshold colors on the %. The 5h reset time is shown whenever we have it so
// both providers render consistently. Empty string when neither provider has
// data.
func RenderTmuxLine() string {
	var blocks []string
	if u, ok := FetchClaude(); ok {
		if s := renderTmuxBlock(u); s != "" {
			blocks = append(blocks, s)
		}
	}
	if u, ok := FetchCodex(); ok {
		if s := renderTmuxBlock(u); s != "" {
			blocks = append(blocks, s)
		}
	}
	return strings.Join(blocks, "#[fg=#475569] | ")
}

func renderTmuxBlock(u Usage) string {
	var segs []string
	if u.FiveHour.Available {
		seg := "#[fg=#cbd5e1]5h " + tmuxPct(u.FiveHour.PctUsed)
		if !u.FiveHour.ResetAt.IsZero() {
			seg += "#[fg=#94a3b8] @" + tmuxShortTime(u.FiveHour.ResetAt)
		}
		segs = append(segs, seg)
	}
	if u.SevenDay.Available {
		seg := "#[fg=#cbd5e1]7d " + tmuxPct(u.SevenDay.PctUsed)
		if !u.SevenDay.ResetAt.IsZero() {
			seg += "#[fg=#94a3b8] @" + tmuxShortDate(u.SevenDay.ResetAt)
		}
		segs = append(segs, seg)
	}
	if u.Extra != nil && u.Extra.Enabled {
		segs = append(segs, fmt.Sprintf("#[fg=#cbd5e1]x %s#[fg=#94a3b8] $%.0f/$%.0f",
			tmuxPct(u.Extra.PctUsed), u.Extra.UsedCredits, u.Extra.MonthlyLimit))
	}
	if len(segs) == 0 {
		return ""
	}
	return sourceColor(u.Source) + shortSource(u.Source) + " " + strings.Join(segs, " ")
}

// sourceColor returns the brand color for the provider label so the eye
// can pick out claude vs. codex blocks at a glance.
func sourceColor(s string) string {
	switch strings.ToLower(s) {
	case "codex":
		return "#[fg=#10a37f]"
	default:
		return "#[fg=#0099ff]"
	}
}

// shortSource abbreviates provider names for the condensed tmux line.
func shortSource(s string) string {
	switch strings.ToLower(s) {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	default:
		return s
	}
}

// tmuxShortTime formats a reset time in the shortest useful form:
// "3pm" on the hour, "3:04pm" otherwise.
func tmuxShortTime(t time.Time) string {
	local := t.Local()
	if local.Minute() == 0 {
		return strings.ToLower(local.Format("3pm"))
	}
	return strings.ToLower(local.Format("3:04pm"))
}

// tmuxShortDate formats a 7-day reset as "M/D" (e.g. "5/5").
func tmuxShortDate(t time.Time) string {
	return t.Local().Format("1/2")
}

func tmuxPct(pct int) string {
	pct = tmuxClamp(pct)
	return fmt.Sprintf("#[fg=%s]%d%%", tmuxPctColor(pct), pct)
}

func tmuxPctColor(pct int) string {
	switch {
	case pct >= 90:
		return "#ff5555"
	case pct >= 70:
		return "#e6c800"
	case pct >= 50:
		return "#ffb055"
	default:
		return "#00a000"
	}
}

func tmuxClamp(pct int) int {
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}
