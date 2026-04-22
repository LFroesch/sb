package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/markdown"
)

// renderAgent dispatches to the active Agent-page mode renderer.
func (m model) renderAgent() string {
	switch m.mode {
	case modeAgentPicker:
		return m.renderAgentPicker()
	case modeAgentLaunch:
		return m.renderAgentLaunch()
	case modeAgentAttached:
		return m.renderAgentAttached()
	case modeAgentManage:
		return m.renderAgentManage()
	}
	return m.renderAgentList()
}

func (m model) renderAgentList() string {
	title := titleStyle.Render("Agent Cockpit")
	if m.cockpitMode != "" {
		title += dimStyle.Render("  [" + m.cockpitMode + "]")
	}
	if cockpit.ExecFallback {
		title += warnStyle.Render("  [exec-fallback]")
	}
	var lines []string
	lines = append(lines, title, "")

	if m.cockpitClient == nil {
		lines = append(lines, dimStyle.Render("  cockpit disabled: "+m.cockpitErr))
		lines = append(lines, "", dimStyle.Render("  esc to go back"))
		return strings.Join(lines, "\n")
	}

	allJobs := m.orderedAgentJobs()
	jobs := m.filteredAgentJobs()
	if len(allJobs) == 0 {
		lines = append(lines, dimStyle.Render("\n\n  no jobs yet — press"),
			keyStyle.Render("n"),
			dimStyle.Render("to launch one \n"))
		return strings.Join(lines, " ")
	}

	// Clamp cursor in case jobs list shrunk (delete, etc).
	cursor := m.agentCursor
	if cursor >= len(jobs) {
		cursor = len(jobs) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	contentHeight := m.agentContentHeight()
	panelHeight := contentHeight - 2
	if panelHeight < 10 {
		panelHeight = 10
	}
	listWidth := m.width * 42 / 100
	if listWidth < 42 {
		listWidth = 42
	}
	rightWidth := m.width - listWidth - 6
	if rightWidth < 34 {
		rightWidth = 34
	}
	innerHeight := panelHeight - 2
	if innerHeight < 4 {
		innerHeight = 4
	}

	renderRow := func(idx int, j cockpit.Job) string {
		prefix := "  "
		if idx == cursor {
			prefix = accentStyle.Render("▸ ")
		}
		age := time.Since(j.CreatedAt).Round(time.Second)
		title := fmt.Sprintf("%s  %s", j.PresetID, statusBadge(j.Status))
		src := ""
		if len(j.Sources) > 0 {
			src = fmt.Sprintf("  %s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
			if len(j.Sources) > 1 {
				src += fmt.Sprintf(" +%d", len(j.Sources)-1)
			}
		} else if j.Repo != "" {
			src = "  " + shortPath(j.Repo)
		}
		row := prefix + title + dimStyle.Render(src) + dimStyle.Render(fmt.Sprintf("  %s ago", age))
		if j.Note != "" {
			row += dimStyle.Render("  ·  " + truncate(j.Note, 28))
		}
		return truncate(row, listWidth-4)
	}

	summaryLines := renderAgentUsageSummary(allJobs, listWidth-6)
	listLines := []string{panelHeaderStyle.Render("  Jobs")}
	listLines = append(listLines, summaryLines...)
	listLines = append(listLines, renderAgentFilterBar(allJobs, m.agentFilter, listWidth-6))
	listLines = append(listLines, "")
	if len(jobs) == 0 {
		listLines = append(listLines, dimStyle.Render("  no jobs in this filter"))
	}
	if len(jobs) > 0 {
		startIdx, endIdx := windowRange(len(jobs), cursor, innerHeight-len(summaryLines)-5)
		if startIdx > 0 {
			listLines = append(listLines, dimStyle.Render(fmt.Sprintf("  ▲ %d more", startIdx)))
		}
		for i := startIdx; i < endIdx; i++ {
			j := jobs[i]
			listLines = append(listLines, renderRow(i, j))
		}
		if endIdx < len(jobs) {
			listLines = append(listLines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", len(jobs)-endIdx)))
		}
	}
	listBody := panelStyle.Width(listWidth).Height(panelHeight).Render(strings.Join(capLines(listLines, innerHeight), "\n"))

	detail := "  no selected job"
	if len(jobs) > 0 {
		detail = m.renderAgentJobDetail(jobs[cursor], rightWidth-4)
	}
	detailLines := strings.Split(detail, "\n")
	detailBody := panelStyle.Width(rightWidth).Height(panelHeight).Render(strings.Join(capLines(detailLines, innerHeight), "\n"))
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, listBody, "  ", detailBody))
	return strings.Join(lines, "\n")
}

func statusBadge(s cockpit.Status) string {
	switch s {
	case cockpit.StatusRunning:
		return primaryStyle.Render("● running")
	case cockpit.StatusIdle:
		return accentStyle.Render("○ idle")
	case cockpit.StatusQueued:
		return dimStyle.Render("◌ queued")
	case cockpit.StatusPaused:
		return dimStyle.Render("‖ paused")
	case cockpit.StatusNeedsReview:
		return warnStyle.Render("! needs review")
	case cockpit.StatusBlocked:
		return warnStyle.Render("⊘ blocked")
	case cockpit.StatusFailed:
		return warnStyle.Render("✗ failed")
	case cockpit.StatusCompleted:
		return accentStyle.Render("✓ completed")
	}
	return string(s)
}

func statusLabel(s cockpit.Status) string {
	switch s {
	case cockpit.StatusRunning:
		return "running"
	case cockpit.StatusIdle:
		return "idle"
	case cockpit.StatusQueued:
		return "queued"
	case cockpit.StatusPaused:
		return "paused"
	case cockpit.StatusNeedsReview:
		return "needs review"
	case cockpit.StatusBlocked:
		return "blocked"
	case cockpit.StatusFailed:
		return "failed"
	case cockpit.StatusCompleted:
		return "completed"
	}
	return string(s)
}

// providerChoices lists the launch-modal provider options: the preset's
// loaded ProviderProfiles. The selected preset simply chooses which
// provider row is selected by default.
func providerChoices(presets []cockpit.LaunchPreset, presetIdx int, providers []cockpit.ProviderProfile) []string {
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		out = append(out, p.Name)
	}
	if len(out) == 0 && presetIdx >= 0 && presetIdx < len(presets) {
		out = append(out, fmt.Sprintf("preset executor (%s)", describeExecutor(presets[presetIdx].Executor)))
	}
	return out
}

func describeExecutor(e cockpit.ExecutorSpec) string {
	if e.Type == "" {
		return "shell"
	}
	if e.Model != "" {
		return e.Type + ":" + e.Model
	}
	return e.Type
}

func shortPath(p string) string {
	if len(p) < 40 {
		return p
	}
	return "…" + p[len(p)-39:]
}

func (m model) renderAgentJobDetail(j cockpit.Job, width int) string {
	previewWidth := width - 4
	if previewWidth < 24 {
		previewWidth = 24
	}
	lines := []string{
		panelHeaderStyle.Render("  Selected job"),
		fmt.Sprintf("  %s  %s", j.PresetID, statusBadge(j.Status)),
		dimStyle.Render("  executor: " + describeExecutor(j.Executor)),
		dimStyle.Render("  runner: " + describeRunner(j.Runner)),
		dimStyle.Render("  age: " + time.Since(j.CreatedAt).Round(time.Second).String()),
		dimStyle.Render("  sync-back: " + string(j.SyncBackState)),
	}
	lines = append(lines, dimStyle.Render("  id: "+string(j.ID)))
	if j.Runner == cockpit.RunnerTmux {
		lines = append(lines, dimStyle.Render("  tmux: "+tmuxWindowState(j)))
	}
	if j.Note != "" {
		lines = append(lines, dimStyle.Render("  note: "+j.Note))
	}
	if len(j.Sources) > 0 {
		src := fmt.Sprintf("  source: %s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
		if len(j.Sources) > 1 {
			src += fmt.Sprintf(" (+%d)", len(j.Sources)-1)
		}
		lines = append(lines, dimStyle.Render(src))
	} else if j.Repo != "" {
		lines = append(lines, dimStyle.Render("  repo: "+shortPath(j.Repo)))
	}
	lines = append(lines, "")
	lines = append(lines, panelHeaderStyle.Render("  Next action"))
	switch {
	case j.Runner == cockpit.RunnerTmux && j.Status == cockpit.StatusRunning:
		lines = append(lines, accentStyle.Render("  enter attach to live tmux window"))
	case j.Runner == cockpit.RunnerTmux:
		lines = append(lines, dimStyle.Render("  enter review log / transcript"))
	case j.Status == cockpit.StatusIdle:
		lines = append(lines, accentStyle.Render("  enter or i to continue conversation"))
	case j.Status == cockpit.StatusRunning:
		lines = append(lines, dimStyle.Render("  stop or wait for the current turn"))
	case j.Status == cockpit.StatusNeedsReview:
		lines = append(lines, warnStyle.Render("  approve after reviewing changes"))
	default:
		lines = append(lines, dimStyle.Render("  retry, delete, or inspect details"))
	}
	lines = append(lines, "")
	lines = append(lines, panelHeaderStyle.Render("  Latest turn"))
	preview := lastJobPreview(j)
	if preview == "" {
		preview = "(no transcript yet)"
	}
	for _, line := range strings.Split(wrapText(preview, previewWidth), "\n") {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "")
	lines = append(lines, panelHeaderStyle.Render("  Conversation"))
	if j.Runner == cockpit.RunnerTmux {
		lines = append(lines, dimStyle.Render("  tmux-backed interactive session"))
		if j.TmuxTarget != "" {
			lines = append(lines, dimStyle.Render("  target: "+j.TmuxTarget))
		}
		if j.Status == cockpit.StatusRunning {
			lines = append(lines, accentStyle.Render("  live now"))
		} else {
			lines = append(lines, dimStyle.Render("  review log / transcript"))
		}
	} else {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  %d total turns · %d assistant replies", countUserVisibleTurns(j), countAssistantTurnsForView(j))))
		if j.Status == cockpit.StatusRunning {
			lines = append(lines, accentStyle.Render("  input available"))
		} else if j.Status == cockpit.StatusIdle {
			lines = append(lines, accentStyle.Render("  waiting for follow-up"))
		} else {
			lines = append(lines, dimStyle.Render("  follow-up closed"))
		}
	}
	return strings.Join(lines, "\n")
}

func describeRunner(r cockpit.Runner) string {
	if r == "" {
		return "exec"
	}
	return string(r)
}

func tmuxWindowState(j cockpit.Job) string {
	if j.Runner != cockpit.RunnerTmux {
		return "n/a"
	}
	if j.TmuxTarget == "" {
		return "missing target"
	}
	alive, err := cockpit.WindowAlive(j.TmuxTarget)
	if err != nil {
		return "check failed"
	}
	if alive {
		return "live (" + j.TmuxTarget + ")"
	}
	return "closed (" + j.TmuxTarget + ")"
}

func lastJobPreview(j cockpit.Job) string {
	for i := len(j.Turns) - 1; i >= 0; i-- {
		content := strings.TrimSpace(j.Turns[i].Content)
		if content == "" {
			continue
		}
		switch j.Turns[i].Role {
		case cockpit.TurnAssistant:
			return "assistant: " + content
		case cockpit.TurnUser:
			return "you: " + content
		}
	}
	return ""
}

func wrapText(s string, width int) string {
	if width < 8 {
		return truncate(s, width)
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var out []string
	line := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(line)+1+lipgloss.Width(word) > width {
			out = append(out, line)
			line = word
			continue
		}
		line += " " + word
	}
	out = append(out, line)
	return strings.Join(out, "\n")
}

func renderChatConversation(turns []cockpit.Turn, liveAssistant string, width int, running bool) string {
	var parts []string
	for _, t := range turns {
		switch t.Role {
		case cockpit.TurnUser:
			body := wrapText(strings.TrimSpace(t.Content), width-4)
			if body == "" {
				continue
			}
			parts = append(parts, accentStyle.Render("You"))
			parts = append(parts, indentLines(body, "  "))
		case cockpit.TurnAssistant:
			body := strings.TrimSpace(t.Content)
			if body == "" {
				continue
			}
			parts = append(parts, primaryStyle.Render("Assistant"))
			parts = append(parts, markdown.Render(body, width))
		}
		parts = append(parts, "")
	}

	if strings.TrimSpace(liveAssistant) != "" {
		parts = append(parts, primaryStyle.Render("Assistant"))
		parts = append(parts, markdown.Render(strings.TrimSpace(liveAssistant), width))
		parts = append(parts, dimStyle.Render("  thinking..."), "")
	} else if running {
		parts = append(parts, dimStyle.Render("  thinking..."))
	}

	return strings.TrimRight(strings.Join(parts, "\n"), "\n")
}

func indentLines(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func windowRange(total, cursor, size int) (int, int) {
	if size <= 0 || total <= size {
		return 0, total
	}
	start := cursor - size/2
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > total {
		end = total
		start = end - size
	}
	return start, end
}

func capLines(lines []string, maxLines int) []string {
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	if maxLines == 1 {
		return []string{truncate(lines[0], 40)}
	}
	out := append([]string{}, lines[:maxLines-1]...)
	out = append(out, dimStyle.Render("  …"))
	return out
}

func (m model) agentContentHeight() int {
	height := m.height - 7
	if height < 8 {
		return 8
	}
	return height
}

func (m model) attachedLayoutDims() (railWidth, chatWidth, panelHeight int) {
	contentHeight := m.agentContentHeight()
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
	panelHeight = contentHeight - 2
	if panelHeight < 10 {
		panelHeight = 10
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

func renderAgentUsageSummary(jobs []cockpit.Job, width int) []string {
	total := len(jobs)
	running := 0
	attention := 0
	live := 0
	usage := map[string]int{}
	for _, j := range jobs {
		if j.Status == cockpit.StatusRunning {
			running++
		}
		if j.Status == cockpit.StatusNeedsReview || j.Status == cockpit.StatusBlocked || j.Status == cockpit.StatusFailed {
			attention++
		}
		if j.Status != cockpit.StatusCompleted {
			live++
		}
		usage[describeExecutor(j.Executor)]++
	}

	lines := []string{
		dimStyle.Render(fmt.Sprintf("  %d total · %d live · %d running · %d attention", total, live, running, attention)),
	}
	if len(usage) == 0 {
		return lines
	}
	type bucket struct {
		label string
		n     int
	}
	var buckets []bucket
	for label, n := range usage {
		buckets = append(buckets, bucket{label: label, n: n})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].n == buckets[j].n {
			return buckets[i].label < buckets[j].label
		}
		return buckets[i].n > buckets[j].n
	})
	parts := make([]string, 0, len(buckets))
	for _, b := range buckets {
		parts = append(parts, fmt.Sprintf("%s×%d", b.label, b.n))
	}
	lines = append(lines, dimStyle.Render("  usage: "+truncate(strings.Join(parts, "  ·  "), width)))
	return lines
}

func renderAgentFilterChip(key, label string, n int, active string) string {
	text := fmt.Sprintf("%s %s:%d", key, label, n)
	if active == "" {
		active = "all"
	}
	if label == active {
		return accentStyle.Render(text)
	}
	return dimStyle.Render(text)
}

func renderAgentFilterBar(jobs []cockpit.Job, active string, width int) string {
	counts := map[string]int{
		"all":       len(jobs),
		"live":      0,
		"running":   0,
		"attention": 0,
		"done":      0,
	}
	for _, j := range jobs {
		if agentJobMatchesFilter(j, "live") {
			counts["live"]++
		}
		if agentJobMatchesFilter(j, "running") {
			counts["running"]++
		}
		if agentJobMatchesFilter(j, "attention") {
			counts["attention"]++
		}
		if agentJobMatchesFilter(j, "done") {
			counts["done"]++
		}
	}
	parts := []string{
		renderAgentFilterChip("1", "all", counts["all"], active),
		renderAgentFilterChip("2", "live", counts["live"], active),
		renderAgentFilterChip("3", "running", counts["running"], active),
		renderAgentFilterChip("4", "attention", counts["attention"], active),
		renderAgentFilterChip("5", "done", counts["done"], active),
	}
	return truncate("  "+strings.Join(parts, "  "), width)
}

func (m model) renderAgentManage() string {
	kindLabel := "Presets"
	if m.agentManageKind == "provider" {
		kindLabel = "Providers"
	}
	title := titleStyle.Render("Agent Settings") +
		dimStyle.Render("  ·  [ presets  ] / [ providers ]")
	if m.agentManageKind == "provider" {
		title = titleStyle.Render("Agent Settings") +
			dimStyle.Render("  ·  [ providers ] / [ presets ]")
	}

	contentHeight := m.agentContentHeight()
	panelHeight := contentHeight - 2
	if panelHeight < 10 {
		panelHeight = 10
	}
	leftWidth := m.width * 32 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := m.width - leftWidth - 6
	if rightWidth < 40 {
		rightWidth = 40
	}
	innerHeight := panelHeight - 2
	if innerHeight < 4 {
		innerHeight = 4
	}

	var items []string
	items = append(items, panelHeaderStyle.Render("  "+kindLabel))
	items = append(items, dimStyle.Render("  n new item"))
	items = append(items, "")
	total := m.agentManageItemCount()
	start, end := windowRange(total, m.agentManageCursor, innerHeight-3)
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.agentManageCursor {
			prefix = accentStyle.Render("▸ ")
		}
		label := m.agentManageItemLabel(i)
		items = append(items, truncate(prefix+label, leftWidth-4))
	}
	if total == 0 {
		items = append(items, dimStyle.Render("  no items"))
	}
	left := panelStyle.Width(leftWidth).Height(panelHeight).Render(strings.Join(capLines(items, innerHeight), "\n"))

	specs := m.agentManageFieldSpecs()
	var detail []string
	detail = append(detail, panelHeaderStyle.Render("  Fields"))
	if total > 0 {
		detail = append(detail, dimStyle.Render("  tab switch focus · enter edit · ctrl+s save"))
	}
	detail = append(detail, "")
	for i, spec := range specs {
		prefix := "  "
		if i == m.agentManageField && m.agentManageFocus == 1 {
			prefix = accentStyle.Render("▸ ")
		}
		value := m.agentManageFieldValue(m.agentManageCursor, i)
		preview := strings.ReplaceAll(value, "\n", "  ")
		detail = append(detail, truncate(prefix+spec.Label+": "+preview, rightWidth-4))
	}
	if m.agentManageEditing {
		spec := specs[m.agentManageField]
		detail = append(detail, "")
		detail = append(detail, panelHeaderStyle.Render("  Editing: "+spec.Label))
		detail = append(detail, m.agentManageEditor.View())
	}
	right := panelStyle.Width(rightWidth).Height(panelHeight).Render(strings.Join(capLines(detail, innerHeight), "\n"))

	return strings.Join([]string{title, "", lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)}, "\n")
}

func countUserVisibleTurns(j cockpit.Job) int {
	n := 0
	for _, t := range j.Turns {
		if t.Role == cockpit.TurnUser || t.Role == cockpit.TurnAssistant {
			n++
		}
	}
	return n
}

func countAssistantTurnsForView(j cockpit.Job) int {
	n := 0
	for _, t := range j.Turns {
		if t.Role == cockpit.TurnAssistant {
			n++
		}
	}
	return n
}

// --- Picker (file → items → multi-select) ---

func (m model) renderAgentPicker() string {
	var lines []string
	lines = append(lines, titleStyle.Render("Pick tasks"), "")

	// Rows available after title(1)+blank(1)+step(1)+blank(1)+topIndicator(1)
	// +bottomIndicator(1)+hint(1) = 7 lines, plus ~4 for page chrome.
	visibleRows := m.height - 11
	if visibleRows < 3 {
		visibleRows = 3
	}

	if m.pickerFile == "" {
		lines = append(lines, dimStyle.Render("  Step 1: pick a file"), "")
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
		lines = append(lines, dimStyle.Render("  enter pick · esc back"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, dimStyle.Render("  Step 2: select items from "+shortPath(m.pickerFile)), "")
	if len(m.pickerItems) == 0 {
		lines = append(lines, dimStyle.Render("    (no `- ` items found)"))
		lines = append(lines, "", dimStyle.Render("  esc back"))
		return strings.Join(lines, "\n")
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
	lines = append(lines, dimStyle.Render(fmt.Sprintf("  %d selected · space toggle · enter continue · b change file · esc back", countSelected(m.pickerSelected))))
	return strings.Join(lines, "\n")
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

// --- Launch modal (preset + brief) ---

func (m model) renderAgentLaunch() string {
	providers := providerChoices(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)

	presetLabel := "(none)"
	if m.launchPresetIdx < len(m.cockpitPresets) {
		presetLabel = m.cockpitPresets[m.launchPresetIdx].Name
	}
	providerLabel := "(none)"
	if m.launchProviderIdx < len(providers) {
		providerLabel = providers[m.launchProviderIdx]
	}

	tab := func(name string, focus int) string {
		if m.launchFocus == focus {
			return accentStyle.Bold(true).Render("▸ " + name)
		}
		return dimStyle.Render("  " + name)
	}
	subtabs := tab("Preset", 0) + dimStyle.Render("  ·  ") +
		tab("Provider", 1) + dimStyle.Render("  ·  ") +
		tab("Brief", 2)

	var lines []string
	lines = append(lines, titleStyle.Render("Launch agent")+"   "+subtabs, "")

	summary := dimStyle.Render("  preset=") + textStyle.Render(presetLabel) +
		dimStyle.Render("  provider=") + textStyle.Render(providerLabel) +
		dimStyle.Render("  repo=") + textStyle.Render(shortPath(m.launchRepo)) +
		dimStyle.Render(fmt.Sprintf("  %d sources", len(m.launchSources)))
	lines = append(lines, summary, "")
	if len(m.launchSources) > 0 {
		var src []string
		for i, s := range m.launchSources {
			if i >= 3 {
				src = append(src, fmt.Sprintf("+%d more", len(m.launchSources)-i))
				break
			}
			src = append(src, truncate(s.Text, 42))
		}
		lines = append(lines, dimStyle.Render("  sources: "+strings.Join(src, "  ·  ")), "")
	}

	// Rows available for the focused panel.
	// title(1)+blank(1)+summary(1)+blank(1)+topIndicator(1)+bottomIndicator(1)
	// +hint(1) = 7 lines; ~4 for page chrome.
	visibleRows := m.height - 11
	if visibleRows < 3 {
		visibleRows = 3
	}

	switch m.launchFocus {
	case 0:
		total := len(m.cockpitPresets)
		startIdx := 0
		if m.launchPresetIdx >= visibleRows {
			startIdx = m.launchPresetIdx - visibleRows + 1
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
			p := m.cockpitPresets[i]
			prefix := "    "
			if i == m.launchPresetIdx {
				prefix = accentStyle.Render("  ▸ ")
			}
			name := p.Name
			if i == m.launchPresetIdx {
				name = accentStyle.Bold(true).Render(name)
			}
			desc := ""
			if p.Role != "" {
				desc = dimStyle.Render("  " + p.Role)
			}
			lines = append(lines, prefix+name+desc)
		}
		if endIdx < total {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", total-endIdx)))
		} else {
			lines = append(lines, "")
		}

	case 1:
		total := len(providers)
		startIdx := 0
		if m.launchProviderIdx >= visibleRows {
			startIdx = m.launchProviderIdx - visibleRows + 1
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
			prefix := "    "
			name := providers[i]
			if i == m.launchProviderIdx {
				prefix = accentStyle.Render("  ▸ ")
				name = accentStyle.Bold(true).Render(name)
			}
			lines = append(lines, prefix+name)
		}
		if endIdx < total {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", total-endIdx)))
		} else {
			lines = append(lines, "")
		}

	case 2:
		m.launchBrief.SetWidth(m.width - 6)
		briefH := visibleRows
		if briefH < 3 {
			briefH = 3
		}
		m.launchBrief.SetHeight(briefH)
		lines = append(lines, dimStyle.Render("  Brief (optional freeform)"))
		lines = append(lines, m.launchBrief.View())
	}

	lines = append(lines, dimStyle.Render("  tab cycle · ↑/↓ pick · enter launch · alt+enter launch (from brief) · esc back"))
	return strings.Join(lines, "\n")
}

// --- Attached view ---

func (m model) renderAgentAttached() string {
	j, ok := m.cockpitClient.GetJob(m.attachedJobID)
	if !ok {
		return titleStyle.Render("Attached") + "\n\n  " +
			warnStyle.Render("job gone") + "\n\n" +
			dimStyle.Render("  esc back")
	}

	// A live job is one the user can still send turns to. Completed /
	// failed / blocked conversations are dead — only scrollback + retry.
	isLive := j.Status != cockpit.StatusCompleted &&
		j.Status != cockpit.StatusFailed &&
		j.Status != cockpit.StatusBlocked
	turnInFlight := j.Status == cockpit.StatusRunning
	isTmux := j.Runner == cockpit.RunnerTmux

	railWidth, chatWidth, panelHeight := m.attachedLayoutDims()

	rail := m.renderAttachedRail(railWidth, panelHeight)

	var lines []string
	titleLabel := "Chat: "
	if isTmux {
		titleLabel = "Session: "
	}
	header := titleStyle.Render(titleLabel) + j.PresetID + "  " + statusBadge(j.Status)
	header += dimStyle.Render("  " + describeExecutor(j.Executor))
	lines = append(lines, header)

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
	lines = append(lines, strings.Join(meta, " "))

	if len(j.Sources) > 0 {
		preview := fmt.Sprintf("  sources: %s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
		if len(j.Sources) > 1 {
			preview += fmt.Sprintf(" (+%d more)", len(j.Sources)-1)
		}
		lines = append(lines, dimStyle.Render(preview))
	}

	turnCount := countUserVisibleTurns(j)
	sectionLabel := "transcript"
	sectionMeta := fmt.Sprintf("%d turns", turnCount)
	if isTmux {
		sectionLabel = "log"
		sectionMeta = "tmux session log"
	}
	if m.attachedFocus == 0 {
		lines = append(lines, "", accentStyle.Render("  ▸ "+sectionLabel)+dimStyle.Render("  "+sectionMeta))
	} else {
		lines = append(lines, "", dimStyle.Render(fmt.Sprintf("  %s · %d turns", sectionLabel, turnCount))+"  "+accentStyle.Render("▸ input"))
	}

	headerLines := len(lines)
	m.attachedInput.SetWidth(m.attachedInputWidth())
	inputLines := 1
	if isLive {
		inputLines += lipgloss.Height(m.attachedInput.View())
	}
	if !isLive {
		inputLines = 2
	}
	hintLines := 1
	h := panelHeight - headerLines - inputLines - hintLines
	if h < 5 {
		h = 5
	}
	m.viewport.Width = chatWidth - 6
	m.viewport.Height = h
	lines = append(lines, m.viewport.View())

	if isTmux {
		if isLive {
			lines = append(lines, accentStyle.Render("  live tmux session — press enter or i from the jobs list to attach"))
		} else {
			lines = append(lines, dimStyle.Render("  tmux session ended — review log above"))
		}
	} else if isLive {
		inputLabel := dimStyle.Render("  message:")
		switch {
		case m.attachedFocus == 1:
			inputLabel = accentStyle.Render("  message (enter send · esc/tab leave)")
		case turnInFlight:
			inputLabel = dimStyle.Render("  message (turn in flight — wait for reply)")
		default:
			inputLabel = accentStyle.Render("  message (ready)")
		}
		lines = append(lines, inputLabel, m.attachedInput.View())
	} else {
		lines = append(lines, dimStyle.Render("  (conversation ended — no more turns)"))
	}

	chat := panelStyle.Width(chatWidth).Height(panelHeight).Render(strings.Join(lines, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, rail, "  ", chat)
}

func renderTmuxLogConversation(j cockpit.Job, width int) string {
	var parts []string
	parts = append(parts, dimStyle.Render("tmux-backed session"))
	if j.LogPath != "" {
		parts = append(parts, dimStyle.Render("log: "+shortPath(j.LogPath)))
	}
	parts = append(parts, "")
	if j.LogPath == "" {
		parts = append(parts, "(no log path recorded)")
		return strings.Join(parts, "\n")
	}
	body, err := os.ReadFile(j.LogPath)
	if err != nil {
		parts = append(parts, "(log unavailable: "+err.Error()+")")
		return strings.Join(parts, "\n")
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		parts = append(parts, "(no log output captured yet)")
		return strings.Join(parts, "\n")
	}
	for _, line := range strings.Split(text, "\n") {
		parts = append(parts, wrapText(line, width))
	}
	return strings.Join(parts, "\n")
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
	innerHeight := height - 2
	if innerHeight < 4 {
		innerHeight = 4
	}
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
		line := fmt.Sprintf("%s%s  %s", prefix, truncate(job.PresetID, maxInt(10, width-20)), statusLabel(job.Status))
		lines = append(lines, truncate(line, width-4))
		modelLine := "    " + truncate(describeExecutor(job.Executor), maxInt(10, width-8))
		lines = append(lines, dimStyle.Render(modelLine))
		preview := lastJobPreview(job)
		if preview == "" {
			preview = "(no messages yet)"
		}
		lines = append(lines, dimStyle.Render("    "+truncate(preview, maxInt(12, width-8))), "")
	}
	if end < len(jobs) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ▼ %d more", len(jobs)-end)))
	}
	lines = append(lines, dimStyle.Render("  [/] switch"))
	return panelStyle.Width(width).Height(height).Render(strings.Join(capLines(lines, innerHeight), "\n"))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
