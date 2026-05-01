package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/cockpit"
)

var agentManageKinds = []string{"preset", "prompt", "hookbundle", "provider"}

// switchAgentManageKind jumps to a kind directly (used by 1/2/3/4 and r/p/h/g
// hotkeys) and resets the cursor + per-item navigation state. No-op if the
// kind is already active or the editor is open.
func (m *model) switchAgentManageKind(kind string) {
	if m.agentManageKind == kind || m.agentManageEditing {
		return
	}
	m.agentManageKind = kind
	m.agentManageCursor = 0
	m.agentManageField = 0
	m.agentManageGroup = 0
	m.agentManageWizard = false
	m.agentManageListOffset = 0
	m.agentManageDetailOffset = 0
	m.agentManageEnsureGroupField()
}

func cycleAgentManageKind(current string, delta int) string {
	idx := 0
	for i, k := range agentManageKinds {
		if k == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(agentManageKinds)) % len(agentManageKinds)
	return agentManageKinds[idx]
}

func marshalIndentOrEmpty(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil || string(b) == "null" {
		return ""
	}
	return string(b)
}

func presetManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity"},
		{Key: "launch_mode", Label: "Launch mode", Group: "Identity"},
		{Key: "permissions", Label: "Permissions", Group: "Identity"},
		{Key: "prompt_id", Label: "Prompt", Group: "Composition"},
		{Key: "hook_bundle_id", Label: "Hook bundle", Group: "Composition"},
		{Key: "engine_id", Label: "Engine", Group: "Composition"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced"},
	}
}

func providerManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity"},
		{Key: "executor.type", Label: "Engine type", Group: "Engine"},
		{Key: "executor.model", Label: "Model", Group: "Engine"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced"},
	}
}

func promptManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity"},
		{Key: "body", Label: "Body (system prompt)", Group: "Body", Multiline: true, Height: 14},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced"},
	}
}

func hookBundleManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity"},
		{Key: "prompt", Label: "Prompt hooks (JSON array)", Group: "Hooks", Multiline: true, Height: 8},
		{Key: "pre_shell", Label: "Pre-shell hooks (JSON array)", Group: "Hooks", Multiline: true, Height: 8},
		{Key: "post_shell", Label: "Post-shell hooks (JSON array)", Group: "Hooks", Multiline: true, Height: 8},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced"},
	}
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

// parseHookBundleIDs splits the comma-separated text the user types in
// the manage editor into a clean ID slice. Empty input → nil slice (so
// the JSON field is omitted entirely instead of stored as []).
func parseHookBundleIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func canonicalizeLaunchPreset(p cockpit.LaunchPreset) cockpit.LaunchPreset {
	p.Name = strings.TrimSpace(p.Name)
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		p.ID = slugifyManagedID(p.Name)
	}
	return p
}

func canonicalizePromptTemplate(p cockpit.PromptTemplate) cockpit.PromptTemplate {
	p.Name = strings.TrimSpace(p.Name)
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		p.ID = slugifyManagedID(p.Name)
	}
	return p
}

func canonicalizeHookBundle(h cockpit.HookBundle) cockpit.HookBundle {
	h.Name = strings.TrimSpace(h.Name)
	h.ID = strings.TrimSpace(h.ID)
	if h.ID == "" {
		h.ID = slugifyManagedID(h.Name)
	}
	if h.Iteration.Mode == "" {
		h.Iteration.Mode = cockpit.IterationOneShot
	}
	return h
}

func (m model) agentManageFieldSpecs() []agentManageFieldSpec {
	switch m.agentManageKind {
	case "provider":
		return providerManageFields()
	case "prompt":
		return promptManageFields()
	case "hookbundle":
		return hookBundleManageFields()
	}
	return presetManageFields()
}

// agentManageGroupOrder is the wizard step order. Advanced is hidden
// until the user toggles `a` so brand-new items aren't dumped into
// rarely-used overrides.
func (m model) agentManageGroupOrder() []string {
	var groups []string
	switch m.agentManageKind {
	case "provider":
		groups = []string{"Identity", "Engine"}
	case "prompt":
		groups = []string{"Identity", "Body"}
	case "hookbundle":
		groups = []string{"Identity", "Hooks"}
	default:
		groups = []string{"Identity", "Composition"}
	}
	if m.agentManageAdvanced {
		groups = append(groups, "Advanced")
	}
	return groups
}

func (m model) agentManageCurrentGroup() string {
	groups := m.agentManageGroupOrder()
	if len(groups) == 0 {
		return ""
	}
	idx := m.agentManageGroup
	if idx < 0 {
		idx = 0
	}
	if idx >= len(groups) {
		idx = len(groups) - 1
	}
	return groups[idx]
}

// agentManageGroupFieldIndices returns spec indices that belong to the
// current group.
func (m model) agentManageGroupFieldIndices() []int {
	group := m.agentManageCurrentGroup()
	specs := m.agentManageFieldSpecs()
	var out []int
	for i, s := range specs {
		if s.Group == group {
			out = append(out, i)
		}
	}
	return out
}

// agentManageEnsureGroupField makes sure agentManageField points to a
// spec that's actually visible in the current group. Useful after group
// switches or iteration-mode changes.
func (m *model) agentManageEnsureGroupField() {
	indices := m.agentManageGroupFieldIndices()
	if len(indices) == 0 {
		return
	}
	for _, idx := range indices {
		if idx == m.agentManageField {
			return
		}
	}
	m.agentManageField = indices[0]
}

func (m *model) agentManageCycleGroup(delta int) {
	groups := m.agentManageGroupOrder()
	if len(groups) == 0 {
		return
	}
	m.agentManageGroup = (m.agentManageGroup + delta + len(groups)) % len(groups)
	m.agentManageEnsureGroupField()
	m.agentManageDetailOffset = 0
}

// enumOptionsForFieldKey is the cycle list for fields that prefer a
// fixed set of values. Empty result means "no enum, fall through to
// free-text editor". Library-ref fields (prompt_id, hook_bundle_id,
// engine_id) get their cycle list from the loaded libraries via the
// model-aware variant below.
func enumOptionsForFieldKey(key string) []string {
	switch key {
	case "permissions":
		return []string{"read-only", "scoped-write", "wide-open"}
	case "launch_mode":
		return []string{cockpit.LaunchModeSingleJob, cockpit.LaunchModeTaskQueueSequence}
	}
	return nil
}

// enumOptionsForFieldKey on the model layers in dynamic options for
// library-ref fields, falling back to the static set for everything else.
func (m model) enumOptionsForFieldKey(key string) []string {
	switch key {
	case "prompt_id":
		var ids []string
		for _, p := range m.cockpitPrompts {
			ids = append(ids, p.ID)
		}
		return ids
	case "engine_id":
		var ids []string
		for _, p := range m.cockpitProviders {
			ids = append(ids, p.ID)
		}
		return ids
	}
	return enumOptionsForFieldKey(key)
}

func cycleEnumValue(options []string, current string, delta int) string {
	if len(options) == 0 {
		return current
	}
	idx := -1
	for i, opt := range options {
		if opt == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	} else {
		idx = (idx + delta + len(options)) % len(options)
	}
	return options[idx]
}

func (m model) agentManageItemCount() int {
	switch m.agentManageKind {
	case "provider":
		return len(m.cockpitProviders)
	case "prompt":
		return len(m.cockpitPrompts)
	case "hookbundle":
		return len(m.cockpitHookBundles)
	}
	return len(m.cockpitPresets)
}

func (m model) agentManageItemLabel(idx int) string {
	switch m.agentManageKind {
	case "provider":
		if idx < 0 || idx >= len(m.cockpitProviders) {
			return ""
		}
		return m.cockpitProviders[idx].Name
	case "prompt":
		if idx < 0 || idx >= len(m.cockpitPrompts) {
			return ""
		}
		return m.cockpitPrompts[idx].Name
	case "hookbundle":
		if idx < 0 || idx >= len(m.cockpitHookBundles) {
			return ""
		}
		return m.cockpitHookBundles[idx].Name
	}
	if idx < 0 || idx >= len(m.cockpitPresets) {
		return ""
	}
	return m.cockpitPresets[idx].Name
}

func (m model) agentManageItemID(idx int) string {
	switch m.agentManageKind {
	case "provider":
		if idx < 0 || idx >= len(m.cockpitProviders) {
			return ""
		}
		return m.cockpitProviders[idx].ID
	case "prompt":
		if idx < 0 || idx >= len(m.cockpitPrompts) {
			return ""
		}
		return m.cockpitPrompts[idx].ID
	case "hookbundle":
		if idx < 0 || idx >= len(m.cockpitHookBundles) {
			return ""
		}
		return m.cockpitHookBundles[idx].ID
	default:
		if idx < 0 || idx >= len(m.cockpitPresets) {
			return ""
		}
		return m.cockpitPresets[idx].ID
	}
}

func (m model) agentManageFindItemByID(id string) int {
	if strings.TrimSpace(id) == "" {
		return -1
	}
	switch m.agentManageKind {
	case "provider":
		for i, p := range m.cockpitProviders {
			if p.ID == id {
				return i
			}
		}
	case "prompt":
		for i, p := range m.cockpitPrompts {
			if p.ID == id {
				return i
			}
		}
	case "hookbundle":
		for i, h := range m.cockpitHookBundles {
			if h.ID == id {
				return i
			}
		}
	default:
		for i, p := range m.cockpitPresets {
			if p.ID == id {
				return i
			}
		}
	}
	return -1
}

func (m model) agentManageFieldValue(idx, field int) string {
	specs := m.agentManageFieldSpecs()
	if idx < 0 || field < 0 || field >= len(specs) {
		return ""
	}
	key := specs[field].Key
	switch m.agentManageKind {
	case "provider":
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
		case "executor.model":
			return p.Executor.Model
		}
		return ""
	case "prompt":
		if idx >= len(m.cockpitPrompts) {
			return ""
		}
		p := m.cockpitPrompts[idx]
		switch key {
		case "id":
			return p.ID
		case "name":
			return p.Name
		case "body":
			return p.Body
		}
		return ""
	case "hookbundle":
		if idx >= len(m.cockpitHookBundles) {
			return ""
		}
		h := m.cockpitHookBundles[idx]
		switch key {
		case "id":
			return h.ID
		case "name":
			return h.Name
		case "prompt":
			return marshalIndentOrEmpty(h.Prompt)
		case "pre_shell":
			return marshalIndentOrEmpty(h.PreShell)
		case "post_shell":
			return marshalIndentOrEmpty(h.PostShell)
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
	case "launch_mode":
		if p.LaunchMode == "" {
			return cockpit.LaunchModeSingleJob
		}
		return p.LaunchMode
	case "permissions":
		return p.Permissions
	case "prompt_id":
		return p.PromptID
	case "hook_bundle_id":
		return strings.Join(p.HookBundleIDs, ", ")
	case "engine_id":
		return p.EngineID
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
	switch m.agentManageKind {
	case "provider":
		if idx >= len(m.cockpitProviders) {
			return fmt.Errorf("invalid provider")
		}
		p := m.cockpitProviders[idx]
		oldID := p.ID
		switch key {
		case "id":
			p.ID = value
		case "name":
			p.Name = value
		case "executor.type":
			p.Executor.Type = value
		case "executor.model":
			p.Executor.Model = value
		}
		p = canonicalizeProviderProfile(p)
		if err := validateProviderProfile(p); err != nil {
			return err
		}
		m.cockpitProviders[idx] = p
		if err := cockpit.SaveProvider(m.cockpitPaths.ProvidersDir, p); err != nil {
			return err
		}
		if oldID != "" && oldID != p.ID {
			return cockpit.DeleteProvider(m.cockpitPaths.ProvidersDir, oldID)
		}
		return nil
	case "prompt":
		if idx >= len(m.cockpitPrompts) {
			return fmt.Errorf("invalid prompt")
		}
		p := m.cockpitPrompts[idx]
		oldID := p.ID
		switch key {
		case "id":
			p.ID = value
		case "name":
			p.Name = value
		case "body":
			p.Body = raw
		}
		p = canonicalizePromptTemplate(p)
		if err := validatePromptTemplate(p); err != nil {
			return err
		}
		m.cockpitPrompts[idx] = p
		if err := cockpit.SavePrompt(m.cockpitPaths.PromptsDir, p); err != nil {
			return err
		}
		if oldID != "" && oldID != p.ID {
			return cockpit.DeletePrompt(m.cockpitPaths.PromptsDir, oldID)
		}
		return nil
	case "hookbundle":
		if idx >= len(m.cockpitHookBundles) {
			return fmt.Errorf("invalid hook bundle")
		}
		h := m.cockpitHookBundles[idx]
		oldID := h.ID
		switch key {
		case "id":
			h.ID = value
		case "name":
			h.Name = value
		case "prompt":
			parsed, err := parsePromptHooksJSON(raw)
			if err != nil {
				return err
			}
			h.Prompt = parsed
		case "pre_shell":
			parsed, err := parseShellHooksJSON(raw)
			if err != nil {
				return err
			}
			h.PreShell = parsed
		case "post_shell":
			parsed, err := parseShellHooksJSON(raw)
			if err != nil {
				return err
			}
			h.PostShell = parsed
		}
		h = canonicalizeHookBundle(h)
		if err := validateHookBundle(h); err != nil {
			return err
		}
		m.cockpitHookBundles[idx] = h
		if err := cockpit.SaveHookBundle(m.cockpitPaths.HooksDir, h); err != nil {
			return err
		}
		if oldID != "" && oldID != h.ID {
			return cockpit.DeleteHookBundle(m.cockpitPaths.HooksDir, oldID)
		}
		return nil
	}
	if idx >= len(m.cockpitPresets) {
		return fmt.Errorf("invalid preset")
	}
	p := m.cockpitPresets[idx]
	oldID := p.ID
	switch key {
	case "id":
		p.ID = value
	case "name":
		p.Name = value
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
	case "permissions":
		p.Permissions = value
	case "prompt_id":
		p.PromptID = value
	case "hook_bundle_id":
		p.HookBundleIDs = parseHookBundleIDs(value)
	case "engine_id":
		p.EngineID = value
	}
	if p.LaunchMode == "" {
		p.LaunchMode = cockpit.LaunchModeSingleJob
	}
	p = canonicalizeLaunchPreset(p)
	if err := validateLaunchPreset(p); err != nil {
		return err
	}
	if err := cockpit.SavePreset(m.cockpitPaths.PresetsDir, p); err != nil {
		return err
	}
	if oldID != "" && oldID != p.ID {
		if err := cockpit.DeletePreset(m.cockpitPaths.PresetsDir, oldID); err != nil {
			return err
		}
	}
	resolved, err := cockpit.ResolvePreset(p, m.cockpitPrompts, m.cockpitHookBundles, m.cockpitProviders)
	if err == nil {
		p = resolved
	}
	m.cockpitPresets[idx] = p
	return nil
}

func parsePromptHooksJSON(raw string) ([]cockpit.PromptHook, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var out []cockpit.PromptHook
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, fmt.Errorf("prompt hooks JSON: %w", err)
	}
	return out, nil
}

func parseShellHooksJSON(raw string) ([]cockpit.ShellHook, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var out []cockpit.ShellHook
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, fmt.Errorf("shell hooks JSON: %w", err)
	}
	return out, nil
}

func validatePromptTemplate(p cockpit.PromptTemplate) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("prompt ID is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("prompt name is required")
	}
	return nil
}

func validateHookBundle(h cockpit.HookBundle) error {
	if strings.TrimSpace(h.ID) == "" {
		return fmt.Errorf("hook bundle ID is required")
	}
	if strings.TrimSpace(h.Name) == "" {
		return fmt.Errorf("hook bundle name is required")
	}
	return nil
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
	width, maxHeight := m.agentManageEditorDims()
	m.agentManageEditor.SetWidth(width)
	height := spec.Height
	if height < 3 {
		height = 3
	}
	if height > maxHeight {
		height = maxHeight
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
		m.agentManageListOffset = clampDecoratedScrollOffset(m.agentManageListOffset, count, innerHeight)
	}
	specs := m.agentManageFieldSpecs()
	if len(specs) <= 0 {
		m.agentManageDetailOffset = 0
	} else {
		m.agentManageDetailOffset = clampDecoratedScrollOffset(m.agentManageDetailOffset, len(specs), m.agentManageDetailVisibleRows())
	}
	if m.agentManageListOffset < 0 {
		m.agentManageListOffset = 0
	}
	if m.agentManageDetailOffset < 0 {
		m.agentManageDetailOffset = 0
	}
}

func (m *model) endAgentManageEdit(save bool) {
	advance := false
	if save {
		prevCursor := m.agentManageCursor
		if err := m.setAgentManageFieldValue(m.agentManageCursor, m.agentManageField, m.agentManageEditor.Value()); err != nil {
			m.statusMsg = "save field: " + err.Error()
			m.statusExpiry = time.Now().Add(4 * time.Second)
		} else {
			savedID := m.agentManageItemID(m.agentManageCursor)
			m.statusMsg = "saved " + m.agentManageItemLabel(m.agentManageCursor)
			m.statusExpiry = time.Now().Add(2 * time.Second)
			m.refreshManagedSelectionAfterSave(savedID, prevCursor)
			advance = m.agentManageWizard
		}
	}
	m.agentManageEditing = false
	m.agentManageEditor.Blur()
	if advance {
		groups := m.agentManageGroupOrder()
		if m.agentManageGroup+1 < len(groups) {
			m.agentManageCycleGroup(1)
		} else {
			m.agentManageWizard = false
			m.statusMsg = "wizard complete — press esc to return to list"
			m.statusExpiry = time.Now().Add(3 * time.Second)
		}
	}
}

func (m *model) refreshManagedSelectionAfterSave(savedID string, fallbackCursor int) {
	m.reloadCockpitCatalogs()
	if idx := m.agentManageFindItemByID(savedID); idx >= 0 {
		m.agentManageCursor = idx
	} else {
		m.agentManageCursor = clampAgentCursor(fallbackCursor, m.agentManageItemCount())
	}
	m.agentManageEnsureGroupField()
	m.clampAgentManageOffsets()
}

func (m *model) createManagedAgentItem() error {
	switch m.agentManageKind {
	case "provider":
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
	case "prompt":
		p := cockpit.PromptTemplate{Name: "New prompt"}
		p = canonicalizePromptTemplate(p)
		if err := cockpit.SavePrompt(m.cockpitPaths.PromptsDir, p); err != nil {
			return err
		}
		m.reloadCockpitCatalogs()
		for i, pt := range m.cockpitPrompts {
			if pt.ID == p.ID {
				m.agentManageCursor = i
				break
			}
		}
		m.agentManageField = 0
		m.agentManageListOffset = m.agentManageCursor
		m.agentManageDetailOffset = 0
		return nil
	case "hookbundle":
		h := cockpit.HookBundle{
			Name:      "New hook bundle",
			Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot},
		}
		h = canonicalizeHookBundle(h)
		if err := cockpit.SaveHookBundle(m.cockpitPaths.HooksDir, h); err != nil {
			return err
		}
		m.reloadCockpitCatalogs()
		for i, hb := range m.cockpitHookBundles {
			if hb.ID == h.ID {
				m.agentManageCursor = i
				break
			}
		}
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
	for i, pr := range m.cockpitPresets {
		if pr.ID == p.ID {
			m.agentManageCursor = i
			break
		}
	}
	m.agentManageField = 0
	m.agentManageListOffset = m.agentManageCursor
	m.agentManageDetailOffset = 0
	return nil
}

func (m model) currentManagedItemID() string {
	idx := m.agentManageCursor
	switch m.agentManageKind {
	case "provider":
		if idx < 0 || idx >= len(m.cockpitProviders) {
			return ""
		}
		return m.cockpitProviders[idx].ID
	case "prompt":
		if idx < 0 || idx >= len(m.cockpitPrompts) {
			return ""
		}
		return m.cockpitPrompts[idx].ID
	case "hookbundle":
		if idx < 0 || idx >= len(m.cockpitHookBundles) {
			return ""
		}
		return m.cockpitHookBundles[idx].ID
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
	switch m.agentManageKind {
	case "provider":
		if err := cockpit.DeleteProvider(m.cockpitPaths.ProvidersDir, id); err != nil {
			return err
		}
	case "prompt":
		if err := cockpit.DeletePrompt(m.cockpitPaths.PromptsDir, id); err != nil {
			return err
		}
	case "hookbundle":
		if err := cockpit.DeleteHookBundle(m.cockpitPaths.HooksDir, id); err != nil {
			return err
		}
	default:
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
	switch m.agentManageKind {
	case "provider":
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
	case "prompt":
		if idx < 0 || idx >= len(m.cockpitPrompts) {
			return fmt.Errorf("no prompt selected")
		}
		src := m.cockpitPrompts[idx]
		dup := src
		dup.ID = src.ID + "-copy-" + stamp
		dup.Name = src.Name + " (copy)"
		if err := cockpit.SavePrompt(m.cockpitPaths.PromptsDir, dup); err != nil {
			return err
		}
		m.reloadCockpitCatalogs()
		for i, p := range m.cockpitPrompts {
			if p.ID == dup.ID {
				m.agentManageCursor = i
				break
			}
		}
		m.agentManageField = 0
		m.agentManageListOffset = m.agentManageCursor
		m.agentManageDetailOffset = 0
		return nil
	case "hookbundle":
		if idx < 0 || idx >= len(m.cockpitHookBundles) {
			return fmt.Errorf("no hook bundle selected")
		}
		src := m.cockpitHookBundles[idx]
		dup := src
		dup.ID = src.ID + "-copy-" + stamp
		dup.Name = src.Name + " (copy)"
		if err := cockpit.SaveHookBundle(m.cockpitPaths.HooksDir, dup); err != nil {
			return err
		}
		m.reloadCockpitCatalogs()
		for i, h := range m.cockpitHookBundles {
			if h.ID == dup.ID {
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
		m.agentManageWizard = false
		m.mode = modeAgentList
		return m, nil
	case "tab":
		if m.agentManageFocus == 0 {
			m.agentManageFocus = 1
			m.agentManageEnsureGroupField()
		} else {
			m.agentManageCycleGroup(1)
		}
		return m, nil
	case "shift+tab":
		if m.agentManageFocus == 0 {
			m.agentManageFocus = 1
			m.agentManageEnsureGroupField()
		} else if m.agentManageGroup == 0 {
			m.agentManageFocus = 0
		} else {
			m.agentManageCycleGroup(-1)
		}
		return m, nil
	case "h", "left":
		if m.agentManageFocus == 1 {
			m.agentManageFocus = 0
			return m, nil
		}
	case "l", "right":
		if m.agentManageFocus == 0 {
			m.agentManageFocus = 1
			m.agentManageEnsureGroupField()
			return m, nil
		}
	case "a":
		m.agentManageAdvanced = !m.agentManageAdvanced
		groups := m.agentManageGroupOrder()
		if m.agentManageGroup >= len(groups) {
			m.agentManageGroup = len(groups) - 1
		}
		m.agentManageEnsureGroupField()
		if m.agentManageAdvanced {
			m.statusMsg = "advanced fields shown"
		} else {
			m.statusMsg = "advanced fields hidden"
		}
		m.statusExpiry = time.Now().Add(2 * time.Second)
		return m, nil
	case "[":
		m.switchAgentManageKind(cycleAgentManageKind(m.agentManageKind, -1))
		return m, nil
	case "]":
		m.switchAgentManageKind(cycleAgentManageKind(m.agentManageKind, 1))
		return m, nil
	case "1":
		m.switchAgentManageKind("preset")
		return m, nil
	case "2":
		m.switchAgentManageKind("prompt")
		return m, nil
	case "3":
		m.switchAgentManageKind("hookbundle")
		return m, nil
	case "4":
		m.switchAgentManageKind("provider")
		return m, nil
	case "n":
		if err := m.createManagedAgentItem(); err != nil {
			m.statusMsg = "new item: " + err.Error()
			m.statusExpiry = time.Now().Add(3 * time.Second)
			return m, nil
		}
		m.statusMsg = "created " + m.agentManageKind + " — fill out the wizard"
		m.statusExpiry = time.Now().Add(3 * time.Second)
		m.agentManageGroup = 0
		m.agentManageFocus = 1
		m.agentManageWizard = true
		m.agentManageEnsureGroupField()
		cmd := m.beginAgentManageEdit()
		return m, cmd
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
			m.agentManageEnsureGroupField()
			return m, nil
		}
		// Enum fields cycle in place instead of opening the editor.
		specs := m.agentManageFieldSpecs()
		if m.agentManageField >= 0 && m.agentManageField < len(specs) {
			key := specs[m.agentManageField].Key
			if options := m.enumOptionsForFieldKey(key); len(options) > 0 {
				current := m.agentManageFieldValue(m.agentManageCursor, m.agentManageField)
				next := cycleEnumValue(options, current, 1)
				if err := m.setAgentManageFieldValue(m.agentManageCursor, m.agentManageField, next); err != nil {
					m.statusMsg = "cycle: " + err.Error()
					m.statusExpiry = time.Now().Add(3 * time.Second)
				} else {
					savedID := m.agentManageItemID(m.agentManageCursor)
					prevCursor := m.agentManageCursor
					m.statusMsg = specs[m.agentManageField].Label + " → " + next
					m.statusExpiry = time.Now().Add(2 * time.Second)
					m.refreshManagedSelectionAfterSave(savedID, prevCursor)
				}
				return m, nil
			}
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
			indices := m.agentManageGroupFieldIndices()
			for i, idx := range indices {
				if idx == m.agentManageField && i+1 < len(indices) {
					m.agentManageField = indices[i+1]
					break
				}
			}
		}
	case "k", "up":
		if m.agentManageFocus == 0 {
			if m.agentManageCursor > 0 {
				m.agentManageCursor--
				m.agentManageListOffset--
			}
		} else {
			indices := m.agentManageGroupFieldIndices()
			for i, idx := range indices {
				if idx == m.agentManageField && i > 0 {
					m.agentManageField = indices[i-1]
					break
				}
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
			m.agentManageCycleGroup(1)
		}
	case "pgup":
		if m.agentManageFocus == 0 {
			m.agentManageCursor -= 5
			if m.agentManageCursor < 0 {
				m.agentManageCursor = 0
			}
			m.agentManageListOffset = m.agentManageCursor
		} else {
			m.agentManageCycleGroup(-1)
		}
	case "g":
		if m.agentManageFocus == 0 {
			m.agentManageCursor = 0
			m.agentManageListOffset = 0
		} else {
			indices := m.agentManageGroupFieldIndices()
			if len(indices) > 0 {
				m.agentManageField = indices[0]
			}
		}
	case "G":
		if m.agentManageFocus == 0 {
			if n := m.agentManageItemCount(); n > 0 {
				m.agentManageCursor = n - 1
				m.agentManageListOffset = m.agentManageCursor
			}
		} else {
			indices := m.agentManageGroupFieldIndices()
			if len(indices) > 0 {
				m.agentManageField = indices[len(indices)-1]
			}
		}
	}
	m.agentManageEnsureGroupField()
	m.clampAgentManageOffsets()
	return m, nil
}
