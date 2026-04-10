package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/diff"
	"github.com/LFroesch/sb/internal/markdown"
	"github.com/LFroesch/sb/internal/ollama"
	"github.com/LFroesch/sb/internal/scripts"
	"github.com/LFroesch/sb/internal/workmd"
)

// --- Favorites ---

func favoritesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sb", "favorites")
}

func loadFavorites() map[string]bool {
	fav := make(map[string]bool)
	data, err := os.ReadFile(favoritesPath())
	if err != nil {
		return fav
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			fav[line] = true
		}
	}
	return fav
}

func saveFavorites(fav map[string]bool) {
	p := favoritesPath()
	os.MkdirAll(filepath.Dir(p), 0755) //nolint:errcheck
	var lines []string
	for k := range fav {
		lines = append(lines, k)
	}
	sort.Strings(lines)
	os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0644) //nolint:errcheck
}

func sortWithFavorites(projects []workmd.Project, fav map[string]bool) {
	sort.SliceStable(projects, func(i, j int) bool {
		return fav[projects[i].Path] && !fav[projects[j].Path]
	})
}

// openInEditor opens a path in the best available editor.
// Priority: $VISUAL → $EDITOR → cursor → code → nvim → vim → nano → vi
func openInEditor(path string) tea.Cmd {
	terminalEditors := map[string]bool{
		"vim": true, "vi": true, "nvim": true, "nano": true,
		"micro": true, "helix": true, "hx": true, "emacs": true,
	}

	// Check env vars first
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(env); e != "" {
			if _, err := exec.LookPath(e); err == nil {
				if terminalEditors[e] {
					return tea.ExecProcess(exec.Command(e, path), func(err error) tea.Msg { return nil })
				}
				exec.Command(e, path).Start() //nolint:errcheck
				return nil
			}
		}
	}

	// Fallback: probe for available editors
	for _, e := range []string{"cursor", "code"} {
		if _, err := exec.LookPath(e); err == nil {
			exec.Command(e, path).Start() //nolint:errcheck
			return nil
		}
	}
	for _, e := range []string{"nvim", "vim", "nano", "vi"} {
		if _, err := exec.LookPath(e); err == nil {
			return tea.ExecProcess(exec.Command(e, path), func(err error) tea.Msg { return nil })
		}
	}
	return nil
}

// copyToClipboard copies text to the system clipboard.
// Tries clip.exe (WSL), xclip, xsel in order.
func copyToClipboard(s string) {
	for _, args := range [][]string{
		{"clip.exe"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(s)
		if err := cmd.Run(); err == nil {
			return
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New(m.width-4, m.height-8)
		if m.page == pageProject && m.selected < len(m.projects) {
			m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
		} else if m.page == pageDashboard && m.cursor < len(m.projects) {
			rightW := m.width - (m.width*25/100) - 6
			if rightW < 20 {
				rightW = 20
			}
			m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
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
		// ctrl+c always quits regardless of mode
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// In text-input modes, route directly to their handlers (don't intercept q, ?, etc.)
		if m.mode == modeEdit {
			return m.updateEdit(msg)
		}
		if m.mode == modeDumpInput || m.mode == modeDumpClarify || m.mode == modeDumpSummary {
			return m.updateDump(msg)
		}
		if m.mode == modeChainCleanupFeedback || m.mode == modeChainCleanupReview || m.mode == modeChainCleanupSummary {
			return m.updateCleanup(msg)
		}

		// Search mode intercept
		if m.mode == modeSearch {
			return m.updateSearch(msg)
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case projectsLoadedMsg:
		m.projects = msg.projects
		sortWithFavorites(m.projects, m.favorites)
		m.loading = false
		if len(m.projects) > 0 && m.width > 0 {
			rightW := m.width - (m.width*25/100) - 6
			if rightW < 20 {
				rightW = 20
			}
			m.viewport.SetContent(markdown.Render(m.projects[0].Content, rightW))
			m.viewport.GotoTop()
		}
		return m, nil

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
		m.dumpItems = msg.items
		m.dumpCursor = 0
		m.dumpAccepted = 0
		m.dumpSkipped = 0
		if len(msg.items) == 0 {
			m.dumpResult = "no items found in dump"
			m.mode = modeDumpInput
			m.dumpArea.Focus()
			return m, m.dumpArea.Cursor.BlinkCmd()
		}
		// If first item needs clarification, go to clarify mode
		if msg.items[0].Project == "CLARIFY" {
			m.mode = modeDumpClarify
			m.dumpClarifyArea.Reset()
			m.dumpClarifyArea.Focus()
			return m, m.dumpClarifyArea.Cursor.BlinkCmd()
		}
		m.mode = modeDumpReview
		return m, nil

	case dumpReroutedMsg:
		if msg.err != nil {
			m.statusMsg = "reroute failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(3 * time.Second)
			// Stay in clarify mode
			m.dumpClarifyArea.Reset()
			m.dumpClarifyArea.Focus()
			return m, m.dumpClarifyArea.Cursor.BlinkCmd()
		}
		// Update the current item with rerouted result
		if m.dumpCursor < len(m.dumpItems) {
			m.dumpItems[m.dumpCursor] = *msg.item
		}
		m.mode = modeDumpReview
		return m, nil

	case todoResultMsg:
		if msg.err != nil {
			m.statusMsg = "todo failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(5 * time.Second)
			m.mode = modeNormal
			return m, nil
		}
		m.todoResult = msg.result
		m.mode = modeTodoResult
		return m, nil

	case cleanupDoneMsg:
		if msg.err != nil {
			if m.mode == modeChainCleanupWait {
				if m.chainCursor < len(m.chainQueue) {
					idx := m.chainQueue[m.chainCursor]
					m.chainResults = append(m.chainResults, chainResult{name: m.projects[idx].Name, action: "error"})
					m.chainSkipped++
				}
				return m.advanceChainCursor()
			}
			m.statusMsg = "cleanup failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(5 * time.Second)
			m.mode = modeNormal
			m.page = pageProject
			return m, nil
		}
		m.cleanupResult = msg.result
		// Compute diff for viewport (shared by single and chain)
		diffLines := diff.Unified(m.cleanupOriginal, msg.result)
		addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
		removeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
		var rendered []string
		for _, l := range diffLines {
			switch l.Type {
			case diff.Added:
				rendered = append(rendered, addStyle.Render("+ "+l.Content))
			case diff.Removed:
				rendered = append(rendered, removeStyle.Render("- "+l.Content))
			case diff.Context:
				rendered = append(rendered, dimStyle.Render("  "+l.Content))
			}
		}
		m.viewport.SetContent(strings.Join(rendered, "\n"))
		m.viewport.GotoTop()
		if m.mode == modeChainCleanupWait {
			m.mode = modeChainCleanupReview
			return m, nil
		}
		m.page = pageCleanup
		m.mode = modeNormal
		return m, nil

	case planResultMsg:
		if msg.err != nil {
			m.statusMsg = "plan failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(5 * time.Second)
			m.mode = modeNormal
			return m, nil
		}
		m.planResult = msg.result
		rightW := m.width - (m.width*25/100) - 6
		if rightW < 20 {
			rightW = 20
		}
		m.viewport.SetContent(msg.result)
		m.viewport.GotoTop()
		m.mode = modePlanResult
		return m, nil

	case scripts.DoneMsg:
		m.scriptOutput = msg.Output
		if msg.Err != nil {
			m.statusMsg = msg.Name + " failed"
		} else {
			m.statusMsg = msg.Name + " done"
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		// Load output into viewport for scrollable view
		available := scripts.Available()
		listH := len(available) + 3 // title + blank + scripts + separator
		vpH := m.height - 6 - listH
		if vpH < 3 {
			vpH = 3
		}
		m.viewport.Width = m.width - 4
		m.viewport.Height = vpH
		m.viewport.SetContent(msg.Output)
		m.viewport.GotoTop()
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
	if m.mode == modeDumpClarify {
		m.dumpClarifyArea, cmd = m.dumpClarifyArea.Update(msg)
		return m, cmd
	}
	if m.mode == modeChainCleanupFeedback {
		m.chainFeedback, cmd = m.chainFeedback.Update(msg)
		return m, cmd
	}

	return m, nil
}

// --- Dashboard ---

func (m model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modePlanResult {
		switch msg.String() {
		case "j", "down", "J", "s":
			m.viewport.LineDown(3)
		case "k", "up", "K", "w":
			m.viewport.LineUp(3)
		case "ctrl+d":
			m.viewport.HalfViewDown()
		case "ctrl+u":
			m.viewport.HalfViewUp()
		default:
			m.mode = modeNormal
			rightW := m.width - (m.width*25/100) - 6
			if rightW < 20 {
				rightW = 20
			}
			if m.cursor < len(m.projects) {
				m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
			}
			m.viewport.GotoTop()
		}
		return m, nil
	}
	if m.mode == modePlanWait {
		return m, nil
	}
	if m.mode == modeTodoResult {
		m.mode = modeNormal
		return m, nil
	}
	if m.mode == modeTodoWait {
		return m, nil // ignore keys while waiting
	}
	prevCursor := m.cursor
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "J", "s":
		m.viewport.LineDown(3)
	case "K", "w":
		m.viewport.LineUp(3)
	case "ctrl+d":
		m.viewport.HalfViewDown()
	case "ctrl+u":
		m.viewport.HalfViewUp()
	case "pgdown":
		m.viewport.ViewDown()
	case "pgup":
		m.viewport.ViewUp()
	case "ctrl+home":
		m.viewport.GotoTop()
	case "ctrl+end":
		m.viewport.GotoBottom()
	case "g":
		m.cursor = 0
		m.viewport.GotoTop()
	case "G":
		m.cursor = len(m.projects) - 1
	case "y":
		if m.cursor < len(m.projects) {
			copyToClipboard(m.projects[m.cursor].Dir)
			m.statusMsg = "copied: " + m.projects[m.cursor].Dir
			m.statusExpiry = time.Now().Add(2 * time.Second)
		}
	case "enter":
		// Full-screen project view
		if m.cursor < len(m.projects) {
			m.selected = m.cursor
			m.page = pageProject
			m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
			m.viewport.GotoTop()
		}
	case "e":
		if m.cursor < len(m.projects) {
			m.selected = m.cursor
			m.mode = modeEdit
			m.editArea.SetValue(m.projects[m.selected].Content)
			rightW := m.width - (m.width*25/100) - 6
			if rightW < 20 {
				rightW = 20
			}
			m.editArea.SetWidth(rightW - 4)
			panelH := m.height - 8 // availableHeight - 2 borders
			m.editArea.SetHeight(panelH - 2) // subtract header + blank line
			m.editArea.Focus()
			return m, m.editArea.Cursor.BlinkCmd()
		}
	case " ":
		if m.cursor < len(m.projects) {
			path := m.projects[m.cursor].Path
			m.selectedProjects[path] = !m.selectedProjects[path]
			if !m.selectedProjects[path] {
				delete(m.selectedProjects, path)
			}
		}
	case "-":
		if m.cursor < len(m.projects) {
			proj := m.projects[m.cursor]
			fixed := workmd.FixNonListLines(proj.Content)
			if fixed == proj.Content {
				m.statusMsg = "no non-list lines found"
			} else {
				if err := workmd.Save(proj.Path, fixed); err != nil {
					m.statusMsg = "save failed: " + err.Error()
				} else {
					m.projects[m.cursor].Content = fixed
					m.projects[m.cursor].NonListCount = 0
					m.statusMsg = "non-list lines fixed"
					rightW := m.width - (m.width*25/100) - 6
					if rightW < 20 {
						rightW = 20
					}
					m.viewport.SetContent(markdown.Render(fixed, rightW))
					m.viewport.GotoTop()
				}
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
		}
	case "c":
		if m.cursor < len(m.projects) {
			m.selected = m.cursor
			m.cleanupOriginal = m.projects[m.selected].Content
			m.mode = modeCleanupWait
			m.statusMsg = "asking ollama to clean up..."
			m.statusExpiry = time.Now().Add(120 * time.Second)
			return m, tea.Batch(cleanupCmd(m.projects[m.selected].Content), m.spinner.Tick)
		}
	case "C":
		var queue []int
		if len(m.selectedProjects) > 0 {
			for i, p := range m.projects {
				if m.selectedProjects[p.Path] {
					queue = append(queue, i)
				}
			}
		}
		if len(queue) == 0 {
			queue = make([]int, len(m.projects))
			for i := range m.projects {
				queue[i] = i
			}
		}
		if len(queue) > 0 {
			m.chainQueue = queue
			m.chainCursor = 0
			m.chainAccepted = 0
			m.chainSkipped = 0
			m.chainResults = nil
			m.chainSummaryScroll = 0
			m.selected = m.chainQueue[0]
			m.cleanupOriginal = m.projects[m.selected].Content
			m.page = pageCleanup
			m.mode = modeChainCleanupWait
			return m, tea.Batch(cleanupCmd(m.cleanupOriginal), m.spinner.Tick)
		}
	case "P":
		var sources []workmd.Project
		if len(m.selectedProjects) > 0 {
			for _, p := range m.projects {
				if m.selectedProjects[p.Path] {
					sources = append(sources, p)
				}
			}
		} else {
			sources = m.projects
		}
		m.mode = modePlanWait
		m.planScroll = 0
		m.statusMsg = "generating daily plan..."
		m.statusExpiry = time.Now().Add(90 * time.Second)
		return m, tea.Batch(planCmd(sources), m.spinner.Tick)
	case "o":
		if m.cursor < len(m.projects) {
			return m, openInEditor(m.projects[m.cursor].Dir)
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
	case "/":
		m.mode = modeSearch
		m.searchQuery = ""
		m.searchMatches = nil
		return m, nil
	case "t":
		if m.cursor < len(m.projects) {
			m.selected = m.cursor
			m.mode = modeTodoWait
			m.todoResult = ""
			m.statusMsg = "asking ollama..."
			m.statusExpiry = time.Now().Add(60 * time.Second)
			return m, tea.Batch(todoCmd(m.projects[m.cursor].Content), m.spinner.Tick)
		}
	case "f":
		if m.cursor < len(m.projects) {
			path := m.projects[m.cursor].Path
			if m.favorites[path] {
				delete(m.favorites, path)
				m.statusMsg = "unfavorited"
			} else {
				m.favorites[path] = true
				m.statusMsg = "favorited ★"
			}
			saveFavorites(m.favorites)
			sortWithFavorites(m.projects, m.favorites)
			// Re-find cursor position after re-sort
			for i, p := range m.projects {
				if p.Path == path {
					m.cursor = i
					break
				}
			}
			m.statusExpiry = time.Now().Add(2 * time.Second)
		}
	case "r":
		m.projects = workmd.Discover()
		sortWithFavorites(m.projects, m.favorites)
		m.statusMsg = "refreshed"
		m.statusExpiry = time.Now().Add(2 * time.Second)
	}

	// Update right-panel viewport when cursor changes
	if m.cursor != prevCursor && m.cursor < len(m.projects) {
		rightW := m.width - (m.width*25/100) - 6
		if rightW < 20 {
			rightW = 20
		}
		m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
		m.viewport.GotoTop()
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
			return m, tea.Batch(cleanupCmd(m.projects[m.selected].Content), m.spinner.Tick)
		}
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "ctrl+d":
		m.viewport.HalfViewDown()
	case "ctrl+u":
		m.viewport.HalfViewUp()
	case "pgdown":
		m.viewport.ViewDown()
	case "pgup":
		m.viewport.ViewUp()
	case "g", "ctrl+home":
		m.viewport.GotoTop()
	case "G", "ctrl+end":
		m.viewport.GotoBottom()
	}

	return m, nil
}

func todoCmd(content string) tea.Cmd {
	return func() tea.Msg {
		client := ollama.New()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, err := client.NextTodo(ctx, content)
		return todoResultMsg{result: result, err: err}
	}
}

func cleanupCmd(content string) tea.Cmd {
	return func() tea.Msg {
		client := ollama.New()
		result, err := client.Cleanup(context.Background(), content)
		return cleanupDoneMsg{result: result, err: err}
	}
}

func (m model) updateCleanup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeChainCleanupReview:
		return m.updateChainCleanupReview(msg)
	case modeChainCleanupFeedback:
		return m.updateChainCleanupFeedback(msg)
	case modeChainCleanupSummary:
		switch msg.String() {
		case "j", "down":
			m.chainSummaryScroll++
		case "k", "up":
			if m.chainSummaryScroll > 0 {
				m.chainSummaryScroll--
			}
		default:
			return m.dismissChainCleanupSummary()
		}
		return m, nil
	case modeCleanupFeedback:
		return m.updateSingleCleanupFeedback(msg)
	}
	// Single-project cleanup diff review
	switch msg.String() {
	case "y", "enter":
		if m.selected < len(m.projects) {
			err := workmd.Save(m.projects[m.selected].Path, m.cleanupResult)
			if err != nil {
				m.statusMsg = "write failed: " + err.Error()
			} else {
				savedPath := m.projects[m.selected].Path
				m.projects[m.selected].Content = m.cleanupResult
				m.projects = workmd.Discover()
				for i, p := range m.projects {
					if p.Path == savedPath {
						m.selected = i
						m.cursor = i
						break
					}
				}
				m.statusMsg = "cleanup saved"
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
		}
		m.page = pageDashboard
		rightW := m.width - (m.width*25/100) - 6
		if rightW < 20 {
			rightW = 20
		}
		m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, rightW))
		m.viewport.GotoTop()
	case "r":
		m.chainFeedback.Reset()
		m.chainFeedback.Focus()
		m.mode = modeCleanupFeedback
	case "n", "esc", "q":
		m.statusMsg = "cleanup discarded"
		m.statusExpiry = time.Now().Add(2 * time.Second)
		m.page = pageDashboard
		rightW := m.width - (m.width*25/100) - 6
		if rightW < 20 {
			rightW = 20
		}
		m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, rightW))
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

func (m model) updateSingleCleanupFeedback(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		feedback := strings.TrimSpace(m.chainFeedback.Value())
		if feedback == "" {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeCleanupWait
		return m, tea.Batch(cleanupWithFeedbackCmd(m.cleanupOriginal, feedback), m.spinner.Tick)
	case "esc":
		m.mode = modeNormal
		return m, nil
	}
	var cmd tea.Cmd
	m.chainFeedback, cmd = m.chainFeedback.Update(msg)
	return m, cmd
}

func (m model) updateChainCleanupReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if m.chainCursor < len(m.chainQueue) {
			idx := m.chainQueue[m.chainCursor]
			err := workmd.Save(m.projects[idx].Path, m.cleanupResult)
			if err != nil {
				m.chainResults = append(m.chainResults, chainResult{name: m.projects[idx].Name, action: "error"})
			} else {
				m.projects[idx].Content = m.cleanupResult
				m.chainAccepted++
				m.chainResults = append(m.chainResults, chainResult{name: m.projects[idx].Name, action: "accepted"})
			}
		}
		return m.advanceChainCursor()
	case "n", "esc":
		if m.chainCursor < len(m.chainQueue) {
			idx := m.chainQueue[m.chainCursor]
			m.chainSkipped++
			m.chainResults = append(m.chainResults, chainResult{name: m.projects[idx].Name, action: "skipped"})
		}
		return m.advanceChainCursor()
	case "q":
		// Abort — mark remaining as skipped and show summary
		for i := m.chainCursor; i < len(m.chainQueue); i++ {
			idx := m.chainQueue[i]
			m.chainResults = append(m.chainResults, chainResult{name: m.projects[idx].Name, action: "skipped"})
			m.chainSkipped++
		}
		if m.chainAccepted > 0 {
			m.projects = workmd.Discover()
		}
		m.mode = modeChainCleanupSummary
		return m, nil
	case "r":
		m.mode = modeChainCleanupFeedback
		m.chainFeedback.Reset()
		m.chainFeedback.Focus()
		return m, m.chainFeedback.Cursor.BlinkCmd()
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "ctrl+d":
		m.viewport.HalfViewDown()
	case "ctrl+u":
		m.viewport.HalfViewUp()
	case "pgdown":
		m.viewport.ViewDown()
	case "pgup":
		m.viewport.ViewUp()
	}
	return m, nil
}

func (m model) updateChainCleanupFeedback(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeChainCleanupReview
		return m, nil
	case "enter":
		feedback := m.chainFeedback.Value()
		if feedback == "" {
			m.mode = modeChainCleanupReview
			return m, nil
		}
		m.mode = modeChainCleanupWait
		return m, tea.Batch(cleanupWithFeedbackCmd(m.cleanupOriginal, feedback), m.spinner.Tick)
	}
	var cmd tea.Cmd
	m.chainFeedback, cmd = m.chainFeedback.Update(msg)
	return m, cmd
}

func (m model) advanceChainCursor() (tea.Model, tea.Cmd) {
	m.chainCursor++
	if m.chainCursor >= len(m.chainQueue) {
		if m.chainAccepted > 0 {
			m.projects = workmd.Discover()
		}
		m.mode = modeChainCleanupSummary
		return m, nil
	}
	idx := m.chainQueue[m.chainCursor]
	m.selected = idx
	m.cleanupOriginal = m.projects[idx].Content
	m.mode = modeChainCleanupWait
	return m, tea.Batch(cleanupCmd(m.cleanupOriginal), m.spinner.Tick)
}

func (m model) dismissChainCleanupSummary() (tea.Model, tea.Cmd) {
	m.chainQueue = nil
	m.chainCursor = 0
	m.chainAccepted = 0
	m.chainSkipped = 0
	m.chainResults = nil
	m.chainSummaryScroll = 0
	m.page = pageDashboard
	m.mode = modeNormal
	rightW := m.width - (m.width*25/100) - 6
	if rightW < 20 {
		rightW = 20
	}
	if m.cursor < len(m.projects) {
		m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
	}
	m.viewport.GotoTop()
	return m, nil
}

func planCmd(projects []workmd.Project) tea.Cmd {
	return func() tea.Msg {
		var sb strings.Builder
		for _, p := range projects {
			var bugs, cur []string
			for _, t := range p.Tasks {
				if t.Done {
					continue
				}
				switch t.Section {
				case "bugs":
					bugs = append(bugs, t.Name)
				case "current":
					cur = append(cur, t.Name)
				}
			}
			if len(bugs) == 0 && len(cur) == 0 {
				continue
			}
			sb.WriteString("Project: " + p.Name + "\n")
			for _, t := range bugs {
				sb.WriteString("  [BUG] " + t + "\n")
			}
			for _, t := range cur {
				sb.WriteString("  - " + t + "\n")
			}
			sb.WriteString("\n")
		}
		summary := sb.String()
		if summary == "" {
			return planResultMsg{result: "No current tasks found across projects.", err: nil}
		}
		client := ollama.New()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		result, err := client.DailyPlan(ctx, summary)
		return planResultMsg{result: result, err: err}
	}
}

func cleanupWithFeedbackCmd(content, feedback string) tea.Cmd {
	return func() tea.Msg {
		client := ollama.New()
		result, err := client.CleanupWithFeedback(context.Background(), content, feedback)
		return cleanupDoneMsg{result: result, err: err}
	}
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
	case "ctrl+d":
		// Delete current line, reposition cursor to same line number
		value := m.editArea.Value()
		lineNum := m.editArea.Line()
		lines := strings.Split(value, "\n")
		if lineNum < len(lines) {
			newLines := append(lines[:lineNum], lines[lineNum+1:]...)
			m.editArea.SetValue(strings.Join(newLines, "\n"))
			// SetValue leaves cursor at end; move up to target line
			for i := 0; i < len(newLines)-1-lineNum; i++ {
				m.editArea.CursorUp()
			}
			m.editArea.CursorStart()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.editArea, cmd = m.editArea.Update(msg)
	return m, cmd
}

// --- Brain dump ---

func (m model) updateDump(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeDumpReview:
		return m.updateDumpReview(msg)
	case modeDumpClarify:
		return m.updateDumpClarify(msg)
	case modeDumpSummary:
		switch msg.String() {
		case "j", "down":
			m.dumpSummaryScroll++
		case "k", "up":
			if m.dumpSummaryScroll > 0 {
				m.dumpSummaryScroll--
			}
		default:
			return m.dismissDumpSummary()
		}
		return m, nil
	}

	// modeDumpInput
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
		return m, tea.Batch(routeDumpCmd(text, m.projects), m.spinner.Tick)
	}

	var cmd tea.Cmd
	m.dumpArea, cmd = m.dumpArea.Update(msg)
	return m, cmd
}

func (m model) updateDumpReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dumpCursor >= len(m.dumpItems) {
		// All done — back to input
		return m.finishDumpReview()
	}

	item := m.dumpItems[m.dumpCursor]

	switch msg.String() {
	case "y", "enter":
		// Accept — write to target
		writeErr := m.writeDumpItem(item)
		if writeErr != nil {
			m.statusMsg = "write failed: " + writeErr.Error()
			m.statusExpiry = time.Now().Add(3 * time.Second)
		} else {
			m.dumpAccepted++
		}
		return m.advanceDumpCursor()

	case "n":
		// Skip this item
		m.dumpSkipped++
		m.dumpSkippedList = append(m.dumpSkippedList, item)
		return m.advanceDumpCursor()

	case "r":
		// Manual reroute — let user specify which project
		m.mode = modeDumpClarify
		m.dumpClarifyArea.Reset()
		m.dumpClarifyArea.Focus()
		return m, m.dumpClarifyArea.Cursor.BlinkCmd()

	case "esc":
		// Abort remaining — show summary of what was already accepted
		return m.finishDumpReview()
	}
	return m, nil
}

func (m model) updateDumpClarify(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Skip this item
		m.dumpSkipped++
		if m.dumpCursor < len(m.dumpItems) {
			m.dumpSkippedList = append(m.dumpSkippedList, m.dumpItems[m.dumpCursor])
		}
		return m.advanceDumpCursor()
	case "enter":
		clarification := m.dumpClarifyArea.Value()
		if clarification == "" {
			return m, nil
		}
		m.mode = modeDumpRouting
		m.statusMsg = "rerouting..."
		m.statusExpiry = time.Now().Add(30 * time.Second)
		item := m.dumpItems[m.dumpCursor]
		return m, tea.Batch(rerouteDumpCmd(item.Text, clarification, m.projects), m.spinner.Tick)
	}

	var cmd tea.Cmd
	m.dumpClarifyArea, cmd = m.dumpClarifyArea.Update(msg)
	return m, cmd
}

// writeDumpItem appends an item to the correct file.
func (m model) writeDumpItem(item ollama.RouteItem) error {
	// Normalize: strip leading #, trim spaces
	proj := strings.TrimSpace(strings.TrimPrefix(item.Project, "#"))
	projLower := strings.ToLower(proj)

	// IDEAS target
	if projLower == "ideas" {
		home, _ := os.UserHomeDir()
		ideasPath := filepath.Join(home, "projects/active/SECOND_BRAIN/ideas/WORK.md")
		return workmd.AppendToSection(ideasPath, "inbox", item.Text)
	}

	// SECOND_BRAIN catch-all → main SECOND_BRAIN WORK.md
	if projLower == "second_brain" || projLower == "second brain" {
		home, _ := os.UserHomeDir()
		sbPath := filepath.Join(home, "projects/active/SECOND_BRAIN/WORK.md")
		return workmd.AppendToSection(sbPath, item.Section, item.Text)
	}

	// Find matching project — exact match first, then fuzzy (suffix/substring)
	for _, p := range m.projects {
		if strings.ToLower(p.Name) == projLower {
			return workmd.AppendToSection(p.Path, item.Section, item.Text)
		}
	}
	for _, p := range m.projects {
		lower := strings.ToLower(p.Name)
		if strings.HasSuffix(lower, "/"+projLower) || strings.Contains(lower, projLower) {
			return workmd.AppendToSection(p.Path, item.Section, item.Text)
		}
	}
	return fmt.Errorf("project %q not found", item.Project)
}

// advanceDumpCursor moves to the next item or finishes review.
func (m model) advanceDumpCursor() (tea.Model, tea.Cmd) {
	m.dumpCursor++
	if m.dumpCursor >= len(m.dumpItems) {
		return m.finishDumpReview()
	}
	// Check if next item needs clarification
	if m.dumpItems[m.dumpCursor].Project == "CLARIFY" {
		m.mode = modeDumpClarify
		m.dumpClarifyArea.Reset()
		m.dumpClarifyArea.Focus()
		return m, m.dumpClarifyArea.Cursor.BlinkCmd()
	}
	m.mode = modeDumpReview
	return m, nil
}

// finishDumpReview ends the review and shows the summary screen.
func (m model) finishDumpReview() (tea.Model, tea.Cmd) {
	if m.dumpAccepted > 0 {
		m.projects = workmd.Discover()
	}
	m.mode = modeDumpSummary
	return m, nil
}

// dismissDumpSummary clears state and returns to dump input.
func (m model) dismissDumpSummary() (tea.Model, tea.Cmd) {
	m.dumpItems = nil
	m.dumpCursor = 0
	m.dumpText = ""
	m.dumpSkippedList = nil
	m.dumpAccepted = 0
	m.dumpSkipped = 0
	m.dumpResult = ""
	m.dumpSummaryScroll = 0
	m.dumpArea.Reset()
	m.dumpArea.Focus()
	m.mode = modeDumpInput
	return m, m.dumpArea.Cursor.BlinkCmd()
}

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.searchQuery = ""
		m.searchMatches = nil
		return m, nil
	case "enter":
		if len(m.searchMatches) > 0 && m.cursor < len(m.searchMatches) {
			idx := m.searchMatches[m.cursor].projectIdx
			m.selected = idx
			m.cursor = idx
			m.mode = modeNormal
			m.searchQuery = ""
			m.searchMatches = nil
			rightW := m.width - (m.width*25/100) - 6
			if rightW < 20 {
				rightW = 20
			}
			m.viewport.SetContent(markdown.Render(m.projects[idx].Content, rightW))
			m.viewport.GotoTop()
		}
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.cursor = 0
		}
	case "j", "down":
		if m.cursor < len(m.searchMatches)-1 {
			m.cursor++
		}
		return m, nil
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.searchQuery += msg.String()
			m.cursor = 0
		}
	}

	// Recompute matches
	m.searchMatches = nil
	if m.searchQuery != "" {
		q := strings.ToLower(m.searchQuery)
		seen := map[int]bool{}
		for i, p := range m.projects {
			if strings.Contains(strings.ToLower(p.Name), q) && !seen[i] {
				m.searchMatches = append(m.searchMatches, searchMatch{projectIdx: i, line: p.Name})
				seen[i] = true
			}
		}
		for i, p := range m.projects {
			if seen[i] {
				continue
			}
			for _, line := range strings.Split(p.Content, "\n") {
				if strings.Contains(strings.ToLower(line), q) {
					m.searchMatches = append(m.searchMatches, searchMatch{projectIdx: i, line: strings.TrimSpace(line)})
					seen[i] = true
					break
				}
			}
		}
	}

	return m, nil
}

func routeDumpCmd(text string, projects []workmd.Project) tea.Cmd {
	return func() tea.Msg {
		client := ollama.New()
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		items, err := client.RouteMulti(ctx, text, names)
		return dumpRoutedMsg{items: items, err: err}
	}
}

func rerouteDumpCmd(text, clarification string, projects []workmd.Project) tea.Cmd {
	return func() tea.Msg {
		client := ollama.New()
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000)
		defer cancel()
		item, err := client.RerouteSingle(ctx, text, clarification, names)
		return dumpReroutedMsg{item: item, err: err}
	}
}

// --- Scripts ---

func (m model) updateScripts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	available := scripts.Available()
	switch msg.String() {
	case "q", "esc":
		m.page = pageDashboard
		m.scriptOutput = ""
		return m, nil
	case "j", "down":
		if m.scriptCursor < len(available)-1 {
			m.scriptCursor++
		}
	case "k", "up":
		if m.scriptCursor > 0 {
			m.scriptCursor--
		}
	case "J":
		m.viewport.LineDown(3)
	case "K":
		m.viewport.LineUp(3)
	case "pgdn":
		m.viewport.HalfViewDown()
	case "pgup":
		m.viewport.HalfViewUp()
	case "c":
		m.scriptOutput = ""
	case "enter":
		if m.scriptCursor < len(available) {
			s := available[m.scriptCursor]
			m.scriptOutput = ""
			m.statusMsg = "running " + s.Name + "..."
			m.statusExpiry = time.Now().Add(30 * time.Second)
			return m, scripts.RunCmd(s)
		}
	}
	return m, nil
}
