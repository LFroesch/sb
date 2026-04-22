package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/workmd"
)

// --- Messages ---

type cockpitEventMsg struct{ event cockpit.Event }

type cockpitJobsMsg struct{ jobs []cockpit.Job }

// cockpitWatchCmd pulls events off the manager subscription and emits
// them as tea messages. It also periodically snapshots the job list so
// the UI stays correct even if we miss a status_changed event (shouldn't
// happen, but cheap insurance).
func cockpitWatchCmd(ch <-chan cockpit.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return cockpitEventMsg{event: e}
	}
}

func cockpitRefreshCmd(client cockpit.Client) tea.Cmd {
	return func() tea.Msg {
		return cockpitJobsMsg{jobs: client.ListJobs()}
	}
}

// handleCockpitEvent refreshes the transcript buffer and job snapshot
// whenever an event arrives. It always chains another watch command so
// events keep flowing.
func (m model) handleCockpitEvent(msg cockpitEventMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{cockpitWatchCmd(m.cockpitEvents), cockpitRefreshCmd(m.cockpitClient)}

	switch msg.event.Kind {
	case cockpit.EventStdout:
		if msg.event.JobID == m.attachedJobID {
			if s, ok := msg.event.Payload.(string); ok {
				m.transcriptBuf += s
				m.refreshAttachedViewport(false)
			}
		}
	case cockpit.EventTurnStarted:
		if msg.event.JobID == m.attachedJobID {
			m.transcriptBuf = ""
			m.syncAttachedJobState()
			m.refreshAttachedViewport(true)
		}
	case cockpit.EventTurnFinished:
		if msg.event.JobID == m.attachedJobID {
			m.transcriptBuf = ""
			m.syncAttachedJobState()
			m.refreshAttachedViewport(true)
		}
	case cockpit.EventStatusChanged:
		if msg.event.JobID == m.attachedJobID {
			m.syncAttachedJobState()
			m.refreshAttachedViewport(false)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *model) syncAttachedJobState() {
	if m.cockpitClient == nil || m.attachedJobID == "" {
		return
	}
	j, ok := m.cockpitClient.GetJob(m.attachedJobID)
	if !ok {
		return
	}
	m.attachedTurns = mergeAttachedTurns(m.attachedTurns, j.Turns)
}

func mergeAttachedTurns(local, server []cockpit.Turn) []cockpit.Turn {
	if len(local) == 0 {
		return append([]cockpit.Turn(nil), server...)
	}
	if len(server) >= len(local) {
		return append([]cockpit.Turn(nil), server...)
	}
	if len(server) == 0 || len(server) > len(local) {
		return append([]cockpit.Turn(nil), server...)
	}
	for i := range server {
		if !sameTurnForMerge(local[i], server[i]) {
			return append([]cockpit.Turn(nil), server...)
		}
	}
	return append([]cockpit.Turn(nil), local...)
}

func sameTurnForMerge(a, b cockpit.Turn) bool {
	return a.Role == b.Role &&
		strings.TrimSpace(a.Content) == strings.TrimSpace(b.Content) &&
		a.Note == b.Note &&
		a.ExitCode == b.ExitCode
}

func (m *model) refreshAttachedViewport(forceBottom bool) {
	if m.attachedJobID == "" {
		return
	}
	m.recalcAttachedViewportLayout()
	width := m.attachedTranscriptWidth()
	oldOffset := m.viewport.YOffset
	follow := forceBottom || m.attachedFocus == 1 || m.viewport.AtBottom()
	if m.cockpitClient != nil {
		if j, ok := m.cockpitClient.GetJob(m.attachedJobID); ok {
			if j.Runner == cockpit.RunnerTmux {
				m.viewport.SetContent(renderTmuxLogConversation(j, width))
				if follow {
					m.viewport.GotoBottom()
					return
				}
				m.viewport.SetYOffset(oldOffset)
				return
			}
			running := j.Status == cockpit.StatusRunning
			m.viewport.SetContent(renderChatConversation(m.attachedTurns, m.transcriptBuf, width, running))
			if follow {
				m.viewport.GotoBottom()
				return
			}
			m.viewport.SetYOffset(oldOffset)
			return
		}
	}
}

func (m *model) recalcAttachedViewportLayout() {
	if m.cockpitClient == nil || m.attachedJobID == "" {
		return
	}
	j, ok := m.cockpitClient.GetJob(m.attachedJobID)
	if !ok {
		return
	}
	_, chatWidth, panelHeight := m.attachedLayoutDims()
	isLive := j.Status != cockpit.StatusCompleted &&
		j.Status != cockpit.StatusFailed &&
		j.Status != cockpit.StatusBlocked

	headerLines := 4
	if len(j.Sources) > 0 {
		headerLines++
	}

	m.attachedInput.SetWidth(m.attachedInputWidth())
	inputLines := 2
	if j.Runner == cockpit.RunnerTmux {
		inputLines = 2
	} else if isLive {
		inputLines = 1 + lipgloss.Height(m.attachedInput.View())
	}
	hintLines := 1
	h := panelHeight - headerLines - inputLines - hintLines
	if h < 5 {
		h = 5
	}
	m.viewport.Width = chatWidth - 6
	m.viewport.Height = h
}

func (m model) orderedAgentJobs() []cockpit.Job {
	return orderAgentJobs(m.cockpitJobs)
}

func (m model) filteredAgentJobs() []cockpit.Job {
	jobs := m.orderedAgentJobs()
	if m.agentFilter == "" || m.agentFilter == "all" {
		return jobs
	}
	out := make([]cockpit.Job, 0, len(jobs))
	for _, job := range jobs {
		if agentJobMatchesFilter(job, m.agentFilter) {
			out = append(out, job)
		}
	}
	return out
}

func agentJobMatchesFilter(j cockpit.Job, filter string) bool {
	switch filter {
	case "live":
		return j.Status != cockpit.StatusCompleted
	case "running":
		return j.Status == cockpit.StatusRunning
	case "attention":
		return j.Status == cockpit.StatusNeedsReview || j.Status == cockpit.StatusBlocked || j.Status == cockpit.StatusFailed
	case "done":
		return j.Status == cockpit.StatusCompleted
	default:
		return true
	}
}

func agentFilterOrder() []string {
	return []string{"all", "live", "running", "attention", "done"}
}

func nextAgentFilter(current string, delta int) string {
	order := agentFilterOrder()
	idx := 0
	for i, item := range order {
		if item == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(order)) % len(order)
	return order[idx]
}

func (m model) attachedJobStep(delta int) (cockpit.JobID, bool) {
	jobs := m.orderedAgentJobs()
	if len(jobs) == 0 || m.attachedJobID == "" {
		return "", false
	}
	idx := 0
	found := false
	for i, job := range jobs {
		if job.ID == m.attachedJobID {
			idx = i
			found = true
			break
		}
	}
	if !found {
		return "", false
	}
	next := idx + delta
	if next < 0 || next >= len(jobs) {
		return "", false
	}
	return jobs[next].ID, true
}

// orderAgentJobs groups jobs the same way renderAgentList displays them
// (needs-attn → running → recent) so the cursor index points at the
// visually-selected job. Without this, selection and display drift apart.
func orderAgentJobs(jobs []cockpit.Job) []cockpit.Job {
	var needsAttn, running, recent []cockpit.Job
	for _, j := range jobs {
		switch j.Status {
		case cockpit.StatusRunning, cockpit.StatusPaused:
			running = append(running, j)
		case cockpit.StatusNeedsReview, cockpit.StatusBlocked, cockpit.StatusFailed:
			if j.SyncBackState != cockpit.SyncBackApplied {
				needsAttn = append(needsAttn, j)
			} else {
				recent = append(recent, j)
			}
		default:
			recent = append(recent, j)
		}
	}
	out := make([]cockpit.Job, 0, len(jobs))
	out = append(out, needsAttn...)
	out = append(out, running...)
	out = append(out, recent...)
	return out
}

// --- Agent page update ---

func (m model) updateAgent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cockpitClient == nil {
		if msg.String() == "esc" {
			m.page = pageDashboard
		}
		return m, nil
	}
	switch m.mode {
	case modeAgentPicker:
		return m.updateAgentPicker(msg)
	case modeAgentLaunch:
		return m.updateAgentLaunch(msg)
	case modeAgentAttached:
		return m.updateAgentAttached(msg)
	}
	return m.updateAgentList(msg)
}

func (m model) updateAgentList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	jobs := m.filteredAgentJobs()

	// Resolve pending agent action confirm before anything else consumes the key.
	if m.agentConfirmActive {
		switch msg.String() {
		case "y", "Y", "enter":
			return m.runConfirmedAgentAction()
		default:
			action := m.agentConfirmKind
			m.clearAgentConfirm()
			m.statusMsg = action + " cancelled"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
	}

	switch msg.String() {
	case "esc":
		m.page = pageDashboard
		return m, nil
	case "tab":
		m.agentFilter = nextAgentFilter(m.agentFilter, 1)
		m.agentCursor = 0
		return m, nil
	case "shift+tab":
		m.agentFilter = nextAgentFilter(m.agentFilter, -1)
		m.agentCursor = 0
		return m, nil
	case "1":
		m.agentFilter = "all"
		m.agentCursor = 0
		return m, nil
	case "2":
		m.agentFilter = "live"
		m.agentCursor = 0
		return m, nil
	case "3":
		m.agentFilter = "running"
		m.agentCursor = 0
		return m, nil
	case "4":
		m.agentFilter = "attention"
		m.agentCursor = 0
		return m, nil
	case "5":
		m.agentFilter = "done"
		m.agentCursor = 0
		return m, nil
	case "p":
		path, err := m.createPresetTemplate()
		if err != nil {
			m.statusMsg = "new preset: " + err.Error()
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, nil
		}
		m.statusMsg = "opened new preset template"
		m.statusExpiry = time.Now().Add(3 * time.Second)
		m.reloadCockpitCatalogs()
		return m, openInEditor(path)
	case "P":
		return m, openInEditor(m.cockpitPaths.PresetsDir)
	case "v":
		path, err := m.createProviderTemplate()
		if err != nil {
			m.statusMsg = "new provider: " + err.Error()
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, nil
		}
		m.statusMsg = "opened new provider template"
		m.statusExpiry = time.Now().Add(3 * time.Second)
		m.reloadCockpitCatalogs()
		return m, openInEditor(path)
	case "V":
		return m, openInEditor(m.cockpitPaths.ProvidersDir)
	case "x":
		if m.cockpitDetachQuit {
			return m, m.quitCmd()
		}
	case "n":
		// n = new job sourced from a WORK.md task bullet
		m.mode = modeAgentPicker
		m.pickerFile = ""
		m.pickerItems = nil
		m.pickerSelected = map[int]bool{}
		m.agentCursor = 0
		return m, nil
	case "N":
		// N = freeform chat: skip the task picker and jump straight to
		// launch with no Sources. The brief textarea carries the prompt.
		m.launchSources = nil
		m.launchRepo = m.defaultLaunchRepo()
		m.launchBrief.Reset()
		m.launchBrief.SetWidth(m.width - 6)
		m.launchBrief.SetHeight(6)
		m.launchBrief.Focus()
		m.launchFocus = 2
		m.launchPresetIdx = 0
		m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, 0, m.cockpitProviders)
		m.mode = modeAgentLaunch
		return m, m.launchBrief.Cursor.BlinkCmd()
	case "j", "down":
		if m.agentCursor < len(jobs)-1 {
			m.agentCursor++
		}
	case "k", "up":
		if m.agentCursor > 0 {
			m.agentCursor--
		}
	case "g":
		m.agentCursor = 0
	case "G":
		if len(jobs) > 0 {
			m.agentCursor = len(jobs) - 1
		}
	case "enter":
		if m.agentCursor < len(jobs) {
			return m.openAgentJob(jobs[m.agentCursor].ID, false)
		}
	case "i":
		if m.agentCursor < len(jobs) {
			return m.openAgentJob(jobs[m.agentCursor].ID, true)
		}
	case "s":
		if m.agentCursor < len(jobs) {
			if err := m.cockpitClient.StopJob(jobs[m.agentCursor].ID); err != nil {
				m.statusMsg = "stop: " + err.Error()
			} else {
				m.statusMsg = "stop requested"
			}
			m.statusExpiry = time.Now().Add(2 * time.Second)
		}
	case "d":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			m.armAgentConfirm("delete", j.ID)
			m.statusMsg = "delete " + string(j.ID) + "? y/n"
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
	case "a":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			m.armAgentConfirm("approve", j.ID)
			m.statusMsg = "approve " + string(j.ID) + " and sync back source lines? y/n"
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
	case "r":
		if m.agentCursor < len(jobs) {
			_, err := m.cockpitClient.RetryJob(jobs[m.agentCursor].ID, m.cockpitPresets)
			if err != nil {
				m.statusMsg = "retry: " + err.Error()
				m.statusExpiry = time.Now().Add(3 * time.Second)
			}
			return m, cockpitRefreshCmd(m.cockpitClient)
		}
	}
	return m, nil
}

func (m *model) reloadCockpitCatalogs() {
	if presets, err := cockpit.LoadPresets(m.cockpitPaths.PresetsDir); err == nil {
		m.cockpitPresets = presets
	}
	if providers, err := cockpit.LoadProviders(m.cockpitPaths.ProvidersDir); err == nil {
		m.cockpitProviders = providers
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

func (m *model) armAgentConfirm(kind string, id cockpit.JobID) {
	m.agentConfirmActive = true
	m.agentConfirmKind = kind
	m.agentConfirmTarget = id
}

func (m *model) clearAgentConfirm() {
	m.agentConfirmActive = false
	m.agentConfirmKind = ""
	m.agentConfirmTarget = ""
}

func (m model) runConfirmedAgentAction() (tea.Model, tea.Cmd) {
	kind := m.agentConfirmKind
	id := m.agentConfirmTarget
	m.clearAgentConfirm()

	switch kind {
	case "delete":
		if err := m.cockpitClient.DeleteJob(id); err != nil {
			m.statusMsg = "delete: " + err.Error()
		} else {
			m.statusMsg = "job deleted"
			if m.agentCursor > 0 {
				jobs := orderAgentJobs(m.cockpitJobs)
				if m.agentCursor >= len(jobs)-1 {
					m.agentCursor--
				}
			}
			if m.mode == modeAgentAttached && m.attachedJobID == id {
				m.mode = modeAgentList
				m.attachedJobID = ""
				m.attachedInput.Blur()
			}
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, cockpitRefreshCmd(m.cockpitClient)
	case "approve":
		j, ok := m.cockpitClient.GetJob(id)
		if !ok {
			m.statusMsg = "approve: job not found"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, cockpitRefreshCmd(m.cockpitClient)
		}
		devlog := devlogPathForJob(j, m.projects)
		if err := m.cockpitClient.ApproveJob(j.ID, devlog); err != nil {
			m.statusMsg = "approve: " + err.Error()
		} else {
			m.statusMsg = "approved; synced back"
			m.projects = m.rediscover()
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, cockpitRefreshCmd(m.cockpitClient)
	default:
		return m, nil
	}
}

func (m model) updateAgentPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Step 1: pick a file
	if m.pickerFile == "" {
		switch msg.String() {
		case "esc":
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
	case "esc":
		m.pickerFile = ""
		m.agentCursor = 0
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
		m.launchSources = sources
		m.launchRepo = m.pickerRepo
		m.launchBrief.Reset()
		m.launchBrief.SetWidth(m.width - 6)
		m.launchBrief.SetHeight(6)
		m.launchBrief.Blur()
		m.launchFocus = 0
		m.launchPresetIdx = 0
		m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, 0, m.cockpitProviders)
		m.mode = modeAgentLaunch
		return m, nil
	}
	return m, nil
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
	case "tab":
		m.launchFocus = (m.launchFocus + 1) % 3
		if m.launchFocus == 2 {
			m.launchBrief.Focus()
			return m, m.launchBrief.Cursor.BlinkCmd()
		}
		m.launchBrief.Blur()
		return m, nil
	case "shift+tab":
		m.launchFocus = (m.launchFocus + 2) % 3
		if m.launchFocus == 2 {
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
		if m.launchFocus != 2 {
			return m.doLaunch()
		}
	}

	// Navigation is only applied when the brief textarea is not focused.
	if m.launchFocus == 2 {
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
			if m.launchProviderIdx < len(m.cockpitProviders)-1 {
				m.launchProviderIdx++
			}
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
		}
	}
	return m, nil
}

// doLaunch fires the current launch-modal selection (preset + optional
// provider override + freeform brief). Extracted so both "enter" (when
// the brief isn't focused) and "ctrl+enter" / "ctrl+j" call the same
// path.
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
		Preset:   preset,
		Sources:  m.launchSources,
		Repo:     repo,
		Freeform: m.launchBrief.Value(),
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
	m.statusMsg = "launched " + preset.Name
	m.statusExpiry = time.Now().Add(3 * time.Second)
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

// updateAgentAttached splits into two focus modes to avoid the chat
// textarea swallowing — or worse, the shortcut keys swallowing — every
// normal letter the user tries to type. Transcript-focus owns shortcuts
// and scroll; input-focus owns typing and only intercepts the send +
// focus-swap keys.
func (m model) updateAgentAttached(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Input mode: type freely; only a tiny key set is intercepted.
	if m.attachedFocus == 1 {
		switch msg.String() {
		case "esc", "tab":
			m.attachedFocus = 0
			m.attachedInput.Blur()
			return m, nil
		case "x":
			if m.cockpitDetachQuit {
				return m, m.quitCmd()
			}
			return m, nil
		case "enter", "alt+enter", "ctrl+enter":
			body := strings.TrimSpace(m.attachedInput.Value())
			data := body + "\n"
			if err := m.cockpitClient.SendInput(m.attachedJobID, []byte(data)); err != nil {
				m.statusMsg = "send: " + err.Error()
				m.statusExpiry = time.Now().Add(2 * time.Second)
			} else {
				m.transcriptBuf = ""
				m.attachedTurns = append(m.attachedTurns, cockpit.Turn{
					Role:      cockpit.TurnUser,
					Content:   body,
					StartedAt: time.Now(),
				})
				m.attachedInput.Reset()
				m.attachedFocus = 0
				m.attachedInput.Blur()
				m.refreshAttachedViewport(true)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.attachedInput, cmd = m.attachedInput.Update(msg)
		return m, cmd
	}

	// Transcript-focus: shortcuts + scroll.
	switch msg.String() {
	case "esc":
		m.mode = modeAgentList
		m.attachedInput.Blur()
		return m, nil
	case "x":
		if m.cockpitDetachQuit {
			return m, m.quitCmd()
		}
		return m, nil
	case "tab", "i":
		j, ok := m.cockpitClient.GetJob(m.attachedJobID)
		if ok && j.Runner == cockpit.RunnerTmux {
			m.statusMsg = "tmux jobs attach natively while live; this panel is for log/review"
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, nil
		}
		if !ok || j.Status == cockpit.StatusCompleted || j.Status == cockpit.StatusFailed || j.Status == cockpit.StatusBlocked {
			m.statusMsg = "job is finished — no follow-up turns"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
		if j.Status == cockpit.StatusRunning {
			m.statusMsg = "turn in flight — wait for it to finish"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
		m.attachedFocus = 1
		m.attachedInput.Focus()
		return m, m.attachedInput.Cursor.BlinkCmd()
	case "a":
		m.armAgentConfirm("approve", m.attachedJobID)
		m.statusMsg = "approve " + string(m.attachedJobID) + " and sync back source lines? y/n"
		m.statusExpiry = time.Now().Add(10 * time.Second)
		return m, nil
	case "s":
		if err := m.cockpitClient.StopJob(m.attachedJobID); err != nil {
			m.statusMsg = "stop: " + err.Error()
		} else {
			m.statusMsg = "stop requested"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, nil
	case "r":
		_, err := m.cockpitClient.RetryJob(m.attachedJobID, m.cockpitPresets)
		if err != nil {
			m.statusMsg = "retry: " + err.Error()
			m.statusExpiry = time.Now().Add(3 * time.Second)
		}
		return m, cockpitRefreshCmd(m.cockpitClient)
	case "d":
		m.armAgentConfirm("delete", m.attachedJobID)
		m.statusMsg = "delete " + string(m.attachedJobID) + "? y/n"
		m.statusExpiry = time.Now().Add(10 * time.Second)
		return m, nil
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
	case "g":
		m.viewport.GotoTop()
	case "G":
		m.viewport.GotoBottom()
	case "[":
		if id, ok := m.attachedJobStep(-1); ok {
			return m.openAgentJob(id, false)
		}
	case "]":
		if id, ok := m.attachedJobStep(1); ok {
			return m.openAgentJob(id, false)
		}
	}
	return m, nil
}

func (m model) openAgentJob(id cockpit.JobID, preferInput bool) (tea.Model, tea.Cmd) {
	j, ok := m.cockpitClient.GetJob(id)
	if !ok {
		m.statusMsg = "job not found"
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, cockpitRefreshCmd(m.cockpitClient)
	}
	if j.Runner == cockpit.RunnerTmux {
		if j.Status == cockpit.StatusRunning {
			if j.TmuxTarget == "" {
				if strings.TrimSpace(j.Note) != "" {
					m.statusMsg = "tmux launch failed: " + j.Note
				} else {
					m.statusMsg = "tmux window not recorded for this job"
				}
				m.statusExpiry = time.Now().Add(3 * time.Second)
				return m, nil
			}
			if err := attachTmuxLocal(j); err != nil {
				m.statusMsg = "attach tmux: " + err.Error()
				m.statusExpiry = time.Now().Add(3 * time.Second)
				return m, nil
			}
			m.mode = modeAgentList
			for i, job := range m.orderedAgentJobs() {
				if job.ID == id {
					m.agentCursor = i
					break
				}
			}
			m.statusMsg = "attached to tmux job"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
		if j.TmuxTarget == "" && j.LogPath == "" {
			if strings.TrimSpace(j.Note) != "" {
				m.statusMsg = "tmux launch failed: " + j.Note
			} else {
				m.statusMsg = "tmux window not recorded for this job"
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, nil
		}
		return m.attachJob(id, false)
	}
	return m.attachJob(id, preferInput)
}

func attachTmuxLocal(j cockpit.Job) error {
	if j.Runner != cockpit.RunnerTmux || j.TmuxTarget == "" {
		return fmt.Errorf("job %s is not tmux-backed", j.ID)
	}
	alive, err := cockpit.WindowAlive(j.TmuxTarget)
	if err != nil {
		return err
	}
	if !alive {
		return fmt.Errorf("tmux window %s is gone", j.TmuxTarget)
	}
	return cockpit.SelectWindow(j.TmuxTarget)
}

func (m model) attachJob(id cockpit.JobID, preferInput bool) (tea.Model, tea.Cmd) {
	for i, job := range m.orderedAgentJobs() {
		if job.ID == id {
			m.agentCursor = i
			break
		}
	}
	m.attachedJobID = id
	m.transcriptBuf = ""
	m.attachedTurns = nil
	m.attachedInput.Reset()
	m.attachedInput.SetWidth(m.attachedInputWidth())
	m.attachedInput.SetHeight(3)
	m.attachedInput.Blur()
	m.attachedFocus = 0
	if preferInput {
		if j, ok := m.cockpitClient.GetJob(id); ok {
			if j.Status != cockpit.StatusRunning &&
				j.Status != cockpit.StatusCompleted &&
				j.Status != cockpit.StatusFailed &&
				j.Status != cockpit.StatusBlocked {
				m.attachedFocus = 1
				m.attachedInput.Focus()
			}
		}
	}
	m.mode = modeAgentAttached
	m.syncAttachedJobState()
	m.refreshAttachedViewport(true)
	return m, nil
}

func defaultProviderIndex(presets []cockpit.LaunchPreset, presetIdx int, providers []cockpit.ProviderProfile) int {
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

func sameExecutor(a, b cockpit.ExecutorSpec) bool {
	return a.Type == b.Type && a.Model == b.Model && a.Cmd == b.Cmd
}

// devlogPathForJob picks the DEVLOG.md path next to the first source
// file, falling back to <repo>/DEVLOG.md. The projects slice lets us
// look up alternate repo paths by name later; V0 doesn't use it.
func devlogPathForJob(j cockpit.Job, projects []workmd.Project) string {
	_ = projects
	if len(j.Sources) > 0 {
		return filepath.Join(filepath.Dir(j.Sources[0].File), "DEVLOG.md")
	}
	if j.Repo != "" {
		return filepath.Join(j.Repo, "DEVLOG.md")
	}
	return ""
}
