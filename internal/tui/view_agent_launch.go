package tui

import (
	"fmt"
	"strings"
)

func (m model) renderAgentLaunch() string {
	providers := providerChoices(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
	contentHeight := m.agentContentHeight()

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
	subtabs := tab("Role", 0) + dimStyle.Render("  ·  ") +
		tab("Engine", 1)
	if m.launchHasRepoStep() {
		subtabs += dimStyle.Render("  ·  ") + tab("Repo", 2)
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
	lines = append(lines, dimStyle.Render("  launch mode=")+queueStyle.Render(queueMode), "")
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
	visibleRows := contentHeight - len(lines)
	if visibleRows < 1 {
		visibleRows = 1
	}

	switch {
	case m.launchFocus == 0:
		lines = append(lines, panelHeaderStyle.Render("  Choose Role"), dimStyle.Render("  reusable run behavior and defaults"), "")
		var options []string
		for i, p := range m.cockpitPresets {
			prefix := "  "
			if i == m.launchPresetIdx {
				prefix = accentStyle.Render("▸ ")
			}
			role := ""
			if p.Role != "" {
				role = dimStyle.Render("  " + p.Role)
			}
			options = append(options, prefix+p.Name+role)
		}
		lines = append(lines, scrollWindow(options, maxInt(0, m.launchPresetIdx-visibleRows/2), visibleRows)...)
	case m.launchFocus == 1:
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
		lines = append(lines, scrollWindow(options, maxInt(0, m.launchProviderIdx-visibleRows/2), visibleRows)...)
	case m.launchHasRepoStep() && m.launchFocus == 2:
		lines = append(lines, panelHeaderStyle.Render("  Choose Repo"), dimStyle.Render("  where the agent should run"), "")
		repos := m.launchRepoChoices()
		selected := indexOfLaunchRepo(repos, m.launchRepo)
		var options []string
		for i, repo := range repos {
			prefix := "  "
			label := shortPath(repo)
			if label == "" {
				label = "(current working directory)"
			}
			if i == selected {
				prefix = accentStyle.Render("▸ ")
				label = accentStyle.Bold(true).Render(label)
			}
			options = append(options, prefix+label)
		}
		lines = append(lines, scrollWindow(options, maxInt(0, selected-visibleRows/2), visibleRows)...)
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
		dimStyle.Render("  repo=") + textStyle.Render(shortPath(m.launchRepo)) +
		dimStyle.Render(fmt.Sprintf("  %d sources", len(m.launchSources)))
	lines = append(lines, summary, "")
	return lines
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
	subtabs := tab("Role", 0) + dimStyle.Render("  ·  ") +
		tab("Engine", 1)
	if m.launchHasRepoStep() {
		subtabs += dimStyle.Render("  ·  ") + tab("Repo", 2)
	}
	subtabs += dimStyle.Render("  ·  ") + tab("Note", m.launchNoteFocus()) +
		dimStyle.Render("  ·  ") + tab("Review", m.launchReviewFocus())

	lines := m.renderAgentLaunchPrefixLines(subtabs, presetLabel, providerLabel)
	queueLines := 2
	if len(m.launchSources) > 0 {
		queueLines += 2
	}
	visibleRows := m.agentContentHeight() - len(lines) - queueLines - 1
	if visibleRows < 1 {
		visibleRows = 1
	}
	return visibleRows
}

func (m *model) clampLaunchReviewOffset() {
	m.launchReviewOffset = clampScrollOffset(m.launchReviewOffset, len(launchReviewLines(*m)), m.launchReviewVisibleRows())
}
