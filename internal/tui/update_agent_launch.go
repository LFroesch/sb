package tui

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
)

func (m *model) resetAgentLaunch() {
	m.launchSources = nil
	m.launchRepo = ""
	m.launchBrief.Reset()
	m.launchBrief.SetWidth(m.width - 6)
	m.launchBrief.SetHeight(6)
	m.launchBrief.Blur()
	m.launchFocus = 0
	m.launchPresetIdx = defaultPresetIndex(m.cockpitPresets)
	m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
	m.launchReviewOffset = 0
	m.launchQueueOnly = false
}

func (m model) launchHasRepoStep() bool {
	return len(m.launchSources) == 0
}

func (m model) launchFocusCount() int {
	if m.launchHasRepoStep() {
		return 5
	}
	return 4
}

func (m model) launchNoteFocus() int {
	if m.launchHasRepoStep() {
		return 3
	}
	return 2
}

func (m model) launchReviewFocus() int {
	if m.launchHasRepoStep() {
		return 4
	}
	return 3
}

func (m *model) normalizeLaunchFocus() {
	max := m.launchFocusCount() - 1
	if m.launchFocus > max {
		m.launchFocus = max
	}
}

func (m *model) moveLaunchFocusToNote() tea.Cmd {
	m.launchFocus = m.launchNoteFocus()
	m.launchBrief.Focus()
	return m.launchBrief.Cursor.BlinkCmd()
}

func (m *model) syncLaunchRepoToVisibleChoice() {
	if !m.launchHasRepoStep() {
		return
	}
	repos := m.launchRepoChoices()
	if len(repos) == 0 {
		return
	}
	m.launchRepo = repos[indexOfLaunchRepo(repos, m.launchRepo)]
}

func (m model) updateAgentLaunch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Custom-repo path entry consumes keys until esc or enter.
	if m.launchRepoEditing {
		switch msg.String() {
		case "esc":
			m.launchRepoEditing = false
			m.launchRepoCustom.Blur()
			return m, nil
		case "enter":
			path := strings.TrimSpace(m.launchRepoCustom.Value())
			m.launchRepoEditing = false
			m.launchRepoCustom.Blur()
			if path != "" {
				m.launchRepo = path
				m.statusMsg = "repo set to " + path
				m.statusExpiry = time.Now().Add(2 * time.Second)
				return m, m.moveLaunchFocusToNote()
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.launchRepoCustom, cmd = m.launchRepoCustom.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "esc":
		// Without sources, the user came from the picker's freeform sentinel
		// (or dashboard A on a project without items); either way return to
		// the picker so they can change their mind. Sourced runs also came
		// from the picker, so the same target works for both.
		m.mode = modeAgentPicker
		m.agentCursor = 0
		return m, nil
	case "q":
		// q only acts as back when the brief isn't being typed.
		if m.launchFocus != m.launchNoteFocus() {
			m.mode = modeAgentPicker
			m.agentCursor = 0
			return m, nil
		}
	case "tab":
		m.normalizeLaunchFocus()
		m.launchFocus = (m.launchFocus + 1) % m.launchFocusCount()
		if m.launchFocus == m.launchNoteFocus() {
			m.launchBrief.Focus()
			return m, m.launchBrief.Cursor.BlinkCmd()
		}
		m.launchBrief.Blur()
		return m, nil
	case "shift+tab":
		m.normalizeLaunchFocus()
		m.launchFocus = (m.launchFocus + m.launchFocusCount() - 1) % m.launchFocusCount()
		if m.launchFocus == m.launchNoteFocus() {
			m.launchBrief.Focus()
			return m, m.launchBrief.Cursor.BlinkCmd()
		}
		m.launchBrief.Blur()
		return m, nil
	case "alt+enter", "ctrl+enter":
		// alt+enter is the portable submit-from-anywhere shortcut;
		// ctrl+enter only works in terminals with the kitty keyboard
		// protocol but we accept it when it does land.
		return m.doLaunch()
	case "enter":
		// Enter launches from the preset/provider pickers. When the brief
		// textarea has focus, enter inserts a newline (handled below).
		// On the Repo tab, enter confirms the repo choice and advances to
		// the note editor; if the "(custom path...)" sentinel is selected,
		// it first opens the inline path editor.
		if m.launchHasRepoStep() && m.launchFocus == 2 {
			m.syncLaunchRepoToVisibleChoice()
			if strings.TrimSpace(m.launchRepo) == repoSentinelCustom {
				m.launchRepoCustom.SetValue("")
				m.launchRepoCustom.Width = maxInt(1, m.width-14)
				m.launchRepoCustom.Focus()
				m.launchRepoEditing = true
				return m, m.launchRepoCustom.Cursor.BlinkCmd()
			}
			return m, m.moveLaunchFocusToNote()
		}
		if m.launchFocus != m.launchNoteFocus() {
			return m.doLaunch()
		}
	case "ctrl+t":
		m.launchQueueOnly = !m.launchQueueOnly
		if m.launchQueueOnly {
			m.statusMsg = "this run will be sent to Foreman"
		} else {
			m.statusMsg = "this run will start immediately"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, nil
	}

	// Navigation is only applied when the brief textarea is not focused.
	if m.launchFocus == m.launchNoteFocus() {
		var cmd tea.Cmd
		m.launchBrief, cmd = m.launchBrief.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "j", "down":
		switch m.launchFocus {
		case 0:
			if m.launchPresetIdx < len(m.cockpitPresets)-1 {
				m.launchPresetIdx++
				m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			}
		case 1:
			if m.launchProviderIdx < len(providerChoices(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders))-1 {
				m.launchProviderIdx++
			}
		case 2:
			if m.launchHasRepoStep() {
				repos := m.launchRepoChoices()
				idx := indexOfLaunchRepo(repos, m.launchRepo)
				if idx < len(repos)-1 {
					m.launchRepo = repos[idx+1]
				}
			} else {
				m.launchReviewOffset++
				m.clampLaunchReviewOffset()
			}
		case 3:
			if !m.launchHasRepoStep() {
				m.launchReviewOffset++
				m.clampLaunchReviewOffset()
			}
		case 4:
			m.launchReviewOffset++
			m.clampLaunchReviewOffset()
		}
	case "k", "up":
		switch m.launchFocus {
		case 0:
			if m.launchPresetIdx > 0 {
				m.launchPresetIdx--
				m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			}
		case 1:
			if m.launchProviderIdx > 0 {
				m.launchProviderIdx--
			}
		case 2:
			if m.launchHasRepoStep() {
				repos := m.launchRepoChoices()
				idx := indexOfLaunchRepo(repos, m.launchRepo)
				if idx > 0 {
					m.launchRepo = repos[idx-1]
				}
			} else if m.launchReviewOffset > 0 {
				m.launchReviewOffset--
				m.clampLaunchReviewOffset()
			}
		case 3:
			if !m.launchHasRepoStep() && m.launchReviewOffset > 0 {
				m.launchReviewOffset--
				m.clampLaunchReviewOffset()
			}
		case 4:
			if m.launchReviewOffset > 0 {
				m.launchReviewOffset--
				m.clampLaunchReviewOffset()
			}
		}
	case "pgdown", "pgdn":
		if m.launchFocus == m.launchReviewFocus() {
			m.launchReviewOffset += 5
			m.clampLaunchReviewOffset()
		}
	case "pgup":
		if m.launchFocus == m.launchReviewFocus() {
			m.launchReviewOffset -= 5
			m.clampLaunchReviewOffset()
		}
	}
	return m, nil
}

func (m model) doLaunch() (tea.Model, tea.Cmd) {
	if len(m.cockpitPresets) == 0 {
		m.statusMsg = "no presets available"
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil
	}
	preset := m.cockpitPresets[m.launchPresetIdx]
	repo := strings.TrimSpace(m.launchRepo)
	if repo == repoSentinelCustom {
		// User landed on the "custom path..." sentinel without typing one.
		// Fall back to default rather than passing the marker through.
		repo = m.defaultLaunchRepo()
	}
	req := cockpit.LaunchRequest{
		Preset:    preset,
		Sources:   m.launchSources,
		Repo:      repo,
		Freeform:  m.launchBrief.Value(),
		QueueOnly: m.launchQueueOnly,
	}
	if m.launchProviderIdx >= 0 && m.launchProviderIdx < len(m.cockpitProviders) {
		exec := m.cockpitProviders[m.launchProviderIdx].Executor
		req.Provider = &exec
	}
	job, err := m.cockpitClient.LaunchJob(req)
	if err != nil {
		m.statusMsg = "launch: " + err.Error()
		m.statusExpiry = time.Now().Add(5 * time.Second)
		return m, nil
	}
	if m.launchQueueOnly {
		m.statusMsg = "sent to Foreman: " + preset.Name
	} else {
		m.statusMsg = "launched " + preset.Name
	}
	m.statusExpiry = time.Now().Add(3 * time.Second)
	if m.launchQueueOnly {
		m.mode = modeAgentList
		return m, tea.Batch(cockpitRefreshCmd(m.cockpitClient), cockpitForemanRefreshCmd(m.cockpitClient))
	}
	return m.openAgentJob(job.ID, true)
}

func (m model) defaultLaunchRepo() string {
	if m.page == pageProject && m.selected >= 0 && m.selected < len(m.projects) && m.projects[m.selected].Dir != "" {
		return m.projects[m.selected].Dir
	}
	if m.cursor >= 0 && m.cursor < len(m.projects) && m.projects[m.cursor].Dir != "" {
		return m.projects[m.cursor].Dir
	}
	cwd, err := os.Getwd()
	if err == nil {
		return cwd
	}
	return ""
}

// repoSentinelCustom is a non-path marker. When it's the selected repo
// the view shows "(custom path...)" and pressing enter opens an inline
// text input so the user can type any absolute path — even one that
// isn't a discovered WORK.md project.
const repoSentinelCustom = "\x00custom"

// launchRepoChoices returns the menu shown on the Repo tab: every
// discovered repo path plus a "(custom path...)" entry at the end so
// the user can type a path that isn't tracked by sb.
func (m model) launchRepoChoices() []string {
	seen := map[string]bool{}
	repos := []string{repoSentinelCustom}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || path == repoSentinelCustom || seen[path] {
			return
		}
		seen[path] = true
		repos = append(repos, path)
	}
	add(m.defaultLaunchRepo())
	for _, p := range m.projects {
		add(p.Dir)
	}
	cwd, err := os.Getwd()
	if err == nil {
		add(cwd)
	}
	add(m.launchRepo)
	return repos
}

func indexOfLaunchRepo(repos []string, current string) int {
	current = strings.TrimSpace(current)
	if current == "" {
		if len(repos) > 1 {
			return 1
		}
		return 0
	}
	for i, repo := range repos {
		if strings.TrimSpace(repo) == current {
			return i
		}
	}
	if len(repos) == 0 {
		return 0
	}
	return 0
}

func defaultProviderIndex(presets []cockpit.LaunchPreset, presetIdx int, providers []cockpit.ProviderProfile) int {
	for i, p := range providers {
		if strings.EqualFold(p.ID, "codex") {
			return i
		}
	}
	if presetIdx >= 0 && presetIdx < len(presets) {
		want := presets[presetIdx].Executor
		for i, p := range providers {
			if sameExecutor(p.Executor, want) {
				return i
			}
		}
	}
	if len(providers) == 0 {
		return 0
	}
	return 0
}

func defaultPresetIndex(presets []cockpit.LaunchPreset) int {
	preferred := []string{"senior-dev", "scaffold", "bug-fixer"}
	for _, want := range preferred {
		for i, p := range presets {
			if strings.EqualFold(p.ID, want) {
				return i
			}
		}
	}
	if len(presets) == 0 {
		return 0
	}
	return 0
}

func sameExecutor(a, b cockpit.ExecutorSpec) bool {
	return a.Type == b.Type && a.Model == b.Model && a.Cmd == b.Cmd
}
