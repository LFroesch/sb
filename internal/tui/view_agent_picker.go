package tui

import (
	"fmt"
	"strings"
)

func (m model) renderAgentPicker() string {
	var lines []string
	contentHeight := m.agentContentHeight()
	lines = append(lines, titleStyle.Render("Pick tasks"), "")

	if m.pickerFile == "" {
		lines = append(lines, dimStyle.Render("  Files"), "")
		visibleRows := contentHeight - len(lines)
		if visibleRows < 1 {
			visibleRows = 1
		}
		total := len(m.projects)
		startIdx := 0
		if m.agentCursor >= visibleRows {
			startIdx = m.agentCursor - visibleRows + 1
		}
		endIdx := startIdx + visibleRows
		if endIdx > total {
			endIdx = total
		}
		if startIdx > 0 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▲ %d more", startIdx)))
		} else {
			lines = append(lines, "")
		}
		for i := startIdx; i < endIdx; i++ {
			p := m.projects[i]
			prefix := "    "
			if i == m.agentCursor {
				prefix = accentStyle.Render("  ▸ ")
			}
			lines = append(lines, prefix+textStyle.Render(p.Name)+dimStyle.Render("  "+shortPath(p.Path)))
		}
		if endIdx < total {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", total-endIdx)))
		} else {
			lines = append(lines, "")
		}
		return strings.Join(capLines(lines, contentHeight), "\n")
	}

	lines = append(lines, dimStyle.Render("  Items from "+shortPath(m.pickerFile)), "")
	if len(m.pickerItems) == 0 {
		lines = append(lines, dimStyle.Render("    (no `- ` items found)"))
		return strings.Join(capLines(lines, contentHeight), "\n")
	}
	visibleRows := contentHeight - len(lines) - 1
	if visibleRows < 1 {
		visibleRows = 1
	}
	total := len(m.pickerItems)
	startIdx := 0
	if m.agentCursor >= visibleRows {
		startIdx = m.agentCursor - visibleRows + 1
	}
	endIdx := startIdx + visibleRows
	if endIdx > total {
		endIdx = total
	}
	if startIdx > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▲ %d more", startIdx)))
	} else {
		lines = append(lines, "")
	}
	for i := startIdx; i < endIdx; i++ {
		it := m.pickerItems[i]
		checkbox := "[ ]"
		if m.pickerSelected[i] {
			checkbox = accentStyle.Render("[x]")
		}
		prefix := "    "
		if i == m.agentCursor {
			prefix = accentStyle.Render("  ▸ ")
		}
		indent := strings.Repeat(" ", it.Indent)
		lines = append(lines, prefix+checkbox+" "+dimStyle.Render(indent)+textStyle.Render(it.Text))
	}
	if endIdx < total {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", total-endIdx)))
	} else {
		lines = append(lines, "")
	}
	selected := countSelected(m.pickerSelected)
	lines = append(lines, dimStyle.Render(fmt.Sprintf("  %d selected", selected)))
	return strings.Join(capLines(lines, contentHeight), "\n")
}

func countSelected(sel map[int]bool) int {
	n := 0
	for _, v := range sel {
		if v {
			n++
		}
	}
	return n
}
