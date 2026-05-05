package tui

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

func wrapLine(s string, width int) string {
	if width < 4 {
		return truncate(s, width)
	}
	var wrapped []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, strings.Split(xansi.Wordwrap(line, width, " "), "\n")...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLines(s string, width int) []string {
	if s == "" {
		return nil
	}
	return strings.Split(wrapLine(s, width), "\n")
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
	case pageAgent:
		content = m.renderAgent()
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

func (m model) hasStatusLine() bool {
	return m.statusMsg != "" && time.Now().Before(m.statusExpiry)
}

func (m model) contentAreaHeight() int {
	// Global chrome: header, top separator, bottom separator, footer,
	// plus an optional transient status row.
	height := m.height - 4
	if m.hasStatusLine() {
		height--
	}
	if height < 1 {
		return 1
	}
	return height
}

func (m model) contentViewportHeight(prefixLines int) int {
	height := m.contentAreaHeight() - prefixLines
	if height < 1 {
		return 1
	}
	return height
}

// --- Header ---

func (m model) renderHeader() string {
	title := titleStyle.Render("sb")

	var tabs []string
	for i, pg := range topNavPages() {
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

	var right string
	switch {
	case m.page == pageAgent:
		// Agent page owns its own status rows; no global llm/project stats here.
	case m.page == pageProject && m.selected < len(m.projects):
		p := m.projects[m.selected]
		parts := []string{m.renderProviderBadge(m.cfg.ActiveProviderStatus())}
		parts = append(parts, panelHeaderStyle.Render(p.Name))
		parts = append(parts, currentStyle.Render(fmt.Sprintf("%d current", p.CurrentCount)))
		parts = append(parts, backlogStyle.Render(fmt.Sprintf("%d backlog", p.BacklogCount)))
		right = strings.Join(parts, dimStyle.Render(" · "))
	default:
		var totalCur, totalBacklog int
		for _, p := range m.projects {
			totalCur += p.CurrentCount
			totalBacklog += p.BacklogCount
		}
		parts := []string{m.renderProviderBadge(m.cfg.ActiveProviderStatus())}
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d files", len(m.projects))))
		parts = append(parts, currentStyle.Render(fmt.Sprintf("%d current", totalCur)))
		parts = append(parts, backlogStyle.Render(fmt.Sprintf("%d backlog", totalBacklog)))
		right = strings.Join(parts, dimStyle.Render(" · "))
	}

	maxRight := m.width - lipgloss.Width(left) - 2
	if maxRight < 12 {
		maxRight = 12
	}
	right = truncate(right, maxRight)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}

	return left + strings.Repeat(" ", gap) + right
}

func topNavPages() []struct {
	name string
	p    page
} {
	return []struct {
		name string
		p    page
	}{
		{"Dashboard", pageDashboard},
		{"Dump", pageDump},
		{"Agents", pageAgent},
	}
}

func (m model) headerTabAt(x, y int) (page, bool) {
	if y != 0 {
		return 0, false
	}

	cursor := lipgloss.Width(titleStyle.Render("sb")) + 2
	sepWidth := lipgloss.Width(dimStyle.Render(" │ "))
	for i, tab := range topNavPages() {
		if i > 0 {
			cursor += sepWidth
		}
		tabWidth := lipgloss.Width(tab.name)
		if x >= cursor && x < cursor+tabWidth {
			return tab.p, true
		}
		cursor += tabWidth
	}
	return 0, false
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
		return m.renderEmpty("No task files found", "Check your configured scan roots")
	}

	availableHeight := m.contentAreaHeight()
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
		condensedPinned := m.dashboardCondensePinned()
		if condensedPinned {
			nFav = 0
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
			header := panelHeaderStyle.Render("Projects")
			if condensedPinned {
				header = lipgloss.JoinHorizontal(lipgloss.Left, header, dimStyle.Render("  ·  pinned condensed"))
			}
			leftLines[projectsHeaderIdx] = header
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
	} else if m.mode == modeCleanupWait && m.selected == m.cursor {
		label := "model cleaning up..."
		content := lipgloss.JoinVertical(lipgloss.Center, "", "",
			m.spinner.View()+" "+dimStyle.Render(label),
			"",
			dimStyle.Render("please wait..."),
		)
		rightContent = lipgloss.Place(rightWidth-4, panelHeight, lipgloss.Center, lipgloss.Center, content)
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
		h := m.contentAreaHeight()
		content := lipgloss.JoinVertical(lipgloss.Center, "", "",
			m.spinner.View()+" "+dimStyle.Render("model cleaning up..."),
			"",
			dimStyle.Render("please wait, this may take a minute."),
		)
		return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
	}
	m.viewport.Width = m.width - 4
	m.viewport.Height = m.contentAreaHeight()
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
		h := m.contentAreaHeight()
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
	m.viewport.Width = m.width - 4
	m.viewport.Height = m.contentViewportHeight(2)
	banner := warnStyle.Render("CLEANUP DIFF") +
		dimStyle.Render(" — "+p)
	return lipgloss.JoinVertical(lipgloss.Left, banner, "", m.viewport.View())
}

func (m model) renderSingleCleanupFeedback() string {
	p := ""
	if m.selected < len(m.projects) {
		p = m.projects[m.selected].Name
	}
	banner := warnStyle.Render("FEEDBACK") +
		dimStyle.Render(" — "+p)
	m.viewport.Width = m.width - 4
	inputHeight := lipgloss.Height(m.chainFeedback.View())
	vpH := m.contentViewportHeight(4 + inputHeight)
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
	h := m.contentAreaHeight()
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
		dimStyle.Render(" "+progress+" — "+m.chainProjectName())
	m.viewport.Width = m.width - 4
	vpH := m.contentViewportHeight(2)
	m.viewport.Height = vpH
	return lipgloss.JoinVertical(lipgloss.Left, banner, "", m.viewport.View())
}

func (m model) renderChainCleanupFeedback() string {
	total := len(m.chainQueue)
	pos := m.chainCursor + 1
	progress := fmt.Sprintf("%d/%d", pos, total)
	banner := warnStyle.Render("FEEDBACK") +
		dimStyle.Render(" "+progress+" — "+m.chainProjectName())
	m.viewport.Width = m.width - 4
	inputHeight := lipgloss.Height(m.chainFeedback.View())
	vpH := m.contentViewportHeight(4 + inputHeight)
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
	lines := m.chainCleanupSummaryLines()
	visibleH := m.summaryVisibleHeight()
	scroll := clampScrollOffset(m.chainSummaryScroll, len(lines), visibleH)
	end := scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

func (m model) chainCleanupSummaryLines() []string {
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
	lines = append(lines, "")
	return lines
}

func (m model) renderEditMode() string {
	header := warnStyle.Render("EDITING")
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
			lines = append(lines, accentStyle.Render("  "+item.Text))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("  route to: ")+
				accentStyle.Render(item.Project)+dimStyle.Render(" / ")+accentStyle.Render(item.Section))
		}
		return strings.Join(lines, "\n")

	case modeDumpClarify:
		if m.dumpCursor < len(m.dumpItems) {
			item := m.dumpItems[m.dumpCursor]
			progress := fmt.Sprintf("(%d/%d)", m.dumpCursor+1, len(m.dumpItems))

			lines = append(lines, warnStyle.Render("  CLARIFY ")+dimStyle.Render(progress))
			lines = append(lines, accentStyle.Render("  "+item.Text))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("  which project does this belong to?"))
			m.dumpClarifyArea.SetWidth(m.width - 6)
			m.dumpClarifyArea.SetHeight(m.contentViewportHeight(6))
			lines = append(lines, m.dumpClarifyArea.View())
		}
		return strings.Join(lines, "\n")
	}

	// modeDumpInput (default)
	m.dumpArea.SetWidth(m.width - 6)
	m.dumpArea.SetHeight(m.contentViewportHeight(2))
	lines = append(lines, m.dumpArea.View())
	return strings.Join(lines, "\n")
}

func (m model) renderDumpSummary() string {
	lines := m.dumpSummaryLines()
	visibleH := m.summaryVisibleHeight()
	scroll := clampScrollOffset(m.dumpSummaryScroll, len(lines), visibleH)
	end := scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

func (m model) dumpSummaryLines() []string {
	var lines []string
	lines = append(lines, titleStyle.Render("Brain Dump — Done"), "")

	accepted := fmt.Sprintf("%d routed", m.dumpAccepted)
	skipped := fmt.Sprintf("%d skipped", m.dumpSkipped)
	lines = append(lines, "  "+accentStyle.Render(accepted)+"  "+dimStyle.Render(skipped), "")

	if m.dumpAccepted > 0 {
		lines = append(lines, primaryStyle.Render("  Routed:"))
		for _, it := range m.dumpItems {
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

	lines = append(lines, "")
	return lines
}

func (m model) summaryVisibleHeight() int {
	return m.contentAreaHeight()
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

	// inInput is true when the focused widget accepts text (q/esc would be typed, not navigation).
	inInput := false

	switch m.page {
	case pageDashboard:
		add("↑/↓", "nav")
		add("enter", "open")
		add("d", "dump")
		add("a", "agent")
		add("e", "edit")
		add("f", "pin")
		add("c/C", "cleanup")
	case pageProject:
		add("↑/↓", "scroll")
		add("e", "edit")
	case pageCleanup:
		switch m.mode {
		case modeChainCleanupReview:
			add("y", "accept")
			add("n", "skip")
			add("r", "feedback")
		case modeChainCleanupFeedback, modeCleanupFeedback:
			add("enter", "regenerate")
			add("esc", "cancel")
			inInput = true
		case modeChainCleanupSummary:
			add("any key", "continue")
		default:
			add("y", "accept")
			add("r", "feedback")
			add("n", "discard")
		}
	case pageDump:
		switch m.mode {
		case modeDumpReview:
			add("y", "accept")
			add("n", "skip")
			add("r", "reroute")
		case modeDumpClarify:
			add("enter", "reroute")
			add("esc", "skip")
			inInput = true
		case modeDumpInput:
			add("ctrl+d", "route")
			add("esc", "back")
			inInput = true
		default:
			add("ctrl+d", "route")
		}
	case pageAgent:
		switch m.mode {
		case modeAgentPicker:
			add("space", "toggle")
			add("enter", "continue")
		case modeAgentLaunch:
			add("tab", "focus")
			if m.launchFocus == m.launchReviewFocus() {
				add("enter", "launch")
			} else {
				add("enter", "continue")
			}
			add("alt+enter", "launch")
			add("a", "toggle advanced")
			add("ctrl+t", "toggle Foreman")
			if m.launchFocus == m.launchNoteFocus() {
				inInput = true
			}
		case modeAgentAttached:
			if m.attachedFocus == 1 {
				add("enter", "send")
				add("esc", "leave input")
				add("ctrl+c", "back to jobs")
				add("alt+enter", "newline")
				add("pgup/pgdn", "scroll")
				inInput = true
			} else {
				add("tab/i", "type")
				add("ctrl+c", "back to jobs")
				add("s", "send Esc")
				add("S", "send Ctrl+C")
				add("c", "send continue")
			}
		case modeAgentManage:
			if m.agentManageHookEditing {
				if m.agentManageEditing {
					if spec, ok := m.agentManageHookCurrentFieldSpec(); ok && !spec.Multiline {
						add("enter", "save field")
					}
					add("ctrl+s", "save field")
					add("esc", "cancel field")
					inInput = true
				} else if m.agentManageSelectEditing {
					add("enter", "save field")
					add("esc", "cancel field")
					inInput = true
				} else {
					add("tab", "focus")
					add("enter", "edit / open")
					add("a", "add row")
					add("d", "delete row")
					add("D", "duplicate row")
					add("[/]", "move row")
					add("ctrl+s", "save bundle")
				}
			} else if m.agentManageEditing {
				if spec, ok := m.currentAgentManageFieldSpec(); ok && !spec.Multiline {
					add("enter", "save")
				}
				add("ctrl+s", "save")
				add("esc", "cancel")
				inInput = true
			} else if m.agentManageSelectEditing {
				add("enter", "save")
				add("esc", "cancel")
				inInput = true
			} else {
				add("tab", "focus")
				add("enter", "edit")
				add("n", "new")
				add("d", "delete")
				add("D", "duplicate")
				add("[/]", "switch kind")
			}
		default:
			add("↑/↓", "nav")
			add("n", "new")
			add("enter", "open")
		}
	}

	add("?", "help")
	if !inInput {
		switch {
		case m.page == pageDashboard && m.mode == modeNormal:
			if m.cockpitDetachQuit {
				add("esc/q", "detach")
			} else {
				add("esc/q", "quit")
			}
		default:
			add("esc/q", "back")
		}
	}

	return " " + strings.Join(parts, "")
}

// --- Help ---

func (m model) renderHelp() string {
	lines := m.helpLines()
	dialogW := m.helpDialogWidth()
	visibleH := m.helpVisibleHeight()
	scroll := clampScrollOffset(m.helpScroll, len(lines), visibleH)
	end := scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scroll:end]

	dialog := dialogStyle.Width(dialogW).Render(strings.Join(visible, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

func (m model) helpLines() []string {
	sections := []struct {
		title string
		keys  []struct{ key, desc string }
	}{
		{"Dashboard", []struct{ key, desc string }{
			{"↑/↓", "Navigate projects"},
			{"pgup/pgdn", "Page through project list"},
			{"home/end", "Jump to top / bottom of list"},
			{"g / G", "Jump to top / bottom of list"},
			{"J/K", "Scroll WORK.md preview"},
			{"ctrl+d/u", "Half-page scroll preview"},
			{"ctrl+b/f", "Full-page scroll preview"},
			{"ctrl+home/end", "Preview top / bottom"},
			{"enter", "Full-screen project view"},
			{"f", "Pin / unpin project (sticky at top)"},
			{"space", "Toggle project selection (for C)"},
			{"e", "Edit WORK.md inline"},
			{"-", "Fix non-list lines (save in-place)"},
			{"c", "Cleanup via model (single)"},
			{"C", "Chain cleanup selected (or all)"},
			{"o", "Open project directory in editor"},
			{"y", "Copy project dir path to clipboard"},
			{"d", "Brain dump"},
			{"a", "Agents"},
			{"A", "Open current project directly in Agent task picker"},
			{"/", "Search across all WORK.md files"},
			{"r", "Refresh (re-scan WORK.md files)"},
			{",", "Open sb config dir in editor"},
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
			{"ctrl+home/end", "Top / bottom of file"},
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
		{"Agents", []struct{ key, desc string }{
			{"n", "New run (pick a task file or skip task lines)"},
			{"m", "Open Advanced Setup (roles, prompts, hooks, engines)"},
			{"F", "List: toggle Foreman on/off"},
			{"ctrl+t", "New run: toggle immediate launch vs send to Foreman"},
			{"f / tab", "List: cycle job filters"},
			{"space", "Toggle task in picker"},
			{"b", "Picker: back to file list"},
			{"tab", "New run: cycle role → engine → repo → note → review (advanced overrides optional) · Advanced Setup: cycle item/field focus, then groups · Attached exec-chat: swap transcript ↔ input"},
			{"enter", "New run: continue, note → review, then launch from review · Advanced Setup: open/save overlay editor · open selected job · send when typing"},
			{"alt+enter", "Launch directly from note"},
			{"a", "New run: show/hide prompt, hook, and permission overrides"},
			{"i", "List: attach/focus selected job (tmux attach while live) · Attached exec-chat: focus input"},
			{"ctrl+g", "Live tmux session: jump back to the shared sb main window"},
			{"ctrl+c", "Attached view: return to the jobs list"},
			{"ctrl+r", "Take over eligible Foreman tmux job and relaunch it in attended mode (confirm)"},
			{"R", "Reopen the selected job in New Run with its prior settings prefilled"},
			{"j/k", "Scroll transcript/activity in attached view"},
			{"wheel", "List nav / transcript scroll"},
			{"a", "Accept reviewed result (confirm)"},
			{"r", "Retry selected job immediately with the same setup/runtime"},
			{"K", "Skip queued/reviewed job (confirm)"},
			{"C", "Skip current item and the rest of its queued run sequence (confirm)"},
			{"s", "Interrupt the current turn; keep the session available to re-enter"},
			{"d", "Delete job (confirm)"},
			{"esc", "Back"},
		}},
	}

	dialogW := m.helpDialogWidth()
	innerW := dialogW - dialogStyle.GetHorizontalFrameSize()
	if innerW < 20 {
		innerW = 20
	}
	const (
		helpRowIndent = "  "
		helpKeyColW   = 14
		helpColGap    = "  "
	)
	descW := innerW - lipgloss.Width(helpRowIndent) - helpKeyColW - lipgloss.Width(helpColGap)
	if descW < 18 {
		descW = 18
	}

	keyColStyle := lipgloss.NewStyle().
		Foreground(colorAccent).
		Width(helpKeyColW)
	descColStyle := textStyle.Width(descW)

	var lines []string
	lines = append(lines, titleStyle.Render("Help"), "")

	for _, sec := range sections {
		lines = append(lines, keyStyle.Render(sec.title))
		for _, k := range sec.keys {
			row := lipgloss.JoinHorizontal(
				lipgloss.Top,
				helpRowIndent,
				keyColStyle.Render(k.key),
				helpColGap,
				descColStyle.Render(k.desc),
			)
			lines = append(lines, strings.Split(row, "\n")...)
		}
		lines = append(lines, "")
	}
	return lines
}

func (m model) helpDialogWidth() int {
	dialogW := m.width - 8
	if dialogW > 96 {
		dialogW = 96
	}
	if dialogW < 40 {
		dialogW = 40
	}
	return dialogW
}

func (m model) helpVisibleHeight() int {
	visibleH := m.height - 10
	if visibleH < 5 {
		visibleH = 5
	}
	return visibleH
}

// --- Empty state ---

func (m model) renderEmpty(title, subtitle string) string {
	content := lipgloss.JoinVertical(lipgloss.Center, "", "", dimStyle.Render(title), "", dimStyle.Render(subtitle))
	h := m.contentAreaHeight()
	return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
}
