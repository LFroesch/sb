package tui

import (
	"fmt"
	"os"
	"path/filepath"
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

func (m model) createPresetTemplate() (string, error) {
	if err := os.MkdirAll(m.cockpitPaths.PresetsDir, 0o755); err != nil {
		return "", err
	}
	id := fmt.Sprintf("custom-%s", time.Now().Format("20060102-150405"))
	path := filepath.Join(m.cockpitPaths.PresetsDir, id+".json")
	body := fmt.Sprintf(`{
  "id": "%s",
  "name": "Custom preset",
  "role": "custom",
  "launch_mode": "single_job",
  "system_prompt": "Describe the job this preset should do.",
  "executor": {
    "type": "claude"
  },
  "hooks": {
    "prompt": [
      {
        "kind": "literal",
        "placement": "after",
        "label": "extra context",
        "body": "Optional extra prompt block."
      }
    ],
    "pre_shell": [
      {
        "name": "example pre hook",
        "cmd": "pwd"
      }
    ],
    "post_shell": [
      {
        "name": "example post hook",
        "cmd": "git status --short"
      }
    ],
    "iteration": {
      "mode": "one_shot"
    }
  },
  "permissions": "scoped-write"
}
`, id)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (m model) createProviderTemplate() (string, error) {
	if err := os.MkdirAll(m.cockpitPaths.ProvidersDir, 0o755); err != nil {
		return "", err
	}
	id := fmt.Sprintf("custom-%s", time.Now().Format("20060102-150405"))
	path := filepath.Join(m.cockpitPaths.ProvidersDir, id+".json")
	body := fmt.Sprintf(`{
  "id": "%s",
  "name": "Custom provider",
  "executor": {
    "type": "claude"
  }
}
`, id)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (m model) updateAgentLaunch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Freeform launches (no sources) came from the list; sourced
		// launches came from the picker. Return to whichever we came from.
		if len(m.launchSources) == 0 {
			m.mode = modeAgentList
		} else {
			m.mode = modeAgentPicker
		}
		m.agentCursor = 0
		return m, nil
	case "q":
		// q only acts as back when the brief isn't being typed.
		if m.launchFocus != m.launchNoteFocus() {
			if len(m.launchSources) == 0 {
				m.mode = modeAgentList
			} else {
				m.mode = modeAgentPicker
			}
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
		if m.launchFocus != m.launchNoteFocus() {
			return m.doLaunch()
		}
	case "F":
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
	repo := m.launchRepo
	if repo == "" {
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

func (m model) launchRepoChoices() []string {
	seen := map[string]bool{}
	var repos []string
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		repos = append(repos, path)
	}
	add(m.launchRepo)
	add(m.defaultLaunchRepo())
	for _, p := range m.projects {
		add(p.Dir)
	}
	cwd, err := os.Getwd()
	if err == nil {
		add(cwd)
	}
	if len(repos) == 0 {
		return []string{""}
	}
	return repos
}

func indexOfLaunchRepo(repos []string, current string) int {
	current = strings.TrimSpace(current)
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
