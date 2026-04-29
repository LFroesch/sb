// Package transcript normalizes raw terminal/PTY output (ANSI escapes,
// carriage-return redraws, backspaces, tabs, and box-drawing chrome) into
// plain readable text for the cockpit's review/peek panes.
package transcript

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// Sanitize converts a raw session log into clean text: ANSI stripped,
// \r treated as a line redraw, \b applied, control bytes dropped, blank
// runs collapsed, and pure box-drawing chrome lines removed.
func Sanitize(raw string) string {
	raw = xansi.Strip(raw)
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\t", "    ")

	var (
		lines []string
		line  []rune
	)
	flush := func() {
		lines = append(lines, string(line))
		line = line[:0]
	}

	for _, r := range raw {
		switch {
		case r == '\n':
			flush()
		case r == '\r':
			line = line[:0]
		case r == '\b':
			if len(line) > 0 {
				line = line[:len(line)-1]
			}
		case r == '\t':
			line = append(line, r)
		case r < 0x20 || r == 0x7f:
			continue
		default:
			line = append(line, r)
		}
	}
	if len(line) > 0 {
		flush()
	}
	return cleanLines(lines)
}

func cleanLines(lines []string) string {
	out := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			out = append(out, "")
			continue
		}
		if isChromeLine(trimmed) {
			continue
		}
		lastBlank = false
		out = append(out, trimmed)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// isChromeLine reports whether s is a line that contains only box-drawing
// or block-fill glyphs — the kind of UI border that survives ANSI strip
// but adds no real signal to the review pane.
func isChromeLine(s string) bool {
	if s == "" {
		return false
	}
	hasAlphaNum := false
	hasBox := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			hasAlphaNum = true
		case strings.ContainsRune("│┃─━┄┅┈┉┊┋┌┐└┘├┤┬┴┼╭╮╯╰═║╔╗╝╚╠╣╦╩╬■▁▂▃▄▅▆▇█", r):
			hasBox = true
		}
	}
	return hasBox && !hasAlphaNum
}
