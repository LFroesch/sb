package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
)

func (m *model) resetAgentPicker() {
	m.pickerFile = ""
	m.pickerItems = nil
	m.pickerSelected = map[int]bool{}
	m.pickerProject = ""
	m.pickerRepo = ""
	m.agentCursor = 0
}

func (m model) updateAgentPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Step 1: pick a file
	if m.pickerFile == "" {
		switch msg.String() {
		case "esc", "q":
			m.mode = modeAgentList
			return m, nil
		case "j", "down":
			if m.agentCursor < len(m.projects)-1 {
				m.agentCursor++
			}
		case "k", "up":
			if m.agentCursor > 0 {
				m.agentCursor--
			}
		case "enter":
			if m.agentCursor < len(m.projects) {
				p := m.projects[m.agentCursor]
				items, err := cockpit.ReadItems(p.Path)
				if err != nil {
					m.statusMsg = "read items: " + err.Error()
					m.statusExpiry = time.Now().Add(3 * time.Second)
					return m, nil
				}
				m.pickerFile = p.Path
				m.pickerItems = items
				m.pickerSelected = map[int]bool{}
				m.pickerProject = p.Name
				m.pickerRepo = p.Dir
				m.agentCursor = 0
			}
		}
		return m, nil
	}

	// Step 2: select items
	switch msg.String() {
	case "esc", "q":
		m.resetAgentPicker()
		return m, nil
	case "b":
		m.resetAgentPicker()
		return m, nil
	case "j", "down":
		if m.agentCursor < len(m.pickerItems)-1 {
			m.agentCursor++
		}
	case "k", "up":
		if m.agentCursor > 0 {
			m.agentCursor--
		}
	case " ":
		m.pickerSelected[m.agentCursor] = !m.pickerSelected[m.agentCursor]
	case "enter":
		if countSelected(m.pickerSelected) == 0 {
			return m, nil
		}
		sources := make([]cockpit.SourceTask, 0)
		for i, it := range m.pickerItems {
			if m.pickerSelected[i] {
				sources = append(sources, cockpit.SourceTask{
					File:    m.pickerFile,
					Line:    it.Line,
					Text:    it.Text,
					Project: m.pickerProject,
					Repo:    m.pickerRepo,
				})
			}
		}
		m.resetAgentLaunch()
		m.launchSources = sources
		m.launchRepo = m.pickerRepo
		m.mode = modeAgentLaunch
		return m, nil
	}
	return m, nil
}
