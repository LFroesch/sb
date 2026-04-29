package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
)

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

func (m model) attachedInputWidth() int {
	_, chatWidth, _ := m.attachedLayoutDims()
	width := chatWidth - 8
	if width < 20 {
		width = 20
	}
	return width
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

	// A live job is one the user can still send turns to. Completed /
	// failed / blocked conversations are dead — only scrollback + delete.
	isLive := j.Status != cockpit.StatusCompleted &&
		j.Status != cockpit.StatusFailed &&
		j.Status != cockpit.StatusBlocked
	turnInFlight := j.Status == cockpit.StatusRunning
	isTmux := j.Runner == cockpit.RunnerTmux

	railWidth, chatWidth, panelHeight := m.attachedLayoutDims()
	lineWidth := maxInt(1, chatWidth-4)

	rail := m.renderAttachedRail(railWidth, panelHeight)

	var lines []string
	titleLabel := "Chat: "
	if isTmux {
		titleLabel = "Session: "
	}
	statusText, statusStyle := jobOperatorStatus(j)
	header := titleStyle.Render(titleLabel) + j.PresetID + "  " + statusStyle.Render(statusText)
	header += dimStyle.Render("  " + describeExecutor(j.Executor))
	lines = append(lines, truncate(header, lineWidth))

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
	if isTmux {
		meta = append(meta, dimStyle.Render("· "+tmuxWindowState(j)))
	}
	lines = append(lines, truncate(strings.Join(meta, " "), lineWidth))

	if len(j.Sources) > 0 {
		preview := fmt.Sprintf("  sources: %s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
		if len(j.Sources) > 1 {
			preview += fmt.Sprintf(" (+%d more)", len(j.Sources)-1)
		}
		lines = append(lines, truncate(dimStyle.Render(preview), lineWidth))
	}

	turnCount := countUserVisibleTurns(j)
	sectionLabel := "transcript"
	sectionMeta := fmt.Sprintf("%d turns", turnCount)
	if isTmux {
		sectionLabel = "log"
		sectionMeta = "tmux session log"
	}
	if panelHeight > 8 {
		lines = append(lines, "")
	}
	if m.attachedFocus == 0 {
		lines = append(lines, truncate(accentStyle.Render("  ▸ "+sectionLabel)+dimStyle.Render("  "+sectionMeta), lineWidth))
	} else {
		lines = append(lines, truncate(dimStyle.Render(fmt.Sprintf("  %s · %d turns", sectionLabel, turnCount))+"  "+accentStyle.Render("▸ input"), lineWidth))
	}

	innerHeight := panelHeight
	m.attachedInput.SetWidth(m.attachedInputWidth())
	var footerLines []string
	if !isTmux && isLive {
		maxInputHeight := innerHeight - len(lines) - 2 // input label + at least one viewport row
		if maxInputHeight < 1 {
			maxInputHeight = 1
		}
		inputLabel := accentStyle.Render("  message")
		switch {
		case turnInFlight:
			inputLabel = dimStyle.Render("  message")
		}
		m.attachedInput.SetHeight(maxInputHeight)
		footerLines = append(footerLines, truncate(inputLabel, lineWidth))
		footerLines = append(footerLines, strings.Split(m.attachedInput.View(), "\n")...)
	} else if isTmux {
		if isLive {
			footerLines = append(footerLines, truncate(accentStyle.Render("  live tmux session"), lineWidth))
		} else {
			footerLines = append(footerLines, truncate(dimStyle.Render("  tmux session ended"), lineWidth))
		}
	} else {
		footerLines = append(footerLines, truncate(dimStyle.Render("  conversation ended"), lineWidth))
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
		return panelStyle.Width(width).Height(height).Render(panelHeaderStyle.Render("  Sessions"))
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
		panelHeaderStyle.Render("  Sessions"),
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
