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
		var parts []string
		parts = append(parts, panelHeaderStyle.Render(p.Name))
		parts = append(parts, accentStyle.Render(fmt.Sprintf("%d cur", p.CurrentCount)))
		if p.BugsCount > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("%d bugs", p.BugsCount)))
		}
		if p.UnsortedCount > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("🚨 %d unsorted", p.UnsortedCount)))
		}
		if p.NonListCount > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("❌ %d unclean", p.NonListCount)))
		}
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d backlog", p.BacklogCount)))
		right = strings.Join(parts, dimStyle.Render("  ·  "))
	} else {
		var totalCur, totalBugs, totalUnsorted, totalNonList, totalBacklog int
		for _, p := range m.projects {
			totalCur += p.CurrentCount
			totalBugs += p.BugsCount
			totalUnsorted += p.UnsortedCount
			totalNonList += p.NonListCount
			totalBacklog += p.BacklogCount
		}
		var parts []string
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d projects", len(m.projects))))
		parts = append(parts, accentStyle.Render(fmt.Sprintf("%d current", totalCur)))
		if totalBugs > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("%d bugs", totalBugs)))
		}
		if totalUnsorted > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("🚨 %d unsorted", totalUnsorted)))
		}
		if totalNonList > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("❌ %d unclean", totalNonList)))
		}
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d backlog", totalBacklog)))
		right = strings.Join(parts, dimStyle.Render("  ·  "))
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
		// Count pinned projects (sorted to front by sortWithFavorites)
		nFav := 0
		for _, p := range m.projects {
			if m.favorites[p.Path] {
				nFav++
			}
		}

		// renderProjectRow builds a single project line for the left panel.
		renderProjectRow := func(i int, innerW int) string {
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
			if m.selectedProjects[p.Path] {
				indicators += accentStyle.Render("●") + dimStyle.Render(" · ")
			}
			if p.UnsortedCount > 0 {
				indicators += warnStyle.Render("🚨") + dimStyle.Render(" · ")
			}
			if p.NonListCount > 0 {
				indicators += warnStyle.Render("❌") + dimStyle.Render(" · ")
			}
			if time.Since(p.ModTime) > 30*24*time.Hour {
				indicators += dimStyle.Render("👻") + dimStyle.Render(" · ")
			}
			var counts []string
			if p.CurrentCount > 0 {
				counts = append(counts, accentStyle.Render(fmt.Sprintf("%d", p.CurrentCount)))
			}
			if p.BugsCount > 0 {
				counts = append(counts, warnStyle.Render(fmt.Sprintf("%d", p.BugsCount)))
			}
			if p.BacklogCount > 0 {
				counts = append(counts, dimStyle.Render(fmt.Sprintf("%d", p.BacklogCount)))
			}
			suffix := indicators + strings.Join(counts, dimStyle.Render("/"))
			var line string
			if suffix != "" {
				line = prefix + name + dimStyle.Render(" · ") + suffix
			} else {
				line = prefix + name
			}
			if lipgloss.Width(line) > innerW {
				line = xansi.Truncate(line, innerW, "")
			}
			return line
		}

		// --- Pinned section (always visible, not scrolled) ---
		// pinnedDisplayLines: header(1) + blank(1) + nFav items + sep(1) + blank(1)
		pinnedDisplayLines := 0
		if nFav > 0 {
			pinnedDisplayLines = nFav + 4
			leftLines = append(leftLines, accentStyle.Render("★ Pinned"))
			leftLines = append(leftLines, "")
			for i := 0; i < nFav; i++ {
				leftLines = append(leftLines, renderProjectRow(i, innerLeft))
			}
			leftLines = append(leftLines, dimStyle.Render(strings.Repeat("─", innerLeft)))
			leftLines = append(leftLines, "")
		}

		// --- Scrollable rest ---
		projectsHeaderIdx := len(leftLines)
		leftLines = append(leftLines, panelHeaderStyle.Render("Projects"))
		leftLines = append(leftLines, "")
		projectsBlankIdx := len(leftLines) - 1

		// Available rows for non-pinned project rows
		// panelHeight - pinnedDisplayLines - 2 (header+blank) - 1 (bottom padding)
		maxVisible := panelHeight - pinnedDisplayLines - 3
		if maxVisible < 3 {
			maxVisible = 3
		}

		// Auto-scroll: when cursor is on a non-pinned item, scroll to show it
		scrollCursor := m.cursor - nFav
		if scrollCursor < 0 {
			scrollCursor = 0
		}
		startIdx := nFav
		if scrollCursor >= maxVisible {
			startIdx = nFav + scrollCursor - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(m.projects) {
			endIdx = len(m.projects)
		}

		// Suppress "Projects" header when all projects are pinned
		if nFav >= len(m.projects) {
			leftLines = leftLines[:projectsHeaderIdx] // remove header + blank
		} else {
			for i := startIdx; i < endIdx; i++ {
				leftLines = append(leftLines, renderProjectRow(i, innerLeft))
			}
			// Scroll indicators
			if startIdx > nFav {
				leftLines[projectsBlankIdx] = dimStyle.Render(fmt.Sprintf("  ▲ %d more", startIdx-nFav))
			}
			if endIdx < len(m.projects) {
				leftLines = append(leftLines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", len(m.projects)-endIdx)))
			}
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
	} else if m.mode == modePlanWait {
		content := lipgloss.JoinVertical(lipgloss.Center, "", "",
			m.spinner.View()+" "+dimStyle.Render("generating daily plan..."),
			"",
			dimStyle.Render("please wait..."),
		)
		rightContent = lipgloss.Place(rightWidth-4, panelHeight, lipgloss.Center, lipgloss.Center, content)
	} else if m.mode == modePlanResult {
		m.viewport.Width = rightWidth - 4
		m.viewport.Height = panelHeight - 2
		banner := warnStyle.Render("DAILY PLAN") + dimStyle.Render("  ·  j/k scroll  ·  any key dismiss")
		rightContent = lipgloss.JoinVertical(lipgloss.Left, banner, "", m.viewport.View())
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
	switch m.mode {
	case modeChainCleanupWait:
		return m.renderChainCleanupWait()
	case modeChainCleanupReview:
		return m.renderChainCleanupReview()
	case modeChainCleanupFeedback:
		return m.renderChainCleanupFeedback()
	case modeChainCleanupSummary:
		return m.renderChainCleanupSummary()
	case modeCleanupFeedback:
		return m.renderSingleCleanupFeedback()
	case modeCleanupWait:
		h := m.height - 8
		if h < 5 {
			h = 5
		}
		content := lipgloss.JoinVertical(lipgloss.Center, "", "",
			m.spinner.View()+" "+dimStyle.Render("regenerating with feedback..."),
			"", dimStyle.Render("please wait..."),
		)
		return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
	}
	// Single-project cleanup diff
	p := ""
	if m.selected < len(m.projects) {
		p = m.projects[m.selected].Name
	}
	banner := warnStyle.Render("CLEANUP DIFF") +
		dimStyle.Render(" — "+p+" · y accept · r feedback · n/esc discard · j/k scroll")
	return lipgloss.JoinVertical(lipgloss.Left, banner, "", m.viewport.View())
}

func (m model) renderSingleCleanupFeedback() string {
	p := ""
	if m.selected < len(m.projects) {
		p = m.projects[m.selected].Name
	}
	banner := warnStyle.Render("FEEDBACK") +
		dimStyle.Render(" — "+p+" · enter regenerate · esc cancel")
	vpH := m.height - 14
	if vpH < 5 {
		vpH = 5
	}
	m.viewport.Width = m.width - 4
	m.viewport.Height = vpH
	return lipgloss.JoinVertical(lipgloss.Left,
		banner, "",
		m.viewport.View(),
		"",
		dimStyle.Render("  What needs fixing?"),
		m.chainFeedback.View(),
	)
}

func (m model) chainProjectName() string {
	if m.chainCursor < len(m.chainQueue) {
		idx := m.chainQueue[m.chainCursor]
		if idx < len(m.projects) {
			return m.projects[idx].Name
		}
	}
	return ""
}

func (m model) renderChainCleanupWait() string {
	total := len(m.chainQueue)
	pos := m.chainCursor + 1
	h := m.height - 8
	if h < 5 {
		h = 5
	}
	progress := fmt.Sprintf("%d / %d", pos, total)
	content := lipgloss.JoinVertical(lipgloss.Center, "", "",
		warnStyle.Render("CHAIN CLEANUP")+" "+dimStyle.Render(progress),
		"",
		m.spinner.View()+" "+dimStyle.Render("cleaning: "+m.chainProjectName()),
		"",
		dimStyle.Render("please wait..."),
	)
	return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderChainCleanupReview() string {
	total := len(m.chainQueue)
	pos := m.chainCursor + 1
	progress := fmt.Sprintf("%d/%d", pos, total)
	banner := warnStyle.Render("CHAIN CLEANUP") +
		dimStyle.Render(" "+progress+" — "+m.chainProjectName()+" · y accept · n skip · r feedback · j/k scroll")
	vpH := m.height - 8
	if vpH < 5 {
		vpH = 5
	}
	m.viewport.Width = m.width - 4
	m.viewport.Height = vpH
	return lipgloss.JoinVertical(lipgloss.Left, banner, "", m.viewport.View())
}

func (m model) renderChainCleanupFeedback() string {
	total := len(m.chainQueue)
	pos := m.chainCursor + 1
	progress := fmt.Sprintf("%d/%d", pos, total)
	banner := warnStyle.Render("FEEDBACK") +
		dimStyle.Render(" "+progress+" — "+m.chainProjectName()+" · enter regenerate · esc cancel")
	vpH := m.height - 14
	if vpH < 5 {
		vpH = 5
	}
	m.viewport.Width = m.width - 4
	m.viewport.Height = vpH
	return lipgloss.JoinVertical(lipgloss.Left,
		banner, "",
		m.viewport.View(),
		"",
		dimStyle.Render("  What needs fixing?"),
		m.chainFeedback.View(),
	)
}

func (m model) renderChainCleanupSummary() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Chain Cleanup — Done"), "")
	lines = append(lines, fmt.Sprintf("  %s  %s",
		accentStyle.Render(fmt.Sprintf("%d accepted", m.chainAccepted)),
		dimStyle.Render(fmt.Sprintf("%d skipped", m.chainSkipped)),
	), "")
	for _, r := range m.chainResults {
		switch r.action {
		case "accepted":
			lines = append(lines, accentStyle.Render("  + "+r.name))
		case "skipped":
			lines = append(lines, dimStyle.Render("  - "+r.name+"  (skipped)"))
		case "error":
			lines = append(lines, warnStyle.Render("  ! "+r.name+"  (error)"))
		}
	}
	lines = append(lines, "", dimStyle.Render("  j/k scroll · any other key to continue"))

	visibleH := m.height - 8
	if visibleH < 5 {
		visibleH = 5
	}
	maxScroll := len(lines) - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.chainSummaryScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
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
		lines = append(lines, "", dimStyle.Render(strings.Repeat("─", m.width-4)))
		header := strings.Join(lines, "\n")

		listH := len(lines) + 1 // +1 for the sep line itself
		vpH := m.height - 6 - listH
		if vpH < 3 {
			vpH = 3
		}
		m.viewport.Width = m.width - 4
		m.viewport.Height = vpH

		scrollPct := ""
		if m.viewport.TotalLineCount() > vpH {
			pct := int(m.viewport.ScrollPercent() * 100)
			scrollPct = dimStyle.Render(fmt.Sprintf("  %d%%  J/K scroll · c clear", pct))
		} else {
			scrollPct = dimStyle.Render("  c clear")
		}

		return lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), scrollPct)
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
		add("f", "pin")
		add("space", "select")
		add("e", "edit")
		add("d", "dump")

		add("P", "plan")
		if len(m.selectedProjects) > 0 {
			add("C", fmt.Sprintf("cleanup (%d)", len(m.selectedProjects)))
		} else {
			add("C", "cleanup all")
		}
	case pageProject:
		add("j/k", "scroll")
		add("e", "edit")
		add("esc", "back")
	case pageCleanup:
		switch m.mode {
		case modeChainCleanupReview:
			add("y/enter", "accept")
			add("n/esc", "skip")
			add("r", "feedback")
			add("j/k", "scroll")
		case modeChainCleanupFeedback, modeCleanupFeedback:
			add("enter", "regenerate")
			add("esc", "cancel")
		case modeChainCleanupSummary:
			add("any key", "continue")
		default:
			add("y/enter", "accept")
			add("r", "feedback")
			add("n/esc", "discard")
		}
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
		if m.scriptOutput != "" {
			add("J/K", "scroll output")
			add("c", "clear")
		}
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
			{"f", "Pin / unpin project (sticky at top)"},
			{"space", "Toggle project selection (for C/P)"},
			{"e", "Edit WORK.md inline"},
			{"-", "Fix non-list lines (save in-place)"},
			{"c", "Cleanup via ollama (single)"},
			{"C", "Chain cleanup selected (or all)"},
			{"P", "Daily plan via ollama (grouped tasks)"},
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
			{"j/k", "Navigate scripts"},
			{"enter", "Run script"},
			{"J/K", "Scroll output"},
			{"c", "Clear output"},
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
