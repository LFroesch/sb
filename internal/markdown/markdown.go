package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// Colors — match tui-hub palette
var (
	colorPrimary = lipgloss.Color("#5AF78E")
	colorAccent  = lipgloss.Color("#57C7FF")
	colorDim     = lipgloss.Color("#606060")
	colorText    = lipgloss.Color("#EEEEEE")
	colorYellow  = lipgloss.Color("#F3F99D")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	h2Style    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	h3Style    = lipgloss.NewStyle().Bold(true).Foreground(colorYellow)
	dimStyle   = lipgloss.NewStyle().Foreground(colorDim)
	textStyle  = lipgloss.NewStyle().Foreground(colorText)
	codeStyle  = lipgloss.NewStyle().Foreground(colorText).PaddingLeft(4)
)

// Render applies styling to markdown content (headers, bullets, code blocks, tables, inline).
// Ported from unrot's renderMarkdown.
func Render(content string, w int) string {
	var b strings.Builder
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	inCodeBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				b.WriteString("\n")
				if lang != "" {
					b.WriteString(dimStyle.PaddingLeft(4).Render(lang))
					b.WriteString("\n")
				}
			}
		case inCodeBlock:
			b.WriteString(codeStyle.Width(w).Render(line))
			b.WriteString("\n")

		case isTableSep(trimmed):
			// skip |---|---| rows

		case strings.HasPrefix(trimmed, "|"):
			isHeader := i+1 < len(lines) && isTableSep(strings.TrimSpace(lines[i+1]))
			b.WriteString(renderTableRow(trimmed, isHeader, w))
			b.WriteString("\n")

		case strings.HasPrefix(trimmed, "### "):
			header := strings.TrimPrefix(trimmed, "### ")
			b.WriteString(h3Style.PaddingLeft(2).Render(header))
			b.WriteString("\n")

		case strings.HasPrefix(trimmed, "## "):
			header := strings.TrimPrefix(trimmed, "## ")
			b.WriteString("\n")
			b.WriteString(h2Style.PaddingLeft(2).Render(header))
			b.WriteString("\n")

		case strings.HasPrefix(trimmed, "# "):
			header := strings.TrimPrefix(trimmed, "# ")
			b.WriteString(titleStyle.PaddingLeft(2).Width(w).Render(header))
			b.WriteString("\n")

		case strings.HasPrefix(trimmed, "- "):
			inner := strings.TrimPrefix(trimmed, "- ")
			b.WriteString(renderListItem("    · ", inner, w))
			b.WriteString("\n")

		case strings.HasPrefix(trimmed, "* "):
			inner := strings.TrimPrefix(trimmed, "* ")
			b.WriteString(renderListItem("    · ", inner, w))
			b.WriteString("\n")

		case trimmed == "":
			b.WriteString("\n")

		default:
			b.WriteString(textStyle.PaddingLeft(2).Render(renderInline(line)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func isTableSep(s string) bool {
	return strings.HasPrefix(s, "|") && strings.Contains(s, "---")
}

func renderTableRow(row string, isHeader bool, _ int) string {
	cols := strings.Split(row, "|")
	var parts []string
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		parts = append(parts, c)
	}

	if isHeader {
		var styled []string
		for _, p := range parts {
			styled = append(styled, lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(p))
		}
		return "  " + strings.Join(styled, dimStyle.Render(" │ "))
	}

	var styled []string
	for _, p := range parts {
		styled = append(styled, textStyle.Render(p))
	}
	return "  " + strings.Join(styled, dimStyle.Render(" │ "))
}

func renderListItem(prefix, inner string, width int) string {
	if width <= 0 {
		return textStyle.Render(prefix + renderInline(inner))
	}
	body := renderInline(inner)
	bodyWidth := width - lipgloss.Width(prefix)
	if bodyWidth < 8 {
		bodyWidth = 8
	}
	wrapped := strings.Split(xansi.Wordwrap(body, bodyWidth, " "), "\n")
	for i := range wrapped {
		if i == 0 {
			wrapped[i] = prefix + wrapped[i]
			continue
		}
		wrapped[i] = strings.Repeat(" ", lipgloss.Width(prefix)) + wrapped[i]
	}
	return textStyle.Render(strings.Join(wrapped, "\n"))
}

func renderInline(s string) string {
	// Bold: **text**
	s = renderDelimited(s, "**", lipgloss.NewStyle().Bold(true).Foreground(colorText))
	// Inline code: `text`
	s = renderDelimited(s, "`", lipgloss.NewStyle().Foreground(colorYellow))
	return s
}

func renderDelimited(s, delim string, style lipgloss.Style) string {
	for {
		start := strings.Index(s, delim)
		if start == -1 {
			break
		}
		end := strings.Index(s[start+len(delim):], delim)
		if end == -1 {
			break
		}
		end += start + len(delim)
		inner := s[start+len(delim) : end]
		s = s[:start] + style.Render(inner) + s[end+len(delim):]
	}
	return s
}
