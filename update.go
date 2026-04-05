package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/markdown"
	"github.com/LFroesch/sb/internal/ollama"
	"github.com/LFroesch/sb/internal/scripts"
	"github.com/LFroesch/sb/internal/workmd"
)

// openInCursor opens a directory in Cursor editor.
func openDir(dir string) {
	exec.Command("cursor", dir).Start() //nolint:errcheck
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New(m.width-4, m.height-8)
		if m.page == pageProject && m.selected < len(m.projects) {
			m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
		}
		// Resize dump textarea to fill available space
		dumpH := m.height - 10
		if dumpH < 6 {
			dumpH = 6
		}
		m.dumpArea.SetWidth(m.width - 6)
		m.dumpArea.SetHeight(dumpH)
		return m, nil

	case tea.KeyMsg:
		// In edit mode, route directly to the edit handler (don't intercept q, ?, etc.)
		if m.mode == modeEdit {
			return m.updateEdit(msg)
		}

		// Global keys
		switch msg.String() {
		case "ctrl+c", "q":
			if m.mode == modeNormal && m.page == pageDashboard {
				return m, tea.Quit
			}
			if m.mode != modeNormal {
				m.mode = modeNormal
				m.helpScroll = 0
				return m, nil
			}
			m.page = pageDashboard
			return m, nil
		case "?":
			if m.mode == modeHelp {
				m.mode = modeNormal
				m.helpScroll = 0
			} else {
				m.mode = modeHelp
				m.helpScroll = 0
			}
			return m, nil
		}

		if m.mode == modeHelp {
			switch msg.String() {
			case "j", "down":
				m.helpScroll++
			case "k", "up":
				if m.helpScroll > 0 {
					m.helpScroll--
				}
			default:
				m.mode = modeNormal
				m.helpScroll = 0
			}
			return m, nil
		}

		switch m.page {
		case pageDashboard:
			return m.updateDashboard(msg)
		case pageProject:
			return m.updateProject(msg)
		case pageDump:
			return m.updateDump(msg)
		case pageScripts:
			return m.updateScripts(msg)
		case pageCleanup:
			return m.updateCleanup(msg)
		}

	case tickMsg:
		return m, tickCmd()

	case dumpRoutedMsg:
		if msg.err != nil {
			m.statusMsg = "route failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(5 * time.Second)
			m.dumpResult = "route failed: " + msg.err.Error()
			m.mode = modeDumpInput
			m.dumpArea.SetValue(m.dumpText)
			m.dumpArea.Focus()
			return m, m.dumpArea.Cursor.BlinkCmd()
		}
		m.dumpRoute = msg.project
		m.dumpSection = msg.section
		m.mode = modeDumpConfirm
		return m, nil

	case cleanupDoneMsg:
		if msg.err != nil {
			m.statusMsg = "cleanup failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(5 * time.Second)
			m.mode = modeNormal
			m.page = pageProject
			return m, nil
		}
		m.cleanupResult = msg.result
		m.page = pageCleanup
		m.mode = modeNormal
		m.viewport.SetContent(markdown.Render(msg.result, m.width-4))
		m.viewport.GotoTop()
		return m, nil

	case scripts.DoneMsg:
		m.scriptOutput = msg.Output
		if msg.Err != nil {
			m.statusMsg = msg.Name + " failed"
		} else {
			m.statusMsg = msg.Name + " done"
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil
	}

	// Route all other messages (cursor blink, paste, etc.) to active textarea
	var cmd tea.Cmd
	if m.mode == modeEdit {
		m.editArea, cmd = m.editArea.Update(msg)
		return m, cmd
	}
	if m.mode == modeDumpInput {
		m.dumpArea, cmd = m.dumpArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// --- Dashboard ---

func (m model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
			m.dashRightScroll = 0
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.dashRightScroll = 0
		}
	case "s":
		m.dashRightScroll++
	case "w":
		if m.dashRightScroll > 0 {
			m.dashRightScroll--
		}
	case "g":
		m.cursor = 0
		m.dashRightScroll = 0
	case "G":
		m.cursor = len(m.projects) - 1
		m.dashRightScroll = 0
	case "enter":
		if m.cursor < len(m.projects) {
			m.selected = m.cursor
			m.page = pageProject
			m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
			m.viewport.GotoTop()
		}
	case "e":
		if m.cursor < len(m.projects) {
			m.selected = m.cursor
			m.page = pageProject
			m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
			m.viewport.GotoTop()
			m.mode = modeEdit
			m.editArea.SetValue(m.projects[m.selected].Content)
			m.editArea.SetWidth(m.width - 4)
			m.editArea.SetHeight(m.height - 8)
			m.editArea.Focus()
			return m, m.editArea.Cursor.BlinkCmd()
		}
	case "o":
		if m.cursor < len(m.projects) {
			openDir(m.projects[m.cursor].Dir)
		}
	case "d":
		m.page = pageDump
		m.mode = modeDumpInput
		m.dumpArea.Reset()
		m.dumpArea.Focus()
		return m, m.dumpArea.Cursor.BlinkCmd()
	case "x":
		m.page = pageScripts
		m.scriptCursor = 0
	case "r":
		m.projects = workmd.Discover()
		m.statusMsg = "refreshed"
		m.statusExpiry = time.Now().Add(2 * time.Second)
	}
	return m, nil
}

// --- Project view ---

func (m model) updateProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.page = pageDashboard
		return m, nil
	case "e":
		if m.selected < len(m.projects) {
			m.mode = modeEdit
			m.editArea.SetValue(m.projects[m.selected].Content)
			m.editArea.SetWidth(m.width - 4)
			m.editArea.SetHeight(m.height - 8)
			m.editArea.Focus()
			return m, m.editArea.Cursor.BlinkCmd()
		}
	case "c":
		if m.selected < len(m.projects) {
			m.cleanupOriginal = m.projects[m.selected].Content
			m.mode = modeCleanupWait
			m.statusMsg = "asking ollama to clean up..."
			m.statusExpiry = time.Now().Add(120 * time.Second)
			return m, cleanupCmd(m.projects[m.selected].Content)
		}
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "ctrl+d":
		m.viewport.HalfViewDown()
	case "ctrl+u":
		m.viewport.HalfViewUp()
	case "g":
		m.viewport.GotoTop()
	case "G":
		m.viewport.GotoBottom()
	}

	return m, nil
}

func cleanupCmd(content string) tea.Cmd {
	return func() tea.Msg {
		client := ollama.New()
		result, err := client.Cleanup(context.Background(), content)
		return cleanupDoneMsg{result: result, err: err}
	}
}

func (m model) updateCleanup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		// Write cleaned content
		if m.selected < len(m.projects) {
			err := workmd.Save(m.projects[m.selected].Path, m.cleanupResult)
			if err != nil {
				m.statusMsg = "write failed: " + err.Error()
			} else {
				m.projects[m.selected].Content = m.cleanupResult
				m.projects = workmd.Discover()
				m.statusMsg = "cleanup saved"
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
		}
		m.page = pageProject
		m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
		m.viewport.GotoTop()
	case "n", "esc", "q":
		m.statusMsg = "cleanup discarded"
		m.statusExpiry = time.Now().Add(2 * time.Second)
		m.page = pageProject
		m.viewport.SetContent(markdown.Render(m.cleanupOriginal, m.width-4))
		m.viewport.GotoTop()
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "ctrl+d":
		m.viewport.HalfViewDown()
	case "ctrl+u":
		m.viewport.HalfViewUp()
	}
	return m, nil
}

func (m model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		return m, nil
	case "ctrl+s":
		content := m.editArea.Value()
		if m.selected < len(m.projects) {
			err := workmd.Save(m.projects[m.selected].Path, content)
			if err != nil {
				m.statusMsg = "save failed: " + err.Error()
			} else {
				m.projects[m.selected].Content = content
				m.viewport.SetContent(markdown.Render(content, m.width-4))
				m.statusMsg = "saved"
			}
			m.statusExpiry = time.Now().Add(2 * time.Second)
			m.mode = modeNormal
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.editArea, cmd = m.editArea.Update(msg)
	return m, cmd
}

// --- Brain dump ---

func (m model) updateDump(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeDumpConfirm {
		return m.updateDumpConfirm(msg)
	}

	switch msg.String() {
	case "esc":
		m.page = pageDashboard
		m.mode = modeNormal
		m.dumpArea.Reset()
		m.dumpResult = ""
		return m, nil
	case "ctrl+d":
		text := m.dumpArea.Value()
		if text == "" {
			return m, nil
		}
		m.dumpText = text
		m.mode = modeDumpRouting
		m.dumpResult = ""
		return m, routeDumpCmd(text, m.projects)
	}

	var cmd tea.Cmd
	m.dumpArea, cmd = m.dumpArea.Update(msg)
	return m, cmd
}

func (m model) updateDumpConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		// Write to the routed project
		var writeErr error
		found := false
		for _, p := range m.projects {
			if p.Name == m.dumpRoute {
				writeErr = workmd.AppendToSection(p.Path, m.dumpSection, m.dumpText)
				found = true
				break
			}
		}
		if !found {
			writeErr = fmt.Errorf("project %q not found", m.dumpRoute)
		}
		if writeErr != nil {
			m.dumpResult = "write failed: " + writeErr.Error()
			m.statusMsg = "write failed"
		} else {
			m.dumpResult = "dumped → " + m.dumpRoute + " / " + m.dumpSection
			m.statusMsg = m.dumpResult
			m.projects = workmd.Discover()
		}
		m.statusExpiry = time.Now().Add(4 * time.Second)
		m.dumpText = ""
		m.dumpArea.Reset()
		m.dumpArea.Focus()
		m.mode = modeDumpInput
		return m, m.dumpArea.Cursor.BlinkCmd()
	case "n", "esc":
		// Reject — put text back in textarea so user can re-edit or re-route
		m.mode = modeDumpInput
		m.dumpArea.SetValue(m.dumpText)
		m.dumpArea.Focus()
		m.dumpResult = "route rejected — edit and retry"
		return m, m.dumpArea.Cursor.BlinkCmd()
	}
	return m, nil
}

func routeDumpCmd(text string, projects []workmd.Project) tea.Cmd {
	return func() tea.Msg {
		project, section, err := routeWithOllama(text, projects)
		return dumpRoutedMsg{project: project, section: section, err: err}
	}
}

// --- Scripts ---

func (m model) updateScripts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	available := scripts.Available()
	switch msg.String() {
	case "q", "esc":
		m.page = pageDashboard
		return m, nil
	case "j", "down":
		if m.scriptCursor < len(available)-1 {
			m.scriptCursor++
		}
	case "k", "up":
		if m.scriptCursor > 0 {
			m.scriptCursor--
		}
	case "enter":
		if m.scriptCursor < len(available) {
			s := available[m.scriptCursor]
			m.statusMsg = "running " + s.Name + "..."
			m.statusExpiry = time.Now().Add(30 * time.Second)
			return m, scripts.RunCmd(s)
		}
	}
	return m, nil
}
