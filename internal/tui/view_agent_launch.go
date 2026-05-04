package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (m model) renderAgentLaunch() string {
	contentHeight := m.agentContentHeight()
	lineWidth := maxInt(20, m.width-4)

	presetLabel := "(none)"
	if m.launchPresetIdx < len(m.cockpitPresets) {
		presetLabel = m.cockpitPresets[m.launchPresetIdx].Name
	}
	providerLabel := m.launchProviderLabel()

	tab := func(name string, focus int) string {
		if m.launchFocus == focus {
			return accentStyle.Bold(true).Render("▸ " + name)
		}
		return dimStyle.Render("  " + name)
	}
	var tabs []string
	tabs = append(tabs, tab("Role", launchFocusRole), tab("Engine", launchFocusEngine))
	if m.launchAdvancedVisible() {
		tabs = append(tabs, tab("Prompt", launchFocusPrompt), tab("Hooks", launchFocusHooks), tab("Perms", launchFocusPerms))
	}
	if m.launchHasRepoStep() {
		tabs = append(tabs, tab("Repo", m.launchRepoFocus()))
	}
	tabs = append(tabs, tab("Note", m.launchNoteFocus()), tab("Review", m.launchReviewFocus()))
	subtabs := strings.Join(tabs, dimStyle.Render("  ·  "))

	lines := m.renderAgentLaunchPrefixLines(subtabs, presetLabel, providerLabel)
	queueMode := "start now"
	queueStyle := primaryStyle
	if m.launchQueueOnly {
		queueMode = "send to Foreman"
		queueStyle = accentStyle
	}
	foremanLabel := "OFF"
	foremanStyle := warnStyle
	if m.cockpitForeman.Enabled {
		foremanLabel = "ON"
		foremanStyle = primaryStyle
	}
	lines = append(lines, dimStyle.Render("  launch mode=")+queueStyle.Render(queueMode)+
		dimStyle.Render("  ·  foreman ")+foremanStyle.Bold(true).Render(foremanLabel), "")
	if len(m.launchSources) > 0 {
		var src []string
		for i, s := range m.launchSources {
			if i >= 3 {
				src = append(src, fmt.Sprintf("+%d more", len(m.launchSources)-i))
				break
			}
			src = append(src, strings.TrimSpace(s.Text))
		}
		lines = append(lines, wrapLines(dimStyle.Render("  sources: "+strings.Join(src, "  ·  ")), lineWidth)...)
		lines = append(lines, "")
	}
	visibleRows := contentHeight - len(lines)
	if visibleRows < 1 {
		visibleRows = 1
	}
	rowsReserved := 3
	if m.launchSelectEditing {
		rowsReserved += 3
		m.launchSelectInput.Width = maxInt(1, m.width-14)
	}
	if m.launchHasRepoStep() && m.launchFocus == m.launchRepoFocus() && m.launchRepoEditing {
		rowsReserved += 3
		m.launchRepoCustom.Width = maxInt(1, m.width-14)
	}
	listRows := visibleRows - rowsReserved
	if listRows < 1 {
		listRows = 1
	}

	switch {
	case m.launchFocus == launchFocusRole:
		lines = append(lines, panelHeaderStyle.Render("  Step 1 · Choose Role"), dimStyle.Render("  reusable run behavior and defaults · enter continues · e types a match"), "")
		if m.launchSelectEditing {
			lines = append(lines, dimStyle.Render("  type role id/name · enter to select · esc to cancel"))
			lines = append(lines, "  "+m.launchSelectInput.View(), "")
		}
		var options []string
		for i, p := range m.cockpitPresets {
			prefix := "  "
			if i == m.launchPresetIdx {
				prefix = accentStyle.Render("▸ ")
			}
			options = append(options, prefix+p.Name)
		}
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), m.launchPresetIdx, listRows), listRows)...)
	case m.launchFocus == launchFocusEngine:
		lines = append(lines, panelHeaderStyle.Render("  Step 2 · Choose Engine"), dimStyle.Render("  concrete CLI / model to run · enter continues · e types one"), "")
		if m.launchSelectEditing {
			lines = append(lines, dimStyle.Render("  type engine id/name · blank = role default · enter to select · esc to cancel"))
			lines = append(lines, "  "+m.launchSelectInput.View(), "")
		}
		var options []string
		options = append(options, launchOverrideOption("(role default)", m.launchProviderIdx == -1))
		for i, provider := range m.cockpitProviders {
			prefix := "  "
			name := provider.Name
			if i == m.launchProviderIdx {
				prefix = accentStyle.Render("▸ ")
				name = accentStyle.Bold(true).Render(name)
			}
			options = append(options, prefix+name)
		}
		cursor := m.launchProviderIdx + 1
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), cursor, listRows), listRows)...)
	case m.launchFocus == launchFocusPrompt:
		lines = append(lines, panelHeaderStyle.Render("  Advanced · Prompt Override"), dimStyle.Render("  per-run system prompt override · e to type one"), "")
		if m.launchSelectEditing {
			lines = append(lines, dimStyle.Render("  type prompt id/name · blank = none · 'default' = role default"))
			lines = append(lines, "  "+m.launchSelectInput.View(), "")
		}
		var options []string
		options = append(options, launchOverrideOption("(role default)", m.launchPromptIdx == launchPromptRoleDefault))
		options = append(options, launchOverrideOption("(none)", m.launchPromptIdx == launchPromptNone))
		for i, p := range m.cockpitPrompts {
			options = append(options, launchOverrideOption(p.Name, m.launchPromptIdx == i))
		}
		cursor := m.launchPromptIdx + 2
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), cursor, listRows), listRows)...)
	case m.launchFocus == launchFocusHooks:
		lines = append(lines, panelHeaderStyle.Render("  Advanced · Hook Bundles"), dimStyle.Render("  space/enter toggles · multiple bundles compose · (role default) clears overrides"), "")
		var options []string
		options = append(options, launchHookRow("(role default)", !m.launchHookOverride, m.launchHookCursor == -1, false))
		for i, b := range m.cockpitHookBundles {
			selected := m.launchHookOverride && m.launchHookSelected[b.ID]
			options = append(options, launchHookRow(b.Name, selected, m.launchHookCursor == i, true))
		}
		cursor := m.launchHookCursor + 1
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), cursor, listRows), listRows)...)
	case m.launchFocus == launchFocusPerms:
		lines = append(lines, panelHeaderStyle.Render("  Advanced · Permissions"), dimStyle.Render("  per-run permission override"), "")
		var options []string
		for i, label := range launchPermsLabels {
			options = append(options, launchOverrideOption(label, m.launchPermsIdx == i))
		}
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), m.launchPermsIdx, listRows), listRows)...)
	case m.launchHasRepoStep() && m.launchFocus == m.launchRepoFocus():
		lines = append(lines, panelHeaderStyle.Render("  Step 3 · Choose Repo"))
		if m.launchRepoEditing {
			lines = append(lines, "")
		} else {
			lines = append(lines, dimStyle.Render("  where the agent should run · enter on (custom path…) to type any path"), "")
		}
		repos := m.launchRepoChoices()
		selected := indexOfLaunchRepo(repos, m.launchRepo)
		var options []string
		for i, repo := range repos {
			prefix := "  "
			var label string
			switch repo {
			case repoSentinelCustom:
				label = "(custom path…)"
				if i == selected {
					label = accentStyle.Bold(true).Render(label)
				} else {
					label = dimStyle.Render(label)
				}
			default:
				label = launchRepoPathLabel(repo)
				if label == "" {
					label = "(current working directory)"
				}
				if i == selected {
					label = accentStyle.Bold(true).Render(label)
				}
			}
			if i == selected {
				prefix = accentStyle.Render("▸ ")
			}
			options = append(options, prefix+label)
		}
		if m.launchRepoEditing {
			lines = append(lines, dimStyle.Render("  type repo path · enter to set · esc to cancel"))
			lines = append(lines, "  "+m.launchRepoCustom.View())
			if listRows > 0 {
				lines = append(lines, "")
			}
		}
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), selected, listRows), listRows)...)
	case m.launchFocus == m.launchNoteFocus():
		m.launchBrief.SetWidth(m.width - 6)
		briefH := visibleRows - 2 // section title + subtitle
		if briefH < 1 {
			briefH = 1
		}
		m.launchBrief.SetHeight(briefH)
		lines = append(lines, panelHeaderStyle.Render("  Step 4 · Note"), dimStyle.Render("  optional run note · enter to review · alt+enter launches now"), "")
		lines = append(lines, m.launchBrief.View())
	case m.launchFocus == m.launchReviewFocus():
		lines = append(lines, scrollWindow(launchReviewLines(m), m.launchReviewOffset, visibleRows)...)
	}

	return strings.Join(capLines(lines, contentHeight), "\n")
}

func (m model) renderAgentLaunchPrefixLines(subtabs, presetLabel, providerLabel string) []string {
	var lines []string
	title := titleStyle.Render("New Run")
	if m.launchQueueOnly {
		title += "   " + accentStyle.Bold(true).Render("[Foreman queue]")
	} else {
		title += "   " + dimStyle.Render("[Start now]")
	}
	lines = append(lines, title+"   "+subtabs, "")
	if m.width >= 48 {
		flowHint := dimStyle.Render("  path: role -> engine")
		if m.launchHasRepoStep() {
			flowHint += dimStyle.Render(" -> repo")
		}
		flowHint += dimStyle.Render(" -> note -> review")
		if m.launchAdvancedVisible() {
			flowHint += dimStyle.Render("  ·  ")
			flowHint += accentStyle.Render("advanced overrides visible")
			flowHint += dimStyle.Render(" (a to hide)")
		} else {
			flowHint += dimStyle.Render("  ·  advanced overrides hidden (a to show)")
		}
		lines = append(lines, wrapLines(flowHint, maxInt(20, m.width-4))...)
		lines = append(lines, "")
	}

	summary := dimStyle.Render("  role=") + textStyle.Render(presetLabel) +
		dimStyle.Render("  engine=") + textStyle.Render(providerLabel) +
		dimStyle.Render("  repo=") + textStyle.Render(launchRepoPathLabel(m.launchRepo)) +
		dimStyle.Render(fmt.Sprintf("  %d sources", len(m.launchSources)))
	// Only surface override tags when active — keeps the line short on
	// narrow terminals when the role's defaults are good enough.
	switch {
	case m.launchPromptIdx == launchPromptNone:
		summary += dimStyle.Render("  prompt=") + accentStyle.Render("(none)")
	case m.launchPromptIdx >= 0 && m.launchPromptIdx < len(m.cockpitPrompts):
		summary += dimStyle.Render("  prompt=") + accentStyle.Render(m.cockpitPrompts[m.launchPromptIdx].Name)
	}
	if m.launchHookOverride {
		var names []string
		for _, b := range m.cockpitHookBundles {
			if m.launchHookSelected[b.ID] {
				names = append(names, b.Name)
			}
		}
		label := "(none)"
		if len(names) > 0 {
			label = strings.Join(names, "+")
		}
		summary += dimStyle.Render("  hooks=") + accentStyle.Render(label)
	}
	if v := launchPermsValue(m.launchPermsIdx); v != "" {
		summary += dimStyle.Render("  perms=") + accentStyle.Render(v)
	}
	lines = append(lines, wrapLines(summary, maxInt(20, m.width-4))...)
	lines = append(lines, "")
	return lines
}

func (m model) launchProviderLabel() string {
	if m.launchProviderIdx >= 0 && m.launchProviderIdx < len(m.cockpitProviders) {
		return m.cockpitProviders[m.launchProviderIdx].Name
	}
	if m.launchPresetIdx >= 0 && m.launchPresetIdx < len(m.cockpitPresets) {
		return "(role default: " + describeExecutor(m.cockpitPresets[m.launchPresetIdx].Executor) + ")"
	}
	return "(none)"
}

func launchOverrideOption(label string, selected bool) string {
	if selected {
		return accentStyle.Render("▸ ") + accentStyle.Bold(true).Render(label)
	}
	return "  " + label
}

// launchHookRow renders one row of the multi-select hook list. The cursor
// arrow is the navigation indicator (j/k); the [x]/[ ] checkbox is the
// selection state (toggle with space/enter). showCheckbox is false for
// the "(role default)" sentinel — its "selected" state is just whether
// override is off, shown via the cursor/text styling instead.
func launchHookRow(label string, selected, atCursor, showCheckbox bool) string {
	var arrow, box, text string
	if atCursor {
		arrow = accentStyle.Render("▸ ")
	} else {
		arrow = "  "
	}
	if showCheckbox {
		if selected {
			box = accentStyle.Render("[x] ")
		} else {
			box = dimStyle.Render("[ ] ")
		}
	}
	if selected || atCursor {
		text = accentStyle.Bold(true).Render(label)
	} else {
		text = label
	}
	return arrow + box + text
}

func launchRepoPathLabel(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path == repoSentinelCustom {
		return "(custom path…)"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return "~"
			}
			return filepath.Join("~", rel)
		}
	}
	return path
}

func (m model) launchReviewVisibleRows() int {
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
	var tabs []string
	tabs = append(tabs, tab("Role", launchFocusRole), tab("Engine", launchFocusEngine))
	if m.launchAdvancedVisible() {
		tabs = append(tabs, tab("Prompt", launchFocusPrompt), tab("Hooks", launchFocusHooks), tab("Perms", launchFocusPerms))
	}
	if m.launchHasRepoStep() {
		tabs = append(tabs, tab("Repo", m.launchRepoFocus()))
	}
	tabs = append(tabs, tab("Note", m.launchNoteFocus()), tab("Review", m.launchReviewFocus()))
	subtabs := strings.Join(tabs, dimStyle.Render("  ·  "))

	lines := m.renderAgentLaunchPrefixLines(subtabs, presetLabel, providerLabel)
	queueLines := 2
	if len(m.launchSources) > 0 {
		queueLines += 2
	}
	visibleRows := m.agentContentHeight() - len(lines) - queueLines
	if visibleRows < 1 {
		visibleRows = 1
	}
	return visibleRows
}

func (m *model) clampLaunchReviewOffset() {
	m.launchReviewOffset = clampDecoratedScrollOffset(m.launchReviewOffset, len(launchReviewLines(*m)), m.launchReviewVisibleRows())
}
