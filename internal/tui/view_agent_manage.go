package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderAgentManage() string {
	if m.agentManageHookEditing {
		return m.renderAgentManageHookOverlay()
	}
	if m.agentManageEditing || m.agentManageSelectEditing {
		return m.renderAgentManageOverlay()
	}

	kindLabel := agentManageKindLabel(m.agentManageKind)
	var headerLines []string

	tab := func(name, key, kind string) string {
		label := dimStyle.Render(key+" ") + name
		if m.agentManageKind == kind {
			label = accentStyle.Bold(true).Render("▸ " + key + " " + name)
		} else {
			label = "  " + label
		}
		return label
	}
	tabs := tab("Roles", "1", "preset") + dimStyle.Render("  ·  ") +
		tab("Prompts", "2", "prompt") + dimStyle.Render("  ·  ") +
		tab("Hooks", "3", "hookbundle") + dimStyle.Render("  ·  ") +
		tab("Engines", "4", "provider")
	headerLines = append(headerLines, titleStyle.Render("Advanced Setup")+dimStyle.Render("  reusable roles, prompts, hooks, and engines   "+tabs))
	headerLines = append(headerLines, "")

	panelHeight, innerHeight := m.agentManagePanelHeights()
	leftWidth := m.width * 29 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := m.width - leftWidth - 6
	if rightWidth < 52 {
		rightWidth = 52
	}
	var items []string
	items = append(items, panelHeaderStyle.Render("  "+kindLabel))
	items = append(items, "")
	total := m.agentManageItemCount()
	if total == 0 {
		items = append(items, dimStyle.Render("  no items"))
	}
	start, end := windowRange(total, m.agentManageCursor, innerHeight-3)
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.agentManageCursor {
			prefix = accentStyle.Render("▸ ")
		}
		label := m.agentManageItemLabel(i)
		items = append(items, truncate(prefix+label, leftWidth-4))
	}
	items = scrollWindow(items, m.agentManageListOffset, innerHeight)
	leftStyle := panelStyle
	if m.agentManageFocus == 0 {
		leftStyle = panelActiveStyle
	}
	left := leftStyle.Width(leftWidth).Height(panelHeight).Render(strings.Join(capLines(items, innerHeight), "\n"))

	var detail []string
	detail = append(detail, renderManageSelectedSummary(m, rightWidth-4)...)
	detail = append(detail, "")
	detail = append(detail, panelHeaderStyle.Render("  Editable Fields"))
	detail = append(detail, "")
	detail = append(detail, renderManageFieldList(m, rightWidth-4, innerHeight-len(detail))...)
	rightStyle := panelStyle
	if m.agentManageFocus == 1 {
		rightStyle = panelActiveStyle
	}
	right := rightStyle.Width(rightWidth).Height(panelHeight).Render(strings.Join(capLines(detail, innerHeight), "\n"))

	return strings.Join(append(headerLines, lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)), "\n")
}

func (m model) renderAgentManageOverlay() string {
	kindLabel := agentManageKindLabel(m.agentManageKind)
	lines := []string{
		titleStyle.Render("Advanced Setup") + dimStyle.Render("  "+kindLabel+" · overlay editor"),
		"",
	}

	baseHeight := m.agentContentHeight() - len(lines)
	if baseHeight < 3 {
		baseHeight = 3
	}
	width := m.width - 8
	if width > 96 {
		width = 96
	}
	if width < 28 {
		width = 28
	}
	overlay := m.renderAgentManageEditorDialog(width, maxInt(1, baseHeight-4))
	lines = append(lines, lipgloss.Place(m.width, baseHeight, lipgloss.Center, lipgloss.Center, overlay))
	return strings.Join(capLines(lines, m.agentContentHeight()), "\n")
}

func (m model) renderAgentManageEditorDialog(width, maxBodyLines int) string {
	spec, ok := m.currentAgentManageFieldSpec()
	if !ok {
		return dialogStyle.Width(width).Render("no field selected")
	}
	if width < 28 {
		width = 28
	}
	bodyWidth := width - 6
	if bodyWidth < 20 {
		bodyWidth = 20
	}

	header := panelHeaderStyle.Render("Edit " + spec.Label)
	subheader := dimStyle.Render(fmt.Sprintf("%s · %s", agentManageKindLabel(m.agentManageKind), spec.Group))
	lines := []string{header, subheader}
	if strings.TrimSpace(spec.Help) != "" {
		lines = append(lines, "")
		lines = append(lines, wrapLines(dimStyle.Render(spec.Help), bodyWidth)...)
	}
	lines = append(lines, "")

	if m.agentManageSelectEditing {
		lines = append(lines, dimStyle.Render("type value, id, or name · enter saves · esc cancels"))
		lines = append(lines, "")
		lines = append(lines, m.agentManageSelectInput.View())
	} else {
		if spec.Multiline {
			lines = append(lines, dimStyle.Render("ctrl+s saves · esc cancels"))
		} else {
			lines = append(lines, dimStyle.Render("enter saves · esc cancels"))
		}
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.agentManageEditor.View(), "\n")...)
	}

	if hints := m.agentManageOverlayHints(spec, bodyWidth); len(hints) > 0 {
		lines = append(lines, "", panelHeaderStyle.Render("Hints"))
		lines = append(lines, hints...)
	}
	if choices := m.agentManageOverlayChoices(spec, bodyWidth); len(choices) > 0 {
		lines = append(lines, "", panelHeaderStyle.Render("Choices"))
		lines = append(lines, choices...)
	}

	if maxBodyLines > 0 {
		lines = capLines(lines, maxBodyLines)
	}
	return dialogStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderAgentManageHookOverlay() string {
	kindLabel := agentManageKindLabel(m.agentManageKind)
	lines := []string{
		titleStyle.Render("Advanced Setup") + dimStyle.Render("  "+kindLabel+" · hook editor"),
		"",
	}
	baseHeight := m.agentContentHeight() - len(lines)
	if baseHeight < 3 {
		baseHeight = 3
	}
	width := m.width - 8
	if width > 112 {
		width = 112
	}
	if width < 36 {
		width = 36
	}
	overlay := m.renderAgentManageHookDialog(width, maxInt(1, baseHeight-4))
	lines = append(lines, lipgloss.Place(m.width, baseHeight, lipgloss.Center, lipgloss.Center, overlay))
	return strings.Join(capLines(lines, m.agentContentHeight()), "\n")
}

func (m model) renderAgentManageHookDialog(width, maxBodyLines int) string {
	bodyWidth := width - 6
	if bodyWidth < 28 {
		bodyWidth = 28
	}
	leftWidth := bodyWidth * 35 / 100
	if leftWidth < 18 {
		leftWidth = 18
	}
	rightWidth := bodyWidth - leftWidth - 3
	if rightWidth < 18 {
		rightWidth = 18
	}

	title := panelHeaderStyle.Render("Edit " + strings.ReplaceAll(strings.Title(strings.ReplaceAll(m.agentManageHookArrayKey, "_", " ")), "Shell", "Shell"))
	subtitle := dimStyle.Render("structured hook editor · a add · d delete · D duplicate · [/ ] reorder · ctrl+s saves bundle · esc cancels")
	header := []string{title, subtitle, ""}

	var listLines []string
	listLines = append(listLines, panelHeaderStyle.Render("  Hooks"), "")
	for i := 0; i < m.agentManageHookRowCount(); i++ {
		prefix := "  "
		if i == m.agentManageHookCursor {
			prefix = accentStyle.Render("▸ ")
		}
		label := m.agentManageHookItemLabel(i)
		if i == m.agentManageHookItemsCount() {
			label = primaryStyle.Render(label)
		}
		listLines = append(listLines, truncate(prefix+label, leftWidth))
	}
	listStyle := panelStyle
	if m.agentManageHookFocus == 0 {
		listStyle = panelActiveStyle
	}
	listPanel := listStyle.Width(leftWidth + 2).Render(strings.Join(capLines(listLines, maxBodyLines), "\n"))

	var detailLines []string
	if m.agentManageHookCursor >= m.agentManageHookItemsCount() && m.agentManageHookItemsCount() >= 0 {
		detailLines = append(detailLines, panelHeaderStyle.Render("  Add Hook"), "")
		detailLines = append(detailLines, dimStyle.Render("  press enter or a to append a new hook row"))
	} else {
		detailLines = append(detailLines, panelHeaderStyle.Render("  Fields"), "")
		if m.agentManageSelectEditing {
			if spec, ok := m.agentManageHookCurrentFieldSpec(); ok {
				detailLines = append(detailLines, dimStyle.Render("  "+spec.Label+" · enter saves · esc cancels"), "")
			}
			detailLines = append(detailLines, "  "+m.agentManageSelectInput.View())
			if spec, ok := m.agentManageHookCurrentFieldSpec(); ok {
				if strings.TrimSpace(spec.Help) != "" {
					detailLines = append(detailLines, "", dimStyle.Render("  "+spec.Help))
				}
				if choices := m.agentManageHookOverlayChoices(spec, rightWidth-4); len(choices) > 0 {
					detailLines = append(detailLines, "", panelHeaderStyle.Render("  Choices"))
					detailLines = append(detailLines, choices...)
				}
			}
		} else if m.agentManageEditing {
			if spec, ok := m.agentManageHookCurrentFieldSpec(); ok {
				detailLines = append(detailLines, dimStyle.Render("  "+spec.Label), "")
			}
			detailLines = append(detailLines, strings.Split(m.agentManageEditor.View(), "\n")...)
			if spec, ok := m.agentManageHookCurrentFieldSpec(); ok {
				if strings.TrimSpace(spec.Help) != "" {
					detailLines = append(detailLines, "", dimStyle.Render("  "+spec.Help))
				}
				if hints := m.agentManageHookFieldHints(spec, rightWidth-4); len(hints) > 0 {
					detailLines = append(detailLines, "", panelHeaderStyle.Render("  Example"))
					detailLines = append(detailLines, hints...)
				}
			}
		} else {
			specs := m.agentManageHookFieldSpecs()
			for i, spec := range specs {
				prefix := "  "
				if i == m.agentManageHookField {
					prefix = accentStyle.Render("▸ ")
				}
				value := m.agentManageHookFieldValue(i)
				display := value
				if strings.TrimSpace(display) == "" {
					display = "(empty)"
				}
				if spec.Multiline {
					display = strings.ReplaceAll(display, "\n", "  ")
				}
				detailLines = append(detailLines, prefix+dimStyle.Render(spec.Label+": ")+truncate(display, rightWidth-8))
			}
			if spec, ok := m.agentManageHookCurrentFieldSpec(); ok && strings.TrimSpace(spec.Help) != "" {
				detailLines = append(detailLines, "", dimStyle.Render("  "+spec.Help))
			}
			if hints := m.agentManageOverlayHints(agentManageFieldSpec{Key: m.agentManageHookArrayKey}, rightWidth-4); len(hints) > 0 {
				detailLines = append(detailLines, "", panelHeaderStyle.Render("  Example"))
				detailLines = append(detailLines, hints...)
			}
		}
	}
	detailStyle := panelStyle
	if m.agentManageHookFocus == 1 || m.agentManageEditing || m.agentManageSelectEditing {
		detailStyle = panelActiveStyle
	}
	detailPanel := detailStyle.Width(rightWidth + 2).Render(strings.Join(capLines(detailLines, maxBodyLines), "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top, listPanel, " ", detailPanel)
	lines := append(header, body)
	if maxBodyLines > 0 {
		lines = capLines(lines, maxBodyLines+4)
	}
	return dialogStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) agentManageHookOverlayChoices(spec agentManageFieldSpec, width int) []string {
	choices := enumOptionsForHookField(m.agentManageHookArrayKey, spec.Key)
	if len(choices) == 0 {
		return nil
	}
	lines := make([]string, 0, len(choices))
	for _, choice := range choices {
		lines = append(lines, "  "+truncate(choice, width))
	}
	return lines
}

func (m model) agentManageHookFieldHints(spec agentManageFieldSpec, width int) []string {
	var sample string
	switch spec.Key {
	case "body":
		sample = "Use Body when kind=literal. Use Body Ref instead when kind=file."
	case "cmd":
		sample = "git status --short"
	case "preview_cmd":
		sample = "git diff --stat"
	}
	if strings.TrimSpace(sample) == "" {
		return nil
	}
	var lines []string
	for _, line := range wrapLines(dimStyle.Render(sample), width) {
		lines = append(lines, line)
	}
	return lines
}

func (m model) agentManageOverlayHints(spec agentManageFieldSpec, width int) []string {
	var text string
	switch spec.Key {
	case "body":
		text = "Write the system prompt body directly here."
	case "prompt":
		text = "[\n  {\n    \"kind\": \"literal\",\n    \"label\": \"Context\",\n    \"body\": \"...\"\n  }\n]"
	case "pre_shell", "post_shell":
		text = "[\n  {\n    \"name\": \"example\",\n    \"cmd\": \"git status --short\"\n  }\n]"
	case "hook_bundle_id":
		text = "Comma-separated bundle ids/names. Blank clears all."
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, wrapLines(dimStyle.Render(line), width)...)
	}
	return lines
}

func (m model) agentManageOverlayChoices(spec agentManageFieldSpec, width int) []string {
	var choices []string
	switch spec.Key {
	case "prompt_id":
		choices = append(choices, "(blank) clears")
		for _, p := range m.cockpitPrompts {
			choices = append(choices, p.ID+" · "+p.Name)
		}
	case "engine_id":
		choices = append(choices, "(blank) clears")
		for _, p := range m.cockpitProviders {
			choices = append(choices, p.ID+" · "+p.Name)
		}
	case "hook_bundle_id":
		choices = append(choices, "(blank) clears")
		for _, b := range m.cockpitHookBundles {
			choices = append(choices, b.ID+" · "+b.Name)
		}
	default:
		for _, opt := range m.enumOptionsForFieldKey(spec.Key) {
			if strings.TrimSpace(opt) == "" {
				continue
			}
			choices = append(choices, opt)
		}
	}
	if len(choices) == 0 {
		return nil
	}
	lines := make([]string, 0, len(choices))
	for _, choice := range choices {
		for i, line := range wrapLines(choice, width) {
			if i == 0 {
				lines = append(lines, "  "+line)
			} else {
				lines = append(lines, "    "+line)
			}
		}
	}
	return lines
}

func (m model) agentManagePanelHeights() (panelHeight, innerHeight int) {
	headerLines := 3
	panelHeight = m.agentContentHeight() - headerLines - 2
	if panelHeight < 3 {
		panelHeight = 3
	}
	innerHeight = panelHeight - 2
	if innerHeight < 1 {
		innerHeight = 1
	}
	return panelHeight, innerHeight
}

func (m model) agentManageDetailVisibleRows() int {
	_, innerHeight := m.agentManagePanelHeights()
	detailLines := len(renderManageSelectedSummary(m, 0)) + 3
	if m.agentManageItemCount() > 0 {
		detailLines++
	}
	visible := innerHeight - detailLines
	if visible < 1 {
		visible = 1
	}
	return visible
}

func (m model) agentManageEditorDims() (width, height int) {
	panelHeight, innerHeight := m.agentManagePanelHeights()
	_ = panelHeight
	leftWidth := m.width * 29 / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := m.width - leftWidth - 6
	if rightWidth < 52 {
		rightWidth = 52
	}
	width = rightWidth - 4
	if width < 20 {
		width = 20
	}
	height = innerHeight - 3
	if height < 3 {
		height = 3
	}
	return width, height
}
