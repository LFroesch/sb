package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/diff"
	"github.com/LFroesch/sb/internal/markdown"
	"github.com/LFroesch/sb/internal/statusbar"
	"github.com/LFroesch/sb/internal/transcript"
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
	title := titleStyle.Render("Agents")
	if m.cockpitMode != "" {
		title += dimStyle.Render("  [" + m.cockpitMode + "]")
	}
	if cockpit.ExecFallback {
		title += warnStyle.Render("  [exec-fallback]")
	}
	var lines []string
	lines = append(lines, title, "")
	contentHeight := m.agentContentHeight()

	if m.cockpitClient == nil {
		lines = append(lines, dimStyle.Render("  cockpit disabled: "+m.cockpitErr))
		return strings.Join(capLines(lines, contentHeight), "\n")
	}

	allJobs := m.orderedAgentJobs()
	jobs := m.filteredAgentJobs()
	if len(allJobs) == 0 {
		lines = append(lines, dimStyle.Render("\n\n  no jobs yet — press"),
			keyStyle.Render("n"),
			dimStyle.Render("to start a run \n"))
		return strings.Join(capLines(lines, contentHeight), " ")
	}

	// Clamp cursor in case jobs list shrunk (delete, etc).
	cursor := m.agentCursor
	if cursor >= len(jobs) {
		cursor = len(jobs) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	panelHeight, listWidth, rightWidth, innerHeight := m.agentListLayout(len(lines))

	repoW, advanceW, statusW, presetW := agentRowColumnWidths(listWidth - 4)

	renderRow := func(idx int, j cockpit.Job) string {
		rowWidth := listWidth - 4
		if rowWidth < 24 {
			rowWidth = 24
		}
		taskW := rowWidth - 2 - repoW - advanceW - statusW - presetW - 4
		if taskW < 8 {
			taskW = 8
		}

		isCursor := idx == cursor
		bg := func(s lipgloss.Style) lipgloss.Style {
			if isCursor {
				return s.Background(colorCursorBg)
			}
			return s
		}
		cell := func(s lipgloss.Style, text string, width int, alignRight bool) string {
			if width <= 0 {
				return ""
			}
			raw := xansi.Truncate(strings.TrimSpace(text), width, "")
			pad := width - lipgloss.Width(raw)
			if pad < 0 {
				pad = 0
			}
			space := bg(lipgloss.NewStyle()).Render(strings.Repeat(" ", pad))
			rendered := bg(s).Render(raw)
			if alignRight {
				return space + rendered
			}
			return rendered + space
		}

		prefix := bg(dimStyle).Render("  ")
		if isCursor {
			prefix = bg(accentStyle).Render("▸ ")
		}
		_, statusStyle := jobOperatorStatus(j)
		advanceText, advanceStyle := jobAdvanceState(j)
		repo := jobRepoLabel(j)
		if repo == "" {
			repo = "no-repo"
		}
		task := jobListSummary(j)
		if j.Note != "" {
			task += " · " + j.Note
		}
		if cached, ok := cockpit.LoadPostHookPreviews(j); ok && anyHookWouldFail(cached) {
			task += " · !hook"
		}

		row := prefix
		row += cell(primaryStyle.Bold(true), repo, repoW, false)
		row += bg(lipgloss.NewStyle()).Render(" ")
		row += cell(textStyle, task, taskW, false)
		row += bg(lipgloss.NewStyle()).Render(" ")
		row += cell(advanceStyle, advanceText, advanceW, false)
		row += bg(lipgloss.NewStyle()).Render(" ")
		row += cell(statusStyle, compactJobStatus(j), statusW, false)
		row += bg(lipgloss.NewStyle()).Render(" ")
		row += cell(dimStyle, j.PresetID, presetW, true)
		return row
	}

	columnHeader := renderAgentColumnHeader(listWidth-4, repoW, advanceW, statusW, presetW)

	headerLines := renderAgentJobsHeader(allJobs, m.agentFilter, m.cockpitForeman, listWidth-6)
	listLines := []string{panelHeaderStyle.Render("  Jobs")}
	listLines = append(listLines, headerLines...)
	listLines = append(listLines, "")
	listLines = append(listLines, columnHeader)
	if len(jobs) == 0 {
		listLines = append(listLines, dimStyle.Render("  no jobs in this filter"))
	}
	if len(jobs) > 0 {
		startIdx, endIdx := windowRange(len(jobs), cursor, innerHeight-len(headerLines)-5)
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
		detail = m.renderAgentPeek(jobs[cursor], rightWidth-4, innerHeight, m.agentDetailOffset)
	}
	detailBody := panelStyle.Width(rightWidth).Height(panelHeight).Render(detail)
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, listBody, "  ", detailBody))
	return strings.Join(lines, "\n")
}

func (m model) agentListLayout(prefixLines int) (panelHeight, listWidth, rightWidth, innerHeight int) {
	contentHeight := m.agentContentHeight()
	panelHeight = contentHeight - prefixLines - 2
	if panelHeight < 3 {
		panelHeight = 3
	}
	listWidth = m.width * 42 / 100
	if listWidth < 42 {
		listWidth = 42
	}
	rightWidth = m.width - listWidth - 6
	if rightWidth < 34 {
		rightWidth = 34
	}
	innerHeight = panelHeight - 2
	if innerHeight < 1 {
		innerHeight = 1
	}
	return panelHeight, listWidth, rightWidth, innerHeight
}

// inFlightElapsed returns how long the in-flight turn has been running and
// whether the job is actually awaiting a model reply. Only non-tmux jobs in
// StatusRunning qualify: tmux jobs are interactive sessions, not request/reply.
// Duration is measured from the last user turn's StartedAt, falling back to
// the job's CreatedAt for the first turn that hasn't registered a user entry.
func inFlightElapsed(j cockpit.Job) (time.Duration, bool) {
	if j.Status != cockpit.StatusRunning || j.Runner == cockpit.RunnerTmux {
		return 0, false
	}
	for i := len(j.Turns) - 1; i >= 0; i-- {
		if j.Turns[i].Role == cockpit.TurnAssistant {
			return 0, false // assistant already replied, status just hasn't flipped yet
		}
		if j.Turns[i].Role == cockpit.TurnUser && !j.Turns[i].StartedAt.IsZero() {
			return time.Since(j.Turns[i].StartedAt).Round(time.Second), true
		}
	}
	return time.Since(j.CreatedAt).Round(time.Second), true
}

// formatTurnDuration prints a turn's in-flight time compactly: "4s",
// "1m12s", "2h3m". time.Duration.String() is fine for the short cases but
// trims trailing zero units, which reads cleaner than manual formatting.
func formatTurnDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

func statusLabel(s cockpit.Status) string {
	switch s {
	case cockpit.StatusRunning:
		return "working"
	case cockpit.StatusAwaitingHuman:
		return "waiting on you"
	case cockpit.StatusIdle:
		return "waiting for input"
	case cockpit.StatusQueued:
		return "queued"
	case cockpit.StatusNeedsReview:
		return "needs review"
	case cockpit.StatusBlocked:
		return "blocked"
	case cockpit.StatusFailed:
		return "failed"
	case cockpit.StatusCompleted:
		return "done"
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
	if e.Model != "" {
		return e.Type + ":" + e.Model
	}
	return e.Type
}

func describeLaunchMode(mode string) string {
	switch mode {
	case "", cockpit.LaunchModeSingleJob:
		return "single job"
	case cockpit.LaunchModeTaskQueueSequence:
		return "task queue (serial)"
	default:
		return mode
	}
}

func shortPath(p string) string {
	if len(p) < 21 {
		return p
	}
	return "…" + p[len(p)-20:]
}

func agentManageKindLabel(kind string) string {
	switch kind {
	case "provider":
		return "Runtimes"
	case "prompt":
		return "Prompts"
	case "hookbundle":
		return "Hook bundles"
	}
	return "Templates"
}

func agentManageKindSubtitle(kind string) string {
	switch kind {
	case "provider":
		return "advanced engine definitions: cli, runner, model, args"
	case "prompt":
		return "system prompt bodies referenced by presets"
	case "hookbundle":
		return "hook sets (prompt/pre/post + iteration) referenced by presets"
	}
	return "advanced run defaults: prompting, hooks, policies, suggested engine"
}

func selectedLaunchPreset(m model) cockpit.LaunchPreset {
	if m.launchPresetIdx >= 0 && m.launchPresetIdx < len(m.cockpitPresets) {
		return m.cockpitPresets[m.launchPresetIdx]
	}
	return cockpit.LaunchPreset{}
}

func selectedLaunchProvider(m model) cockpit.ProviderProfile {
	if m.launchProviderIdx >= 0 && m.launchProviderIdx < len(m.cockpitProviders) {
		return m.cockpitProviders[m.launchProviderIdx]
	}
	return cockpit.ProviderProfile{}
}

func countVisibleShellHooks(hooks []cockpit.ShellHook) int {
	n := 0
	for _, h := range hooks {
		if strings.TrimSpace(h.Cmd) != "" {
			n++
		}
	}
	return n
}

func itemSummaryLines(kind string, name string, body []string) []string {
	lines := []string{
		panelHeaderStyle.Render("  " + name),
		dimStyle.Render("  " + agentManageKindSubtitle(kind)),
		"",
	}
	lines = append(lines, body...)
	return lines
}

func renderManageKV(label, value string) string {
	if strings.TrimSpace(value) == "" {
		value = dimStyle.Render("(empty)")
	}
	return "  " + dimStyle.Render(fmt.Sprintf("%-11s", label)) + " " + value
}

func jobTaskText(j cockpit.Job) string {
	if text := strings.TrimSpace(j.Task); text != "" {
		return text
	}
	if len(j.Sources) > 0 {
		texts := make([]string, 0, len(j.Sources))
		for _, src := range j.Sources {
			text := strings.TrimSpace(src.Text)
			if text == "" {
				continue
			}
			texts = append(texts, text)
		}
		return strings.Join(texts, " · ")
	}
	if text := strings.TrimSpace(j.Freeform); text != "" {
		return text
	}
	return strings.TrimSpace(j.Brief)
}

func renderLaunchKV(label, value string) string {
	if strings.TrimSpace(value) == "" {
		value = dimStyle.Render("(empty)")
	}
	return "  " + dimStyle.Render(fmt.Sprintf("%-10s", label)) + " " + value
}

func launchReviewLines(m model) []string {
	preset := selectedLaunchPreset(m)
	provider := selectedLaunchProvider(m)
	finalExecutor := preset.Executor
	if provider.ID != "" || provider.Name != "" {
		finalExecutor = provider.Executor
	}
	lines := []string{
		panelHeaderStyle.Render("  Review Run"),
		dimStyle.Render("  assembled run: source -> role -> engine -> note"),
		"",
		renderLaunchKV("sources", textStyle.Render(fmt.Sprintf("%d selected", len(m.launchSources)))),
	}
	if strings.TrimSpace(m.launchRepo) != "" {
		lines = append(lines[:3], append([]string{renderLaunchKV("repo", textStyle.Render(shortPath(m.launchRepo)))}, lines[3:]...)...)
	}
	startMode := "start immediately"
	if m.launchQueueOnly {
		startMode = "wait for Foreman"
	}
	lines = append(lines, renderLaunchKV("start", textStyle.Render(startMode)))
	if m.launchQueueOnly && !m.cockpitForeman.Enabled {
		lines = append(lines, dimStyle.Render("              will sit in foreman pool until you press F on the list"))
	}
	if preset.ID != "" {
		lines = append(lines,
			renderLaunchKV("role", primaryStyle.Bold(true).Render(preset.Name)),
			renderLaunchKV("launch", textStyle.Render(describeLaunchMode(preset.LaunchMode))),
			renderLaunchKV("engine", textStyle.Render(describeExecutor(finalExecutor))),
			renderLaunchKV("iteration", textStyle.Render(preset.Hooks.Iteration.Mode)),
			renderLaunchKV("policy", textStyle.Render(preset.Permissions)),
			renderLaunchKV("hooks", textStyle.Render(fmt.Sprintf("prompt %d · pre %d · post %d",
				len(preset.Hooks.Prompt),
				countVisibleShellHooks(preset.Hooks.PreShell),
				countVisibleShellHooks(preset.Hooks.PostShell)))),
		)
	}
	if (provider.ID != "" || provider.Name != "") && describeExecutor(provider.Executor) != describeExecutor(preset.Executor) {
		lines = append(lines,
			"",
			renderLaunchKV("override", primaryStyle.Bold(true).Render(provider.Name)),
			renderLaunchKV("default", textStyle.Render(describeExecutor(preset.Executor))),
			renderLaunchKV("runner", textStyle.Render(provider.Executor.Runner)),
		)
	}
	lines = append(lines, "")
	lines = append(lines, panelHeaderStyle.Render("  Source Preview"))
	if len(m.launchSources) == 0 {
		lines = append(lines, dimStyle.Render("  no task source"))
	} else {
		for i, src := range m.launchSources {
			lines = append(lines, "  "+dimStyle.Render(fmt.Sprintf("%2d.", i+1))+" "+textStyle.Render(src.Project))
			lines = append(lines, "     "+truncate(src.Text, maxInt(24, m.width/2)))
			lines = append(lines, "     "+dimStyle.Render(fmt.Sprintf("%s:%d", shortPath(src.File), src.Line)))
		}
	}
	if preset.ID != "" && preset.LaunchMode == cockpit.LaunchModeTaskQueueSequence && len(m.launchSources) > 1 {
		lines = append(lines, "", dimStyle.Render(fmt.Sprintf("  foreman will queue %d one-task runs and work through them serially per repo", len(m.launchSources))))
	}
	lines = append(lines, "", panelHeaderStyle.Render("  Brief"))
	brief := strings.TrimSpace(m.launchBrief.Value())
	if brief == "" {
		lines = append(lines, dimStyle.Render("  no extra launch note"))
	} else {
		for _, line := range strings.Split(wrapText(brief, maxInt(28, m.width/2)), "\n") {
			lines = append(lines, "  "+line)
		}
	}
	return lines
}

func renderManageFieldList(m model, width, visible int) []string {
	specs := m.agentManageFieldSpecs()
	if len(specs) == 0 {
		return []string{dimStyle.Render("  no fields")}
	}
	groups := m.agentManageGroupOrder()
	if len(groups) == 0 {
		return []string{dimStyle.Render("  no fields")}
	}
	groupIdx := m.agentManageGroup
	if groupIdx < 0 {
		groupIdx = 0
	}
	if groupIdx >= len(groups) {
		groupIdx = len(groups) - 1
	}
	groupName := groups[groupIdx]

	var lines []string
	header := fmt.Sprintf("  step %d/%d: %s", groupIdx+1, len(groups), groupName)
	hint := dimStyle.Render("   tab ▸ next group · enter quick action · e detailed edit/select · a to ")
	if m.agentManageAdvanced {
		hint += dimStyle.Render("hide advanced")
	} else {
		hint += dimStyle.Render("show advanced")
	}
	lines = append(lines, panelHeaderStyle.Render(header)+hint)
	lines = append(lines, "")

	indices := m.agentManageGroupFieldIndices()
	if len(indices) == 0 {
		lines = append(lines, dimStyle.Render("  no fields in this group"))
		return scrollWindow(lines, m.agentManageDetailOffset, visible)
	}
	for _, i := range indices {
		spec := specs[i]
		prefix := "  "
		if i == m.agentManageField && m.agentManageFocus == 1 {
			prefix = accentStyle.Render("▸ ")
		}
		value := strings.ReplaceAll(m.agentManageFieldValue(m.agentManageCursor, i), "\n", "  ")
		displayValue := value
		if strings.TrimSpace(displayValue) == "" {
			displayValue = "(empty)"
		}
		valueStyled := textStyle.Render(truncate(displayValue, width-22))
		if strings.TrimSpace(value) == "" {
			valueStyled = dimStyle.Render("(empty)")
		}
		if opts := m.enumOptionsForFieldKey(spec.Key); len(opts) > 0 {
			if strings.TrimSpace(value) == "" {
				displayValue = "(none)"
			}
			valueStyled = primaryStyle.Render(truncate(displayValue, width-22))
		}
		lines = append(lines, prefix+dimStyle.Render(spec.Label+": ")+valueStyled)
	}
	if m.agentManageField >= 0 && m.agentManageField < len(specs) {
		spec := specs[m.agentManageField]
		if strings.TrimSpace(spec.Help) != "" {
			lines = append(lines, "", dimStyle.Render("  "+spec.Help))
		}
	}
	if !m.agentManageAdvanced && groupName != "Hooks" && groupName != "Advanced" {
		lines = append(lines, "", dimStyle.Render("  advanced fields hidden — press a to show"))
	}
	return scrollWindow(lines, m.agentManageDetailOffset, visible)
}

func renderManageSelectedSummary(m model, width int) []string {
	switch m.agentManageKind {
	case "provider":
		if m.agentManageCursor < 0 || m.agentManageCursor >= len(m.cockpitProviders) {
			return itemSummaryLines("provider", "No provider", []string{dimStyle.Render("  create one with n")})
		}
		p := m.cockpitProviders[m.agentManageCursor]
		return itemSummaryLines("provider", p.Name, []string{
			renderManageKV("executor", textStyle.Render(describeExecutor(p.Executor))),
			renderManageKV("model", textStyle.Render(p.Executor.Model)),
			renderManageKV("id", textStyle.Render(p.ID)),
		})
	case "prompt":
		if m.agentManageCursor < 0 || m.agentManageCursor >= len(m.cockpitPrompts) {
			return itemSummaryLines("prompt", "No prompt", []string{dimStyle.Render("  create one with n")})
		}
		p := m.cockpitPrompts[m.agentManageCursor]
		preview := strings.ReplaceAll(p.Body, "\n", " ")
		return itemSummaryLines("prompt", p.Name, []string{
			renderManageKV("body", textStyle.Render(truncate(preview, width-12))),
			renderManageKV("length", textStyle.Render(fmt.Sprintf("%d chars", len(p.Body)))),
			renderManageKV("id", textStyle.Render(p.ID)),
		})
	case "hookbundle":
		if m.agentManageCursor < 0 || m.agentManageCursor >= len(m.cockpitHookBundles) {
			return itemSummaryLines("hookbundle", "No hook bundle", []string{dimStyle.Render("  create one with n")})
		}
		h := m.cockpitHookBundles[m.agentManageCursor]
		return itemSummaryLines("hookbundle", h.Name, []string{
			renderManageKV("hooks", textStyle.Render(fmt.Sprintf("prompt %d · pre %d · post %d",
				len(h.Prompt),
				countVisibleShellHooks(h.PreShell),
				countVisibleShellHooks(h.PostShell)))),
			renderManageKV("id", textStyle.Render(h.ID)),
		})
	}
	if m.agentManageCursor < 0 || m.agentManageCursor >= len(m.cockpitPresets) {
		return itemSummaryLines("preset", "No recipe", []string{dimStyle.Render("  create one with n")})
	}
	p := m.cockpitPresets[m.agentManageCursor]
	lines := []string{
		renderManageKV("launch", textStyle.Render(describeLaunchMode(p.LaunchMode))),
		renderManageKV("prompt", textStyle.Render(p.PromptID)),
		renderManageKV("hooks ref", textStyle.Render(strings.Join(p.HookBundleIDs, ", "))),
		renderManageKV("engine ref", textStyle.Render(p.EngineID)),
		renderManageKV("suggested", textStyle.Render(describeExecutor(p.Executor))),
		renderManageKV("policy", textStyle.Render(p.Permissions)),
		renderManageKV("hooks", textStyle.Render(fmt.Sprintf("prompt %d · pre %d · post %d",
			len(p.Hooks.Prompt),
			countVisibleShellHooks(p.Hooks.PreShell),
			countVisibleShellHooks(p.Hooks.PostShell)))),
	}
	lines = append(lines, renderManageKV("id", textStyle.Render(p.ID)))
	return itemSummaryLines("preset", p.Name, lines)
}

func (m model) agentPeekHeaderLines(j cockpit.Job, bodyWidth int) []string {
	if bodyWidth < 24 {
		bodyWidth = 24
	}
	statusText, statusStyle := jobOperatorStatus(j)
	appendWrapped := func(lines []string, text string) []string {
		for _, line := range wrapLines(text, bodyWidth) {
			lines = append(lines, line)
		}
		return lines
	}

	// Title: repo + short id. Avoids a second "Live Peek" header since the
	// panel chrome already frames this pane.
	lines := appendWrapped(nil, "  "+primaryStyle.Bold(true).Render(jobRepoLabel(j))+
		dimStyle.Render("  "+shortJobID(j.ID)))
	lines = append(lines, "")

	kv := func(label string, value string) []string {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return wrapLines("  "+dimStyle.Render(fmt.Sprintf("%-9s", label))+" "+value, bodyWidth)
	}
	appendKV := func(label, value string) {
		lines = append(lines, kv(label, value)...)
	}

	lines = append(lines, kv("status", statusStyle.Render(statusText))...)
	advanceText, advanceStyle := jobAdvanceState(j)
	appendKV("queue", advanceStyle.Render(advanceText))
	if j.ForemanManaged {
		mode := "managed"
		if j.WaitForForeman {
			mode = "waiting for Foreman"
		}
		appendKV("foreman", textStyle.Render(mode))
	}
	if j.EligibilityReason != "" && j.EligibilityReason != "waiting for foreman" {
		appendKV("deferred", warnStyle.Render(j.EligibilityReason))
	}
	appendKV("role", textStyle.Render(j.PresetID))
	appendKV("engine", textStyle.Render(describeExecutor(j.Executor)))
	if j.Runner == cockpit.RunnerTmux && stoppedLikeStatusLabel(j) == "closed" {
		appendKV("session", dimStyle.Render(tmuxWindowState(j)))
	}
	ageLabel := formatTurnDuration(time.Since(j.CreatedAt))
	if extra := jobElapsedSummary(j); extra != "" {
		ageLabel += dimStyle.Render("  (" + extra + ")")
	}
	lines = append(lines, kv("age", textStyle.Render(ageLabel))...)
	if j.Note != "" {
		appendKV("note", textStyle.Render(j.Note))
	}
	if task := jobTaskText(j); task != "" {
		appendKV("task", textStyle.Render(task))
	}
	if len(j.Sources) > 0 {
		src := fmt.Sprintf("%s:%d", shortPath(j.Sources[0].File), j.Sources[0].Line)
		if len(j.Sources) > 1 {
			src += fmt.Sprintf(" +%d", len(j.Sources)-1)
		}
		appendKV("sources", textStyle.Render(src))
	}
	if len(j.Sources) == 0 && strings.TrimSpace(j.Task) == "" {
		if brief := strings.TrimSpace(j.Brief); brief != "" {
			firstLine := strings.SplitN(brief, "\n", 2)[0]
			appendKV("brief", textStyle.Render(firstLine))
		}
	}
	if reviewLines := m.renderPeekReviewSummary(j, bodyWidth); len(reviewLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, reviewLines...)
	}

	lines = append(lines, "")
	divider := "── latest activity "
	fillW := bodyWidth - len(divider)
	if fillW < 4 {
		fillW = 4
	}
	lines = append(lines, "  "+dimStyle.Render(divider+strings.Repeat("─", fillW)))
	return lines
}

func (m model) renderAgentPeek(j cockpit.Job, width, height, offset int) string {
	bodyWidth := width - 4
	if bodyWidth < 24 {
		bodyWidth = 24
	}
	lines := m.agentPeekHeaderLines(j, bodyWidth)
	bodyLines := jobPeekBody(j, bodyWidth)
	maxHeaderLines := minInt(14, maxInt(8, height-1))
	if len(lines) > maxHeaderLines {
		lines = capLines(lines, maxHeaderLines)
	}
	visibleBody := height - len(lines)
	if visibleBody < 1 {
		visibleBody = 1
	}
	lines = append(lines, scrollWindowFromBottom(bodyLines, offset, visibleBody)...)
	return strings.Join(capLines(lines, height), "\n")
}

func (m model) renderPeekReviewSummary(j cockpit.Job, width int) []string {
	artifact, _ := cockpit.LoadReviewArtifact(j)
	devlog := devlogPathForJob(j, m.projects)
	previews, err := cockpit.PreviewSyncBack(j, devlog)

	var lines []string
	appendLine := func(text string, style lipgloss.Style) {
		if strings.TrimSpace(text) == "" {
			return
		}
		lines = append(lines, "  "+style.Render(truncate(text, maxInt(12, width-4))))
	}

	if len(j.Sources) > 0 && j.SyncBackState != cockpit.SyncBackApplied && err == nil {
		if len(lines) == 0 {
			lines = append(lines, "  "+panelHeaderStyle.Render("review"))
		}
		appendLine(fmt.Sprintf("sync-back: remove %d task lines · update %d file(s)", len(j.Sources), len(previews)), dimStyle)
	}
	if len(artifact.ChangedFiles) > 0 {
		if len(lines) == 0 {
			lines = append(lines, "  "+panelHeaderStyle.Render("review"))
		}
		appendLine(fmt.Sprintf("changed: %d file(s) · %s", len(artifact.ChangedFiles), artifact.ChangedFiles[0]), primaryStyle)
	}
	if len(artifact.PreexistingDirty) > 0 {
		if len(lines) == 0 {
			lines = append(lines, "  "+panelHeaderStyle.Render("review"))
		}
		appendLine(fmt.Sprintf("dirty: %d file(s) already modified", len(artifact.PreexistingDirty)), warnStyle)
	}
	if hookPreviews := postHookPreviewsForReview(j); len(hookPreviews) > 0 {
		var failCount int
		for _, preview := range hookPreviews {
			if preview.Status == cockpit.HookPreviewWouldFail {
				failCount++
			}
		}
		if len(lines) == 0 {
			lines = append(lines, "  "+panelHeaderStyle.Render("review"))
		}
		switch {
		case failCount > 0:
			appendLine(fmt.Sprintf("hooks: %d preview failure(s)", failCount), warnStyle)
		default:
			appendLine(fmt.Sprintf("hooks: %d preview ok", len(hookPreviews)), dimStyle)
		}
	}
	if next := m.nextQueuedTaskSummary(j); next != "" {
		if len(lines) == 0 {
			lines = append(lines, "  "+panelHeaderStyle.Render("queue"))
		}
		appendLine("next: "+next, textStyle)
	}
	return lines
}

func (m model) agentDetailVisibleBody(j cockpit.Job) int {
	prefixLines := 2
	_, _, rightWidth, innerHeight := m.agentListLayout(prefixLines)
	bodyWidth := rightWidth - 8
	if bodyWidth < 24 {
		bodyWidth = 24
	}
	headerLines := len(m.agentPeekHeaderLines(j, bodyWidth))
	maxHeaderLines := minInt(14, maxInt(8, innerHeight-1))
	if headerLines > maxHeaderLines {
		headerLines = maxHeaderLines
	}
	visibleBody := innerHeight - headerLines
	if visibleBody < 3 {
		visibleBody = 3
	}
	return visibleBody
}

func (m *model) clampAgentDetailOffset() {
	jobs := m.filteredAgentJobs()
	m.agentCursor = clampAgentCursor(m.agentCursor, len(jobs))
	if len(jobs) == 0 {
		m.agentDetailOffset = 0
		return
	}
	prefixLines := 2
	_, _, rightWidth, _ := m.agentListLayout(prefixLines)
	bodyWidth := rightWidth - 8
	if bodyWidth < 24 {
		bodyWidth = 24
	}
	bodyLines := jobPeekBody(jobs[m.agentCursor], bodyWidth)
	m.agentDetailOffset = clampDecoratedScrollOffset(m.agentDetailOffset, len(bodyLines), m.agentDetailVisibleBody(jobs[m.agentCursor]))
}

func jobPeekBody(j cockpit.Job, width int) []string {
	text := jobPeekText(j)
	if text == "" {
		return []string{dimStyle.Render("  (no output yet)")}
	}
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		wrapped := wrapText(raw, width)
		if strings.TrimSpace(wrapped) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return out
}

func (m model) renderReviewLines(j cockpit.Job, width int) []string {
	if len(j.Sources) == 0 && strings.TrimSpace(j.Repo) == "" {
		return nil
	}
	artifact, _ := cockpit.LoadReviewArtifact(j)
	devlog := devlogPathForJob(j, m.projects)
	previews, err := cockpit.PreviewSyncBack(j, devlog)
	if len(j.Sources) > 0 && err != nil {
		return []string{
			"  " + warnStyle.Render("review preview unavailable"),
			"  " + dimStyle.Render(truncate(err.Error(), maxInt(12, width-4))),
		}
	}

	lines := []string{
		"  " + panelHeaderStyle.Render("review"),
	}
	if len(j.Sources) > 0 && j.SyncBackState != cockpit.SyncBackApplied {
		lines = append(lines, "  "+dimStyle.Render(fmt.Sprintf("accept will remove %d task lines and update %d file(s)", len(j.Sources), len(previews))))
		for _, preview := range previews {
			lines = append(lines, "  "+primaryStyle.Render(shortPath(preview.Path)))
			for _, line := range summarizeSyncPreview(preview, width-4, 4) {
				lines = append(lines, "    "+line)
			}
		}
	}
	if len(artifact.ChangedFiles) > 0 {
		lines = append(lines, "  "+primaryStyle.Render("changed files"))
		for _, line := range artifact.ChangedFiles {
			lines = append(lines, "    "+truncate(line, maxInt(12, width-4)))
			if len(lines) > 22 {
				break
			}
		}
	}
	if len(artifact.PreexistingDirty) > 0 {
		lines = append(lines, "  "+warnStyle.Render("preexisting dirty"))
		for _, line := range artifact.PreexistingDirty {
			lines = append(lines, "    "+truncate(line, maxInt(12, width-4)))
			if len(lines) > 28 {
				break
			}
		}
	}
	if len(artifact.DiffStat) > 0 {
		lines = append(lines, "  "+primaryStyle.Render("diff stat"))
		for _, line := range artifact.DiffStat {
			lines = append(lines, "    "+truncate(line, maxInt(12, width-4)))
		}
	}
	if next := m.nextQueuedTaskSummary(j); next != "" {
		lines = append(lines, "  "+primaryStyle.Render("next up"))
		for _, line := range strings.Split(wrapText(next, maxInt(12, width-4)), "\n") {
			lines = append(lines, "    "+line)
		}
	}
	if j.QueueTotal > 1 {
		lines = append(lines, "  "+primaryStyle.Render("queue controls"))
		lines = append(lines, "    "+dimStyle.Render("a accept current result"))
		lines = append(lines, "    "+dimStyle.Render("K skip current item"))
		lines = append(lines, "    "+dimStyle.Render("C skip this item + rest of queue"))
	}
	if eligibleForTakeover(j) {
		lines = append(lines, "  "+primaryStyle.Render("take over"))
		lines = append(lines, "    "+dimStyle.Render("ctrl+r relaunch attended from this Foreman session"))
	}
	if len(artifact.HookEvents) > 0 || len(artifact.PendingPostHooks) > 0 {
		lines = append(lines, "  "+primaryStyle.Render("hooks"))
		for _, line := range summarizeHookActivity(artifact, width-4, 4) {
			lines = append(lines, "    "+line)
		}
	}
	if previews := postHookPreviewsForReview(j); len(previews) > 0 {
		header := fmt.Sprintf("post-hook preview (%d)", len(previews))
		if anyHookWouldFail(previews) {
			lines = append(lines, "  "+warnStyle.Render(header))
		} else {
			lines = append(lines, "  "+primaryStyle.Render(header))
		}
		for _, line := range summarizeHookPreviews(previews, width-4) {
			lines = append(lines, "    "+line)
		}
	}
	if len(j.Sources) > 0 && j.SyncBackState != cockpit.SyncBackApplied {
		if anyHookWouldFail(postHookPreviewsForReview(j)) {
			lines = append(lines, "  "+warnStyle.Render("post-hook would fail — review output before pressing a"))
		} else {
			lines = append(lines, "  "+dimStyle.Render("press a to accept and sync back"))
		}
	}
	return lines
}

// postHookPreviewsForReview returns the cached preview if available, or
// triggers a fresh dry-run for jobs that look approve-eligible. Skipped
// for transient states (queued/running) since hooks haven't had a chance
// to be relevant yet.
func postHookPreviewsForReview(j cockpit.Job) []cockpit.HookPreview {
	switch j.Status {
	case cockpit.StatusIdle, cockpit.StatusAwaitingHuman, cockpit.StatusNeedsReview, cockpit.StatusCompleted, cockpit.StatusFailed, cockpit.StatusBlocked:
	default:
		return nil
	}
	if len(j.Hooks.PostShell) == 0 {
		return nil
	}
	return cockpit.PreviewPostHooks(j)
}

func anyHookWouldFail(previews []cockpit.HookPreview) bool {
	for _, p := range previews {
		if p.Status == cockpit.HookPreviewWouldFail {
			return true
		}
	}
	return false
}

func summarizeHookPreviews(previews []cockpit.HookPreview, width int) []string {
	if width < 12 {
		width = 12
	}
	out := make([]string, 0, len(previews))
	for _, p := range previews {
		var glyph string
		var style lipgloss.Style
		switch p.Status {
		case cockpit.HookPreviewOK:
			glyph, style = "✓", primaryStyle
		case cockpit.HookPreviewWouldFail:
			glyph, style = "✗", warnStyle
		case cockpit.HookPreviewSkipped:
			glyph, style = "⊘", dimStyle
		case cockpit.HookPreviewError:
			glyph, style = "!", warnStyle
		default:
			glyph, style = "·", dimStyle
		}
		label := strings.TrimSpace(p.Name)
		if label == "" {
			label = strings.TrimSpace(p.Cmd)
		}
		var trail string
		switch p.Status {
		case cockpit.HookPreviewOK:
			if p.DurationMS > 0 {
				trail = fmt.Sprintf(" (%s)", formatHookDuration(p.DurationMS))
			}
		case cockpit.HookPreviewWouldFail:
			trail = fmt.Sprintf(" exit %d", p.ExitCode)
		case cockpit.HookPreviewSkipped, cockpit.HookPreviewError:
			if p.SkipReason != "" {
				trail = " — " + p.SkipReason
			}
		}
		row := truncate(glyph+" "+label+trail, width)
		out = append(out, style.Render(row))
		if p.Status == cockpit.HookPreviewWouldFail && strings.TrimSpace(p.Output) != "" {
			first := strings.SplitN(strings.TrimSpace(p.Output), "\n", 2)[0]
			out = append(out, dimStyle.Render(truncate(first, width)))
		}
	}
	return out
}

func formatHookDuration(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dm%02ds", ms/60_000, (ms%60_000)/1000)
	}
}

func summarizeSyncPreview(preview cockpit.SyncBackPreview, width, maxLines int) []string {
	if width < 8 {
		width = 8
	}
	diffLines := diff.Unified(preview.Before, preview.After)
	out := make([]string, 0, maxLines)
	for _, line := range diffLines {
		if line.Type == diff.Context {
			continue
		}
		prefix := "+"
		style := primaryStyle
		if line.Type == diff.Removed {
			prefix = "-"
			style = warnStyle
		}
		out = append(out, style.Render(prefix+truncate(line.Content, width-1)))
		if len(out) >= maxLines {
			break
		}
	}
	if len(out) == 0 {
		return []string{dimStyle.Render("(no content changes)")}
	}
	return out
}

func summarizeHookActivity(artifact cockpit.ReviewArtifact, width, maxLines int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, hook := range artifact.HookEvents {
		if hook.Name == "" {
			continue
		}
		line := strings.TrimSpace(hook.Phase + " " + hook.Name)
		if hook.ExitCode != 0 {
			line += fmt.Sprintf(" (exit %d)", hook.ExitCode)
			out = append(out, warnStyle.Render(truncate(line, width)))
		} else {
			out = append(out, primaryStyle.Render(truncate(line, width)))
		}
		if text := strings.TrimSpace(hook.Output); text != "" {
			first := strings.SplitN(text, "\n", 2)[0]
			out = append(out, dimStyle.Render(truncate(first, width)))
		}
		if len(out) >= maxLines {
			return out[:maxLines]
		}
	}
	for _, pending := range artifact.PendingPostHooks {
		out = append(out, dimStyle.Render("pending post: "+truncate(pending, maxInt(4, width-14))))
		if len(out) >= maxLines {
			return out[:maxLines]
		}
	}
	if len(out) == 0 {
		return []string{dimStyle.Render("(no hooks)")}
	}
	return out
}

func (m model) nextQueuedTaskSummary(current cockpit.Job) string {
	if current.CampaignID == "" || current.QueueTotal <= 1 {
		return ""
	}
	for _, job := range m.orderedAgentJobs() {
		if job.CampaignID != current.CampaignID || job.QueueIndex <= current.QueueIndex {
			continue
		}
		if job.Status != cockpit.StatusQueued {
			continue
		}
		return fmt.Sprintf("%d/%d %s", job.QueueIndex+1, job.QueueTotal, jobTaskText(job))
	}
	return ""
}

func jobOperatorStatus(j cockpit.Job) (string, lipgloss.Style) {
	switch j.Status {
	case cockpit.StatusNeedsReview:
		return "needs review", warnStyle
	case cockpit.StatusBlocked:
		return "blocked", warnStyle
	case cockpit.StatusFailed:
		return "failed", warnStyle
	case cockpit.StatusQueued:
		if j.EligibilityReason != "" && j.EligibilityReason != "waiting for foreman" {
			return "deferred", warnStyle
		}
		if j.WaitForForeman {
			return "waiting for foreman", accentStyle
		}
		return "queued", dimStyle
	case cockpit.StatusCompleted:
		if j.SupersededBy != "" {
			return "taken over", dimStyle
		}
		if j.SyncBackState == cockpit.SyncBackSkipped || strings.Contains(strings.ToLower(j.Note), "skipped") {
			return "skipped", dimStyle
		}
		return "done", dimStyle
	case cockpit.StatusAwaitingHuman:
		return "waiting on you", accentStyle
	case cockpit.StatusIdle:
		switch stoppedLikeStatusLabel(j) {
		case "closed":
			return "closed", dimStyle
		case "stopped":
			return "stopped", warnStyle
		}
		return "waiting for input", accentStyle
	case cockpit.StatusRunning:
		if j.Runner == cockpit.RunnerTmux && tmuxJobLikelyWaiting(j) {
			return "waiting for input", accentStyle
		}
		return "working", primaryStyle
	default:
		return statusLabel(j.Status), dimStyle
	}
}

func jobAdvanceState(j cockpit.Job) (string, lipgloss.Style) {
	deferred := j.EligibilityReason != "" && j.EligibilityReason != "waiting for foreman"
	if j.QueueTotal <= 1 {
		switch j.Status {
		case cockpit.StatusAwaitingHuman:
			return "input", accentStyle
		case cockpit.StatusNeedsReview:
			return "review", warnStyle
		case cockpit.StatusRunning, cockpit.StatusIdle:
			if stoppedLikeStatusLabel(j) != "" {
				return "hold", warnStyle
			}
			return "active", primaryStyle
		case cockpit.StatusCompleted:
			if j.SupersededBy != "" {
				return "taken over", dimStyle
			}
			if j.SyncBackState == cockpit.SyncBackSkipped || strings.Contains(strings.ToLower(j.Note), "skipped") {
				return "skipped", dimStyle
			}
			return "done", dimStyle
		case cockpit.StatusFailed, cockpit.StatusBlocked:
			return "hold", warnStyle
		default:
			if deferred {
				return "hold", warnStyle
			}
			if j.WaitForForeman {
				return "foreman", accentStyle
			}
			return "solo", dimStyle
		}
	}

	label := fmt.Sprintf("%d/%d", j.QueueIndex+1, j.QueueTotal)
	switch j.Status {
	case cockpit.StatusQueued:
		if deferred {
			return label + " hold", warnStyle
		}
		if j.WaitForForeman {
			return label + " foreman", accentStyle
		}
		return label + " next", dimStyle
	case cockpit.StatusAwaitingHuman:
		return label + " input", accentStyle
	case cockpit.StatusNeedsReview:
		return label + " review", warnStyle
	case cockpit.StatusRunning, cockpit.StatusIdle:
		if stoppedLikeStatusLabel(j) != "" {
			return label + " hold", warnStyle
		}
		return label + " active", primaryStyle
	case cockpit.StatusCompleted:
		if j.SupersededBy != "" {
			return label + " taken over", dimStyle
		}
		if j.SyncBackState == cockpit.SyncBackSkipped || strings.Contains(strings.ToLower(j.Note), "skipped") {
			return label + " skipped", dimStyle
		}
		return label + " done", dimStyle
	case cockpit.StatusFailed, cockpit.StatusBlocked:
		return label + " hold", warnStyle
	default:
		return label, dimStyle
	}
}

func compactJobStatus(j cockpit.Job) string {
	switch status, _ := jobOperatorStatus(j); status {
	case "waiting on you":
		return "human"
	case "waiting for input":
		return "waiting"
	case "waiting for foreman":
		return "foreman"
	case "needs review":
		return "review"
	default:
		return status
	}
}

func jobRepoLabel(j cockpit.Job) string {
	if j.Repo != "" {
		base := filepath.Base(j.Repo)
		if strings.TrimSpace(base) != "" && base != "." && base != string(filepath.Separator) {
			return base
		}
		return shortPath(j.Repo)
	}
	if len(j.Sources) > 0 && strings.TrimSpace(j.Sources[0].Project) != "" {
		return j.Sources[0].Project
	}
	return "no-repo"
}

func jobListSummary(j cockpit.Job) string {
	if task := strings.TrimSpace(j.Task); task != "" {
		return compactSingleLine(task)
	}
	if len(j.Sources) > 0 {
		summary := compactSingleLine(j.Sources[0].Text)
		if len(j.Sources) > 1 {
			summary += fmt.Sprintf(" +%d", len(j.Sources)-1)
		}
		return summary
	}
	if strings.TrimSpace(j.Brief) != "" {
		return compactSingleLine(j.Brief)
	}
	return j.PresetID
}

func compactSingleLine(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func shortJobID(id cockpit.JobID) string {
	s := string(id)
	if len(s) <= 6 {
		return s
	}
	return s[len(s)-6:]
}

func stoppedLikeStatusLabel(j cockpit.Job) string {
	note := strings.ToLower(strings.TrimSpace(j.Note))
	switch {
	case strings.Contains(note, "tmux window closed"), strings.Contains(note, "tmux session ended"):
		return "closed"
	case strings.Contains(note, "interrupted by restart"), strings.Contains(note, "interrupt"), strings.Contains(note, "stopped"):
		return "stopped"
	default:
		return ""
	}
}

func tmuxJobLikelyWaiting(j cockpit.Job) bool {
	if j.Runner != cockpit.RunnerTmux {
		return false
	}
	if j.LogPath == "" {
		return false
	}
	st, err := os.Stat(j.LogPath)
	if err != nil {
		return false
	}
	if time.Since(st.ModTime()) <= cockpit.SupervisorQuietPeriod {
		return false
	}
	return true
}

func jobElapsedSummary(j cockpit.Job) string {
	switch j.Status {
	case cockpit.StatusRunning:
		if d, ok := inFlightElapsed(j); ok {
			return "current turn " + formatTurnDuration(d)
		}
		if j.Runner == cockpit.RunnerTmux {
			return "session open " + formatTurnDuration(time.Since(j.CreatedAt))
		}
	case cockpit.StatusAwaitingHuman:
		return "waiting on you for " + formatTurnDuration(time.Since(j.CreatedAt))
	case cockpit.StatusIdle:
		if stoppedLikeStatusLabel(j) != "" {
			return ""
		}
		return "idle for " + formatTurnDuration(time.Since(j.CreatedAt))
	}
	return ""
}

func jobPeekText(j cockpit.Job) string {
	if j.Runner == cockpit.RunnerTmux {
		if text := tmuxSessionText(j); text != "" {
			return text
		}
	} else {
		if j.TranscriptPath == "" {
			return ""
		}
		body, err := os.ReadFile(j.TranscriptPath)
		if err != nil {
			return ""
		}
		text := transcript.Sanitize(string(body))
		if text == "" {
			return ""
		}
		return text
	}
	if j.LogPath == "" {
		return ""
	}
	body, err := os.ReadFile(j.LogPath)
	if err != nil {
		return ""
	}
	return transcript.Sanitize(string(body))
}

func tmuxSessionText(j cockpit.Job) string {
	if j.TmuxTarget != "" {
		if body, err := cockpit.CapturePane(j.TmuxTarget); err == nil {
			if text := transcript.Sanitize(body); text != "" {
				return text
			}
		}
	}
	if j.LogPath == "" {
		return ""
	}
	body, err := os.ReadFile(j.LogPath)
	if err != nil {
		return ""
	}
	return transcript.Sanitize(string(body))
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
	return wrapLine(strings.TrimSpace(s), width)
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

func scrollOffsetForCursor(total, cursor, visible int) int {
	if total <= 0 || visible <= 0 {
		return 0
	}
	size := visible
	if total > visible {
		size = visible - 2 // top/bottom indicators can each consume one row
		if size < 1 {
			size = 1
		}
	}
	start, _ := windowRange(total, cursor, size)
	return start
}

func clampDecoratedScrollOffset(offset, total, visible int) int {
	if offset < 0 {
		return 0
	}
	if visible <= 0 || total <= visible {
		return 0
	}
	maxOffset := total - visible + 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func clampScrollOffset(offset, total, visible int) int {
	if offset < 0 {
		return 0
	}
	if visible <= 0 || total <= visible {
		return 0
	}
	maxOffset := total - visible
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func scrollWindowFromBottom(lines []string, offset, visible int) []string {
	if visible <= 0 || len(lines) <= visible {
		return lines
	}
	offset = clampDecoratedScrollOffset(offset, len(lines), visible)
	end := len(lines) - offset
	if end > len(lines) {
		end = len(lines)
	}
	start := end - visible
	if start < 0 {
		start = 0
	}
	out := make([]string, 0, visible)
	if start > 0 {
		out = append(out, dimStyle.Render(fmt.Sprintf("  ▲ %d older", start)))
	}
	remaining := visible - len(out)
	if end-start > remaining {
		start = end - remaining
	}
	if start < 0 {
		start = 0
	}
	out = append(out, lines[start:end]...)
	if end < len(lines) {
		out = append(out, dimStyle.Render(fmt.Sprintf("  ▼ %d newer", len(lines)-end)))
	}
	return out
}

// scrollWindow returns a slice of `lines` starting at `offset`, decorating
// with "▲ N more" / "▼ N more" indicators so the viewer can tell scrolling
// is possible. If the content already fits in `visible`, returns as-is.
func scrollWindow(lines []string, offset, visible int) []string {
	if visible <= 0 || len(lines) <= visible {
		return lines
	}
	offset = clampDecoratedScrollOffset(offset, len(lines), visible)
	out := make([]string, 0, visible)
	if offset > 0 {
		out = append(out, dimStyle.Render(fmt.Sprintf("  ▲ %d more", offset)))
	}
	remaining := visible - len(out)
	end := offset + remaining
	hasMoreBelow := end < len(lines)
	if hasMoreBelow {
		end--
	}
	if end > len(lines) {
		end = len(lines)
	}
	if end < offset {
		end = offset
	}
	out = append(out, lines[offset:end]...)
	if hasMoreBelow {
		out = append(out, dimStyle.Render(fmt.Sprintf("  ▼ %d more", len(lines)-end)))
	}
	return out
}

func jobOrderRank(j cockpit.Job) int {
	status, _ := jobOperatorStatus(j)
	switch status {
	case "working":
		return 0
	case "waiting on you":
		return 1
	case "waiting for input":
		return 2
	case "needs review":
		return 3
	case "queued", "waiting for foreman", "deferred":
		return 4
	case "blocked", "failed", "stopped", "closed":
		return 5
	case "done", "skipped":
		return 6
	default:
		return 7
	}
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
	height := m.contentAreaHeight()
	if height < 3 {
		return 3
	}
	return height
}

// renderAgentJobsHeader builds the jobs-panel header in a single pass over
// the jobs list: current filter, summary counts, executor mix, and provider
// limit rows. The top line makes the active filter explicit instead of relying
// on numbered shortcuts.
func renderAgentJobsHeader(jobs []cockpit.Job, active string, foreman cockpit.ForemanState, width int) []string {
	if active == "" {
		active = "all"
	}

	foremanLabel := "OFF"
	foremanStyle := warnStyle
	if foreman.Enabled {
		foremanLabel = "ON"
		foremanStyle = primaryStyle
	}
	focus := "  " + accentStyle.Bold(true).Render(strings.ToUpper(active[:1])+active[1:])
	focus += dimStyle.Render("  filter")
	focus += dimStyle.Render("  ·  foreman ") + foremanStyle.Bold(true).Render(foremanLabel)
	return wrapLines(focus, width)
}

type foremanPool struct {
	parked       int
	eligible     int
	deferred     int
	active       int
	maxActive    int
	foremanState cockpit.ForemanState
}

func (p foremanPool) hasAny() bool {
	return p.parked > 0 || p.deferred > 0 || p.active > 0
}

func (p foremanPool) render() string {
	parts := []string{
		textStyle.Render(fmt.Sprintf("%d parked", p.parked)),
		textStyle.Render(fmt.Sprintf("%d eligible", p.eligible)),
	}
	deferredStyle := textStyle
	if p.deferred > 0 {
		deferredStyle = warnStyle
	}
	parts = append(parts, deferredStyle.Render(fmt.Sprintf("%d deferred", p.deferred)))
	activeStyle := textStyle
	if p.maxActive > 0 && p.active >= p.maxActive {
		activeStyle = warnStyle
	}
	parts = append(parts, activeStyle.Render(fmt.Sprintf("%d/%d active", p.active, p.maxActive)))
	return strings.Join(parts, dimStyle.Render(" · "))
}

func foremanPoolCounts(jobs []cockpit.Job, state cockpit.ForemanState) foremanPool {
	max := state.MaxConcurrent
	if max <= 0 {
		max = cockpit.ForemanMaxConcurrentDefault
	}
	pool := foremanPool{foremanState: state, maxActive: max}
	for _, j := range jobs {
		if j.WaitForForeman && j.Status == cockpit.StatusQueued {
			pool.parked++
			if state.Enabled {
				pool.eligible++
			}
		}
		if j.EligibilityReason != "" && j.EligibilityReason != "waiting for foreman" && j.Status == cockpit.StatusQueued {
			pool.deferred++
		}
		if j.ForemanManaged {
			switch j.Status {
			case cockpit.StatusRunning, cockpit.StatusIdle:
				pool.active++
			}
		}
	}
	return pool
}

// renderProviderLimitsLines emits one row per provider (claude, codex)
// with 5h/7d percent + reset time. Returns nil when neither provider
// has usable data so the header doesn't gain blank rows.
func renderProviderLimitsLines(width int) []string {
	var lines []string
	if u, ok := statusbar.FetchClaude(); ok {
		if s := renderProviderLimitsRow(u, width); s != "" {
			lines = append(lines, s)
		}
	}
	if u, ok := statusbar.FetchCodex(); ok {
		if s := renderProviderLimitsRow(u, width); s != "" {
			lines = append(lines, s)
		}
	}
	return lines
}

func renderProviderLimitsRow(u statusbar.Usage, width int) string {
	labelW := 7
	windowLabelW := 5
	valueW := 6
	resetW := 7
	seg := func(label string, pct int, reset time.Time, showReset bool) string {
		left := textStyle.Width(windowLabelW).Render(label)
		value := limitPctStyle(pct).Width(valueW).Align(lipgloss.Right).Render(fmt.Sprintf("%d%%", clampPct(pct)))
		if !showReset {
			return left + value
		}
		resetText := dimStyle.Width(resetW).Render("")
		if !reset.IsZero() {
			resetText = dimStyle.Width(resetW).Align(lipgloss.Right).Render(formatLimitReset(reset, false))
		}
		return left + value + dimStyle.Render(" ") + resetText
	}

	var segs []string
	if u.FiveHour.Available {
		segs = append(segs, seg("5h", u.FiveHour.PctUsed, u.FiveHour.ResetAt, true))
	}
	if u.SevenDay.Available {
		segs = append(segs, seg("7d", u.SevenDay.PctUsed, u.SevenDay.ResetAt, false))
	}
	if u.Extra != nil && u.Extra.Enabled {
		extraLabel := textStyle.Width(windowLabelW).Render("extra")
		extraValue := limitPctStyle(u.Extra.PctUsed).Render(fmt.Sprintf("$%.2f/$%.2f", u.Extra.UsedCredits, u.Extra.MonthlyLimit))
		segs = append(segs, extraLabel+extraValue)
	}
	if len(segs) == 0 {
		return ""
	}
	source := activeTabStyle.Width(labelW).Render(u.Source)
	return wrapLine("  "+source+" "+strings.Join(segs, dimStyle.Render("  ·  ")), width)
}

func limitPctStyle(pct int) lipgloss.Style {
	switch {
	case pct >= 90:
		return warnStyle
	case pct >= 70:
		return currentStyle
	case pct >= 50:
		return accentStyle
	default:
		return primaryStyle
	}
}

func clampPct(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func formatLimitReset(t time.Time, long bool) string {
	local := t.Local()
	if long {
		return strings.ToLower(local.Format("Jan 2, 3:04pm"))
	}
	return strings.ToLower(local.Format("3:04pm"))
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

// --- Picker (file → items → multi-select) ---

// --- Launch modal (preset + brief) ---

// --- Attached view ---

// agentRowColumnWidths returns stable repo/status/preset widths sized to the
// list panel. Task is the flex column; these three stay constant across
// resizes so the eye can scan straight down each column.
func agentRowColumnWidths(rowWidth int) (repo, advance, status, preset int) {
	repo = 12
	advance = 10
	status = 9
	preset = 12
	if rowWidth < 60 {
		preset = 10
	}
	if rowWidth < 70 {
		advance = 8
	}
	if rowWidth < 52 {
		repo = 10
	}
	if rowWidth < 44 {
		status = 7
	}
	return repo, advance, status, preset
}

func renderAgentColumnHeader(rowWidth, repoW, advanceW, statusW, presetW int) string {
	taskW := rowWidth - 2 - repoW - advanceW - statusW - presetW - 4
	if taskW < 8 {
		taskW = 8
	}
	pad := func(s string, w int, right bool) string {
		if len(s) > w {
			s = s[:w]
		}
		gap := strings.Repeat(" ", w-len(s))
		if right {
			return gap + s
		}
		return s + gap
	}
	header := "  " +
		pad("repo", repoW, false) + " " +
		pad("task", taskW, false) + " " +
		pad("queue", advanceW, false) + " " +
		pad("status", statusW, false) + " " +
		pad("role", presetW, true)
	return dimStyle.Render(header)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
