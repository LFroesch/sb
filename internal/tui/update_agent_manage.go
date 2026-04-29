package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
)

func presetManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity"},
		{Key: "launch_mode", Label: "Launch mode", Group: "Identity"},
		{Key: "permissions", Label: "Permissions", Group: "Identity"},
		{Key: "system_prompt", Label: "System prompt", Group: "Prompting", Multiline: true, Height: 8},
		{Key: "executor.type", Label: "Suggested engine type", Group: "Suggested Engine"},
		{Key: "executor.runner", Label: "Suggested runner", Group: "Suggested Engine"},
		{Key: "executor.model", Label: "Suggested model", Group: "Suggested Engine"},
		{Key: "executor.cmd", Label: "Suggested command", Group: "Suggested Engine"},
		{Key: "executor.args", Label: "Suggested args (one per line)", Group: "Suggested Engine", Multiline: true, Height: 5},
		{Key: "hooks.prompt", Label: "Prompt hooks (JSON)", Group: "Hooks", Multiline: true, Height: 8},
		{Key: "hooks.pre_shell", Label: "Pre hooks (JSON)", Group: "Hooks", Multiline: true, Height: 8},
		{Key: "hooks.post_shell", Label: "Post hooks (JSON)", Group: "Hooks", Multiline: true, Height: 8},
		{Key: "hooks.iteration.mode", Label: "Iteration mode", Group: "Iteration"},
		{Key: "hooks.iteration.n", Label: "Iteration N (loop_n)", Group: "Iteration"},
		{Key: "hooks.iteration.signal", Label: "Iteration signal", Group: "Iteration"},
		{Key: "hooks.iteration.on_file", Label: "Iteration on_file", Group: "Iteration"},
		{Key: "role", Label: "Persona / role (optional)", Group: "Advanced"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced"},
	}
}

func providerManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity"},
		{Key: "executor.type", Label: "Engine type", Group: "Engine"},
		{Key: "executor.runner", Label: "Runner", Group: "Engine"},
		{Key: "executor.model", Label: "Model", Group: "Engine"},
		{Key: "executor.cmd", Label: "Command", Group: "Engine"},
		{Key: "executor.args", Label: "Args (one per line)", Group: "Engine", Multiline: true, Height: 5},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced"},
	}
}

func formatJSONValue(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func splitLinesValue(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func slugifyManagedID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

func canonicalizeProviderProfile(p cockpit.ProviderProfile) cockpit.ProviderProfile {
	p.Name = strings.TrimSpace(p.Name)
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		p.ID = slugifyManagedID(p.Name)
	}
	return p
}

func canonicalizeLaunchPreset(p cockpit.LaunchPreset) cockpit.LaunchPreset {
	p.Name = strings.TrimSpace(p.Name)
	p.ID = strings.TrimSpace(p.ID)
	p.Role = strings.TrimSpace(p.Role)
	if p.ID == "" {
		p.ID = slugifyManagedID(p.Name)
	}
	return p
}

func (m model) agentManageFieldSpecs() []agentManageFieldSpec {
	if m.agentManageKind == "provider" {
		return providerManageFields()
	}
	return presetManageFields()
}

func (m model) agentManageItemCount() int {
	if m.agentManageKind == "provider" {
		return len(m.cockpitProviders)
	}
	return len(m.cockpitPresets)
}

func (m model) agentManageItemLabel(idx int) string {
	if m.agentManageKind == "provider" {
		if idx < 0 || idx >= len(m.cockpitProviders) {
			return ""
		}
		return m.cockpitProviders[idx].Name
	}
	if idx < 0 || idx >= len(m.cockpitPresets) {
		return ""
	}
	return m.cockpitPresets[idx].Name
}

func (m model) agentManageFieldValue(idx, field int) string {
	specs := m.agentManageFieldSpecs()
	if idx < 0 || field < 0 || field >= len(specs) {
		return ""
	}
	key := specs[field].Key
	if m.agentManageKind == "provider" {
		if idx >= len(m.cockpitProviders) {
			return ""
		}
		p := m.cockpitProviders[idx]
		switch key {
		case "id":
			return p.ID
		case "name":
			return p.Name
		case "executor.type":
			return p.Executor.Type
		case "executor.runner":
			return p.Executor.Runner
		case "executor.model":
			return p.Executor.Model
		case "executor.cmd":
			return p.Executor.Cmd
		case "executor.args":
			return strings.Join(p.Executor.Args, "\n")
		}
		return ""
	}
	if idx >= len(m.cockpitPresets) {
		return ""
	}
	p := m.cockpitPresets[idx]
	switch key {
	case "id":
		return p.ID
	case "name":
		return p.Name
	case "role":
		return p.Role
	case "launch_mode":
		if p.LaunchMode == "" {
			return cockpit.LaunchModeSingleJob
		}
		return p.LaunchMode
	case "system_prompt":
		return p.SystemPrompt
	case "executor.type":
		return p.Executor.Type
	case "executor.runner":
		return p.Executor.Runner
	case "executor.model":
		return p.Executor.Model
	case "executor.cmd":
		return p.Executor.Cmd
	case "executor.args":
		return strings.Join(p.Executor.Args, "\n")
	case "permissions":
		return p.Permissions
	case "hooks.prompt":
		return formatJSONValue(p.Hooks.Prompt)
	case "hooks.pre_shell":
		return formatJSONValue(p.Hooks.PreShell)
	case "hooks.post_shell":
		return formatJSONValue(p.Hooks.PostShell)
	case "hooks.iteration.mode":
		return p.Hooks.Iteration.Mode
	case "hooks.iteration.n":
		if p.Hooks.Iteration.N == 0 {
			return ""
		}
		return strconv.Itoa(p.Hooks.Iteration.N)
	case "hooks.iteration.signal":
		return p.Hooks.Iteration.Signal
	case "hooks.iteration.on_file":
		return p.Hooks.Iteration.OnFile
	}
	return ""
}

func (m *model) setAgentManageFieldValue(idx, field int, raw string) error {
	specs := m.agentManageFieldSpecs()
	if idx < 0 || field < 0 || field >= len(specs) {
		return fmt.Errorf("invalid field")
	}
	key := specs[field].Key
	value := strings.TrimSpace(raw)
	if m.agentManageKind == "provider" {
		if idx >= len(m.cockpitProviders) {
			return fmt.Errorf("invalid provider")
		}
		p := m.cockpitProviders[idx]
		switch key {
		case "id":
			p.ID = value
		case "name":
			p.Name = value
		case "executor.type":
			p.Executor.Type = value
		case "executor.runner":
			p.Executor.Runner = value
		case "executor.model":
			p.Executor.Model = value
		case "executor.cmd":
			p.Executor.Cmd = value
		case "executor.args":
			p.Executor.Args = splitLinesValue(raw)
		}
		p = canonicalizeProviderProfile(p)
		if err := validateProviderProfile(p); err != nil {
			return err
		}
		m.cockpitProviders[idx] = p
		return cockpit.SaveProvider(m.cockpitPaths.ProvidersDir, p)
	}
	if idx >= len(m.cockpitPresets) {
		return fmt.Errorf("invalid preset")
	}
	p := m.cockpitPresets[idx]
	switch key {
	case "id":
		p.ID = value
	case "name":
		p.Name = value
	case "role":
		p.Role = value
	case "launch_mode":
		switch value {
		case "", cockpit.LaunchModeSingleJob, cockpit.LaunchModeTaskQueueSequence:
			if value == "" {
				p.LaunchMode = cockpit.LaunchModeSingleJob
			} else {
				p.LaunchMode = value
			}
		default:
			return fmt.Errorf("launch mode must be single_job|task_queue_sequence")
		}
	case "system_prompt":
		p.SystemPrompt = strings.TrimSpace(raw)
	case "executor.type":
		p.Executor.Type = value
	case "executor.runner":
		p.Executor.Runner = value
	case "executor.model":
		p.Executor.Model = value
	case "executor.cmd":
		p.Executor.Cmd = value
	case "executor.args":
		p.Executor.Args = splitLinesValue(raw)
	case "permissions":
		p.Permissions = value
	case "hooks.prompt":
		var hooks []cockpit.PromptHook
		if value != "" {
			if err := json.Unmarshal([]byte(raw), &hooks); err != nil {
				return fmt.Errorf("prompt hooks JSON: %w", err)
			}
		}
		p.Hooks.Prompt = hooks
	case "hooks.pre_shell":
		var hooks []cockpit.ShellHook
		if value != "" {
			if err := json.Unmarshal([]byte(raw), &hooks); err != nil {
				return fmt.Errorf("pre hooks JSON: %w", err)
			}
		}
		p.Hooks.PreShell = hooks
	case "hooks.post_shell":
		var hooks []cockpit.ShellHook
		if value != "" {
			if err := json.Unmarshal([]byte(raw), &hooks); err != nil {
				return fmt.Errorf("post hooks JSON: %w", err)
			}
		}
		p.Hooks.PostShell = hooks
	case "hooks.iteration.mode":
		switch value {
		case "", cockpit.IterationOneShot, "loop_n", "until_signal":
			if value == "" {
				p.Hooks.Iteration.Mode = cockpit.IterationOneShot
			} else {
				p.Hooks.Iteration.Mode = value
			}
		default:
			return fmt.Errorf("iteration mode must be one_shot|loop_n|until_signal")
		}
	case "hooks.iteration.n":
		if value == "" {
			p.Hooks.Iteration.N = 0
		} else {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return fmt.Errorf("iteration N must be a non-negative integer")
			}
			p.Hooks.Iteration.N = n
		}
	case "hooks.iteration.signal":
		p.Hooks.Iteration.Signal = value
	case "hooks.iteration.on_file":
		p.Hooks.Iteration.OnFile = value
	}
	if p.Hooks.Iteration.Mode == "" {
		p.Hooks.Iteration.Mode = cockpit.IterationOneShot
	}
	if p.LaunchMode == "" {
		p.LaunchMode = cockpit.LaunchModeSingleJob
	}
	p = canonicalizeLaunchPreset(p)
	if err := validateLaunchPreset(p); err != nil {
		return err
	}
	m.cockpitPresets[idx] = p
	return cockpit.SavePreset(m.cockpitPaths.PresetsDir, p)
}

func validateProviderProfile(p cockpit.ProviderProfile) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("provider ID is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("provider name is required")
	}
	if strings.TrimSpace(p.Executor.Type) == "" {
		return fmt.Errorf("executor type is required")
	}
	return nil
}

func validateLaunchPreset(p cockpit.LaunchPreset) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("preset ID is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("preset name is required")
	}
	if strings.TrimSpace(p.Executor.Type) == "" {
		return fmt.Errorf("executor type is required")
	}
	switch p.LaunchMode {
	case "", cockpit.LaunchModeSingleJob, cockpit.LaunchModeTaskQueueSequence:
	default:
		return fmt.Errorf("launch mode must be single_job or task_queue_sequence")
	}
	return nil
}

func (m *model) beginAgentManageEdit() tea.Cmd {
	specs := m.agentManageFieldSpecs()
	if m.agentManageField < 0 || m.agentManageField >= len(specs) {
		return nil
	}
	spec := specs[m.agentManageField]
	m.agentManageEditing = true
	m.agentManageEditor.Reset()
	m.agentManageEditor.SetValue(m.agentManageFieldValue(m.agentManageCursor, m.agentManageField))
	m.agentManageEditor.SetWidth(maxInt(32, m.width/2))
	height := spec.Height
	if height < 3 {
		height = 3
	}
	m.agentManageEditor.SetHeight(height)
	m.agentManageEditor.Focus()
	return m.agentManageEditor.Cursor.BlinkCmd()
}

func (m *model) clampAgentManageOffsets() {
	count := m.agentManageItemCount()
	if count <= 0 {
		m.agentManageListOffset = 0
	} else {
		_, innerHeight := m.agentManagePanelHeights()
		m.agentManageListOffset = clampScrollOffset(m.agentManageListOffset, count, innerHeight)
	}
	specs := m.agentManageFieldSpecs()
	if len(specs) <= 0 {
		m.agentManageDetailOffset = 0
	} else {
		m.agentManageDetailOffset = clampScrollOffset(m.agentManageDetailOffset, len(specs), m.agentManageDetailVisibleRows())
	}
	if m.agentManageListOffset < 0 {
		m.agentManageListOffset = 0
	}
	if m.agentManageDetailOffset < 0 {
		m.agentManageDetailOffset = 0
	}
}

func (m *model) endAgentManageEdit(save bool) {
	if save {
		if err := m.setAgentManageFieldValue(m.agentManageCursor, m.agentManageField, m.agentManageEditor.Value()); err != nil {
			m.statusMsg = "save field: " + err.Error()
			m.statusExpiry = time.Now().Add(4 * time.Second)
		} else {
			m.statusMsg = "saved " + m.agentManageItemLabel(m.agentManageCursor)
			m.statusExpiry = time.Now().Add(2 * time.Second)
			m.reloadCockpitCatalogs()
		}
	}
	m.agentManageEditing = false
	m.agentManageEditor.Blur()
}

func (m *model) createManagedAgentItem() error {
	if m.agentManageKind == "provider" {
		p := cockpit.ProviderProfile{
			Name: "New engine",
			Executor: cockpit.ExecutorSpec{
				Type: "codex",
			},
		}
		p = canonicalizeProviderProfile(p)
		if err := cockpit.SaveProvider(m.cockpitPaths.ProvidersDir, p); err != nil {
			return err
		}
		m.reloadCockpitCatalogs()
		m.agentManageCursor = len(m.cockpitProviders) - 1
		m.agentManageField = 0
		m.agentManageListOffset = m.agentManageCursor
		m.agentManageDetailOffset = 0
		return nil
	}
	p := cockpit.LaunchPreset{
		Name:       "New role",
		LaunchMode: cockpit.LaunchModeSingleJob,
		Executor: cockpit.ExecutorSpec{
			Type: "codex",
		},
		Hooks: cockpit.HookSpec{
			Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot},
		},
		Permissions: "scoped-write",
	}
	p = canonicalizeLaunchPreset(p)
	if err := cockpit.SavePreset(m.cockpitPaths.PresetsDir, p); err != nil {
		return err
	}
	m.reloadCockpitCatalogs()
	m.agentManageCursor = len(m.cockpitPresets) - 1
	m.agentManageField = 0
	m.agentManageListOffset = m.agentManageCursor
	m.agentManageDetailOffset = 0
	return nil
}

func (m model) currentManagedItemID() string {
	idx := m.agentManageCursor
	if m.agentManageKind == "provider" {
		if idx < 0 || idx >= len(m.cockpitProviders) {
			return ""
		}
		return m.cockpitProviders[idx].ID
	}
	if idx < 0 || idx >= len(m.cockpitPresets) {
		return ""
	}
	return m.cockpitPresets[idx].ID
}

func (m *model) deleteManagedAgentItem(id string) error {
	if id == "" {
		return fmt.Errorf("missing id")
	}
	if m.agentManageKind == "provider" {
		if err := cockpit.DeleteProvider(m.cockpitPaths.ProvidersDir, id); err != nil {
			return err
		}
	} else {
		if err := cockpit.DeletePreset(m.cockpitPaths.PresetsDir, id); err != nil {
			return err
		}
	}
	m.reloadCockpitCatalogs()
	if m.agentManageCursor >= m.agentManageItemCount() {
		m.agentManageCursor = m.agentManageItemCount() - 1
	}
	if m.agentManageCursor < 0 {
		m.agentManageCursor = 0
	}
	m.agentManageField = 0
	m.clampAgentManageOffsets()
	return nil
}

func (m *model) duplicateManagedAgentItem() error {
	idx := m.agentManageCursor
	stamp := time.Now().Format("150405")
	if m.agentManageKind == "provider" {
		if idx < 0 || idx >= len(m.cockpitProviders) {
			return fmt.Errorf("no provider selected")
		}
		src := m.cockpitProviders[idx]
		dup := src
		dup.ID = src.ID + "-copy-" + stamp
		dup.Name = src.Name + " (copy)"
		if err := cockpit.SaveProvider(m.cockpitPaths.ProvidersDir, dup); err != nil {
			return err
		}
		m.reloadCockpitCatalogs()
		for i, p := range m.cockpitProviders {
			if p.ID == dup.ID {
				m.agentManageCursor = i
				break
			}
		}
		m.agentManageField = 0
		m.agentManageListOffset = m.agentManageCursor
		m.agentManageDetailOffset = 0
		return nil
	}
	if idx < 0 || idx >= len(m.cockpitPresets) {
		return fmt.Errorf("no preset selected")
	}
	src := m.cockpitPresets[idx]
	dup := src
	dup.ID = src.ID + "-copy-" + stamp
	dup.Name = src.Name + " (copy)"
	if err := cockpit.SavePreset(m.cockpitPaths.PresetsDir, dup); err != nil {
		return err
	}
	m.reloadCockpitCatalogs()
	for i, p := range m.cockpitPresets {
		if p.ID == dup.ID {
			m.agentManageCursor = i
			break
		}
	}
	m.agentManageField = 0
	m.agentManageListOffset = m.agentManageCursor
	m.agentManageDetailOffset = 0
	return nil
}

func (m model) updateAgentManage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.agentManageEditing {
		switch msg.String() {
		case "esc":
			m.endAgentManageEdit(false)
			return m, nil
		case "ctrl+s":
			m.endAgentManageEdit(true)
			return m, nil
		}
		var cmd tea.Cmd
		m.agentManageEditor, cmd = m.agentManageEditor.Update(msg)
		return m, cmd
	}

	if m.agentManageConfirm != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			if m.agentManageConfirm == "delete" {
				if err := m.deleteManagedAgentItem(m.agentManageConfirmID); err != nil {
					m.statusMsg = "delete: " + err.Error()
				} else {
					m.statusMsg = "deleted " + m.agentManageConfirmID
				}
				m.statusExpiry = time.Now().Add(3 * time.Second)
			}
			m.agentManageConfirm = ""
			m.agentManageConfirmID = ""
			return m, nil
		default:
			m.agentManageConfirm = ""
			m.agentManageConfirmID = ""
			m.statusMsg = "delete cancelled"
			m.statusExpiry = time.Now().Add(2 * time.Second)
			return m, nil
		}
	}

	switch msg.String() {
	case "esc", "q":
		m.mode = modeAgentList
		return m, nil
	case "tab":
		m.agentManageFocus = (m.agentManageFocus + 1) % 2
		return m, nil
	case "shift+tab":
		m.agentManageFocus = (m.agentManageFocus + 1) % 2
		return m, nil
	case "[":
		m.agentManageKind = "preset"
		m.agentManageCursor = 0
		m.agentManageField = 0
		m.agentManageListOffset = 0
		m.agentManageDetailOffset = 0
		return m, nil
	case "]":
		m.agentManageKind = "provider"
		m.agentManageCursor = 0
		m.agentManageField = 0
		m.agentManageListOffset = 0
		m.agentManageDetailOffset = 0
		return m, nil
	case "n":
		if err := m.createManagedAgentItem(); err != nil {
			m.statusMsg = "new item: " + err.Error()
		} else {
			m.statusMsg = "created " + m.agentManageKind
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil
	case "D":
		if err := m.duplicateManagedAgentItem(); err != nil {
			m.statusMsg = "duplicate: " + err.Error()
		} else {
			m.statusMsg = "duplicated " + m.agentManageKind
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil
	case "d":
		id := m.currentManagedItemID()
		if id == "" {
			return m, nil
		}
		m.agentManageConfirm = "delete"
		m.agentManageConfirmID = id
		m.statusMsg = "delete " + m.agentManageKind + " " + id + "? y/n"
		m.statusExpiry = time.Now().Add(10 * time.Second)
		return m, nil
	case "enter", "e":
		if m.agentManageFocus == 0 {
			m.agentManageFocus = 1
			m.agentManageDetailOffset = m.agentManageField
			return m, nil
		}
		cmd := m.beginAgentManageEdit()
		return m, cmd
	case "j", "down":
		if m.agentManageFocus == 0 {
			if m.agentManageCursor < m.agentManageItemCount()-1 {
				m.agentManageCursor++
				m.agentManageListOffset++
			}
		} else {
			if m.agentManageField < len(m.agentManageFieldSpecs())-1 {
				m.agentManageField++
				m.agentManageDetailOffset++
			}
		}
	case "k", "up":
		if m.agentManageFocus == 0 {
			if m.agentManageCursor > 0 {
				m.agentManageCursor--
				m.agentManageListOffset--
			}
		} else {
			if m.agentManageField > 0 {
				m.agentManageField--
				m.agentManageDetailOffset--
			}
		}
	case "pgdown", "pgdn":
		if m.agentManageFocus == 0 {
			m.agentManageCursor += 5
			if n := m.agentManageItemCount(); m.agentManageCursor >= n {
				m.agentManageCursor = n - 1
			}
			m.agentManageListOffset = m.agentManageCursor
		} else {
			m.agentManageField += 5
			if n := len(m.agentManageFieldSpecs()); m.agentManageField >= n {
				m.agentManageField = n - 1
			}
			m.agentManageDetailOffset = m.agentManageField
		}
	case "pgup":
		if m.agentManageFocus == 0 {
			m.agentManageCursor -= 5
			if m.agentManageCursor < 0 {
				m.agentManageCursor = 0
			}
			m.agentManageListOffset = m.agentManageCursor
		} else {
			m.agentManageField -= 5
			if m.agentManageField < 0 {
				m.agentManageField = 0
			}
			m.agentManageDetailOffset = m.agentManageField
		}
	case "g":
		if m.agentManageFocus == 0 {
			m.agentManageCursor = 0
			m.agentManageListOffset = 0
		} else {
			m.agentManageField = 0
			m.agentManageDetailOffset = 0
		}
	case "G":
		if m.agentManageFocus == 0 {
			if n := m.agentManageItemCount(); n > 0 {
				m.agentManageCursor = n - 1
				m.agentManageListOffset = m.agentManageCursor
			}
		} else {
			if n := len(m.agentManageFieldSpecs()); n > 0 {
				m.agentManageField = n - 1
				m.agentManageDetailOffset = m.agentManageField
			}
		}
	}
	m.clampAgentManageOffsets()
	return m, nil
}
