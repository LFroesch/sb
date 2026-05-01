package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderAgentManage() string {
	kindLabel := agentManageKindLabel(m.agentManageKind)
	var headerLines []string
	hint := "  ·  advanced  ·  focus: " + strings.ToLower(kindLabel) +
		"  ·  [ / ] cycle: presets · prompts · hooks · runtimes"
	headerLines = append(headerLines, titleStyle.Render("Agent Setup")+dimStyle.Render(hint))
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

	specs := m.agentManageFieldSpecs()
	var detail []string
	if m.agentManageEditing && m.agentManageField >= 0 && m.agentManageField < len(specs) {
		spec := specs[m.agentManageField]
		detail = append(detail, panelHeaderStyle.Render("  Editing: "+spec.Label))
		detail = append(detail, dimStyle.Render("  "+spec.Group))
		detail = append(detail, "")
		editorLines := strings.Split(m.agentManageEditor.View(), "\n")
		detail = append(detail, editorLines...)
	} else {
		detail = append(detail, renderManageSelectedSummary(m, rightWidth-4)...)
		detail = append(detail, "")
		detail = append(detail, panelHeaderStyle.Render("  Editable Fields"))
		detail = append(detail, "")
		detail = append(detail, renderManageFieldList(m, rightWidth-4, innerHeight-len(detail))...)
	}
	rightStyle := panelStyle
	if m.agentManageFocus == 1 {
		rightStyle = panelActiveStyle
	}
	right := rightStyle.Width(rightWidth).Height(panelHeight).Render(strings.Join(capLines(detail, innerHeight), "\n"))

	return strings.Join(append(headerLines, lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)), "\n")
}

func (m model) agentManagePanelHeights() (panelHeight, innerHeight int) {
	headerLines := 2
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
