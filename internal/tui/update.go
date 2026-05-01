package tui

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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/diff"
	"github.com/LFroesch/sb/internal/llm"
	"github.com/LFroesch/sb/internal/markdown"
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

// reposViewMsg is a no-op message used to trigger textarea.repositionView()
// after manual cursor navigation (CursorUp/CursorDown don't update the viewport).
type reposViewMsg struct{}

// rightPanelWidth returns the width of the dashboard right panel (markdown preview / diff).
// Left pane is 25% of width; right gets the rest minus a 6-col border/padding budget,
// clamped to a 20-col minimum so content doesn't collapse on narrow terminals.
func (m model) rightPanelWidth() int {
	w := m.width - (m.width * 25 / 100) - 6
	if w < 20 {
		w = 20
	}
	return w
}

func mouseWheelDelta(msg tea.MouseMsg) int {
	if msg.Action != tea.MouseActionPress {
		return 0
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return -3
	case tea.MouseButtonWheelDown:
		return 3
	default:
		return 0
	}
}

func addScrollOffset(offset *int, delta int) {
	*offset += delta
	if *offset < 0 {
		*offset = 0
	}
}

func (m model) dashboardPinnedCount() int {
	count := 0
	for _, p := range m.projects {
		if m.favorites[p.Path] {
			count++
		}
	}
	return count
}

func (m model) dashboardListPageSize() int {
	availableHeight := m.height - 6 // header + separators + footer
	if availableHeight < 5 {
		availableHeight = 5
	}
	panelHeight := availableHeight - 2 // border top/bottom
	pinnedDisplayLines := 0
	if pinned := m.dashboardPinnedCount(); pinned > 0 {
		pinnedDisplayLines = pinned + 4
	}
	pageSize := panelHeight - pinnedDisplayLines - 3
	if pageSize < 3 {
		pageSize = 3
	}
	return pageSize
}

func isDumpMode(mode mode) bool {
	switch mode {
	case modeDumpInput, modeDumpRouting, modeDumpConfirm, modeDumpReview, modeDumpClarify, modeDumpSummary:
		return true
	default:
		return false
	}
}

func isAgentMode(mode mode) bool {
	switch mode {
	case modeAgentList, modeAgentPicker, modeAgentLaunch, modeAgentAttached, modeAgentManage:
		return true
	default:
		return false
	}
}

func (m model) switchTopNavPage(target page) (tea.Model, tea.Cmd) {
	switch target {
	case pageDashboard:
		m.page = pageDashboard
		m.mode = modeNormal
		if len(m.projects) > 0 && m.cursor < len(m.projects) {
			rightW := m.rightPanelWidth()
			m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
			m.viewport.GotoTop()
		}
		return m, nil
	case pageDump:
		m.page = pageDump
		if !isDumpMode(m.mode) {
			m.mode = modeDumpInput
		}
		m.dumpArea.Focus()
		return m, m.dumpArea.Cursor.BlinkCmd()
	case pageAgent:
		m.page = pageAgent
		if !isAgentMode(m.mode) {
			m.mode = modeAgentList
		}
		if m.cockpitClient != nil {
			return m, cockpitRefreshCmd(m.cockpitClient)
		}
		return m, nil
	default:
		return m, nil
	}
}

// rediscover re-scans configured scan dirs / idea dirs for markdown files
// and refreshes the routing-context index. Shared across refresh, post-cleanup
// save, and chain cleanup completion.
func (m model) rediscover() []workmd.Project {
	projects := workmd.Discover(
		m.cfg.ExpandedScanRoots(),
		m.cfg.FilePatterns,
		m.cfg.ExpandedIdeaDirs(),
		m.cfg,
	)
	var targets []workmd.SpecialTarget
	if t := m.cfg.CatchallTarget; t != nil {
		targets = append(targets, workmd.SpecialTarget{
			Name: t.Name, Path: t.Path, Description: "catch-all for general notes",
		})
	}
	if t := m.cfg.IdeasTarget; t != nil {
		targets = append(targets, workmd.SpecialTarget{
			Name: t.Name, Path: t.Path, Description: "ideas not tied to a project",
		})
	}
	_ = workmd.WriteIndex(m.cfg.ExpandedIndexPath(), projects, targets, workmd.IndexOptions{
		ScanRoots:    m.cfg.ExpandedScanRoots(),
		FilePatterns: m.cfg.FilePatterns,
		IdeaDirs:     m.cfg.ExpandedIdeaDirs(),
	})
	return projects
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New(m.width-4, m.contentAreaHeight())
		if m.page == pageProject && m.selected < len(m.projects) {
			m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
		} else if m.page == pageDashboard && m.cursor < len(m.projects) {
			rightW := m.rightPanelWidth()
			m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
		} else if m.page == pageAgent && m.mode == modeAgentAttached {
			m.refreshAttachedViewport(false)
		}
		// Resize dump textarea to fill available space
		dumpH := m.contentViewportHeight(2)
		if dumpH < 3 {
			dumpH = 3
		}
		m.dumpArea.SetWidth(m.width - 6)
		m.dumpArea.SetHeight(dumpH)
		m.launchBrief.SetWidth(m.width - 6)
		m.launchRepoCustom.Width = maxInt(1, m.width-14)
		m.attachedInput.SetWidth(m.attachedInputWidth())
		return m, nil

	case tea.MouseMsg:
		if delta := mouseWheelDelta(msg); delta != 0 {
			return m.handleMouseWheel(delta), nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if target, ok := m.headerTabAt(msg.X, msg.Y); ok {
				return m.switchTopNavPage(target)
			}
		}
		return m, nil

	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode
		if msg.String() == "ctrl+c" {
			return m, m.quitCmd()
		}

		// In text-input modes, route directly to their handlers (don't intercept ?, etc.)
		if m.mode == modeEdit {
			return m.updateEdit(msg)
		}
		if m.mode == modeDumpInput || m.mode == modeDumpClarify || m.mode == modeDumpSummary {
			return m.updateDump(msg)
		}
		if m.mode == modeChainCleanupFeedback || m.mode == modeChainCleanupReview || m.mode == modeChainCleanupSummary {
			return m.updateCleanup(msg)
		}
		if m.mode == modeCleanupFeedback {
			return m.updateCleanup(msg)
		}

		// Search mode intercept
		if m.mode == modeSearch {
			return m.updateSearch(msg)
		}

		// Agent text-input modes — let q/? through to be typed.
		if m.mode == modeAgentAttached && m.attachedFocus == 1 {
			return m.updateAgent(msg)
		}
		if m.mode == modeAgentManage && m.agentManageEditing {
			return m.updateAgent(msg)
		}
		if m.mode == modeAgentLaunch && (m.launchFocus == m.launchNoteFocus() || m.launchRepoEditing) {
			return m.updateAgent(msg)
		}

		// Global keys
		switch msg.String() {
		case "?":
			if m.mode == modeHelp {
				m.mode = modeNormal
				m.helpScroll = 0
			} else {
				m.mode = modeHelp
				m.helpScroll = 0
			}
			return m, nil
		case "ctrl+r":
			if m.cockpitClient != nil {
				if next, cmd, ok := m.beginTakeoverFromPendingTarget(); ok {
					return next, cmd
				}
			}
		}

		if m.mode == modeHelp {
			switch msg.String() {
			case "j", "down":
				m.helpScroll++
				m.helpScroll = clampScrollOffset(m.helpScroll, len(m.helpLines()), m.helpVisibleHeight())
			case "k", "up":
				if m.helpScroll > 0 {
					m.helpScroll--
				}
				m.helpScroll = clampScrollOffset(m.helpScroll, len(m.helpLines()), m.helpVisibleHeight())
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
		case pageCleanup:
			return m.updateCleanup(msg)
		case pageAgent:
			return m.updateAgent(msg)
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
			switch m.page {
			case pageDashboard:
				rightW := m.rightPanelWidth()
				m.viewport.SetContent(markdown.Render(m.projects[0].Content, rightW))
				m.viewport.GotoTop()
			case pageProject:
				if m.selected >= 0 && m.selected < len(m.projects) {
					m.viewport.SetContent(markdown.Render(m.projects[m.selected].Content, m.width-4))
					m.viewport.GotoTop()
				}
			}
		}
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		if m.page == pageAgent && m.cockpitClient != nil {
			cmds = append(cmds, cockpitRefreshCmd(m.cockpitClient))
		}
		return m, tea.Batch(cmds...)

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

	case cockpitEventMsg:
		return m.handleCockpitEvent(msg)

	case cockpitJobsMsg:
		m.cockpitJobs = msg.jobs
		if m.page == pageAgent && m.mode == modeAgentAttached {
			m.syncAttachedJobState()
			m.refreshAttachedViewport(false)
		}
		return m, nil

	case cockpitForemanMsg:
		m.cockpitForeman = msg.state
		return m, nil

	case planResultMsg:
		if msg.err != nil {
			m.statusMsg = "plan failed: " + msg.err.Error()
			m.statusExpiry = time.Now().Add(5 * time.Second)
			m.mode = modeNormal
			return m, nil
		}
		m.planResult = msg.result
		rightW := m.rightPanelWidth()
		m.viewport.SetContent(markdown.Render(msg.result, rightW-4))
		m.viewport.GotoTop()
		m.mode = modePlanResult
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

func (m model) handleMouseWheel(delta int) tea.Model {
	if m.mode == modeHelp {
		addScrollOffset(&m.helpScroll, delta/3)
		m.helpScroll = clampScrollOffset(m.helpScroll, len(m.helpLines()), m.helpVisibleHeight())
		return m
	}

	switch m.page {
	case pageDashboard:
		switch m.mode {
		case modePlanResult, modeTodoResult:
			if delta > 0 {
				m.viewport.LineDown(delta)
			} else {
				m.viewport.LineUp(-delta)
			}
			return m
		case modeNormal:
			if delta > 0 {
				m.viewport.LineDown(delta)
			} else {
				m.viewport.LineUp(-delta)
			}
			return m
		}
	case pageProject:
		if m.mode == modeNormal {
			if delta > 0 {
				m.viewport.LineDown(delta)
			} else {
				m.viewport.LineUp(-delta)
			}
		}
		return m
	case pageCleanup:
		switch m.mode {
		case modeChainCleanupReview, modeNormal:
			if delta > 0 {
				m.viewport.LineDown(delta)
			} else {
				m.viewport.LineUp(-delta)
			}
		case modeChainCleanupSummary:
			addScrollOffset(&m.chainSummaryScroll, delta/3)
			m.chainSummaryScroll = clampScrollOffset(m.chainSummaryScroll, len(m.chainCleanupSummaryLines()), m.summaryVisibleHeight())
		}
		return m
	case pageDump:
		if m.mode == modeDumpSummary {
			addScrollOffset(&m.dumpSummaryScroll, delta/3)
			m.dumpSummaryScroll = clampScrollOffset(m.dumpSummaryScroll, len(m.dumpSummaryLines()), m.summaryVisibleHeight())
		}
		return m
	case pageAgent:
		return m.handleAgentMouseWheel(delta)
	}
	return m
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
			rightW := m.rightPanelWidth()
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
	case "q", "esc":
		return m, m.quitCmd()
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
	case "ctrl+f":
		m.viewport.ViewDown()
	case "ctrl+b":
		m.viewport.ViewUp()
	case "pgdown":
		if len(m.projects) > 0 {
			m.cursor += m.dashboardListPageSize()
			if m.cursor >= len(m.projects) {
				m.cursor = len(m.projects) - 1
			}
		}
	case "pgup":
		m.cursor -= m.dashboardListPageSize()
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "home":
		m.cursor = 0
	case "end":
		if len(m.projects) > 0 {
			m.cursor = len(m.projects) - 1
		}
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
			rightW := m.rightPanelWidth()
			m.editArea.SetWidth(rightW - 4)
			panelH := m.contentAreaHeight() - 2
			if panelH < 3 {
				panelH = 3
			}
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
					rightW := m.rightPanelWidth()
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
			m.statusMsg = "asking model to clean up..."
			m.statusExpiry = time.Now().Add(10 * time.Second)
			return m, tea.Batch(cleanupCmd(m.cfg, m.projects[m.selected].Content), m.spinner.Tick)
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
			return m, tea.Batch(cleanupCmd(m.cfg, m.cleanupOriginal), m.spinner.Tick)
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
		m.statusExpiry = time.Now().Add(10 * time.Second)
		return m, tea.Batch(planCmd(m.cfg, sources), m.spinner.Tick)
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
	case "a":
		m.page = pageAgent
		m.mode = modeAgentList
		m.agentCursor = 0
		if m.cockpitClient != nil {
			return m, cockpitRefreshCmd(m.cockpitClient)
		}
		return m, nil
	case "A":
		m.page = pageAgent
		if !m.openCurrentProjectPicker() {
			m.mode = modeAgentList
		}
		if m.cockpitClient != nil {
			return m, cockpitRefreshCmd(m.cockpitClient)
		}
		return m, nil
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
			m.statusMsg = "asking model..."
			m.statusExpiry = time.Now().Add(10 * time.Second)
			return m, tea.Batch(todoCmd(m.cfg, m.projects[m.cursor].Content), m.spinner.Tick)
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
		m.projects = m.rediscover()
		sortWithFavorites(m.projects, m.favorites)
		m.statusMsg = "refreshed"
		m.statusExpiry = time.Now().Add(2 * time.Second)
	case ",":
		if path, err := config.Dir(); err == nil {
			return m, openInEditor(path)
		}
	}

	// Update right-panel viewport when cursor changes
	if m.cursor != prevCursor && m.cursor < len(m.projects) {
		rightW := m.rightPanelWidth()
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
			m.editArea.SetHeight(m.contentViewportHeight(2))
			m.editArea.Focus()
			return m, m.editArea.Cursor.BlinkCmd()
		}
	case "c":
		if m.selected < len(m.projects) {
			m.cleanupOriginal = m.projects[m.selected].Content
			m.mode = modeCleanupWait
			m.statusMsg = "asking model to clean up..."
			m.statusExpiry = time.Now().Add(10 * time.Second)
			return m, tea.Batch(cleanupCmd(m.cfg, m.projects[m.selected].Content), m.spinner.Tick)
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

func todoCmd(cfg *config.Config, content string) tea.Cmd {
	return func() tea.Msg {
		client := llm.New(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, err := client.NextTodo(ctx, content)
		return todoResultMsg{result: result, err: err}
	}
}

func (m model) quitCmd() tea.Cmd {
	if m.cockpitDetachQuit {
		return func() tea.Msg {
			_ = cockpit.DetachClient()
			return tea.Quit()
		}
	}
	return tea.Quit
}

func cleanupCmd(cfg *config.Config, content string) tea.Cmd {
	return func() tea.Msg {
		client := llm.New(cfg)
		result, err := client.Cleanup(context.Background(), content, "")
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
			m.chainSummaryScroll = clampScrollOffset(m.chainSummaryScroll, len(m.chainCleanupSummaryLines()), m.summaryVisibleHeight())
		case "k", "up":
			if m.chainSummaryScroll > 0 {
				m.chainSummaryScroll--
			}
			m.chainSummaryScroll = clampScrollOffset(m.chainSummaryScroll, len(m.chainCleanupSummaryLines()), m.summaryVisibleHeight())
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
				m.projects = m.rediscover()
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
		rightW := m.rightPanelWidth()
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
		rightW := m.rightPanelWidth()
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
		return m, tea.Batch(cleanupWithFeedbackCmd(m.cfg, m.cleanupOriginal, feedback), m.spinner.Tick)
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
			m.projects = m.rediscover()
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
		return m, tea.Batch(cleanupWithFeedbackCmd(m.cfg, m.cleanupOriginal, feedback), m.spinner.Tick)
	}
	var cmd tea.Cmd
	m.chainFeedback, cmd = m.chainFeedback.Update(msg)
	return m, cmd
}

func (m model) advanceChainCursor() (tea.Model, tea.Cmd) {
	m.chainCursor++
	if m.chainCursor >= len(m.chainQueue) {
		if m.chainAccepted > 0 {
			m.projects = m.rediscover()
		}
		m.mode = modeChainCleanupSummary
		return m, nil
	}
	idx := m.chainQueue[m.chainCursor]
	m.selected = idx
	m.cleanupOriginal = m.projects[idx].Content
	m.mode = modeChainCleanupWait
	return m, tea.Batch(cleanupCmd(m.cfg, m.cleanupOriginal), m.spinner.Tick)
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
	rightW := m.rightPanelWidth()
	if m.cursor < len(m.projects) {
		m.viewport.SetContent(markdown.Render(m.projects[m.cursor].Content, rightW))
	}
	m.viewport.GotoTop()
	return m, nil
}

func planCmd(cfg *config.Config, projects []workmd.Project) tea.Cmd {
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
		client := llm.New(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		result, err := client.DailyPlan(ctx, summary)
		return planResultMsg{result: result, err: err}
	}
}

func cleanupWithFeedbackCmd(cfg *config.Config, content, feedback string) tea.Cmd {
	return func() tea.Msg {
		client := llm.New(cfg)
		result, err := client.Cleanup(context.Background(), content, feedback)
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
	case "ctrl+home":
		m.editArea.SetValue(m.editArea.Value())
		m.editArea, _ = m.editArea.Update(reposViewMsg{})
		return m, nil
	case "ctrl+end":
		value := m.editArea.Value()
		lines := strings.Split(value, "\n")
		if len(lines) == 0 {
			return m, nil
		}
		m.editArea.SetValue(value)
		for i := 0; i < len(lines)-1; i++ {
			m.editArea.CursorDown()
		}
		m.editArea.CursorEnd()
		m.editArea, _ = m.editArea.Update(reposViewMsg{})
		return m, nil
	case "ctrl+d":
		// Delete current line, reposition cursor to same line number
		value := m.editArea.Value()
		lineNum := m.editArea.Line()
		lines := strings.Split(value, "\n")
		if lineNum < len(lines) {
			newLines := append(lines[:lineNum], lines[lineNum+1:]...)
			m.editArea.SetValue(strings.Join(newLines, "\n"))
			// SetValue leaves cursor at end; move up to target line.
			target := lineNum
			if target >= len(newLines) {
				target = len(newLines) - 1
			}
			if target < 0 {
				target = 0
			}
			for i := 0; i < len(newLines)-1-target; i++ {
				m.editArea.CursorUp()
			}
			m.editArea.CursorStart()
			// CursorUp doesn't call repositionView; pass a no-op msg to trigger it.
			m.editArea, _ = m.editArea.Update(reposViewMsg{})
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
			m.dumpSummaryScroll = clampScrollOffset(m.dumpSummaryScroll, len(m.dumpSummaryLines()), m.summaryVisibleHeight())
		case "k", "up":
			if m.dumpSummaryScroll > 0 {
				m.dumpSummaryScroll--
			}
			m.dumpSummaryScroll = clampScrollOffset(m.dumpSummaryScroll, len(m.dumpSummaryLines()), m.summaryVisibleHeight())
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
		return m, tea.Batch(routeDumpCmd(text, m.projects, m.cfg), m.spinner.Tick)
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

	case "esc", "q":
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
		m.statusExpiry = time.Now().Add(10 * time.Second)
		item := m.dumpItems[m.dumpCursor]
		return m, tea.Batch(rerouteDumpCmd(item.Text, clarification, m.projects, m.cfg), m.spinner.Tick)
	}

	var cmd tea.Cmd
	m.dumpClarifyArea, cmd = m.dumpClarifyArea.Update(msg)
	return m, cmd
}

// writeDumpItem appends an item to the correct file.
func (m model) writeDumpItem(item llm.RouteItem) error {
	// Normalize: strip leading #, trim spaces
	proj := strings.TrimSpace(strings.TrimPrefix(item.Project, "#"))
	projLower := strings.ToLower(proj)

	// Configured ideas bucket — section default is "inbox" so items land in the
	// well-known unsorted spot regardless of what the model guessed.
	if t := m.cfg.IdeasTarget; t != nil && t.Name != "" && projLower == strings.ToLower(t.Name) {
		return workmd.AppendToSection(config.ExpandHome(t.Path), "inbox", item.Text)
	}

	// Configured catch-all bucket — preserves the section the model chose.
	if t := m.cfg.CatchallTarget; t != nil && t.Name != "" && projLower == strings.ToLower(t.Name) {
		return workmd.AppendToSection(config.ExpandHome(t.Path), item.Section, item.Text)
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
		m.projects = m.rediscover()
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
			rightW := m.rightPanelWidth()
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

// projectDescs converts the model's discovered projects into the name+description
// shape RouteMulti/RerouteSingle expect.
func projectDescs(projects []workmd.Project) []llm.ProjectDesc {
	out := make([]llm.ProjectDesc, len(projects))
	for i, p := range projects {
		out[i] = llm.ProjectDesc{
			Name:        p.Name,
			Description: p.Description,
			Phase:       p.Phase,
			Preview:     p.ActivePreview,
		}
	}
	return out
}

// targetFromConfig maps a *config.Target into the prompt-time SpecialTarget
// (with a fixed description string for the prompt body), or nil if unset.
func targetFromConfig(t *config.Target, desc string) *llm.SpecialTarget {
	if t == nil || t.Name == "" {
		return nil
	}
	return &llm.SpecialTarget{Name: t.Name, Description: desc}
}

func routeDumpCmd(text string, projects []workmd.Project, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		client := llm.New(cfg)
		catchall := targetFromConfig(cfg.CatchallTarget, "catch-all for general notes that don't belong to any project above")
		ideas := targetFromConfig(cfg.IdeasTarget, "ideas not tied to a specific project")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		items, err := client.RouteMulti(ctx, text, projectDescs(projects), catchall, ideas)
		return dumpRoutedMsg{items: items, err: err}
	}
}

func rerouteDumpCmd(text, clarification string, projects []workmd.Project, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		client := llm.New(cfg)
		catchall := targetFromConfig(cfg.CatchallTarget, "catch-all for general notes")
		ideas := targetFromConfig(cfg.IdeasTarget, "ideas not tied to a specific project")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		item, err := client.RerouteSingle(ctx, text, clarification, projectDescs(projects), catchall, ideas)
		return dumpReroutedMsg{item: item, err: err}
	}
}
