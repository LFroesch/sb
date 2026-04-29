package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/workmd"
)

// --- Messages ---

type cockpitEventMsg struct{ event cockpit.Event }

type cockpitJobsMsg struct{ jobs []cockpit.Job }

type cockpitForemanMsg struct{ state cockpit.ForemanState }

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

func cockpitForemanRefreshCmd(client cockpit.Client) tea.Cmd {
	return func() tea.Msg {
		return cockpitForemanMsg{state: client.GetForemanState()}
	}
}

// handleCockpitEvent refreshes the transcript buffer and job snapshot
// whenever an event arrives. It always chains another watch command so
// events keep flowing.
func (m model) handleCockpitEvent(msg cockpitEventMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{cockpitWatchCmd(m.cockpitEvents), cockpitRefreshCmd(m.cockpitClient), cockpitForemanRefreshCmd(m.cockpitClient)}

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
	status, _ := jobOperatorStatus(j)
	switch filter {
	case "live":
		return status == "working" || status == "waiting for input" || status == "queued" || status == "waiting for foreman"
	case "running":
		return status == "working"
	case "attention":
		return status == "needs review" || status == "blocked" || status == "failed" || status == "stopped" || status == "closed"
	case "foreman":
		return j.ForemanManaged
	case "done":
		return status == "done" || status == "skipped"
	default:
		return true
	}
}

func agentFilterOrder() []string {
	return []string{"all", "live", "running", "attention", "foreman", "done"}
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

type agentManageFieldSpec struct {
	Key       string
	Label     string
	Group     string
	Multiline bool
	Height    int
}

// orderAgentJobs sorts the operator-facing cockpit order:
// working → waiting for input → needs review → queued → done.
func orderAgentJobs(jobs []cockpit.Job) []cockpit.Job {
	out := append([]cockpit.Job(nil), jobs...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := jobOrderRank(out[i]), jobOrderRank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// --- Agent page update ---

func (m model) updateAgent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cockpitClient == nil {
		if s := msg.String(); s == "esc" || s == "q" {
			m.page = pageDashboard
		}
		return m, nil
	}
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
	switch m.mode {
	case modeAgentPicker:
		return m.updateAgentPicker(msg)
	case modeAgentLaunch:
		return m.updateAgentLaunch(msg)
	case modeAgentAttached:
		return m.updateAgentAttached(msg)
	case modeAgentManage:
		return m.updateAgentManage(msg)
	}
	return m.updateAgentList(msg)
}

func (m model) handleAgentMouseWheel(delta int) tea.Model {
	switch m.mode {
	case modeAgentAttached:
		if delta > 0 {
			m.viewport.LineDown(delta)
		} else {
			m.viewport.LineUp(-delta)
		}
	case modeAgentManage:
		if m.agentManageEditing {
			return m
		}
		if m.agentManageFocus == 0 {
			if delta > 0 && m.agentManageCursor < m.agentManageItemCount()-1 {
				m.agentManageCursor++
				m.agentManageListOffset++
			}
			if delta < 0 && m.agentManageCursor > 0 {
				m.agentManageCursor--
				m.agentManageListOffset--
			}
		} else {
			specs := m.agentManageFieldSpecs()
			if delta > 0 && m.agentManageField < len(specs)-1 {
				m.agentManageField++
				m.agentManageDetailOffset++
			}
			if delta < 0 && m.agentManageField > 0 {
				m.agentManageField--
				m.agentManageDetailOffset--
			}
		}
		m.clampAgentManageOffsets()
	case modeAgentPicker:
		if m.pickerFile == "" {
			if delta > 0 && m.agentCursor < len(m.projects)-1 {
				m.agentCursor++
			}
			if delta < 0 && m.agentCursor > 0 {
				m.agentCursor--
			}
			return m
		}
		if delta > 0 && m.agentCursor < len(m.pickerItems)-1 {
			m.agentCursor++
		}
		if delta < 0 && m.agentCursor > 0 {
			m.agentCursor--
		}
	case modeAgentLaunch:
		m.normalizeLaunchFocus()
		switch m.launchFocus {
		case 0:
			if delta > 0 && m.launchPresetIdx < len(m.cockpitPresets)-1 {
				m.launchPresetIdx++
				m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			}
			if delta < 0 && m.launchPresetIdx > 0 {
				m.launchPresetIdx--
				m.launchProviderIdx = defaultProviderIndex(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			}
		case 1:
			providers := providerChoices(m.cockpitPresets, m.launchPresetIdx, m.cockpitProviders)
			if delta > 0 && m.launchProviderIdx < len(providers)-1 {
				m.launchProviderIdx++
			}
			if delta < 0 && m.launchProviderIdx > 0 {
				m.launchProviderIdx--
			}
		case 2:
			if m.launchHasRepoStep() {
				repos := m.launchRepoChoices()
				idx := indexOfLaunchRepo(repos, m.launchRepo)
				if delta > 0 && idx < len(repos)-1 {
					m.launchRepo = repos[idx+1]
				}
				if delta < 0 && idx > 0 {
					m.launchRepo = repos[idx-1]
				}
				return m
			}
			fallthrough
		case 3:
			if m.launchFocus == m.launchReviewFocus() {
				m.launchReviewOffset += delta
				m.clampLaunchReviewOffset()
			}
		case 4:
			m.launchReviewOffset += delta
			m.clampLaunchReviewOffset()
		}
	default:
		jobs := m.filteredAgentJobs()
		if delta > 0 && m.agentCursor < len(jobs)-1 {
			m.agentCursor++
			m.agentDetailOffset = 0
		}
		if delta < 0 && m.agentCursor > 0 {
			m.agentCursor--
			m.agentDetailOffset = 0
		}
	}
	return m
}

func (m model) updateAgentList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	jobs := m.filteredAgentJobs()
	m.agentCursor = clampAgentCursor(m.agentCursor, len(jobs))

	switch msg.String() {
	case "esc", "q":
		m.page = pageDashboard
		return m, nil
	case "f":
		m.agentFilter = nextAgentFilter(m.agentFilter, 1)
		m.agentCursor = 0
		m.agentDetailOffset = 0
		return m, nil
	case "tab":
		m.agentFilter = nextAgentFilter(m.agentFilter, 1)
		m.agentCursor = 0
		m.agentDetailOffset = 0
		return m, nil
	case "shift+tab":
		m.agentFilter = nextAgentFilter(m.agentFilter, -1)
		m.agentCursor = 0
		m.agentDetailOffset = 0
		return m, nil
	case "F":
		state, err := m.cockpitClient.SetForemanEnabled(!m.cockpitForeman.Enabled)
		if err != nil {
			m.statusMsg = "foreman: " + err.Error()
		} else {
			m.cockpitForeman = state
			if state.Enabled {
				m.statusMsg = "Foreman enabled"
			} else {
				m.statusMsg = "Foreman disabled"
			}
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, tea.Batch(cockpitRefreshCmd(m.cockpitClient), cockpitForemanRefreshCmd(m.cockpitClient))
	case "pgup":
		if m.agentDetailOffset > 0 {
			m.agentDetailOffset -= 5
			if m.agentDetailOffset < 0 {
				m.agentDetailOffset = 0
			}
		}
		m.clampAgentDetailOffset()
		return m, nil
	case "pgdown", "pgdn":
		m.agentDetailOffset += 5
		m.clampAgentDetailOffset()
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
	case "m":
		m.mode = modeAgentManage
		m.agentManageKind = "preset"
		m.agentManageFocus = 0
		m.agentManageCursor = 0
		m.agentManageField = 0
		m.agentManageListOffset = 0
		m.agentManageDetailOffset = 0
		m.agentManageEditing = false
		m.agentManageEditor.Blur()
		return m, nil
	case "n":
		m.resetAgentPicker()
		m.resetAgentLaunch()
		m.mode = modeAgentPicker
		return m, nil
	case "N":
		// N = freeform chat: skip the task picker and jump straight to
		// launch with no Sources. The brief textarea carries the prompt.
		m.resetAgentPicker()
		m.resetAgentLaunch()
		m.launchRepo = m.defaultLaunchRepo()
		m.launchBrief.Focus()
		m.launchFocus = 3
		m.mode = modeAgentLaunch
		return m, m.launchBrief.Cursor.BlinkCmd()
	case "j", "down":
		if m.agentCursor < len(jobs)-1 {
			m.agentCursor++
			m.agentDetailOffset = 0
		}
	case "k", "up":
		if m.agentCursor > 0 {
			m.agentCursor--
			m.agentDetailOffset = 0
		}
	case "g":
		m.agentCursor = 0
		m.agentDetailOffset = 0
	case "G":
		if len(jobs) > 0 {
			m.agentCursor = len(jobs) - 1
			m.agentDetailOffset = 0
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
			if err := m.cockpitClient.SoftStopJob(jobs[m.agentCursor].ID); err != nil {
				m.statusMsg = "soft stop: " + err.Error()
			} else {
				m.statusMsg = "sent Esc"
			}
			m.statusExpiry = time.Now().Add(2 * time.Second)
		}
	case "S":
		if m.agentCursor < len(jobs) {
			if err := m.cockpitClient.StopJob(jobs[m.agentCursor].ID); err != nil {
				m.statusMsg = "interrupt: " + err.Error()
			} else {
				m.statusMsg = "sent Ctrl+C"
			}
			m.statusExpiry = time.Now().Add(2 * time.Second)
		}
	case "c":
		if m.agentCursor < len(jobs) {
			if err := m.cockpitClient.ContinueJob(jobs[m.agentCursor].ID); err != nil {
				m.statusMsg = "continue: " + err.Error()
			} else {
				m.statusMsg = "sent continue"
			}
			m.statusExpiry = time.Now().Add(2 * time.Second)
		}
	case "R":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			if j.Status == cockpit.StatusQueued {
				job, err := m.cockpitClient.StartJob(j.ID)
				if err != nil {
					m.statusMsg = "start: " + err.Error()
					m.statusExpiry = time.Now().Add(3 * time.Second)
					return m, nil
				}
				m.statusMsg = "started " + job.PresetID
				m.statusExpiry = time.Now().Add(2 * time.Second)
				return m.openAgentJob(job.ID, true)
			}
			return m.openAgentJob(j.ID, true)
		}
	case "K":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			m.armAgentConfirm("skip", j.ID)
			m.statusMsg = m.agentSkipPrompt(j)
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
	case "C":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			if j.CampaignID == "" || j.QueueTotal <= 1 {
				m.statusMsg = "skip-rest only applies to queued runs"
				m.statusExpiry = time.Now().Add(3 * time.Second)
				return m, nil
			}
			m.armAgentConfirm("skip_campaign", j.ID)
			m.statusMsg = m.agentSkipCampaignPrompt(j)
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
	case "a":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			m.armAgentConfirm("approve", j.ID)
			m.statusMsg = m.agentApprovePrompt(j)
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
	case "d":
		if m.agentCursor < len(jobs) {
			j := jobs[m.agentCursor]
			m.armAgentConfirm("delete", j.ID)
			m.statusMsg = m.agentDeletePrompt(j)
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
	}
	return m, nil
}

func (m *model) openCurrentProjectPicker() bool {
	var idx = -1
	if m.selected >= 0 && m.selected < len(m.projects) {
		idx = m.selected
	} else if m.cursor >= 0 && m.cursor < len(m.projects) {
		idx = m.cursor
	}
	if idx < 0 || idx >= len(m.projects) {
		return false
	}
	p := m.projects[idx]
	items, err := cockpit.ReadItems(p.Path)
	if err != nil {
		m.statusMsg = "read items: " + err.Error()
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return false
	}
	m.mode = modeAgentPicker
	m.pickerFile = p.Path
	m.pickerItems = items
	m.pickerSelected = map[int]bool{}
	m.pickerProject = p.Name
	m.pickerRepo = p.Dir
	m.agentCursor = 0
	if len(items) == 0 {
		m.statusMsg = "current project has no task bullets; pick another file"
		m.statusExpiry = time.Now().Add(3 * time.Second)
	}
	return true
}

func (m *model) reloadCockpitCatalogs() {
	if presets, err := cockpit.LoadPresets(m.cockpitPaths.PresetsDir); err == nil {
		m.cockpitPresets = presets
	}
	if providers, err := cockpit.LoadProviders(m.cockpitPaths.ProvidersDir); err == nil {
		m.cockpitProviders = providers
	}
}

func clampAgentCursor(cursor, length int) int {
	if length <= 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	if cursor < 0 {
		return 0
	}
	return cursor
}

func (m *model) armAgentConfirm(kind string, id cockpit.JobID) {
	m.agentConfirmActive = true
	m.agentConfirmKind = kind
	m.agentConfirmTarget = id
}

func (m model) agentDeletePrompt(j cockpit.Job) string {
	if j.Status == cockpit.StatusRunning {
		return "delete " + string(j.ID) + "? y/n (will interrupt the running job and remove it)"
	}
	return "delete " + string(j.ID) + "? y/n"
}

func (m model) agentSkipPrompt(j cockpit.Job) string {
	if j.QueueTotal > 1 {
		return "skip " + string(j.ID) + "? y/n (mark skipped, keep history, and let foreman continue)"
	}
	return "skip " + string(j.ID) + "? y/n (mark skipped and keep history)"
}

func (m model) agentSkipCampaignPrompt(j cockpit.Job) string {
	if j.CampaignID == "" || j.QueueTotal <= 1 {
		return "skip rest unavailable for " + string(j.ID) + " (not in a queued run sequence)"
	}
	remaining := j.QueueTotal - j.QueueIndex
	if remaining < 1 {
		remaining = 1
	}
	return "skip rest of queue for " + string(j.ID) + "? y/n (mark this item + " + fmt.Sprintf("%d", remaining-1) + " later item(s) skipped)"
}

func (m model) agentApprovePrompt(j cockpit.Job) string {
	if len(j.Sources) == 0 {
		return "accept " + string(j.ID) + "? y/n (mark complete; no WORK.md sync-back)"
	}
	devlog := devlogPathForJob(j, m.projects)
	if strings.TrimSpace(devlog) == "" {
		return "accept " + string(j.ID) + "? y/n (sync back source tasks)"
	}
	return "accept " + string(j.ID) + "? y/n (sync WORK.md + " + filepath.Base(devlog) + ")"
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
	case "skip":
		if err := m.cockpitClient.SkipJob(id); err != nil {
			m.statusMsg = "skip: " + err.Error()
		} else {
			m.statusMsg = "job skipped"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, cockpitRefreshCmd(m.cockpitClient)
	case "skip_campaign":
		if err := m.cockpitClient.SkipCampaign(id); err != nil {
			m.statusMsg = "skip queue: " + err.Error()
		} else {
			m.statusMsg = "rest of queue skipped"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, cockpitRefreshCmd(m.cockpitClient)
	case "approve":
		j, ok := m.cockpitClient.GetJob(id)
		if !ok {
			m.statusMsg = "accept: job not found"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, cockpitRefreshCmd(m.cockpitClient)
		}
		devlog := devlogPathForJob(j, m.projects)
		if err := m.cockpitClient.ApproveJob(j.ID, devlog); err != nil {
			m.statusMsg = "accept: " + err.Error()
		} else {
			m.statusMsg = "accepted; synced back"
			m.projects = m.rediscover()
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, cockpitRefreshCmd(m.cockpitClient)
	default:
		return m, nil
	}
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
