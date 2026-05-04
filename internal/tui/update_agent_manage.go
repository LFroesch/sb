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
	if m.agentManageKind == kind || m.agentManageEditing || m.agentManageSelectEditing {
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
		{Key: "name", Label: "Name", Group: "Identity", Help: "human label shown in pickers; ID auto-follows when blank"},
		{Key: "launch_mode", Label: "Launch mode", Group: "Identity", Help: "single job vs task queue sequence"},
		{Key: "permissions", Label: "Permissions", Group: "Identity", Help: "read-only, scoped-write, or wide-open"},
		{Key: "prompt_id", Label: "Prompt", Group: "Composition", Help: "blank clears the prompt ref"},
		{Key: "hook_bundle_id", Label: "Hook bundles", Group: "Composition", Help: "comma-separated bundle ids/names; blank clears all"},
		{Key: "engine_id", Label: "Engine", Group: "Composition", Help: "blank clears the engine ref"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced", Help: "filename slug under ~/.config/sb/..."},
	}
}

func providerManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity", Help: "human label shown in the launch composer"},
		{Key: "executor.type", Label: "Engine type", Group: "Engine", Help: "claude, codex, ollama, or shell"},
		{Key: "executor.model", Label: "Model", Group: "Engine", Help: "provider-specific model identifier"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced", Help: "filename slug under ~/.config/sb/providers"},
	}
}

func promptManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity", Help: "human label shown in pickers"},
		{Key: "body", Label: "Body (system prompt)", Group: "Body", Multiline: true, Height: 14, Help: "full system prompt body; multiline text"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced", Help: "filename slug under ~/.config/sb/prompts"},
	}
}

func hookBundleManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Group: "Identity", Help: "human label shown in pickers"},
		{Key: "prompt", Label: "Prompt hooks", Group: "Hooks", Multiline: true, Height: 8, Help: "structured editor for prompt hook rows"},
		{Key: "pre_shell", Label: "Pre-shell hooks", Group: "Hooks", Multiline: true, Height: 8, Help: "structured editor for shell hooks run before launch"},
		{Key: "post_shell", Label: "Post-shell hooks", Group: "Hooks", Multiline: true, Height: 8, Help: "structured editor for shell hooks run after completion"},
		{Key: "id", Label: "File ID (auto if empty)", Group: "Advanced", Help: "filename slug under ~/.config/sb/hooks"},
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
	case "executor.type":
		return []string{"claude", "codex", "ollama", "shell"}
	}
	return nil
}

// enumOptionsForFieldKey on the model layers in dynamic options for
// library-ref fields, falling back to the static set for everything else.
func (m model) enumOptionsForFieldKey(key string) []string {
	switch key {
	case "prompt_id":
		ids := []string{""}
		for _, p := range m.cockpitPrompts {
			ids = append(ids, p.ID)
		}
		return ids
	case "hook_bundle_id":
		ids := []string{""}
		for _, b := range m.cockpitHookBundles {
			ids = append(ids, b.ID)
		}
		return ids
	case "engine_id":
		ids := []string{""}
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

func usesAgentManageSelectInput(spec agentManageFieldSpec) bool {
	switch spec.Key {
	case "launch_mode", "permissions", "prompt_id", "hook_bundle_id", "engine_id", "executor.type":
		return true
	}
	return false
}

func normalizeManagedLookup(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func resolveManagedLookupToken(raw string, choices map[string]string) (string, error) {
	q := normalizeManagedLookup(raw)
	if q == "" {
		return "", nil
	}
	if v, ok := choices[q]; ok {
		return v, nil
	}
	match := ""
	for k, v := range choices {
		if strings.Contains(k, q) {
			if match != "" && match != v {
				return "", fmt.Errorf("ambiguous match %q", raw)
			}
			match = v
		}
	}
	if match == "" {
		return "", fmt.Errorf("no match for %q", raw)
	}
	return match, nil
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

func (m *model) beginAgentManageSelectEdit() tea.Cmd {
	specs := m.agentManageFieldSpecs()
	if m.agentManageField < 0 || m.agentManageField >= len(specs) {
		return nil
	}
	spec := specs[m.agentManageField]
	if !usesAgentManageSelectInput(spec) {
		return nil
	}
	m.agentManageSelectEditing = true
	m.agentManageSelectInput.SetValue(m.agentManageFieldValue(m.agentManageCursor, m.agentManageField))
	width, _ := m.agentManageEditorDims()
	m.agentManageSelectInput.Width = maxInt(20, width)
	switch spec.Key {
	case "launch_mode":
		m.agentManageSelectInput.Placeholder = "single_job or task_queue_sequence"
	case "permissions":
		m.agentManageSelectInput.Placeholder = "read-only, scoped-write, or wide-open"
	case "executor.type":
		m.agentManageSelectInput.Placeholder = "claude, codex, ollama, or shell"
	case "prompt_id":
		m.agentManageSelectInput.Placeholder = "blank clears; type prompt id/name"
	case "hook_bundle_id":
		m.agentManageSelectInput.Placeholder = "blank clears; comma-separated hook ids/names"
	case "engine_id":
		m.agentManageSelectInput.Placeholder = "blank clears; type engine id/name"
	default:
		m.agentManageSelectInput.Placeholder = "type value"
	}
	m.agentManageSelectInput.Focus()
	return m.agentManageSelectInput.Cursor.BlinkCmd()
}

func (m model) resolveManagedSelectionRaw(spec agentManageFieldSpec, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch spec.Key {
	case "launch_mode":
		switch raw {
		case "", cockpit.LaunchModeSingleJob:
			return cockpit.LaunchModeSingleJob, nil
		case cockpit.LaunchModeTaskQueueSequence:
			return cockpit.LaunchModeTaskQueueSequence, nil
		default:
			return "", fmt.Errorf("launch mode must be single_job or task_queue_sequence")
		}
	case "permissions":
		switch raw {
		case "", "read-only", "scoped-write", "wide-open":
			return raw, nil
		default:
			return "", fmt.Errorf("permissions must be read-only, scoped-write, or wide-open")
		}
	case "executor.type":
		switch raw {
		case "claude", "codex", "ollama", "shell":
			return raw, nil
		default:
			return "", fmt.Errorf("engine type must be claude, codex, ollama, or shell")
		}
	case "prompt_id":
		choices := map[string]string{}
		for _, p := range m.cockpitPrompts {
			choices[normalizeManagedLookup(p.ID)] = p.ID
			choices[normalizeManagedLookup(p.Name)] = p.ID
		}
		return resolveManagedLookupToken(raw, choices)
	case "engine_id":
		choices := map[string]string{}
		for _, p := range m.cockpitProviders {
			choices[normalizeManagedLookup(p.ID)] = p.ID
			choices[normalizeManagedLookup(p.Name)] = p.ID
		}
		return resolveManagedLookupToken(raw, choices)
	case "hook_bundle_id":
		if raw == "" {
			return "", nil
		}
		choices := map[string]string{}
		for _, b := range m.cockpitHookBundles {
			choices[normalizeManagedLookup(b.ID)] = b.ID
			choices[normalizeManagedLookup(b.Name)] = b.ID
		}
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		seen := map[string]bool{}
		for _, part := range parts {
			id, err := resolveManagedLookupToken(part, choices)
			if err != nil {
				return "", err
			}
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		return strings.Join(out, ", "), nil
	}
	return raw, nil
}

func (m *model) endAgentManageSelectEdit(save bool) {
	if save {
		specs := m.agentManageFieldSpecs()
		if m.agentManageField >= 0 && m.agentManageField < len(specs) {
			spec := specs[m.agentManageField]
			prevCursor := m.agentManageCursor
			value, err := m.resolveManagedSelectionRaw(spec, m.agentManageSelectInput.Value())
			if err != nil {
				m.statusMsg = "save field: " + err.Error()
				m.statusExpiry = time.Now().Add(4 * time.Second)
			} else if err := m.setAgentManageFieldValue(m.agentManageCursor, m.agentManageField, value); err != nil {
				m.statusMsg = "save field: " + err.Error()
				m.statusExpiry = time.Now().Add(4 * time.Second)
			} else {
				savedID := m.agentManageItemID(m.agentManageCursor)
				m.statusMsg = "saved " + m.agentManageItemLabel(m.agentManageCursor)
				m.statusExpiry = time.Now().Add(2 * time.Second)
				m.refreshManagedSelectionAfterSave(savedID, prevCursor)
			}
		}
	}
	m.agentManageSelectEditing = false
	m.agentManageSelectInput.Blur()
	m.agentManageSelectInput.SetValue("")
}

func (m *model) beginAgentManageEdit() tea.Cmd {
	specs := m.agentManageFieldSpecs()
	if m.agentManageField < 0 || m.agentManageField >= len(specs) {
		return nil
	}
	spec := specs[m.agentManageField]
	m.agentManageEditing = true
	m.agentManageEditor.Reset()
	value := m.agentManageFieldValue(m.agentManageCursor, m.agentManageField)
	if value == "" {
		switch spec.Key {
		case "prompt", "pre_shell", "post_shell":
			value = "[]"
		}
	}
	m.agentManageEditor.SetValue(value)
	switch spec.Key {
	case "body":
		m.agentManageEditor.Placeholder = "system prompt text..."
	case "prompt":
		m.agentManageEditor.Placeholder = "[\n  {\n    \"kind\": \"literal\",\n    \"label\": \"Context\",\n    \"body\": \"...\"\n  }\n]"
	case "pre_shell", "post_shell":
		m.agentManageEditor.Placeholder = "[\n  {\n    \"name\": \"example\",\n    \"cmd\": \"git status --short\"\n  }\n]"
	default:
		m.agentManageEditor.Placeholder = "edit field value…"
	}
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

func isHookBundleStructuredField(key string) bool {
	switch key {
	case "prompt", "pre_shell", "post_shell":
		return true
	}
	return false
}

func promptHookManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "kind", Label: "Kind", Help: "literal uses Body directly; file reads Body Ref from disk"},
		{Key: "placement", Label: "Placement", Help: "before or after the task brief"},
		{Key: "label", Label: "Label", Help: "optional markdown heading shown before the block"},
		{Key: "body", Label: "Body", Multiline: true, Height: 10, Help: "literal prompt-hook body"},
		{Key: "body_ref", Label: "Body Ref", Help: "path used when kind=file"},
	}
}

func shellHookManageFields() []agentManageFieldSpec {
	return []agentManageFieldSpec{
		{Key: "name", Label: "Name", Help: "short operator-facing label"},
		{Key: "cmd", Label: "Command", Multiline: true, Height: 6, Help: "shell command to run"},
		{Key: "cwd", Label: "Working Dir", Help: "blank = job repo"},
		{Key: "timeout", Label: "Timeout", Help: "Go duration like 30s or 2m"},
		{Key: "preview_cmd", Label: "Preview Cmd", Help: "optional safer preview command for review time"},
		{Key: "preview_safe", Label: "Preview Safe", Help: "true if Cmd itself is safe to preview"},
	}
}

func (m *model) beginAgentManageHookEdit() {
	if m.agentManageKind != "hookbundle" || m.agentManageCursor < 0 || m.agentManageCursor >= len(m.cockpitHookBundles) {
		return
	}
	spec, ok := m.currentAgentManageFieldSpec()
	if !ok || !isHookBundleStructuredField(spec.Key) {
		return
	}
	h := m.cockpitHookBundles[m.agentManageCursor]
	m.agentManageHookEditing = true
	m.agentManageHookFocus = 0
	m.agentManageHookCursor = 0
	m.agentManageHookField = 0
	m.agentManageHookArrayKey = spec.Key
	m.agentManagePromptDraft = append([]cockpit.PromptHook(nil), h.Prompt...)
	switch spec.Key {
	case "pre_shell":
		m.agentManageShellDraft = append([]cockpit.ShellHook(nil), h.PreShell...)
	case "post_shell":
		m.agentManageShellDraft = append([]cockpit.ShellHook(nil), h.PostShell...)
	default:
		m.agentManageShellDraft = nil
	}
}

func (m *model) endAgentManageHookEdit(save bool) {
	if save && m.agentManageCursor >= 0 && m.agentManageCursor < len(m.cockpitHookBundles) {
		h := m.cockpitHookBundles[m.agentManageCursor]
		switch m.agentManageHookArrayKey {
		case "prompt":
			h.Prompt = append([]cockpit.PromptHook(nil), m.agentManagePromptDraft...)
		case "pre_shell":
			h.PreShell = append([]cockpit.ShellHook(nil), m.agentManageShellDraft...)
		case "post_shell":
			h.PostShell = append([]cockpit.ShellHook(nil), m.agentManageShellDraft...)
		}
		h = canonicalizeHookBundle(h)
		if err := validateHookBundle(h); err != nil {
			m.statusMsg = "save hooks: " + err.Error()
			m.statusExpiry = time.Now().Add(4 * time.Second)
		} else if err := cockpit.SaveHookBundle(m.cockpitPaths.HooksDir, h); err != nil {
			m.statusMsg = "save hooks: " + err.Error()
			m.statusExpiry = time.Now().Add(4 * time.Second)
		} else {
			savedID := h.ID
			prevCursor := m.agentManageCursor
			m.statusMsg = "saved " + h.Name
			m.statusExpiry = time.Now().Add(2 * time.Second)
			m.refreshManagedSelectionAfterSave(savedID, prevCursor)
		}
	}
	m.agentManageHookEditing = false
	m.agentManageHookFocus = 0
	m.agentManageHookCursor = 0
	m.agentManageHookField = 0
	m.agentManageHookArrayKey = ""
	m.agentManagePromptDraft = nil
	m.agentManageShellDraft = nil
}

func (m model) agentManageHookFieldSpecs() []agentManageFieldSpec {
	if m.agentManageHookArrayKey == "prompt" {
		return promptHookManageFields()
	}
	return shellHookManageFields()
}

func (m model) agentManageHookItemsCount() int {
	switch m.agentManageHookArrayKey {
	case "prompt":
		return len(m.agentManagePromptDraft)
	case "pre_shell", "post_shell":
		return len(m.agentManageShellDraft)
	}
	return 0
}

func (m model) agentManageHookRowCount() int {
	return m.agentManageHookItemsCount() + 1
}

func (m model) agentManageHookCurrentFieldSpec() (agentManageFieldSpec, bool) {
	specs := m.agentManageHookFieldSpecs()
	if m.agentManageHookField < 0 || m.agentManageHookField >= len(specs) {
		return agentManageFieldSpec{}, false
	}
	return specs[m.agentManageHookField], true
}

func (m model) agentManageHookItemLabel(idx int) string {
	if idx == m.agentManageHookItemsCount() {
		return "+ add hook"
	}
	switch m.agentManageHookArrayKey {
	case "prompt":
		h := m.agentManagePromptDraft[idx]
		label := strings.TrimSpace(h.Label)
		if label == "" {
			label = strings.TrimSpace(h.Kind)
		}
		if label == "" {
			label = fmt.Sprintf("prompt hook %d", idx+1)
		}
		placement := strings.TrimSpace(h.Placement)
		if placement == "" {
			placement = "after"
		}
		return fmt.Sprintf("%d. %s · %s", idx+1, label, placement)
	default:
		h := m.agentManageShellDraft[idx]
		label := strings.TrimSpace(h.Name)
		if label == "" {
			label = strings.TrimSpace(h.Cmd)
		}
		if label == "" {
			label = fmt.Sprintf("shell hook %d", idx+1)
		}
		return fmt.Sprintf("%d. %s", idx+1, label)
	}
}

func (m model) agentManageHookFieldValue(field int) string {
	if m.agentManageHookCursor < 0 || m.agentManageHookCursor >= m.agentManageHookItemsCount() {
		return ""
	}
	specs := m.agentManageHookFieldSpecs()
	if field < 0 || field >= len(specs) {
		return ""
	}
	key := specs[field].Key
	switch m.agentManageHookArrayKey {
	case "prompt":
		h := m.agentManagePromptDraft[m.agentManageHookCursor]
		switch key {
		case "kind":
			return h.Kind
		case "placement":
			return h.Placement
		case "label":
			return h.Label
		case "body":
			return h.Body
		case "body_ref":
			return h.BodyRef
		}
	default:
		h := m.agentManageShellDraft[m.agentManageHookCursor]
		switch key {
		case "name":
			return h.Name
		case "cmd":
			return h.Cmd
		case "cwd":
			return h.Cwd
		case "timeout":
			if h.Timeout <= 0 {
				return ""
			}
			return h.Timeout.String()
		case "preview_cmd":
			return h.PreviewCmd
		case "preview_safe":
			if h.PreviewSafe {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

func enumOptionsForHookField(arrayKey, key string) []string {
	switch key {
	case "kind":
		return []string{"literal", "file"}
	case "placement":
		return []string{"after", "before"}
	case "preview_safe":
		return []string{"false", "true"}
	}
	return nil
}

func (m *model) setAgentManageHookFieldValue(field int, raw string) error {
	if m.agentManageHookCursor < 0 || m.agentManageHookCursor >= m.agentManageHookItemsCount() {
		return fmt.Errorf("invalid hook row")
	}
	specs := m.agentManageHookFieldSpecs()
	if field < 0 || field >= len(specs) {
		return fmt.Errorf("invalid hook field")
	}
	key := specs[field].Key
	value := strings.TrimSpace(raw)
	switch m.agentManageHookArrayKey {
	case "prompt":
		h := m.agentManagePromptDraft[m.agentManageHookCursor]
		switch key {
		case "kind":
			if value == "" {
				value = "literal"
			}
			if value != "literal" && value != "file" {
				return fmt.Errorf("kind must be literal or file")
			}
			h.Kind = value
		case "placement":
			if value == "" {
				value = "after"
			}
			if value != "before" && value != "after" {
				return fmt.Errorf("placement must be before or after")
			}
			h.Placement = value
		case "label":
			h.Label = value
		case "body":
			h.Body = raw
		case "body_ref":
			h.BodyRef = value
		}
		m.agentManagePromptDraft[m.agentManageHookCursor] = h
	default:
		h := m.agentManageShellDraft[m.agentManageHookCursor]
		switch key {
		case "name":
			h.Name = value
		case "cmd":
			h.Cmd = raw
		case "cwd":
			h.Cwd = value
		case "timeout":
			if value == "" {
				h.Timeout = 0
			} else {
				d, err := time.ParseDuration(value)
				if err != nil {
					return fmt.Errorf("timeout: %w", err)
				}
				h.Timeout = d
			}
		case "preview_cmd":
			h.PreviewCmd = raw
		case "preview_safe":
			switch value {
			case "", "false":
				h.PreviewSafe = false
			case "true":
				h.PreviewSafe = true
			default:
				return fmt.Errorf("preview safe must be true or false")
			}
		}
		m.agentManageShellDraft[m.agentManageHookCursor] = h
	}
	return nil
}

func (m *model) addAgentManageHookRow() {
	switch m.agentManageHookArrayKey {
	case "prompt":
		m.agentManagePromptDraft = append(m.agentManagePromptDraft, cockpit.PromptHook{Kind: "literal", Placement: "after"})
	case "pre_shell", "post_shell":
		m.agentManageShellDraft = append(m.agentManageShellDraft, cockpit.ShellHook{})
	}
	if n := m.agentManageHookItemsCount(); n > 0 {
		m.agentManageHookCursor = n - 1
	}
	m.agentManageHookFocus = 1
	m.agentManageHookField = 0
}

func (m *model) deleteAgentManageHookRow() {
	if m.agentManageHookCursor < 0 || m.agentManageHookCursor >= m.agentManageHookItemsCount() {
		return
	}
	switch m.agentManageHookArrayKey {
	case "prompt":
		m.agentManagePromptDraft = append(m.agentManagePromptDraft[:m.agentManageHookCursor], m.agentManagePromptDraft[m.agentManageHookCursor+1:]...)
	default:
		m.agentManageShellDraft = append(m.agentManageShellDraft[:m.agentManageHookCursor], m.agentManageShellDraft[m.agentManageHookCursor+1:]...)
	}
	if m.agentManageHookCursor >= m.agentManageHookItemsCount() {
		m.agentManageHookCursor = m.agentManageHookItemsCount() - 1
	}
	if m.agentManageHookCursor < 0 {
		m.agentManageHookCursor = 0
		m.agentManageHookFocus = 0
	}
}

func (m *model) duplicateAgentManageHookRow() {
	if m.agentManageHookCursor < 0 || m.agentManageHookCursor >= m.agentManageHookItemsCount() {
		return
	}
	switch m.agentManageHookArrayKey {
	case "prompt":
		h := m.agentManagePromptDraft[m.agentManageHookCursor]
		m.agentManagePromptDraft = append(m.agentManagePromptDraft[:m.agentManageHookCursor+1], append([]cockpit.PromptHook{h}, m.agentManagePromptDraft[m.agentManageHookCursor+1:]...)...)
	default:
		h := m.agentManageShellDraft[m.agentManageHookCursor]
		m.agentManageShellDraft = append(m.agentManageShellDraft[:m.agentManageHookCursor+1], append([]cockpit.ShellHook{h}, m.agentManageShellDraft[m.agentManageHookCursor+1:]...)...)
	}
	m.agentManageHookCursor++
}

func (m *model) moveAgentManageHookRow(delta int) {
	if m.agentManageHookCursor < 0 || m.agentManageHookCursor >= m.agentManageHookItemsCount() {
		return
	}
	next := m.agentManageHookCursor + delta
	if next < 0 || next >= m.agentManageHookItemsCount() {
		return
	}
	switch m.agentManageHookArrayKey {
	case "prompt":
		rows := m.agentManagePromptDraft
		rows[m.agentManageHookCursor], rows[next] = rows[next], rows[m.agentManageHookCursor]
		m.agentManagePromptDraft = rows
	default:
		rows := m.agentManageShellDraft
		rows[m.agentManageHookCursor], rows[next] = rows[next], rows[m.agentManageHookCursor]
		m.agentManageShellDraft = rows
	}
	m.agentManageHookCursor = next
}

func (m *model) beginAgentManageHookFieldEdit() tea.Cmd {
	spec, ok := m.agentManageHookCurrentFieldSpec()
	if !ok {
		return nil
	}
	if len(enumOptionsForHookField(m.agentManageHookArrayKey, spec.Key)) > 0 {
		m.agentManageSelectEditing = true
		m.agentManageSelectInput.SetValue(m.agentManageHookFieldValue(m.agentManageHookField))
		width, _ := m.agentManageEditorDims()
		m.agentManageSelectInput.Width = maxInt(20, width)
		m.agentManageSelectInput.Placeholder = "type value"
		m.agentManageSelectInput.Focus()
		return m.agentManageSelectInput.Cursor.BlinkCmd()
	}
	m.agentManageEditing = true
	m.agentManageEditor.Reset()
	m.agentManageEditor.SetValue(m.agentManageHookFieldValue(m.agentManageHookField))
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
	switch spec.Key {
	case "body":
		m.agentManageEditor.Placeholder = "hook body..."
	case "cmd", "preview_cmd":
		m.agentManageEditor.Placeholder = "shell command..."
	default:
		m.agentManageEditor.Placeholder = "edit field value…"
	}
	m.agentManageEditor.Focus()
	return m.agentManageEditor.Cursor.BlinkCmd()
}

func (m *model) endAgentManageHookFieldEdit(save bool) {
	if save {
		if err := m.setAgentManageHookFieldValue(m.agentManageHookField, m.agentManageEditor.Value()); err != nil {
			m.statusMsg = "save hook field: " + err.Error()
			m.statusExpiry = time.Now().Add(4 * time.Second)
		}
	}
	m.agentManageEditing = false
	m.agentManageEditor.Blur()
}

func (m *model) endAgentManageHookSelectEdit(save bool) {
	if save {
		if err := m.setAgentManageHookFieldValue(m.agentManageHookField, m.agentManageSelectInput.Value()); err != nil {
			m.statusMsg = "save hook field: " + err.Error()
			m.statusExpiry = time.Now().Add(4 * time.Second)
		}
	}
	m.agentManageSelectEditing = false
	m.agentManageSelectInput.Blur()
	m.agentManageSelectInput.SetValue("")
}

func (m model) currentAgentManageFieldSpec() (agentManageFieldSpec, bool) {
	specs := m.agentManageFieldSpecs()
	if m.agentManageField < 0 || m.agentManageField >= len(specs) {
		return agentManageFieldSpec{}, false
	}
	return specs[m.agentManageField], true
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
	if m.agentManageHookEditing {
		if m.agentManageSelectEditing {
			switch msg.String() {
			case "esc":
				m.endAgentManageHookSelectEdit(false)
				return m, nil
			case "enter", "ctrl+s":
				m.endAgentManageHookSelectEdit(true)
				return m, nil
			}
			var cmd tea.Cmd
			m.agentManageSelectInput, cmd = m.agentManageSelectInput.Update(msg)
			return m, cmd
		}
		if m.agentManageEditing {
			switch msg.String() {
			case "esc":
				m.endAgentManageHookFieldEdit(false)
				return m, nil
			case "enter":
				if spec, ok := m.agentManageHookCurrentFieldSpec(); ok && !spec.Multiline {
					m.endAgentManageHookFieldEdit(true)
					return m, nil
				}
			case "ctrl+s":
				m.endAgentManageHookFieldEdit(true)
				return m, nil
			}
			var cmd tea.Cmd
			m.agentManageEditor, cmd = m.agentManageEditor.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "esc":
			m.endAgentManageHookEdit(false)
			return m, nil
		case "ctrl+s":
			m.endAgentManageHookEdit(true)
			return m, nil
		case "[":
			if m.agentManageHookFocus == 0 {
				m.moveAgentManageHookRow(-1)
			}
			return m, nil
		case "]":
			if m.agentManageHookFocus == 0 {
				m.moveAgentManageHookRow(1)
			}
			return m, nil
		case "tab", "shift+tab":
			m.agentManageHookFocus = 1 - m.agentManageHookFocus
			return m, nil
		case "j", "down":
			if m.agentManageHookFocus == 0 {
				if m.agentManageHookCursor < m.agentManageHookRowCount()-1 {
					m.agentManageHookCursor++
				}
			} else {
				if m.agentManageHookField < len(m.agentManageHookFieldSpecs())-1 {
					m.agentManageHookField++
				}
			}
			return m, nil
		case "k", "up":
			if m.agentManageHookFocus == 0 {
				if m.agentManageHookCursor > 0 {
					m.agentManageHookCursor--
				}
			} else {
				if m.agentManageHookField > 0 {
					m.agentManageHookField--
				}
			}
			return m, nil
		case "a":
			m.addAgentManageHookRow()
			return m, nil
		case "D":
			m.duplicateAgentManageHookRow()
			return m, nil
		case "d":
			m.deleteAgentManageHookRow()
			return m, nil
		case "enter", "e":
			if m.agentManageHookFocus == 0 {
				if m.agentManageHookCursor == m.agentManageHookItemsCount() {
					m.addAgentManageHookRow()
					return m, nil
				}
				m.agentManageHookFocus = 1
				return m, nil
			}
			if m.agentManageHookItemsCount() == 0 {
				m.addAgentManageHookRow()
			}
			cmd := m.beginAgentManageHookFieldEdit()
			return m, cmd
		}
		return m, nil
	}

	if m.agentManageSelectEditing {
		switch msg.String() {
		case "esc":
			m.endAgentManageSelectEdit(false)
			return m, nil
		case "enter", "ctrl+s":
			m.endAgentManageSelectEdit(true)
			return m, nil
		}
		var cmd tea.Cmd
		m.agentManageSelectInput, cmd = m.agentManageSelectInput.Update(msg)
		return m, cmd
	}

	if m.agentManageEditing {
		switch msg.String() {
		case "esc":
			m.endAgentManageEdit(false)
			return m, nil
		case "enter":
			if spec, ok := m.currentAgentManageFieldSpec(); ok && !spec.Multiline {
				m.endAgentManageEdit(true)
				return m, nil
			}
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
	case "enter":
		if m.agentManageFocus == 0 {
			m.agentManageFocus = 1
			m.agentManageEnsureGroupField()
			return m, nil
		}
		specs := m.agentManageFieldSpecs()
		if m.agentManageField >= 0 && m.agentManageField < len(specs) && isHookBundleStructuredField(specs[m.agentManageField].Key) {
			m.beginAgentManageHookEdit()
			return m, nil
		}
		if m.agentManageField >= 0 && m.agentManageField < len(specs) && usesAgentManageSelectInput(specs[m.agentManageField]) {
			cmd := m.beginAgentManageSelectEdit()
			return m, cmd
		}
		cmd := m.beginAgentManageEdit()
		return m, cmd
	case "e":
		if m.agentManageFocus == 0 {
			m.agentManageFocus = 1
			m.agentManageEnsureGroupField()
		}
		specs := m.agentManageFieldSpecs()
		if m.agentManageField >= 0 && m.agentManageField < len(specs) && isHookBundleStructuredField(specs[m.agentManageField].Key) {
			m.beginAgentManageHookEdit()
			return m, nil
		}
		if m.agentManageField >= 0 && m.agentManageField < len(specs) && usesAgentManageSelectInput(specs[m.agentManageField]) {
			cmd := m.beginAgentManageSelectEdit()
			return m, cmd
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
