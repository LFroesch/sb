package main

import (
	"fmt"
	"strings"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/ollama"
	"github.com/LFroesch/sb/internal/scripts"
)

func truncate(s string, max int) string {
	if max < 4 {
		return ""
	}
	return xansi.Truncate(s, max, "...")
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	if m.mode == modeHelp {
		return m.renderHelp()
	}

	header := m.renderHeader()
	sep := dimStyle.Render(strings.Repeat("─", m.width))

	var content string
	switch m.page {
	case pageDashboard:
		content = m.renderDashboard()
	case pageProject:
		content = m.renderProject()
	case pageDump:
		content = m.renderDump()
	case pageScripts:
		content = m.renderScripts()
	case pageCleanup:
		content = m.renderCleanup()
	}

	var statusLine string
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		statusLine = statusStyle.Render("  " + m.statusMsg)
	}

	footer := m.renderFooter()

	var parts []string
	parts = append(parts, header, sep, content)
	if statusLine != "" {
		parts = append(parts, statusLine)
	}
	parts = append(parts, sep, footer)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// --- Header ---

func (m model) renderHeader() string {
	title := titleStyle.Render("sb")

	pages := []struct {
		name string
		p    page
	}{
		{"Dashboard", pageDashboard},
		{"Dump", pageDump},
		{"Scripts", pageScripts},
	}

	var tabs []string
	for i, pg := range pages {
		if i > 0 {
			tabs = append(tabs, dimStyle.Render(" │ "))
		}
		if pg.p == m.page {
			tabs = append(tabs, activeTabStyle.Render(pg.name))
		} else {
			tabs = append(tabs, dimStyle.Render(pg.name))
		}
	}

	left := title + "  " + strings.Join(tabs, "")

	// Right side: project-specific info on project page, global stats otherwise
	var right string
	if m.page == pageProject && m.selected < len(m.projects) {
		p := m.projects[m.selected]
		right = panelHeaderStyle.Render(p.Name) + dimStyle.Render("  ·  ") +
			accentStyle.Render(fmt.Sprintf("%d", p.CurrentCount)) + dimStyle.Render(" cur · ") +
			warnStyle.Render(fmt.Sprintf("%d", p.InboxCount)) + dimStyle.Render(" inbox · ") +
			dimStyle.Render(fmt.Sprintf("%d backlog", p.BacklogCount))
	} else {
		var totalCur, totalInbox, totalBacklog int
		for _, p := range m.projects {
			totalCur += p.CurrentCount
			totalInbox += p.InboxCount
			totalBacklog += p.BacklogCount
		}
		right = dimStyle.Render(fmt.Sprintf(
			"%d projects · %d current · %d inbox · %d backlog",
			len(m.projects), totalCur, totalInbox, totalBacklog,
		))
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}

	return left + strings.Repeat(" ", gap) + right
}

// --- Dashboard ---

func (m model) renderDashboard() string {
	if m.loading {
		return m.renderEmpty(m.spinner.View()+" scanning ~/projects…", "")
	}
	if len(m.projects) == 0 {
		return m.renderEmpty("No WORK.md files found", "Check ~/projects")
	}

	availableHeight := m.height - 6 // header + separators + footer
	if availableHeight < 5 {
		availableHeight = 5
	}
	panelHeight := availableHeight - 2 // border top/bottom

	leftWidth := m.width * 25 / 100
	rightWidth := m.width - leftWidth - 2 // gap between panels
	if leftWidth < 20 {
		leftWidth = 20
	}
	innerLeft := leftWidth - 4  // border + padding

	// --- Left pane: project list or search ---
	var leftLines []string
	if m.mode == modeSearch {
		leftLines = append(leftLines, accentStyle.Render("/")+" "+m.searchQuery+dimStyle.Render("█"))
		leftLines = append(leftLines, "")
		if len(m.searchMatches) == 0 && m.searchQuery != "" {
			leftLines = append(leftLines, dimStyle.Render("  no matches"))
		}
		for i, match := range m.searchMatches {
			prefix := "  "
			if i == m.cursor {
				prefix = accentStyle.Render("▸ ")
			}
			pName := m.projects[match.projectIdx].Name
			name := pName
			if i == m.cursor {
				name = accentStyle.Bold(true).Render(name)
			} else {
				name = textStyle.Render(name)
			}
			hint := ""
			if match.line != pName {
				hint = dimStyle.Render("  " + truncate(match.line, innerLeft-lipgloss.Width(name)-6))
			}
			leftLines = append(leftLines, prefix+name+hint)
		}
	} else {
		leftLines = append(leftLines, panelHeaderStyle.Render("Projects"))
		leftLines = append(leftLines, "")

	// Scrolling for project list
	maxVisible := panelHeight - 3 // header + blank + bottom padding
	if maxVisible < 3 {
		maxVisible = 3
	}
	startIdx := 0
	if m.cursor >= maxVisible {
		startIdx = m.cursor - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.projects) {
		endIdx = len(m.projects)
	}

	for i := startIdx; i < endIdx; i++ {
		p := m.projects[i]
		prefix := "  "
		if i == m.cursor {
			prefix = accentStyle.Render("▸ ")
		}

		name := p.Name
		if i == m.cursor {
			name = accentStyle.Bold(true).Render(name)
		} else {
			name = textStyle.Render(name)
		}

		var indicators string
		if p.InboxCount > 0 {
			indicators += warnStyle.Render("🚨") + dimStyle.Render(" · ")
		}
		if time.Since(p.ModTime) > 30*24*time.Second {
			indicators += dimStyle.Render("👻") + dimStyle.Render(" · ")
		}

		// Task counts
		var counts []string
		if p.CurrentCount > 0 {
			counts = append(counts, accentStyle.Render(fmt.Sprintf("%d", p.CurrentCount)))
		}
		if p.InboxCount > 0 {
			counts = append(counts, warnStyle.Render(fmt.Sprintf("%d", p.InboxCount)))
		}
		if p.BacklogCount > 0 {
			counts = append(counts, dimStyle.Render(fmt.Sprintf("%d", p.BacklogCount)))
		}

		// Single · separator between name and (indicators + counts)
		suffix := indicators + strings.Join(counts, dimStyle.Render("/"))
		var line string
		if suffix != "" {
			line = prefix + name + dimStyle.Render(" · ") + suffix
		} else {
			line = prefix + name
		}
		// Truncate if too wide (xansi handles ANSI codes + wide chars)
		if lipgloss.Width(line) > innerLeft {
			line = xansi.Truncate(line, innerLeft, "")
		}
		leftLines = append(leftLines, line)
	}

	// Scroll indicators
	if startIdx > 0 {
		leftLines[1] = dimStyle.Render(fmt.Sprintf("  ▲ %d more", startIdx))
	}
	if endIdx < len(m.projects) {
		leftLines = append(leftLines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", len(m.projects)-endIdx)))
	}

	} // end search/project-list if-else

	// Pad to fill height
	for len(leftLines) < panelHeight {
		leftLines = append(leftLines, "")
	}

	leftContent := strings.Join(leftLines[:panelHeight], "\n")
	leftPanel := panelActiveStyle.Width(leftWidth - 2).Render(leftContent)

	// --- Right pane: rendered WORK.md via viewport ---
	var rightContent string
	if m.mode == modeEdit && m.selected == m.cursor {
		rightContent = m.renderEditMode()
	} else if (m.mode == modeCleanupWait || m.mode == modeTodoWait) && m.selected == m.cursor {
		label := "ollama cleaning up..."
		if m.mode == modeTodoWait {
			label = "asking ollama what to work on..."
		}
		content := lipgloss.JoinVertical(lipgloss.Center, "", "",
			m.spinner.View()+" "+dimStyle.Render(label),
			"",
			dimStyle.Render("please wait..."),
		)
		rightContent = lipgloss.Place(rightWidth-4, panelHeight, lipgloss.Center, lipgloss.Center, content)
	} else if m.mode == modeTodoResult && m.selected == m.cursor {
		var lines []string
		proj := ""
		if m.selected < len(m.projects) {
			proj = m.projects[m.selected].Name
		}
		lines = append(lines, accentStyle.Render("What to work on: ")+dimStyle.Render(proj), "")
		for _, l := range strings.Split(m.todoResult, "\n") {
			lines = append(lines, "  "+textStyle.Render(l))
		}
		lines = append(lines, "", dimStyle.Render("  any key to dismiss"))
		rightContent = strings.Join(lines, "\n")
	} else {
		// Size the viewport to fit the right panel
		m.viewport.Width = rightWidth - 4
		m.viewport.Height = panelHeight
		rightContent = m.viewport.View()
	}
	rightPanel := panelStyle.Width(rightWidth - 2).Render(rightContent)

	// Force same height and join
	leftStyled := lipgloss.NewStyle().Height(availableHeight).Render(leftPanel)
	rightStyled := lipgloss.NewStyle().Height(availableHeight).Render(rightPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, " ", rightStyled)
}

// --- Project view ---

func (m model) renderProject() string {
	if m.selected >= len(m.projects) {
		return m.renderEmpty("No project selected", "")
	}

	if m.mode == modeEdit {
		return m.renderEditMode()
	}

	if m.mode == modeCleanupWait {
		h := m.height - 8
		if h < 5 {
			h = 5
		}
		content := lipgloss.JoinVertical(lipgloss.Center, "", "",
			m.spinner.View()+" "+dimStyle.Render("ollama cleaning up..."),
			"",
			dimStyle.Render("please wait, this may take a minute."),
		)
		return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
	}

	return m.viewport.View()
}

func (m model) renderCleanup() string {
	p := ""
	if m.selected < len(m.projects) {
		p = m.projects[m.selected].Name
	}
	banner := warnStyle.Render("CLEANUP DIFF") +
		dimStyle.Render(" — "+p+" · y/enter accept · n/esc discard · j/k scroll")
	return lipgloss.JoinVertical(lipgloss.Left, banner, "", m.viewport.View())
}

func (m model) renderEditMode() string {
	header := warnStyle.Render("EDITING") + dimStyle.Render(" — ctrl+s save · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, "", m.editArea.View())
}

// --- Brain dump ---

func (m model) renderDump() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Brain Dump"), "")

	switch m.mode {
	case modeDumpSummary:
		return m.renderDumpSummary()
	case modeDumpRouting:
		lines = append(lines, "  "+m.spinner.View()+" "+dimStyle.Render("routing via ollama..."))
		preview := m.dumpText
		if len(preview) > 120 {
			preview = preview[:117] + "..."
		}
		lines = append(lines, "", dimStyle.Render("  \""+preview+"\""))
		return strings.Join(lines, "\n")

	case modeDumpReview:
		if m.dumpCursor < len(m.dumpItems) {
			item := m.dumpItems[m.dumpCursor]
			progress := fmt.Sprintf("(%d/%d)", m.dumpCursor+1, len(m.dumpItems))

			lines = append(lines, warnStyle.Render("  REVIEW ")+dimStyle.Render(progress))
			lines = append(lines, "")
			lines = append(lines, accentStyle.Render("  "+item.Text))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("  route to: ")+
				accentStyle.Render(item.Project)+dimStyle.Render(" / ")+accentStyle.Render(item.Section))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("  y/enter accept · n skip · r reroute · esc abort"))
		}
		return strings.Join(lines, "\n")

	case modeDumpClarify:
		if m.dumpCursor < len(m.dumpItems) {
			item := m.dumpItems[m.dumpCursor]
			progress := fmt.Sprintf("(%d/%d)", m.dumpCursor+1, len(m.dumpItems))

			lines = append(lines, warnStyle.Render("  CLARIFY ")+dimStyle.Render(progress))
			lines = append(lines, "")
			lines = append(lines, accentStyle.Render("  "+item.Text))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("  which project does this belong to?"))
			lines = append(lines, "")
			lines = append(lines, m.dumpClarifyArea.View())
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("  enter to reroute · esc to skip"))
		}
		return strings.Join(lines, "\n")
	}

	// modeDumpInput (default)
	lines = append(lines, m.dumpArea.View(), "")
	lines = append(lines, dimStyle.Render("  ctrl+d to route · esc to cancel"))
	return strings.Join(lines, "\n")
}

func (m model) renderDumpSummary() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Brain Dump — Done"), "")

	accepted := fmt.Sprintf("%d routed", m.dumpAccepted)
	skipped := fmt.Sprintf("%d skipped", m.dumpSkipped)
	lines = append(lines, "  "+accentStyle.Render(accepted)+"  "+dimStyle.Render(skipped), "")

	if m.dumpAccepted > 0 {
		lines = append(lines, primaryStyle.Render("  Routed:"))
		for _, it := range m.dumpItems {
			// dumpItems at this point are all items; accepted = those not in skippedList
			if !isDumpSkipped(it, m.dumpSkippedList) {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("    • %s → %s / %s", it.Text, it.Project, it.Section)))
			}
		}
		lines = append(lines, "")
	}

	if len(m.dumpSkippedList) > 0 {
		lines = append(lines, warnStyle.Render("  Skipped:"))
		for _, it := range m.dumpSkippedList {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("    • %s → %s / %s", it.Text, it.Project, it.Section)))
		}
		lines = append(lines, "")
	}

	lines = append(lines, dimStyle.Render("  j/k scroll · any other key to continue"))

	visibleH := m.height - 8
	if visibleH < 5 {
		visibleH = 5
	}
	maxScroll := len(lines) - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.dumpSummaryScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

func isDumpSkipped(item ollama.RouteItem, skipped []ollama.RouteItem) bool {
	for _, s := range skipped {
		if s.Text == item.Text {
			return true
		}
	}
	return false
}

// --- Scripts ---

func (m model) renderScripts() string {
	available := scripts.Available()
	if len(available) == 0 {
		return m.renderEmpty("No scripts found", "")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Maintenance Scripts"), "")

	for i, s := range available {
		prefix := "  "
		if i == m.scriptCursor {
			prefix = accentStyle.Render("▸ ")
		}

		name := s.Name
		if i == m.scriptCursor {
			name = accentStyle.Bold(true).Render(name)
		}

		lines = append(lines, prefix+name+dimStyle.Render("  "+s.Description))
	}

	if m.scriptOutput != "" {
		lines = append(lines, "", dimStyle.Render(strings.Repeat("─", m.width-4)), "")
		// Show last N lines of output
		outLines := strings.Split(m.scriptOutput, "\n")
		maxOut := m.height - len(available) - 10
		if maxOut < 5 {
			maxOut = 5
		}
		if len(outLines) > maxOut {
			outLines = outLines[len(outLines)-maxOut:]
		}
		for _, l := range outLines {
			lines = append(lines, "  "+dimStyle.Render(l))
		}
	}

	return strings.Join(lines, "\n")
}

// --- Footer ---

func (m model) renderFooter() string {
	var parts []string
	add := func(key, action string) {
		if len(parts) > 0 {
			parts = append(parts, dimStyle.Render(" · "))
		}
		parts = append(parts, keyStyle.Render(key), " ", actionStyle.Render(action))
	}

	switch m.page {
	case pageDashboard:
		add("j/k", "nav")
		add("enter", "open")
		add("e", "edit")
		add("d", "dump")
		add("y", "copy path")
	case pageProject:
		add("j/k", "scroll")
		add("e", "edit")
		add("esc", "back")
	case pageCleanup:
		add("y/enter", "accept")
		add("n/esc", "discard")
	case pageDump:
		switch m.mode {
		case modeDumpReview:
			add("y/enter", "accept")
			add("n", "skip")
			add("r", "reroute")
			add("esc", "abort")
		case modeDumpClarify:
			add("enter", "reroute")
			add("esc", "skip")
		default:
			add("ctrl+d", "route")
			add("esc", "back")
		}
	case pageScripts:
		add("j/k", "nav")
		add("enter", "run")
		add("esc", "back")
	}

	add("?", "help")
	add("q", "quit")

	return " " + strings.Join(parts, "")
}

// --- Help ---

func (m model) renderHelp() string {
	sections := []struct {
		title string
		keys  []struct{ key, desc string }
	}{
		{"Dashboard", []struct{ key, desc string }{
			{"j/k", "Navigate projects"},
			{"g / G", "Jump to top / bottom of list"},
			{"J/K", "Scroll WORK.md preview"},
			{"ctrl+d/u", "Half-page scroll preview"},
			{"pgup/pgdn", "Full-page scroll preview"},
			{"ctrl+home/end", "Preview top / bottom"},
			{"enter", "Full-screen project view"},
			{"e", "Edit WORK.md inline"},
			{"c", "Cleanup via ollama"},
			{"t", "Ask ollama what to work on"},
			{"o", "Open project directory in editor"},
			{"y", "Copy project dir path to clipboard"},
			{"d", "Brain dump"},
			{"x", "Maintenance scripts"},
			{"/", "Search across all WORK.md files"},
			{"r", "Refresh (re-scan WORK.md files)"},
		}},
		{"Project View", []struct{ key, desc string }{
			{"j/k", "Scroll"},
			{"g / G", "Jump to top / bottom"},
			{"pgup/pgdn", "Full-page scroll"},
			{"ctrl+home/end", "Top / bottom"},
			{"e", "Edit inline"},
			{"ctrl+s", "Save edits"},
			{"c", "Cleanup via ollama (normalizes format)"},
			{"esc", "Back / cancel edit"},
		}},
		{"Edit Mode", []struct{ key, desc string }{
			{"ctrl+s", "Save"},
			{"esc", "Cancel"},
			{"home / ctrl+a", "Start of line"},
			{"end / ctrl+e", "End of line"},
			{"ctrl+d", "Delete current line"},
			{"ctrl+k", "Delete to end of line"},
		}},
		{"Brain Dump", []struct{ key, desc string }{
			{"ctrl+d", "Route dump via ollama (splits into items)"},
			{"y/enter", "Accept routed item"},
			{"n", "Skip item"},
			{"esc", "Cancel / abort remaining"},
		}},
		{"Scripts", []struct{ key, desc string }{
			{"enter", "Run script"},
			{"esc", "Back"},
		}},
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Help"), "")

	for _, sec := range sections {
		lines = append(lines, keyStyle.Render(sec.title))
		for _, k := range sec.keys {
			lines = append(lines, fmt.Sprintf("  %s  %s",
				lipgloss.NewStyle().Foreground(colorAccent).Width(14).Render(k.key),
				k.desc))
		}
		lines = append(lines, "")
	}

	lines = append(lines, dimStyle.Render("press any key to close"))

	// Apply scroll
	visibleH := m.height - 10
	if visibleH < 5 {
		visibleH = 5
	}
	if m.helpScroll > len(lines)-visibleH {
		m.helpScroll = max(0, len(lines)-visibleH)
	}
	end := m.helpScroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[m.helpScroll:end]

	scrollHint := ""
	if len(lines) > visibleH {
		scrollHint = "\n" + dimStyle.Render(fmt.Sprintf("j/k scroll (%d/%d)", m.helpScroll+1, len(lines)-visibleH+1))
	}

	dialog := dialogStyle.Width(55).Render(strings.Join(visible, "\n") + scrollHint)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// --- Empty state ---

func (m model) renderEmpty(title, subtitle string) string {
	content := lipgloss.JoinVertical(lipgloss.Center, "", "", dimStyle.Render(title), "", dimStyle.Render(subtitle))
	h := m.height - 8
	if h < 5 {
		h = 5
	}
	return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
}
