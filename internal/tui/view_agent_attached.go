package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
)

func (m model) attachedExecDims() (width, panelHeight int) {
	width = m.width
	if width < 40 {
		width = 40
	}
	panelHeight = m.agentContentHeight()
	if panelHeight < 1 {
		panelHeight = 1
	}
	return width, panelHeight
}

func (m model) attachedLayoutDims() (railWidth, chatWidth, panelHeight int) {
	railWidth = m.width * 31 / 100
	if railWidth < 32 {
		railWidth = 32
	}
	chatWidth = m.width - railWidth - 5
	if chatWidth < 36 {
		chatWidth = 36
		railWidth = m.width - chatWidth - 5
		if railWidth < 24 {
			railWidth = 24
		}
	}
	panelHeight = m.agentContentHeight() - 2
	if panelHeight < 1 {
		panelHeight = 1
	}
	return railWidth, chatWidth, panelHeight
}

func (m model) attachedTranscriptWidth() int {
	_, chatWidth, _ := m.attachedLayoutDims()
	width := chatWidth - 6
	if width < 20 {
		width = 20
	}
	return width
}

func (m model) attachedExecTranscriptWidth() int {
	width, _ := m.attachedExecDims()
	content := width - 6
	if content < 24 {
		content = 24
	}
	return content
}

func (m model) attachedExecInputWidth() int {
	width, _ := m.attachedExecDims()
	content := width - 8
	if content < 20 {
		content = 20
	}
	return content
}

func (m model) attachedInputWidth() int {
	_, chatWidth, _ := m.attachedLayoutDims()
	width := chatWidth - 8
	if width < 20 {
		width = 20
	}
	return width
}

func (m *model) attachedExecHeaderLines(j cockpit.Job, lineWidth int) []string {
	statusText, statusStyle := jobOperatorStatus(j)

	var lines []string
	header := titleStyle.Render("Run")
	header += dimStyle.Render("  ·  ")
	header += primaryStyle.Bold(true).Render(j.PresetID)
	header += dimStyle.Render("  ·  ")
	header += statusStyle.Render(statusText)
	lines = append(lines, wrapLines(header, lineWidth)...)

	meta := []string{
		dimStyle.Render(shortJobID(j.ID)),
		dimStyle.Render("· " + describeExecutor(j.Executor)),
	}
	if age := time.Since(j.CreatedAt).Round(time.Second); age > 0 {
		meta = append(meta, dimStyle.Render("· "+age.String()+" ago"))
	}
	if j.Note != "" {
		meta = append(meta, dimStyle.Render("· "+j.Note))
	}
	if len(j.Sources) > 0 {
		source := fmt.Sprintf("source %s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
		if len(j.Sources) > 1 {
			source += fmt.Sprintf(" (+%d)", len(j.Sources)-1)
		}
		meta = append(meta, dimStyle.Render("· "+source))
	}
	lines = append(lines, wrapLines("  "+strings.Join(meta, " "), lineWidth)...)

	scrollMeta := dimStyle.Render(fmt.Sprintf("%d turns", countUserVisibleTurns(j)))
	if m.viewport.TotalLineCount() > m.viewport.Height && m.viewport.Height > 0 {
		visibleEnd := m.viewport.YOffset + m.viewport.Height
		if visibleEnd > m.viewport.TotalLineCount() {
			visibleEnd = m.viewport.TotalLineCount()
		}
		scrollMeta += dimStyle.Render(fmt.Sprintf("  ·  %d-%d/%d", m.viewport.YOffset+1, visibleEnd, m.viewport.TotalLineCount()))
	}
	if m.attachedFocus == 0 {
		lines = append(lines, wrapLines(accentStyle.Render("▸ transcript")+"  "+scrollMeta, lineWidth)...)
	} else {
		lines = append(lines, wrapLines(dimStyle.Render("transcript")+"  "+scrollMeta+"  "+accentStyle.Render("▸ composer"), lineWidth)...)
	}
	return lines
}

func (m *model) attachedExecFooterLines(j cockpit.Job, panelHeight, innerHeight, lineWidth int) []string {
	isLive := j.Status != cockpit.StatusCompleted &&
		j.Status != cockpit.StatusFailed &&
		j.Status != cockpit.StatusBlocked
	turnInFlight := j.Status == cockpit.StatusRunning

	composerWidth := m.attachedExecInputWidth()
	m.attachedInput.SetWidth(composerWidth)
	border := colorDim
	if m.attachedFocus == 1 && isLive && !turnInFlight {
		border = colorAccent
	}

	var footerLines []string
	if isLive {
		maxInputHeight := innerHeight - 6
		switch {
		case panelHeight <= 10:
			maxInputHeight = 2
		case panelHeight <= 13 && maxInputHeight > 3:
			maxInputHeight = 3
		case maxInputHeight > 5:
			maxInputHeight = 5
		}
		if maxInputHeight < 2 {
			maxInputHeight = 2
		}
		m.attachedInput.SetHeight(maxInputHeight)
		composer := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1).
			Render(m.attachedInput.View())
		label := accentStyle.Render("message")
		if turnInFlight {
			label = dimStyle.Render("message  ·  waiting for reply")
		}
		footerLines = append(footerLines, wrapLines(label, lineWidth)...)
		footerLines = append(footerLines, strings.Split(composer, "\n")...)
		return footerLines
	}

	return append(footerLines, wrapLines(dimStyle.Render("run ended"), lineWidth)...)
}

func (m *model) attachedExecViewportDims(j cockpit.Job) (contentWidth, viewportHeight int, separator bool) {
	width, panelHeight := m.attachedExecDims()
	lineWidth := maxInt(24, width-2)
	contentWidth = maxInt(24, width-4)
	headerLines := m.attachedExecHeaderLines(j, lineWidth)
	footerLines := m.attachedExecFooterLines(j, panelHeight, panelHeight, lineWidth)

	viewportHeight = panelHeight - len(headerLines) - len(footerLines)
	if len(footerLines) > 0 && viewportHeight > 4 {
		separator = true
		viewportHeight--
	}
	if viewportHeight < 3 {
		separator = false
		viewportHeight = panelHeight - len(headerLines) - len(footerLines)
	}
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	return contentWidth, viewportHeight, separator
}

func (m *model) attachedPanelHeights(j cockpit.Job, panelHeight, headerLines int) (innerHeight, footerLines, viewportHeight int) {
	innerHeight = panelHeight

	isLive := j.Status != cockpit.StatusCompleted &&
		j.Status != cockpit.StatusFailed &&
		j.Status != cockpit.StatusBlocked
	isTmux := j.Runner == cockpit.RunnerTmux

	m.attachedInput.SetWidth(m.attachedInputWidth())
	footerLines = 1
	if !isTmux && isLive {
		inputHeight := lipgloss.Height(m.attachedInput.View())
		if inputHeight < 1 {
			inputHeight = 1
		}
		maxInputHeight := innerHeight - headerLines - 2 // input label + at least one viewport row
		if maxInputHeight < 1 {
			maxInputHeight = 1
		}
		if inputHeight > maxInputHeight {
			inputHeight = maxInputHeight
		}
		m.attachedInput.SetHeight(inputHeight)
		footerLines = 1 + lipgloss.Height(m.attachedInput.View())
	}

	viewportHeight = innerHeight - headerLines - footerLines
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	return innerHeight, footerLines, viewportHeight
}

func (m model) renderAgentAttached() string {
	j, ok := m.cockpitClient.GetJob(m.attachedJobID)
	if !ok {
		return titleStyle.Render("Attached") + "\n\n  " + warnStyle.Render("job gone")
	}
	if j.Runner != cockpit.RunnerTmux {
		return m.renderAttachedExecChat(j)
	}
	return m.renderAttachedTmux(j)
}

func (m model) renderAttachedTmux(j cockpit.Job) string {
	// A live job is one the user can still send turns to. Completed /
	// failed / blocked conversations are dead — only scrollback + delete.
	isLive := j.Status != cockpit.StatusCompleted &&
		j.Status != cockpit.StatusFailed &&
		j.Status != cockpit.StatusBlocked

	railWidth, chatWidth, panelHeight := m.attachedLayoutDims()
	lineWidth := maxInt(1, chatWidth-4)

	rail := m.renderAttachedRail(railWidth, panelHeight)

	var lines []string
	statusText, statusStyle := jobOperatorStatus(j)
	header := titleStyle.Render("Run: ") + j.PresetID + "  " + statusStyle.Render(statusText)
	header += dimStyle.Render("  " + describeExecutor(j.Executor))
	lines = append(lines, wrapLines(header, lineWidth)...)

	meta := []string{dimStyle.Render("  " + string(j.ID)), dimStyle.Render("· " + describeRunner(j.Runner))}
	if age := time.Since(j.CreatedAt).Round(time.Second); age > 0 {
		meta = append(meta, dimStyle.Render("· "+age.String()+" ago"))
	}
	if !j.FinishedAt.IsZero() && !isLive {
		meta = append(meta, dimStyle.Render(fmt.Sprintf("· exit %d", j.ExitCode)))
	}
	if j.Note != "" {
		meta = append(meta, dimStyle.Render("· "+j.Note))
	}
	meta = append(meta, dimStyle.Render("· "+tmuxWindowState(j)))
	lines = append(lines, wrapLines(strings.Join(meta, " "), lineWidth)...)

	if len(j.Sources) > 0 {
		preview := fmt.Sprintf("  sources: %s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
		if len(j.Sources) > 1 {
			preview += fmt.Sprintf(" (+%d more)", len(j.Sources)-1)
		}
		lines = append(lines, wrapLines(dimStyle.Render(preview), lineWidth)...)
	}

	if reviewLines := m.renderPeekReviewSummary(j, lineWidth); len(reviewLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, reviewLines...)
	}

	turnCount := countUserVisibleTurns(j)
	sectionLabel := "activity"
	sectionMeta := strings.Join([]string{fmt.Sprintf("%d turns", turnCount), tmuxActivitySourceLabel(j)}, "  ·  ")
	if panelHeight > 8 {
		lines = append(lines, "")
	}
	if m.attachedFocus == 0 {
		lines = append(lines, wrapLines(accentStyle.Render("  ▸ "+sectionLabel)+dimStyle.Render("  "+sectionMeta), lineWidth)...)
	} else {
		lines = append(lines, wrapLines(dimStyle.Render("  "+sectionLabel+"  "+sectionMeta)+"  "+accentStyle.Render("▸ input"), lineWidth)...)
	}

	innerHeight := panelHeight
	m.attachedInput.SetWidth(m.attachedInputWidth())
	var footerLines []string
	if isLive {
		footerLines = append(footerLines, truncate(accentStyle.Render("  live tmux run"), lineWidth))
	} else {
		footerLines = append(footerLines, truncate(dimStyle.Render("  run ended"), lineWidth))
	}
	h := innerHeight - len(lines) - len(footerLines)
	if h < 1 {
		h = 1
	}
	m.viewport.Width = chatWidth - 6
	m.viewport.Height = h
	viewportLines := strings.Split(m.viewport.View(), "\n")
	lines = append(lines, capLines(viewportLines, h)...)
	lines = append(lines, footerLines...)

	chat := panelStyle.Width(chatWidth).Height(panelHeight).Render(strings.Join(lines, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, rail, "  ", chat)
}

func (m model) renderAttachedExecChat(j cockpit.Job) string {
	width, panelHeight := m.attachedExecDims()
	lineWidth := maxInt(24, width-2)
	contentWidth, viewportHeight, separator := m.attachedExecViewportDims(j)
	lines := m.attachedExecHeaderLines(j, lineWidth)
	footerLines := m.attachedExecFooterLines(j, panelHeight, panelHeight, lineWidth)

	followBottom := m.viewport.AtBottom()
	m.viewport.Width = contentWidth
	m.viewport.Height = viewportHeight
	if followBottom {
		m.viewport.GotoBottom()
	}
	viewportLines := strings.Split(m.viewport.View(), "\n")
	lines = append(lines, capLines(viewportLines, viewportHeight)...)
	if separator {
		lines = append(lines, "")
	}
	lines = append(lines, footerLines...)
	return strings.Join(capLines(lines, panelHeight), "\n")
}

func renderTmuxLogConversation(j cockpit.Job, width int) string {
	text := tmuxSessionText(j)
	if text == "" {
		if j.LogPath == "" && j.TmuxTarget == "" {
			return "(no tmux session or log recorded)"
		} else {
			return "(no log output captured yet)"
		}
	}
	var parts []string
	for _, line := range strings.Split(text, "\n") {
		parts = append(parts, wrapText(line, width))
	}
	return strings.Join(parts, "\n")
}

func tmuxActivitySourceLabel(j cockpit.Job) string {
	if j.TmuxTarget != "" {
		return "live pane snapshot"
	}
	if j.LogPath != "" {
		return "captured output"
	}
	return "no capture yet"
}

func (m model) attachedConversationText(j cockpit.Job, width int) string {
	var parts []string
	if len(j.Sources) > 0 && j.SyncBackState != cockpit.SyncBackApplied {
		review := m.renderReviewLines(j, width)
		if len(review) > 0 {
			parts = append(parts, strings.Join(review, "\n"))
		}
	}
	if j.Runner == cockpit.RunnerTmux {
		parts = append(parts, renderTmuxLogConversation(j, width))
	} else {
		running := j.Status == cockpit.StatusRunning
		parts = append(parts, renderChatConversation(m.attachedTurns, m.transcriptBuf, width, running))
	}
	return strings.Join(parts, "\n\n")
}

func (m model) renderAttachedRail(width, height int) string {
	jobs := m.orderedAgentJobs()
	if len(jobs) == 0 {
		return panelStyle.Width(width).Height(height).Render(panelHeaderStyle.Render("  Runs"))
	}
	cursor := 0
	for i, job := range jobs {
		if job.ID == m.attachedJobID {
			cursor = i
			break
		}
	}
	innerHeight := height
	lines := []string{
		panelHeaderStyle.Render("  Runs"),
		dimStyle.Render(fmt.Sprintf("  %d jobs · current %d/%d", len(jobs), cursor+1, len(jobs))),
		"",
	}
	start, end := windowRange(len(jobs), cursor, innerHeight-4)
	if start > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▲ %d more", start)))
	}
	for i := start; i < end; i++ {
		job := jobs[i]
		prefix := "  "
		if job.ID == m.attachedJobID {
			prefix = accentStyle.Render("▸ ")
		}
		statusText, _ := jobOperatorStatus(job)
		line := fmt.Sprintf("%s%s  %s", prefix, truncate(jobRepoLabel(job), maxInt(10, width-20)), statusText)
		lines = append(lines, truncate(line, width-4))
		modelLine := "    " + truncate(jobListSummary(job), maxInt(10, width-8))
		lines = append(lines, dimStyle.Render(modelLine))
		preview := lastJobPreview(job)
		if preview == "" {
			preview = describeExecutor(job.Executor) + " · " + shortJobID(job.ID)
		}
		lines = append(lines, dimStyle.Render("    "+truncate(preview, maxInt(12, width-8))), "")
	}
	if end < len(jobs) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", len(jobs)-end)))
	}
	lines = append(lines, dimStyle.Render("  [/] switch"))
	return panelStyle.Width(width).Height(height).Render(strings.Join(capLines(lines, innerHeight), "\n"))
}
