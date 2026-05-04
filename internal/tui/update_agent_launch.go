package tui

import (
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/config"
)

func (m *model) resetAgentLaunch() {
	m.launchSources = nil
	m.launchRepo = ""
	m.launchBrief.Reset()
	m.launchBrief.SetWidth(m.width - 6)
	m.launchBrief.SetHeight(6)
	m.launchBrief.Blur()
	m.launchFocus = 0
	m.launchPresetIdx = defaultPresetIndexWithConfig(m.cockpitPresets, m.cfg)
	m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
	m.launchPromptIdx = -1
	m.launchHookCursor = -1
	m.launchHookOverride = false
	m.launchHookSelected = map[string]bool{}
	m.launchPermsIdx = 0
	m.launchShowAdvanced = false
	m.launchReviewOffset = 0
	m.launchQueueOnly = false
	m.launchSelectEditing = false
	m.launchSelectInput.Blur()
	m.launchSelectInput.SetValue("")
}

// Focus tab layout:
//
//	0=Role  1=Engine  2=Prompt  3=Hooks  4=Perms  [5=Repo]  Note  Review
//
// Repo only appears when launching without sources (freeform); the rest
// of the steps are always present.
const (
	launchFocusRole   = 0
	launchFocusEngine = 1
	launchFocusPrompt = 2
	launchFocusHooks  = 3
	launchFocusPerms  = 4
)

func (m model) launchHasRepoStep() bool {
	return len(m.launchSources) == 0
}

func (m model) launchRepoFocus() int {
	if m.launchHasRepoStep() {
		return 5
	}
	return -1
}

func (m model) launchFocusCount() int {
	return len(m.launchVisibleFocuses())
}

func (m model) launchNoteFocus() int {
	if m.launchHasRepoStep() {
		return 6
	}
	return 5
}

func (m model) launchReviewFocus() int {
	if m.launchHasRepoStep() {
		return 7
	}
	return 6
}

var launchPermsLabels = []string{"(role)", "read-only", "scoped-write", "wide-open"}

const (
	launchPromptRoleDefault = -1
	launchPromptNone        = -2
)

func launchPermsValue(idx int) string {
	switch idx {
	case 1:
		return "read-only"
	case 2:
		return "scoped-write"
	case 3:
		return "wide-open"
	}
	return ""
}

func launchPermsIndex(value string) int {
	switch strings.TrimSpace(value) {
	case "read-only":
		return 1
	case "scoped-write":
		return 2
	case "wide-open":
		return 3
	}
	return 0
}

func (m *model) normalizeLaunchFocus() {
	visible := m.launchVisibleFocuses()
	if len(visible) == 0 {
		m.launchFocus = launchFocusRole
		return
	}
	for _, focus := range visible {
		if m.launchFocus == focus {
			return
		}
	}
	m.launchFocus = visible[0]
}

func (m model) launchOverridesActive() bool {
	promptOverride := m.launchPromptIdx == launchPromptNone ||
		(m.launchPromptIdx >= 0 && m.launchPromptIdx < len(m.cockpitPrompts))
	return promptOverride || m.launchHookOverride || m.launchPermsIdx != 0
}

func (m model) launchAdvancedVisible() bool {
	return m.launchShowAdvanced || m.launchOverridesActive()
}

func (m model) launchVisibleFocuses() []int {
	focuses := []int{launchFocusRole, launchFocusEngine}
	if m.launchAdvancedVisible() {
		focuses = append(focuses, launchFocusPrompt, launchFocusHooks, launchFocusPerms)
	}
	if m.launchHasRepoStep() {
		focuses = append(focuses, m.launchRepoFocus())
	}
	focuses = append(focuses, m.launchNoteFocus(), m.launchReviewFocus())
	return focuses
}

func (m *model) cycleLaunchFocus(delta int) {
	visible := m.launchVisibleFocuses()
	if len(visible) == 0 {
		return
	}
	current := 0
	for i, focus := range visible {
		if focus == m.launchFocus {
			current = i
			break
		}
	}
	next := (current + delta + len(visible)) % len(visible)
	m.launchFocus = visible[next]
}

func (m *model) moveLaunchFocusToNote() tea.Cmd {
	m.launchFocus = m.launchNoteFocus()
	m.launchBrief.Focus()
	return m.launchBrief.Cursor.BlinkCmd()
}

func (m *model) prepareRetryLaunch(j cockpit.Job) tea.Cmd {
	m.resetAgentLaunch()
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.launchSources = slices.Clone(j.Sources)
	m.launchRepo = strings.TrimSpace(j.Repo)
	if m.launchRepo == "" {
		m.launchRepo = m.defaultLaunchRepo()
	}
	m.launchBrief.SetValue(j.Freeform)
	m.launchQueueOnly = j.ForemanManaged
	m.launchShowAdvanced = strings.TrimSpace(j.Prompt) != "" ||
		j.Permissions != "" ||
		len(j.Hooks.Prompt) > 0 ||
		len(j.Hooks.PreShell) > 0 ||
		len(j.Hooks.PostShell) > 0

	for i, p := range m.cockpitPresets {
		if p.ID == j.PresetID {
			m.launchPresetIdx = i
			break
		}
	}
	m.launchProviderIdx = -1
	if m.launchPresetIdx >= 0 && m.launchPresetIdx < len(m.cockpitPresets) {
		preset := m.cockpitPresets[m.launchPresetIdx]
		if !sameExecutor(preset.Executor, j.Executor) {
			m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			for i, p := range m.cockpitProviders {
				if sameExecutor(p.Executor, j.Executor) {
					m.launchProviderIdx = i
					break
				}
			}
		}
	} else {
		m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
	}

	if m.launchPresetIdx >= 0 && m.launchPresetIdx < len(m.cockpitPresets) {
		preset := m.cockpitPresets[m.launchPresetIdx]
		m.launchPermsIdx = launchPermsIndex(j.Permissions)
		if j.Permissions == "" || j.Permissions == preset.Permissions {
			m.launchPermsIdx = 0
		}
	}

	return m.moveLaunchFocusToNote()
}

func normalizeLaunchLookup(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func exactOrUniqueMatch(raw string, count int, idAt func(int) string, nameAt func(int) string) int {
	q := normalizeLaunchLookup(raw)
	if q == "" {
		return -1
	}
	for i := 0; i < count; i++ {
		if normalizeLaunchLookup(idAt(i)) == q {
			return i
		}
	}
	for i := 0; i < count; i++ {
		if normalizeLaunchLookup(nameAt(i)) == q {
			return i
		}
	}
	match := -1
	for i := 0; i < count; i++ {
		if strings.Contains(normalizeLaunchLookup(idAt(i)), q) || strings.Contains(normalizeLaunchLookup(nameAt(i)), q) {
			if match >= 0 {
				return -2
			}
			match = i
		}
	}
	return match
}

func (m model) currentLaunchSelectValue() string {
	switch m.launchFocus {
	case launchFocusRole:
		if m.launchPresetIdx >= 0 && m.launchPresetIdx < len(m.cockpitPresets) {
			return m.cockpitPresets[m.launchPresetIdx].ID
		}
	case launchFocusEngine:
		if m.launchProviderIdx >= 0 && m.launchProviderIdx < len(m.cockpitProviders) {
			return m.cockpitProviders[m.launchProviderIdx].ID
		}
	case launchFocusPrompt:
		switch m.launchPromptIdx {
		case launchPromptRoleDefault:
			return "default"
		case launchPromptNone:
			return ""
		default:
			if m.launchPromptIdx >= 0 && m.launchPromptIdx < len(m.cockpitPrompts) {
				return m.cockpitPrompts[m.launchPromptIdx].ID
			}
		}
	}
	return ""
}

func (m *model) beginLaunchSelectionEdit() tea.Cmd {
	switch m.launchFocus {
	case launchFocusRole:
		m.launchSelectInput.Placeholder = "type role id/name"
	case launchFocusEngine:
		m.launchSelectInput.Placeholder = "blank = role default, or type engine id/name"
	case launchFocusPrompt:
		m.launchSelectInput.Placeholder = "blank = none, 'default' = role default, or type prompt id/name"
	default:
		return nil
	}
	m.launchSelectEditing = true
	m.launchSelectInput.SetValue(m.currentLaunchSelectValue())
	m.launchSelectInput.Width = maxInt(20, m.width-14)
	m.launchSelectInput.Focus()
	return m.launchSelectInput.Cursor.BlinkCmd()
}

func (m *model) applyLaunchSelectionEdit() error {
	raw := strings.TrimSpace(m.launchSelectInput.Value())
	switch m.launchFocus {
	case launchFocusRole:
		idx := exactOrUniqueMatch(raw, len(m.cockpitPresets),
			func(i int) string { return m.cockpitPresets[i].ID },
			func(i int) string { return m.cockpitPresets[i].Name })
		switch idx {
		case -1:
			return os.ErrInvalid
		case -2:
			return os.ErrExist
		default:
			m.launchPresetIdx = idx
			m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			m.launchPromptIdx = launchPromptRoleDefault
			m.launchHookCursor = -1
			m.launchHookOverride = false
			m.launchHookSelected = map[string]bool{}
			m.launchPermsIdx = 0
			return nil
		}
	case launchFocusEngine:
		if raw == "" || normalizeLaunchLookup(raw) == "default" || normalizeLaunchLookup(raw) == "role" {
			m.launchProviderIdx = -1
			return nil
		}
		idx := exactOrUniqueMatch(raw, len(m.cockpitProviders),
			func(i int) string { return m.cockpitProviders[i].ID },
			func(i int) string { return m.cockpitProviders[i].Name })
		switch idx {
		case -1:
			return os.ErrInvalid
		case -2:
			return os.ErrExist
		default:
			m.launchProviderIdx = idx
			return nil
		}
	case launchFocusPrompt:
		switch normalizeLaunchLookup(raw) {
		case "":
			m.launchPromptIdx = launchPromptNone
			return nil
		case "default", "role":
			m.launchPromptIdx = launchPromptRoleDefault
			return nil
		}
		idx := exactOrUniqueMatch(raw, len(m.cockpitPrompts),
			func(i int) string { return m.cockpitPrompts[i].ID },
			func(i int) string { return m.cockpitPrompts[i].Name })
		switch idx {
		case -1:
			return os.ErrInvalid
		case -2:
			return os.ErrExist
		default:
			m.launchPromptIdx = idx
			return nil
		}
	}
	return nil
}

func (m model) retryJobNow(id cockpit.JobID) (tea.Model, tea.Cmd) {
	job, err := m.cockpitClient.RetryJob(id, m.cockpitPresets)
	if err != nil {
		m.statusMsg = "retry: " + err.Error()
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil
	}
	m.statusMsg = "retried " + job.PresetID
	m.statusExpiry = time.Now().Add(2 * time.Second)
	if job.WaitForForeman {
		m.mode = modeAgentList
		return m, tea.Batch(cockpitRefreshCmd(m.cockpitClient), cockpitForemanRefreshCmd(m.cockpitClient))
	}
	return m.openAgentJob(job.ID, true)
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
	if m.launchSelectEditing {
		switch msg.String() {
		case "esc", "ctrl+[":
			m.launchSelectEditing = false
			m.launchSelectInput.Blur()
			m.launchSelectInput.SetValue("")
			return m, nil
		case "enter":
			if err := m.applyLaunchSelectionEdit(); err != nil {
				switch err {
				case os.ErrInvalid:
					m.statusMsg = "no matching selection"
				case os.ErrExist:
					m.statusMsg = "selection is ambiguous"
				default:
					m.statusMsg = "select: " + err.Error()
				}
				m.statusExpiry = time.Now().Add(3 * time.Second)
				return m, nil
			}
			m.launchSelectEditing = false
			m.launchSelectInput.Blur()
			m.launchSelectInput.SetValue("")
			m.statusMsg = "selection updated"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
		var cmd tea.Cmd
		m.launchSelectInput, cmd = m.launchSelectInput.Update(msg)
		return m, cmd
	}

	// Custom-repo path entry consumes keys until esc or enter.
	if m.launchRepoEditing {
		switch msg.String() {
		case "esc", "ctrl+[":
			// Cancel custom-path entry: drop editing flag, clear the
			// input buffer so a re-entry starts blank, and snap the repo
			// selection off the sentinel back to the default repo so the
			// user can clearly see the cancel landed.
			m.launchRepoEditing = false
			m.launchRepoCustom.Blur()
			m.launchRepoCustom.SetValue("")
			if strings.TrimSpace(m.launchRepo) == repoSentinelCustom {
				if def := strings.TrimSpace(m.defaultLaunchRepo()); def != "" {
					m.launchRepo = def
				} else {
					m.launchRepo = ""
				}
			}
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
		m.cycleLaunchFocus(+1)
		if m.launchFocus == m.launchNoteFocus() {
			m.launchBrief.Focus()
			return m, m.launchBrief.Cursor.BlinkCmd()
		}
		m.launchBrief.Blur()
		return m, nil
	case "shift+tab":
		m.normalizeLaunchFocus()
		m.cycleLaunchFocus(-1)
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
		// Enter advances the guided path. The Review step is the only place
		// where plain Enter actually launches.
		// On the Repo tab, enter confirms the repo choice and advances to
		// the note editor; if the "(custom path...)" sentinel is selected,
		// it first opens the inline path editor.
		if m.launchHasRepoStep() && m.launchFocus == m.launchRepoFocus() {
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
		if m.launchFocus == launchFocusHooks {
			m.toggleLaunchHookAtCursor()
			return m, nil
		}
		if m.launchFocus == m.launchReviewFocus() {
			return m.doLaunch()
		}
		if m.launchFocus == m.launchNoteFocus() {
			m.launchBrief.Blur()
			m.launchFocus = m.launchReviewFocus()
			return m, nil
		}
		if m.launchFocus != m.launchNoteFocus() {
			m.cycleLaunchFocus(+1)
			if m.launchFocus == m.launchNoteFocus() {
				m.launchBrief.Focus()
				return m, m.launchBrief.Cursor.BlinkCmd()
			}
			return m, nil
		}
	case " ", "space", "x":
		if m.launchFocus == launchFocusHooks {
			m.toggleLaunchHookAtCursor()
			return m, nil
		}
	case "a":
		m.launchShowAdvanced = !m.launchShowAdvanced
		if !m.launchShowAdvanced && !m.launchOverridesActive() {
			m.normalizeLaunchFocus()
			if m.launchFocus == m.launchNoteFocus() {
				m.launchBrief.Focus()
				return m, m.launchBrief.Cursor.BlinkCmd()
			}
			m.launchBrief.Blur()
		}
		return m, nil
	case "e":
		if cmd := m.beginLaunchSelectionEdit(); cmd != nil {
			return m, cmd
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
		m.launchListMove(+1)
	case "k", "up":
		m.launchListMove(-1)
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

// launchListMove handles j/k on every focus tab. delta is +1 (down) or -1 (up).
func (m *model) launchListMove(delta int) {
	switch m.launchFocus {
	case launchFocusRole:
		next := m.launchPresetIdx + delta
		if next >= 0 && next < len(m.cockpitPresets) {
			m.launchPresetIdx = next
			m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			// Reset overrides when role changes — fresh start on a new role.
			m.launchPromptIdx = launchPromptRoleDefault
			m.launchHookCursor = -1
			m.launchHookOverride = false
			m.launchHookSelected = map[string]bool{}
			m.launchPermsIdx = 0
		}
	case launchFocusEngine:
		next := m.launchProviderIdx + delta
		if next >= -1 && next < len(m.cockpitProviders) {
			m.launchProviderIdx = next
		}
	case launchFocusPrompt:
		next := m.launchPromptIdx + delta
		// -2 = "(none)", -1 = "(role default)".
		if next >= launchPromptNone && next < len(m.cockpitPrompts) {
			m.launchPromptIdx = next
		}
	case launchFocusHooks:
		next := m.launchHookCursor + delta
		if next >= -1 && next < len(m.cockpitHookBundles) {
			m.launchHookCursor = next
		}
	case launchFocusPerms:
		next := m.launchPermsIdx + delta
		if next >= 0 && next < len(launchPermsLabels) {
			m.launchPermsIdx = next
		}
	default:
		if m.launchFocus == m.launchRepoFocus() {
			repos := m.launchRepoChoices()
			idx := indexOfLaunchRepo(repos, m.launchRepo)
			next := idx + delta
			if next >= 0 && next < len(repos) {
				m.launchRepo = repos[next]
			}
			return
		}
		if m.launchFocus == m.launchReviewFocus() {
			m.launchReviewOffset += delta
			if m.launchReviewOffset < 0 {
				m.launchReviewOffset = 0
			}
			m.clampLaunchReviewOffset()
		}
	}
}

// toggleLaunchHookAtCursor flips the hook bundle under the cursor, or
// resets to "(role default)" if the cursor sits on the sentinel row. The
// override flag tracks whether to honour the user's selection (even an
// empty one — meaning "no hooks") versus inheriting from the role.
func (m *model) toggleLaunchHookAtCursor() {
	if m.launchHookCursor < 0 {
		m.launchHookOverride = false
		m.launchHookSelected = map[string]bool{}
		return
	}
	if m.launchHookCursor >= len(m.cockpitHookBundles) {
		return
	}
	if m.launchHookSelected == nil {
		m.launchHookSelected = map[string]bool{}
	}
	id := m.cockpitHookBundles[m.launchHookCursor].ID
	m.launchHookSelected[id] = !m.launchHookSelected[id]
	m.launchHookOverride = true
}

// effectiveLaunchPreset folds the per-launch overrides into a one-off
// LaunchPreset. The role provides defaults; Prompt/Hooks/Perms tabs swap
// individual fields without persisting anything.
func (m model) effectiveLaunchPreset() cockpit.LaunchPreset {
	preset := m.cockpitPresets[m.launchPresetIdx]
	switch {
	case m.launchPromptIdx == launchPromptNone:
		preset.PromptID = ""
		preset.SystemPrompt = ""
	case m.launchPromptIdx >= 0 && m.launchPromptIdx < len(m.cockpitPrompts):
		p := m.cockpitPrompts[m.launchPromptIdx]
		preset.PromptID = p.ID
		preset.SystemPrompt = p.Body
	}
	if m.launchHookOverride {
		var ids []string
		merged := cockpit.HookSpec{}
		for _, b := range m.cockpitHookBundles {
			if !m.launchHookSelected[b.ID] {
				continue
			}
			ids = append(ids, b.ID)
			merged.Prompt = append(merged.Prompt, b.Prompt...)
			merged.PreShell = append(merged.PreShell, b.PreShell...)
			merged.PostShell = append(merged.PostShell, b.PostShell...)
			if merged.Iteration.Mode == "" && b.Iteration.Mode != "" && b.Iteration.Mode != cockpit.IterationOneShot {
				merged.Iteration = b.Iteration
			}
		}
		if merged.Iteration.Mode == "" {
			merged.Iteration.Mode = cockpit.IterationOneShot
		}
		preset.HookBundleIDs = ids
		preset.Hooks = merged
	}
	if v := launchPermsValue(m.launchPermsIdx); v != "" {
		preset.Permissions = v
	}
	return preset
}

func (m model) doLaunch() (tea.Model, tea.Cmd) {
	if len(m.cockpitPresets) == 0 {
		m.statusMsg = "no presets available"
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil
	}
	preset := m.effectiveLaunchPreset()
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

// defaultPresetIndexWithConfig honours config.DefaultPresetID first
// (so users can pin their own pick), then falls back to the seed
// preference order.
func defaultPresetIndexWithConfig(presets []cockpit.LaunchPreset, cfg *config.Config) int {
	if cfg != nil {
		if want := strings.TrimSpace(cfg.DefaultPresetID); want != "" {
			for i, p := range presets {
				if strings.EqualFold(p.ID, want) {
					return i
				}
			}
		}
	}
	preferred := []string{"senior-dev", "scaffold", "bug-fixer"}
	for _, want := range preferred {
		for i, p := range presets {
			if strings.EqualFold(p.ID, want) {
				return i
			}
		}
	}
	return 0
}

func sameExecutor(a, b cockpit.ExecutorSpec) bool {
	return a.Type == b.Type && a.Model == b.Model && a.Cmd == b.Cmd
}
