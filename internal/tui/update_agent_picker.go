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
	// Step 1: pick a file (row 0 is the "no task source" sentinel)
	if m.pickerFile == "" {
		max := len(m.projects) // sentinel + N projects → cursor range [0, N]
		switch msg.String() {
		case "esc", "q":
			m.mode = modeAgentList
			return m, nil
		case "j", "down":
			if m.agentCursor < max {
				m.agentCursor++
			}
		case "k", "up":
			if m.agentCursor > 0 {
				m.agentCursor--
			}
		case "enter":
			if m.agentCursor == 0 {
				m.resetAgentLaunch()
				m.launchSources = nil
				m.launchRepo = m.defaultLaunchRepo()
				// Land on the Repo tab so the user explicitly picks (or
				// types) the repo before composing the brief — Lucas's
				// feedback was the implicit-cwd default felt "stuck".
				m.launchFocus = 2
				m.launchBrief.Blur()
				m.mode = modeAgentLaunch
				return m, nil
			}
			projectIdx := m.agentCursor - 1
			if projectIdx >= 0 && projectIdx < len(m.projects) {
				p := m.projects[projectIdx]
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
