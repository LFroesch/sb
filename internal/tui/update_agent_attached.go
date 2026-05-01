package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
)

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
			m.viewport.SetContent(m.attachedConversationText(j, width))
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
	headerLines := 4
	if panelHeight <= 8 {
		headerLines--
	}
	if len(j.Sources) > 0 {
		headerLines++
	}

	_, _, h := m.attachedPanelHeights(j, panelHeight, headerLines)
	m.viewport.Width = chatWidth - 6
	m.viewport.Height = h
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
	case "esc", "q":
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
	case "s":
		if err := m.cockpitClient.SoftStopJob(m.attachedJobID); err != nil {
			m.statusMsg = "soft stop: " + err.Error()
		} else {
			m.statusMsg = "sent Esc"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, nil
	case "S":
		if err := m.cockpitClient.StopJob(m.attachedJobID); err != nil {
			m.statusMsg = "interrupt: " + err.Error()
		} else {
			m.statusMsg = "sent Ctrl+C"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, nil
	case "c":
		err := m.cockpitClient.ContinueJob(m.attachedJobID)
		if err != nil {
			m.statusMsg = "continue: " + err.Error()
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
		m.statusMsg = "sent continue"
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, nil
	case "R":
		if j, ok := m.cockpitClient.GetJob(m.attachedJobID); ok && j.Status == cockpit.StatusQueued {
			job, err := m.cockpitClient.StartJob(m.attachedJobID)
			if err != nil {
				m.statusMsg = "start: " + err.Error()
				m.statusExpiry = time.Now().Add(3 * time.Second)
				return m, nil
			}
			m.statusMsg = "started " + job.PresetID
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m.openAgentJob(job.ID, true)
		}
		return m.openAgentJob(m.attachedJobID, true)
	case "ctrl+r":
		return m.beginTakeover(m.attachedJobID)
	case "K":
		j, _ := m.cockpitClient.GetJob(m.attachedJobID)
		m.armAgentConfirm("skip", m.attachedJobID)
		m.statusMsg = m.agentSkipPrompt(j)
		m.statusExpiry = time.Now().Add(10 * time.Second)
		return m, nil
	case "C":
		j, _ := m.cockpitClient.GetJob(m.attachedJobID)
		if j.CampaignID == "" || j.QueueTotal <= 1 {
			m.statusMsg = "skip-rest only applies to queued runs"
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, nil
		}
		m.armAgentConfirm("skip_campaign", m.attachedJobID)
		m.statusMsg = m.agentSkipCampaignPrompt(j)
		m.statusExpiry = time.Now().Add(10 * time.Second)
		return m, nil
	case "a":
		if j, ok := m.cockpitClient.GetJob(m.attachedJobID); ok {
			m.armAgentConfirm("approve", m.attachedJobID)
			m.statusMsg = m.agentApprovePrompt(j)
			m.statusExpiry = time.Now().Add(10 * time.Second)
		}
		return m, nil
	case "d":
		j, _ := m.cockpitClient.GetJob(m.attachedJobID)
		m.armAgentConfirm("delete", m.attachedJobID)
		m.statusMsg = m.agentDeletePrompt(j)
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
		if j.Status == cockpit.StatusQueued {
			reason := strings.TrimSpace(j.EligibilityReason)
			switch {
			case reason != "":
				m.statusMsg = "job queued: " + reason
			case j.WaitForForeman:
				m.statusMsg = "job queued: waiting for foreman"
			default:
				m.statusMsg = "job queued: press R to start"
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, tea.Batch(cockpitRefreshCmd(m.cockpitClient), cockpitForemanRefreshCmd(m.cockpitClient))
		}
		if j.TmuxTarget != "" {
			if err := attachTmuxLocal(j); err == nil {
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
		}
		if j.TmuxTarget == "" && j.LogPath == "" {
			if strings.TrimSpace(j.Note) != "" {
				m.statusMsg = "tmux launch failed: " + j.Note
			} else {
				m.statusMsg = "tmux session still initializing"
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, cockpitRefreshCmd(m.cockpitClient)
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
