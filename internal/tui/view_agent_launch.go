package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (m model) renderAgentLaunch() string {
	providers := providerChoices(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
	contentHeight := m.agentContentHeight()
	lineWidth := maxInt(20, m.width-4)

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
	subtabs := tab("Role", launchFocusRole) + dimStyle.Render("  ·  ") +
		tab("Engine", launchFocusEngine) + dimStyle.Render("  ·  ") +
		tab("Prompt", launchFocusPrompt) + dimStyle.Render("  ·  ") +
		tab("Hooks", launchFocusHooks) + dimStyle.Render("  ·  ") +
		tab("Perms", launchFocusPerms)
	if m.launchHasRepoStep() {
		subtabs += dimStyle.Render("  ·  ") + tab("Repo", m.launchRepoFocus())
	}
	subtabs += dimStyle.Render("  ·  ") + tab("Note", m.launchNoteFocus()) +
		dimStyle.Render("  ·  ") + tab("Review", m.launchReviewFocus())

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
	if m.launchHasRepoStep() && m.launchFocus == 2 && m.launchRepoEditing {
		rowsReserved += 3
		m.launchRepoCustom.Width = maxInt(1, m.width-14)
	}
	listRows := visibleRows - rowsReserved
	if listRows < 1 {
		listRows = 1
	}

	switch {
	case m.launchFocus == launchFocusRole:
		lines = append(lines, panelHeaderStyle.Render("  Choose Role"), dimStyle.Render("  reusable run behavior and defaults"), "")
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
		lines = append(lines, panelHeaderStyle.Render("  Choose Engine"), dimStyle.Render("  concrete CLI / model to run"), "")
		var options []string
		for i := range providers {
			prefix := "  "
			name := providers[i]
			if i == m.launchProviderIdx {
				prefix = accentStyle.Render("▸ ")
				name = accentStyle.Bold(true).Render(name)
			}
			options = append(options, prefix+name)
		}
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), m.launchProviderIdx, listRows), listRows)...)
	case m.launchFocus == launchFocusPrompt:
		lines = append(lines, panelHeaderStyle.Render("  Choose Prompt"), dimStyle.Render("  override the role's system prompt for this run"), "")
		var options []string
		options = append(options, launchOverrideOption("(role default)", m.launchPromptIdx == -1))
		for i, p := range m.cockpitPrompts {
			options = append(options, launchOverrideOption(p.Name, m.launchPromptIdx == i))
		}
		cursor := m.launchPromptIdx + 1
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), cursor, listRows), listRows)...)
	case m.launchFocus == launchFocusHooks:
		lines = append(lines, panelHeaderStyle.Render("  Choose Hook Bundle"), dimStyle.Render("  override the role's pre/post hooks for this run"), "")
		var options []string
		options = append(options, launchOverrideOption("(role default)", m.launchHookBundleIdx == -1))
		for i, b := range m.cockpitHookBundles {
			options = append(options, launchOverrideOption(b.Name, m.launchHookBundleIdx == i))
		}
		cursor := m.launchHookBundleIdx + 1
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), cursor, listRows), listRows)...)
	case m.launchFocus == launchFocusPerms:
		lines = append(lines, panelHeaderStyle.Render("  Choose Permissions"), dimStyle.Render("  override the role's permission level for this run"), "")
		var options []string
		for i, label := range launchPermsLabels {
			options = append(options, launchOverrideOption(label, m.launchPermsIdx == i))
		}
		lines = append(lines, scrollWindow(options, scrollOffsetForCursor(len(options), m.launchPermsIdx, listRows), listRows)...)
	case m.launchHasRepoStep() && m.launchFocus == m.launchRepoFocus():
		lines = append(lines, panelHeaderStyle.Render("  Choose Repo"))
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
		lines = append(lines, panelHeaderStyle.Render("  Note"), dimStyle.Render("  optional note for this specific run"))
		lines = append(lines, m.launchBrief.View())
	case m.launchFocus == m.launchReviewFocus():
		lines = append(lines, scrollWindow(launchReviewLines(m), m.launchReviewOffset, visibleRows)...)
	}

	return strings.Join(capLines(lines, contentHeight), "\n")
}

func (m model) renderAgentLaunchPrefixLines(subtabs, presetLabel, providerLabel string) []string {
	var lines []string
	lines = append(lines, titleStyle.Render("New Run")+"   "+subtabs, "")
	lines = append(lines, "")

	summary := dimStyle.Render("  role=") + textStyle.Render(presetLabel) +
		dimStyle.Render("  engine=") + textStyle.Render(providerLabel) +
		dimStyle.Render("  repo=") + textStyle.Render(launchRepoPathLabel(m.launchRepo)) +
		dimStyle.Render(fmt.Sprintf("  %d sources", len(m.launchSources)))
	// Only surface override tags when active — keeps the line short on
	// narrow terminals when the role's defaults are good enough.
	if m.launchPromptIdx >= 0 && m.launchPromptIdx < len(m.cockpitPrompts) {
		summary += dimStyle.Render("  prompt=") + accentStyle.Render(m.cockpitPrompts[m.launchPromptIdx].Name)
	}
	if m.launchHookBundleIdx >= 0 && m.launchHookBundleIdx < len(m.cockpitHookBundles) {
		summary += dimStyle.Render("  hooks=") + accentStyle.Render(m.cockpitHookBundles[m.launchHookBundleIdx].Name)
	}
	if v := launchPermsValue(m.launchPermsIdx); v != "" {
		summary += dimStyle.Render("  perms=") + accentStyle.Render(v)
	}
	lines = append(lines, wrapLines(summary, maxInt(20, m.width-4))...)
	lines = append(lines, "")
	return lines
}

func launchOverrideOption(label string, selected bool) string {
	if selected {
		return accentStyle.Render("▸ ") + accentStyle.Bold(true).Render(label)
	}
	return "  " + label
}

func launchRepoPathLabel(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == repoSentinelCustom {
		return path
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
	subtabs := tab("Role", launchFocusRole) + dimStyle.Render("  ·  ") +
		tab("Engine", launchFocusEngine) + dimStyle.Render("  ·  ") +
		tab("Prompt", launchFocusPrompt) + dimStyle.Render("  ·  ") +
		tab("Hooks", launchFocusHooks) + dimStyle.Render("  ·  ") +
		tab("Perms", launchFocusPerms)
	if m.launchHasRepoStep() {
		subtabs += dimStyle.Render("  ·  ") + tab("Repo", m.launchRepoFocus())
	}
	subtabs += dimStyle.Render("  ·  ") + tab("Note", m.launchNoteFocus()) +
		dimStyle.Render("  ·  ") + tab("Review", m.launchReviewFocus())

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
