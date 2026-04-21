package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/llm"
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

	providerStatus := m.cfg.ActiveProviderStatus()
	providerBadge := m.renderProviderBadge(providerStatus)

	// Right side: project-specific info on project page, global stats otherwise
	var right string
	if m.page == pageProject && m.selected < len(m.projects) {
		p := m.projects[m.selected]
		var parts []string
		parts = append(parts, providerBadge)
		parts = append(parts, panelHeaderStyle.Render(p.Name))
		parts = append(parts, currentStyle.Render(fmt.Sprintf("%d current", p.CurrentCount)))
		if p.BugsCount > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("%d bugs", p.BugsCount)))
		}
		if p.UnsortedCount > 0 {
			parts = append(parts, unsortedStyle.Render(fmt.Sprintf("%d unsorted", p.UnsortedCount)))
		}
		parts = append(parts, backlogStyle.Render(fmt.Sprintf("%d backlog", p.BacklogCount)))
		right = strings.Join(parts, dimStyle.Render(" · "))
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
		parts = append(parts, providerBadge)
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d files", len(m.projects))))
		parts = append(parts, currentStyle.Render(fmt.Sprintf("%d current", totalCur)))
		if totalBugs > 0 {
			parts = append(parts, warnStyle.Render(fmt.Sprintf("%d bugs", totalBugs)))
		}
		if totalUnsorted > 0 {
			parts = append(parts, unsortedStyle.Render(fmt.Sprintf("%d unsorted", totalUnsorted)))
		}
		parts = append(parts, backlogStyle.Render(fmt.Sprintf("%d backlog", totalBacklog)))
		right = strings.Join(parts, dimStyle.Render(" · "))
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}

	return left + strings.Repeat(" ", gap) + right
}

func (m model) renderProviderBadge(status config.ProviderStatus) string {
	if !status.Enabled {
		if status.Problem == "" {
			return warnStyle.Render("no llm provider enabled")
		}
		return warnStyle.Render("llm disabled") + dimStyle.Render(" ("+status.Problem+")")
	}

	label := status.Name
	if label == "" {
		label = status.Type
	}
	if status.Model != "" {
		label += ":" + status.Model
	}
	return accentStyle.Render("llm") + dimStyle.Render("=") + dimStyle.Render(label)
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
	innerLeft := leftWidth - 4 // border + padding

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
		// Counts are right-aligned to the panel edge so columns stay consistent.
		// Cursor row gets a subtle background across the full width.
		renderProjectRow := func(i int, innerW int) string {
			p := m.projects[i]
			isCursor := i == m.cursor
			const prefixW = 2

			// bg applies cursor background to any style
			bg := func(s lipgloss.Style) lipgloss.Style {
				if isCursor {
					return s.Background(colorCursorBg)
				}
				return s
			}

			styledPrefix := bg(dimStyle).Render("  ")
			if isCursor {
				styledPrefix = bg(accentStyle).Render("▸ ")
			}

			// Build styled counts + raw string for width calculation
			sep := bg(dimStyle).Render("/")
			var styledParts []string
			var rawParts []string
			if m.selectedProjects[p.Path] {
				styledParts = append(styledParts, bg(accentStyle).Render("●"))
				rawParts = append(rawParts, "●")
			}
			if p.CurrentCount > 0 {
				styledParts = append(styledParts, bg(currentStyle).Render(fmt.Sprintf("%d", p.CurrentCount)))
				rawParts = append(rawParts, fmt.Sprintf("%d", p.CurrentCount))
			}
			if p.UnsortedCount > 0 {
				styledParts = append(styledParts, bg(unsortedStyle).Render(fmt.Sprintf("%d", p.UnsortedCount)))
				rawParts = append(rawParts, fmt.Sprintf("%d", p.UnsortedCount))
			}
			if p.BugsCount > 0 {
				styledParts = append(styledParts, bg(warnStyle).Render(fmt.Sprintf("%d", p.BugsCount)))
				rawParts = append(rawParts, fmt.Sprintf("%d", p.BugsCount))
			}
			if p.BacklogCount > 0 {
				styledParts = append(styledParts, bg(backlogStyle).Render(fmt.Sprintf("%d", p.BacklogCount)))
				rawParts = append(rawParts, fmt.Sprintf("%d", p.BacklogCount))
			}

			nameStyle := bg(textStyle)
			if isCursor {
				nameStyle = bg(accentStyle).Bold(true)
			}

			if len(rawParts) == 0 {
				rawName := xansi.Truncate(p.Name, innerW-prefixW, "")
				gap := innerW - prefixW - len(rawName)
				return styledPrefix + nameStyle.Render(rawName) + bg(lipgloss.NewStyle()).Render(strings.Repeat(" ", gap))
			}

			styledSuffix := strings.Join(styledParts, sep)
			rawSuffix := strings.Join(rawParts, "/")
			suffixW := len(rawSuffix)

			// Right-align suffix: name fills left, counts flush to right edge
			nameAvail := innerW - prefixW - 1 - suffixW
			if nameAvail < 3 {
				nameAvail = 3
			}
			rawName := xansi.Truncate(p.Name, nameAvail, "")

			gap := innerW - prefixW - len(rawName) - suffixW
			if gap < 1 {
				gap = 1
			}
			return styledPrefix + nameStyle.Render(rawName) + bg(lipgloss.NewStyle()).Render(strings.Repeat(" ", gap)) + styledSuffix
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
		label := "model cleaning up..."
		if m.mode == modeTodoWait {
			label = "asking model what to work on..."
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
			m.spinner.View()+" "+dimStyle.Render("model cleaning up..."),
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
	hints := dimStyle.Render("  ctrl+d del line · ctrl+k del to eol · home/end line start/end")
	return lipgloss.JoinVertical(lipgloss.Left, header, hints, "", m.editArea.View())
}

// --- Brain dump ---

func (m model) renderDump() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Brain Dump"), "")

	switch m.mode {
	case modeDumpSummary:
		return m.renderDumpSummary()
	case modeDumpRouting:
		lines = append(lines, "  "+m.spinner.View()+" "+dimStyle.Render("routing via model..."))
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

func isDumpSkipped(item llm.RouteItem, skipped []llm.RouteItem) bool {
	for _, s := range skipped {
		if s.Text == item.Text {
			return true
		}
	}
	return false
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
		add("↑/↓", "nav")
		add("o", "open")
		add("f", "pin")
		add("space", "select")
		add("e", "edit")
		add("d", "dump")

		add("P", "plan")
		if len(m.selectedProjects) > 0 {
			add("C", fmt.Sprintf("cleanup (%d)", len(m.selectedProjects)))
		} else {
			add("c/C", "cleanup selected/all")
		}
	case pageProject:
		add("↑/↓", "scroll")
		add("e", "edit")
		add("esc", "back")
	case pageCleanup:
		switch m.mode {
		case modeChainCleanupReview:
			add("y/enter", "accept")
			add("n/esc", "skip")
			add("r", "feedback")
			add("↑/↓", "scroll")
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
			{"↑/↓", "Navigate projects"},
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
			{"c", "Cleanup via model (single)"},
			{"C", "Chain cleanup selected (or all)"},
			{"P", "Daily plan via model (grouped tasks)"},
			{"t", "Ask model what to work on"},
			{"o", "Open project directory in editor"},
			{"y", "Copy project dir path to clipboard"},
			{"d", "Brain dump"},
			{"/", "Search across all WORK.md files"},
			{"r", "Refresh (re-scan WORK.md files)"},
			{",", "Open config.json in editor"},
			{"?", "Toggle this help overlay"},
		}},
		{"Project View", []struct{ key, desc string }{
			{"↑/↓", "Scroll"},
			{"g / G", "Jump to top / bottom"},
			{"pgup/pgdn", "Full-page scroll"},
			{"ctrl+home/end", "Top / bottom"},
			{"e", "Edit inline"},
			{"ctrl+s", "Save edits"},
			{"c", "Cleanup via model (normalizes format)"},
			{"esc", "Back / cancel edit"},
		}},
		{"Edit Mode", []struct{ key, desc string }{
			{"ctrl+s", "Save"},
			{"esc", "Cancel"},
			{"home", "Start of line"},
			{"end / ctrl+e", "End of line"},
			{"ctrl+d", "Delete current line"},
			{"ctrl+k", "Delete to end of line"},
		}},
		{"Brain Dump", []struct{ key, desc string }{
			{"ctrl+d", "Route dump via model (splits into items)"},
			{"y/enter", "Accept routed item"},
			{"n", "Skip item"},
			{"esc", "Cancel / abort remaining"},
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

	// Width adapts to terminal: wide enough for longest line, capped so it doesn't
	// stretch across a huge screen. On narrow terminals, shrink to fit.
	dialogW := 72
	if m.width-8 < dialogW {
		dialogW = m.width - 8
	}
	if dialogW < 40 {
		dialogW = 40
	}
	dialog := dialogStyle.Width(dialogW).Render(strings.Join(visible, "\n") + scrollHint)
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
