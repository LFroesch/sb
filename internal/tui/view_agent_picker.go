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
		var options []string
		// Row 0: freeform sentinel — start a run without picking task lines.
		sentinelPrefix := "    "
		sentinelLabel := primaryStyle.Render("★ New run without task source")
		if m.agentCursor == 0 {
			sentinelPrefix = accentStyle.Render("  ▸ ")
			sentinelLabel = primaryStyle.Bold(true).Render("★ New run without task source")
		}
		options = append(options, sentinelPrefix+sentinelLabel+dimStyle.Render("  type a brief, no task lines attached"))
		for i := range m.projects {
			p := m.projects[i]
			prefix := "    "
			if i+1 == m.agentCursor {
				prefix = accentStyle.Render("  ▸ ")
			}
			options = append(options, prefix+textStyle.Render(p.Name)+dimStyle.Render("  "+shortPath(p.Path)))
		}
		offset := scrollOffsetForCursor(len(options), m.agentCursor, visibleRows)
		lines = append(lines, scrollWindow(options, offset, visibleRows)...)
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
	var options []string
	for i := range m.pickerItems {
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
		options = append(options, prefix+checkbox+" "+dimStyle.Render(indent)+textStyle.Render(it.Text))
	}
	offset := scrollOffsetForCursor(len(options), m.agentCursor, visibleRows)
	lines = append(lines, scrollWindow(options, offset, visibleRows)...)
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
