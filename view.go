package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/scripts"
	"github.com/LFroesch/sb/internal/workmd"
)

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
		{"Project", pageProject},
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
		slug := p.Path
		// trim home prefix for brevity
		if home, err := os.UserHomeDir(); err == nil {
			slug = strings.TrimPrefix(slug, home+"/")
		}
		right = panelHeaderStyle.Render(slug) + dimStyle.Render("  ·  ") +
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
	innerRight := rightWidth - 4

	// --- Left pane: project list ---
	var leftLines []string
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

		// Compact badge
		var badge string
		total := p.CurrentCount + p.InboxCount + p.BacklogCount
		if total > 0 {
			parts := []string{}
			if p.CurrentCount > 0 {
				parts = append(parts, accentStyle.Render(fmt.Sprintf("%d", p.CurrentCount)))
			}
			if p.InboxCount > 0 {
				parts = append(parts, warnStyle.Render(fmt.Sprintf("%d", p.InboxCount)))
			}
			if p.BacklogCount > 0 {
				parts = append(parts, dimStyle.Render(fmt.Sprintf("%d", p.BacklogCount)))
			}
			badge = " " + dimStyle.Render("· ") + strings.Join(parts, dimStyle.Render("/"))
		}

		line := prefix + name + badge
		// Truncate if too wide
		if lipgloss.Width(line) > innerLeft {
			// Just show prefix + name truncated
			line = prefix + name
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

	// Pad to fill height
	for len(leftLines) < panelHeight {
		leftLines = append(leftLines, "")
	}

	leftContent := strings.Join(leftLines[:panelHeight], "\n")
	leftPanel := panelActiveStyle.Width(leftWidth - 2).Render(leftContent)

	// --- Right pane: selected project tasks ---
	var rightLines []string
	if m.cursor < len(m.projects) {
		p := m.projects[m.cursor]

		// Header with project name and
		header := panelHeaderStyle.Render(p.Name)
		rightLines = append(rightLines, header)
		rightLines = append(rightLines, "")

		sections := []struct {
			key   string
			label string
			style lipgloss.Style
		}{
			{"current", "Current Tasks", accentStyle},
			{"inbox", "Inbox", warnStyle},
			{"backlog", "Backlog", dimStyle},
		}

		hasAnyTasks := false
		for _, sec := range sections {
			var secTasks []workmd.Task
			for _, t := range p.Tasks {
				if t.Section == sec.key && !t.Done {
					secTasks = append(secTasks, t)
				}
			}
			if len(secTasks) == 0 {
				continue
			}
			hasAnyTasks = true

			rightLines = append(rightLines, sec.style.Render("── "+sec.label)+
				dimStyle.Render(fmt.Sprintf(" (%d)", len(secTasks))))

			for j, t := range secTasks {
				secTasksViewLimit := 20
				if j >= secTasksViewLimit {
					rightLines = append(rightLines, dimStyle.Render(fmt.Sprintf("   + %d more", len(secTasks)-secTasksViewLimit)))
					break
				}
				icon := dimStyle.Render("·")
				if sec.key == "current" {
					icon = accentStyle.Render("›")
				} else if sec.key == "inbox" {
					icon = warnStyle.Render("·")
				}
				name := t.Name
				if len(name) > innerRight-6 && innerRight > 10 {
					name = name[:innerRight-9] + "..."
				}
				rightLines = append(rightLines, fmt.Sprintf("  %s %s", icon, textStyle.Render(name)))
			}
			rightLines = append(rightLines, "")
		}

		if !hasAnyTasks {
			rightLines = append(rightLines, dimStyle.Render("  No tasks"))
		}
	} else {
		rightLines = append(rightLines, dimStyle.Render("No project selected"))
	}

	// Apply scroll to right panel
	visibleRight := panelHeight
	if m.dashRightScroll > 0 && len(rightLines) > visibleRight {
		maxScroll := len(rightLines) - visibleRight
		scroll := m.dashRightScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}
		rightLines = rightLines[scroll:]
	}

	// Pad to fill height
	for len(rightLines) < panelHeight {
		rightLines = append(rightLines, "")
	}
	if len(rightLines) > panelHeight {
		rightLines = rightLines[:panelHeight]
	}

	rightContent := strings.Join(rightLines, "\n")
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
		return m.renderEmpty("ollama cleaning up...", "please wait, this may take a minute.")
	}

	return m.viewport.View()
}

func (m model) renderCleanup() string {
	p := ""
	if m.selected < len(m.projects) {
		p = m.projects[m.selected].Name
	}
	banner := warnStyle.Render("CLEANUP PREVIEW") +
		dimStyle.Render(" — "+p+" · y/enter accept · n/esc discard")
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

	if m.mode == modeDumpRouting {
		lines = append(lines, dimStyle.Render("  routing via ollama..."))
		// Show truncated preview of what's being routed
		preview := m.dumpText
		if len(preview) > 120 {
			preview = preview[:117] + "..."
		}
		lines = append(lines, "", dimStyle.Render("  \""+preview+"\""))
		return strings.Join(lines, "\n")
	}

	if m.mode == modeDumpConfirm {
		lines = append(lines, warnStyle.Render("  Route to: ")+
			accentStyle.Render(m.dumpRoute)+" / "+accentStyle.Render(m.dumpSection))
		lines = append(lines, "")
		// Show the dump text (truncated if huge)
		preview := m.dumpText
		previewLines := strings.Split(preview, "\n")
		maxPreview := m.height - 12
		if maxPreview < 5 {
			maxPreview = 5
		}
		if len(previewLines) > maxPreview {
			previewLines = previewLines[:maxPreview]
			previewLines = append(previewLines, fmt.Sprintf("  ... +%d more lines", len(strings.Split(m.dumpText, "\n"))-maxPreview))
		}
		for _, l := range previewLines {
			lines = append(lines, dimStyle.Render("  "+l))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("  y/enter accept · n/esc reject & re-edit"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, m.dumpArea.View(), "")
	lines = append(lines, dimStyle.Render("  ctrl+d to route · esc to cancel"))

	if m.dumpResult != "" {
		lines = append(lines, "")
		lines = append(lines, primaryStyle.Render("  "+m.dumpResult))
	}

	return strings.Join(lines, "\n")
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
		add("j/k", "navigate")
		add("enter", "view")
		add("e", "edit")
		add("w/s", "scroll right")
		add("o", "open dir")
		add("d", "dump")
		add("x", "scripts")
		add("r", "refresh")
	case pageProject:
		add("j/k", "scroll")
		add("e", "edit")
		add("c", "cleanup")
		add("esc", "back")
	case pageCleanup:
		add("y/enter", "accept")
		add("n/esc", "discard")
		add("j/k", "scroll")
	case pageDump:
		if m.mode == modeDumpConfirm {
			add("y/enter", "accept")
			add("n/esc", "reject")
		} else {
			add("ctrl+d", "route")
			add("esc", "back")
		}
	case pageScripts:
		add("j/k", "navigate")
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
			{"j/k", "Navigate projects (sorted by recently updated)"},
			{"enter", "View project WORK.md"},
			{"e", "Edit project WORK.md directly"},
			{"w/s", "Scroll right panel"},
			{"o", "Open project directory"},
			{"d", "Brain dump"},
			{"x", "Maintenance scripts"},
			{"r", "Refresh (re-scan WORK.md files)"},
		}},
		{"Project View", []struct{ key, desc string }{
			{"j/k", "Scroll"},
			{"e", "Edit inline"},
			{"ctrl+s", "Save edits"},
			{"c", "Cleanup via ollama (normalizes format)"},
			{"esc", "Back / cancel edit"},
		}},
		{"Brain Dump", []struct{ key, desc string }{
			{"enter", "Newline"},
			{"ctrl+d", "Route to project via ollama"},
			{"esc", "Cancel"},
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
