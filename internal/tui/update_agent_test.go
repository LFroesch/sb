package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/workmd"
)

func TestUpdateAllowsQuestionMarkInAgentLaunchBrief(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{{Text: "keep draft state"}}
	m.launchFocus = m.launchNoteFocus()
	m.launchSources = []cockpit.SourceTask{{Text: "keep draft state"}}
	m.launchRepo = "/tmp/demo"
	m.launchBrief.Focus()

	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	next := got.(model)
	if next.mode != modeAgentLaunch {
		t.Fatalf("mode = %v, want modeAgentLaunch", next.mode)
	}
	if next.launchFocus != m.launchNoteFocus() {
		t.Fatalf("launchFocus = %d, want %d", next.launchFocus, m.launchNoteFocus())
	}
	if next.launchRepo != "/tmp/demo" {
		t.Fatalf("launchRepo = %q, want /tmp/demo", next.launchRepo)
	}
	if len(next.launchSources) != 1 || next.launchSources[0].Text != "keep draft state" {
		t.Fatalf("launchSources changed unexpectedly: %+v", next.launchSources)
	}
}

func TestUpdateAgentLaunchCtrlTTogglesForemanModeFromNote(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{{Text: "keep draft state"}}
	m.launchFocus = m.launchNoteFocus()
	m.launchRepo = "/tmp/demo"
	m.launchBrief.Focus()

	got, _ := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyCtrlT})
	next := got.(model)
	if !next.launchQueueOnly {
		t.Fatalf("launchQueueOnly = false, want true")
	}
	if next.statusMsg != "this run will be sent to Foreman" {
		t.Fatalf("statusMsg = %q, want Foreman toggle message", next.statusMsg)
	}
}

func TestUpdateAgentLaunchNoteAllowsUppercaseF(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{{Text: "keep draft state"}}
	m.launchFocus = m.launchNoteFocus()
	m.launchRepo = "/tmp/demo"
	m.launchBrief.Focus()

	got, _ := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	next := got.(model)
	if next.launchQueueOnly {
		t.Fatalf("launchQueueOnly = true, want false")
	}
	if !strings.Contains(next.launchBrief.Value(), "F") {
		t.Fatalf("launchBrief = %q, want typed uppercase F", next.launchBrief.Value())
	}
}

func TestUpdateAgentLaunchEnterFromNoteAdvancesToReview(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{{Text: "keep draft state"}}
	m.launchFocus = m.launchNoteFocus()
	m.launchRepo = "/tmp/demo"
	m.launchBrief.SetValue("tighten the launcher")
	m.launchBrief.Focus()

	got, cmd := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if next.launchFocus != next.launchReviewFocus() {
		t.Fatalf("launchFocus = %d, want review focus %d", next.launchFocus, next.launchReviewFocus())
	}
	if next.launchBrief.Value() != "tighten the launcher" {
		t.Fatalf("launchBrief = %q, want preserved note", next.launchBrief.Value())
	}
	if next.launchBrief.Focused() {
		t.Fatalf("launchBrief should blur after advancing to review")
	}
	if cmd != nil {
		t.Fatalf("enter from note should not return a command")
	}
}

func TestPrepareRetryLaunchPrefillsComposerFromJob(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentList
	m.width = 120
	m.cockpitPresets = []cockpit.LaunchPreset{
		{ID: "senior-dev", Name: "Senior dev", Permissions: "scoped-write", Executor: cockpit.ExecutorSpec{Type: "claude"}},
		{ID: "bug-fixer", Name: "Bug fixer", Permissions: "read-only", Executor: cockpit.ExecutorSpec{Type: "codex"}},
	}
	m.cockpitProviders = []cockpit.ProviderProfile{
		{ID: "claude-sonnet", Name: "Claude Sonnet", Executor: cockpit.ExecutorSpec{Type: "claude", Model: "claude-sonnet-4-6"}},
		{ID: "codex-gpt5", Name: "Codex GPT-5", Executor: cockpit.ExecutorSpec{Type: "codex", Model: "gpt-5"}},
	}

	job := cockpit.Job{
		PresetID:       "bug-fixer",
		Sources:        []cockpit.SourceTask{{File: "/tmp/demo/WORK.md", Line: 4, Text: "fix the edge case"}},
		Repo:           "/tmp/demo",
		Freeform:       "focus on the retry path",
		Executor:       cockpit.ExecutorSpec{Type: "codex", Model: "gpt-5"},
		Permissions:    "wide-open",
		ForemanManaged: true,
	}

	m.prepareRetryLaunch(job)

	if m.mode != modeAgentLaunch {
		t.Fatalf("mode = %v, want modeAgentLaunch", m.mode)
	}
	if m.launchPresetIdx != 1 {
		t.Fatalf("launchPresetIdx = %d, want 1", m.launchPresetIdx)
	}
	if m.launchProviderIdx != 1 {
		t.Fatalf("launchProviderIdx = %d, want 1", m.launchProviderIdx)
	}
	if m.launchPermsIdx != 3 {
		t.Fatalf("launchPermsIdx = %d, want 3 for wide-open override", m.launchPermsIdx)
	}
	if !m.launchQueueOnly {
		t.Fatalf("launchQueueOnly = false, want true")
	}
	if m.launchRepo != "/tmp/demo" {
		t.Fatalf("launchRepo = %q, want /tmp/demo", m.launchRepo)
	}
	if got := m.launchBrief.Value(); got != "focus on the retry path" {
		t.Fatalf("launchBrief = %q, want preserved retry note", got)
	}
	if len(m.launchSources) != 1 || m.launchSources[0].Text != "fix the edge case" {
		t.Fatalf("launchSources = %+v, want original task source", m.launchSources)
	}
	if m.launchFocus != m.launchNoteFocus() {
		t.Fatalf("launchFocus = %d, want note focus %d", m.launchFocus, m.launchNoteFocus())
	}
}

func TestPrepareRetryLaunchLeavesEngineAtRoleDefaultWhenExecutorMatchesPreset(t *testing.T) {
	m := newModel(nil)
	m.cockpitPresets = []cockpit.LaunchPreset{
		{ID: "senior-dev", Name: "Senior dev", Permissions: "scoped-write", Executor: cockpit.ExecutorSpec{Type: "codex", Model: "gpt-5"}},
	}
	m.cockpitProviders = []cockpit.ProviderProfile{
		{ID: "codex-gpt5", Name: "Codex GPT-5", Executor: cockpit.ExecutorSpec{Type: "codex", Model: "gpt-5"}},
	}

	job := cockpit.Job{
		PresetID:    "senior-dev",
		Repo:        "/tmp/demo",
		Executor:    cockpit.ExecutorSpec{Type: "codex", Model: "gpt-5"},
		Permissions: "scoped-write",
	}

	m.prepareRetryLaunch(job)

	if m.launchProviderIdx != -1 {
		t.Fatalf("launchProviderIdx = %d, want -1 for role default", m.launchProviderIdx)
	}
}

func TestUpdateAgentLaunchEditKeyAllowsTypedEngineDefault(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.launchFocus = launchFocusEngine
	m.launchRepo = "/tmp/demo"
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev", Executor: cockpit.ExecutorSpec{Type: "claude"}}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}}
	m.launchProviderIdx = 0

	got, cmd := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	next := got.(model)
	if !next.launchSelectEditing {
		t.Fatalf("launchSelectEditing = false, want true")
	}
	if cmd == nil {
		t.Fatalf("edit key should return input blink cmd")
	}

	next.launchSelectInput.SetValue("")
	got, _ = next.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	next = got.(model)
	if next.launchProviderIdx != -1 {
		t.Fatalf("launchProviderIdx = %d, want -1 after blank typed engine selection", next.launchProviderIdx)
	}
	if next.launchSelectEditing {
		t.Fatalf("launchSelectEditing = true, want false after apply")
	}
}

func TestUpdateAgentLaunchEditKeyAllowsTypedPromptNone(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.launchFocus = launchFocusPrompt
	m.launchRepo = "/tmp/demo"
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitPrompts = []cockpit.PromptTemplate{{ID: "bug-fixer", Name: "Bug fixer", Body: "fix bugs"}}
	m.launchPromptIdx = 0

	got, cmd := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	next := got.(model)
	if !next.launchSelectEditing {
		t.Fatalf("launchSelectEditing = false, want true")
	}
	if cmd == nil {
		t.Fatalf("edit key should return input blink cmd")
	}

	next.launchSelectInput.SetValue("")
	got, _ = next.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	next = got.(model)
	if next.launchPromptIdx != launchPromptNone {
		t.Fatalf("launchPromptIdx = %d, want %d for typed blank prompt", next.launchPromptIdx, launchPromptNone)
	}
}

func TestUpdateAgentListLowercaseRRetriesSelectedJob(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentList
	m.width = 120
	m.height = 40
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	job := cockpit.Job{
		ID:       "job-1",
		PresetID: "senior-dev",
		Status:   cockpit.StatusBlocked,
		Repo:     "/tmp/demo",
	}
	retried := cockpit.Job{
		ID:       "job-2",
		PresetID: "senior-dev",
		Status:   cockpit.StatusRunning,
		Repo:     "/tmp/demo",
	}
	retryCalls := 0
	m.cockpitClient = stubCockpitClient{
		jobs:        map[cockpit.JobID]cockpit.Job{job.ID: job, retried.ID: retried},
		retryResult: retried,
		retryCalls:  &retryCalls,
	}
	m.cockpitJobs = []cockpit.Job{job}

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	next := got.(model)
	if retryCalls != 1 {
		t.Fatalf("retryCalls = %d, want 1", retryCalls)
	}
	if next.statusMsg != "retried senior-dev" {
		t.Fatalf("statusMsg = %q, want retry confirmation", next.statusMsg)
	}
}

func TestUpdateCtrlCLeavesAttachedTranscriptForAgentList(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.attachedJobID = "job-1"
	m.cockpitClient = stubCockpitClient{
		jobs: map[cockpit.JobID]cockpit.Job{
			"job-1": {ID: "job-1", Runner: cockpit.RunnerTmux, Status: cockpit.StatusRunning},
		},
	}

	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	next := got.(model)
	if next.page != pageAgent {
		t.Fatalf("page = %v, want pageAgent", next.page)
	}
	if next.mode != modeAgentList {
		t.Fatalf("mode = %v, want modeAgentList", next.mode)
	}
	if next.attachedFocus != 0 {
		t.Fatalf("attachedFocus = %d, want 0", next.attachedFocus)
	}
}

func TestUpdateCtrlCLeavesAttachedInputForAgentList(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.attachedJobID = "job-1"
	m.attachedFocus = 1
	m.attachedInput.Focus()
	m.attachedInput.SetValue("draft follow-up")
	m.cockpitClient = stubCockpitClient{
		jobs: map[cockpit.JobID]cockpit.Job{
			"job-1": {ID: "job-1", Runner: cockpit.RunnerExec, Status: cockpit.StatusIdle},
		},
	}

	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	next := got.(model)
	if next.page != pageAgent {
		t.Fatalf("page = %v, want pageAgent", next.page)
	}
	if next.mode != modeAgentList {
		t.Fatalf("mode = %v, want modeAgentList", next.mode)
	}
	if next.attachedFocus != 0 {
		t.Fatalf("attachedFocus = %d, want 0", next.attachedFocus)
	}
	if next.attachedInput.Focused() {
		t.Fatalf("attached input should be blurred after ctrl+c")
	}
}

func TestSetAgentManageFieldValueAutoFillsPresetIDFromName(t *testing.T) {
	dir := t.TempDir()
	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir}
	m.cockpitPresets = []cockpit.LaunchPreset{{
		Name: "New role",
		Executor: cockpit.ExecutorSpec{
			Type: "codex",
		},
		Hooks: cockpit.HookSpec{
			Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot},
		},
	}}
	m.agentManageKind = "preset"

	field := -1
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "name" {
			field = i
			break
		}
	}
	if field < 0 {
		t.Fatalf("name field not found")
	}
	if err := m.setAgentManageFieldValue(0, field, "Senior Dev Narrow"); err != nil {
		t.Fatalf("setAgentManageFieldValue(name): %v", err)
	}
	if got := m.cockpitPresets[0].ID; got != "senior-dev-narrow" {
		t.Fatalf("preset ID = %q, want senior-dev-narrow", got)
	}
}

func TestEndAgentManageEditRenamesPresetFileAndKeepsSelectionOnEditedPreset(t *testing.T) {
	dir := t.TempDir()
	original := cockpit.LaunchPreset{
		ID:          "new-role",
		Name:        "New role",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SavePreset(dir, original); err != nil {
		t.Fatalf("SavePreset(original): %v", err)
	}

	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir, PromptsDir: dir, HooksDir: dir}
	m.cockpitPresets = []cockpit.LaunchPreset{original}
	m.agentManageKind = "preset"
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "id" {
			m.agentManageField = i
			break
		}
	}
	m.agentManageEditing = true
	m.agentManageEditor.SetValue("senior-dev-narrow")

	m.endAgentManageEdit(true)

	if m.agentManageEditing {
		t.Fatalf("agentManageEditing = true, want false")
	}
	if len(m.cockpitPresets) != 1 {
		t.Fatalf("cockpitPresets len = %d, want 1 after rename reload", len(m.cockpitPresets))
	}
	if got := m.cockpitPresets[0].ID; got != "senior-dev-narrow" {
		t.Fatalf("preset ID after save = %q, want senior-dev-narrow", got)
	}
	if got := m.cockpitPresets[m.agentManageCursor].ID; got != "senior-dev-narrow" {
		t.Fatalf("selected preset id = %q, want senior-dev-narrow", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "new-role.json")); !os.IsNotExist(err) {
		t.Fatalf("old preset file still exists or wrong error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "senior-dev-narrow.json")); err != nil {
		t.Fatalf("renamed preset file missing: %v", err)
	}
}

func TestEndAgentManageEditUpdatesPresetNameAfterReload(t *testing.T) {
	dir := t.TempDir()
	original := cockpit.LaunchPreset{
		ID:          "senior-dev",
		Name:        "Senior dev",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SavePreset(dir, original); err != nil {
		t.Fatalf("SavePreset(original): %v", err)
	}

	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir, PromptsDir: dir, HooksDir: dir}
	m.cockpitPresets = []cockpit.LaunchPreset{original}
	m.agentManageKind = "preset"
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "name" {
			m.agentManageField = i
			break
		}
	}
	m.agentManageEditing = true
	m.agentManageEditor.SetValue("Senior dev narrow")

	m.endAgentManageEdit(true)

	if len(m.cockpitPresets) != 1 {
		t.Fatalf("cockpitPresets len = %d, want 1 after name edit reload", len(m.cockpitPresets))
	}
	if got := m.cockpitPresets[m.agentManageCursor].Name; got != "Senior dev narrow" {
		t.Fatalf("selected preset name = %q, want Senior dev narrow", got)
	}
}

func TestEndAgentManageEditUpdatesPresetPromptAfterReload(t *testing.T) {
	dir := t.TempDir()
	promptA := cockpit.PromptTemplate{ID: "senior-dev", Name: "Senior dev", Body: "A"}
	promptB := cockpit.PromptTemplate{ID: "bug-fixer", Name: "Bug fixer", Body: "B"}
	if err := cockpit.SavePrompt(dir, promptA); err != nil {
		t.Fatalf("SavePrompt(promptA): %v", err)
	}
	if err := cockpit.SavePrompt(dir, promptB); err != nil {
		t.Fatalf("SavePrompt(promptB): %v", err)
	}
	provider := cockpit.ProviderProfile{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}
	if err := cockpit.SaveProvider(dir, provider); err != nil {
		t.Fatalf("SaveProvider(provider): %v", err)
	}
	preset := cockpit.LaunchPreset{
		ID:          "senior-dev",
		Name:        "Senior dev",
		PromptID:    "senior-dev",
		EngineID:    "codex",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SavePreset(dir, preset); err != nil {
		t.Fatalf("SavePreset(preset): %v", err)
	}

	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir, PromptsDir: dir, HooksDir: dir}
	m.cockpitPrompts = []cockpit.PromptTemplate{promptA, promptB}
	m.cockpitProviders = []cockpit.ProviderProfile{provider}
	resolved, err := cockpit.ResolvePreset(preset, m.cockpitPrompts, nil, m.cockpitProviders)
	if err != nil {
		t.Fatalf("ResolvePreset(preset): %v", err)
	}
	m.cockpitPresets = []cockpit.LaunchPreset{resolved}
	m.agentManageKind = "preset"
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "prompt_id" {
			m.agentManageField = i
			break
		}
	}
	m.agentManageEditing = true
	m.agentManageEditor.SetValue("bug-fixer")

	m.endAgentManageEdit(true)

	if got := m.cockpitPresets[m.agentManageCursor].PromptID; got != "bug-fixer" {
		t.Fatalf("selected preset prompt_id = %q, want bug-fixer", got)
	}
	if got := m.cockpitPresets[m.agentManageCursor].SystemPrompt; got != "B" {
		t.Fatalf("selected preset system prompt = %q, want B", got)
	}
}

func TestUpdateAgentManageEnterOpensPromptSelectorOverlay(t *testing.T) {
	dir := t.TempDir()
	promptA := cockpit.PromptTemplate{ID: "senior-dev", Name: "Senior dev", Body: "A"}
	promptB := cockpit.PromptTemplate{ID: "bug-fixer", Name: "Bug fixer", Body: "B"}
	if err := cockpit.SavePrompt(dir, promptA); err != nil {
		t.Fatalf("SavePrompt(promptA): %v", err)
	}
	if err := cockpit.SavePrompt(dir, promptB); err != nil {
		t.Fatalf("SavePrompt(promptB): %v", err)
	}
	provider := cockpit.ProviderProfile{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}
	if err := cockpit.SaveProvider(dir, provider); err != nil {
		t.Fatalf("SaveProvider(provider): %v", err)
	}
	preset := cockpit.LaunchPreset{
		ID:          "senior-dev",
		Name:        "Senior dev",
		PromptID:    "senior-dev",
		EngineID:    "codex",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SavePreset(dir, preset); err != nil {
		t.Fatalf("SavePreset(preset): %v", err)
	}

	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir, PromptsDir: dir, HooksDir: dir}
	m.cockpitPrompts = []cockpit.PromptTemplate{promptA, promptB}
	m.cockpitProviders = []cockpit.ProviderProfile{provider}
	resolved, err := cockpit.ResolvePreset(preset, m.cockpitPrompts, nil, m.cockpitProviders)
	if err != nil {
		t.Fatalf("ResolvePreset(preset): %v", err)
	}
	m.cockpitPresets = []cockpit.LaunchPreset{resolved}
	m.mode = modeAgentManage
	m.agentManageKind = "preset"
	m.agentManageFocus = 1
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "prompt_id" {
			m.agentManageField = i
			break
		}
	}

	got, _ := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if !next.agentManageSelectEditing {
		t.Fatalf("agentManageSelectEditing = false, want true")
	}
	if got := next.agentManageSelectInput.Value(); got != "senior-dev" {
		t.Fatalf("selector value = %q, want existing prompt id", got)
	}
}

func TestUpdateAgentManagePromptSelectorClearsPromptOnBlank(t *testing.T) {
	dir := t.TempDir()
	prompt := cockpit.PromptTemplate{ID: "senior-dev", Name: "Senior dev", Body: "A"}
	provider := cockpit.ProviderProfile{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}
	preset := cockpit.LaunchPreset{
		ID:          "senior-dev",
		Name:        "Senior dev",
		PromptID:    "senior-dev",
		EngineID:    "codex",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SavePrompt(dir, prompt); err != nil {
		t.Fatalf("SavePrompt(prompt): %v", err)
	}
	if err := cockpit.SaveProvider(dir, provider); err != nil {
		t.Fatalf("SaveProvider(provider): %v", err)
	}
	if err := cockpit.SavePreset(dir, preset); err != nil {
		t.Fatalf("SavePreset(preset): %v", err)
	}

	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir, PromptsDir: dir, HooksDir: dir}
	m.cockpitPrompts = []cockpit.PromptTemplate{prompt}
	m.cockpitProviders = []cockpit.ProviderProfile{provider}
	resolved, err := cockpit.ResolvePreset(preset, m.cockpitPrompts, nil, m.cockpitProviders)
	if err != nil {
		t.Fatalf("ResolvePreset(preset): %v", err)
	}
	m.cockpitPresets = []cockpit.LaunchPreset{resolved}
	m.mode = modeAgentManage
	m.agentManageKind = "preset"
	m.agentManageFocus = 1
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "prompt_id" {
			m.agentManageField = i
			break
		}
	}

	got, cmd := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)
	if !next.agentManageSelectEditing {
		t.Fatalf("agentManageSelectEditing = false, want true")
	}
	if cmd == nil {
		t.Fatalf("enter should open selector overlay")
	}

	next.agentManageSelectInput.SetValue("")
	got, _ = next.updateAgentManage(tea.KeyMsg{Type: tea.KeyEnter})
	next = got.(model)

	if got := next.cockpitPresets[next.agentManageCursor].PromptID; got != "" {
		t.Fatalf("prompt_id = %q, want empty", got)
	}
	if got := next.cockpitPresets[next.agentManageCursor].SystemPrompt; got != "" {
		t.Fatalf("system prompt = %q, want empty", got)
	}
}

func TestUpdateAgentManageEditKeyOpensSelectorForPresetEngineField(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentManage
	m.agentManageKind = "preset"
	m.agentManageFocus = 1
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}}
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:          "senior-dev",
		Name:        "Senior dev",
		EngineID:    "codex",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}}
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "engine_id" {
			m.agentManageField = i
			break
		}
	}

	got, cmd := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	next := got.(model)

	if !next.agentManageSelectEditing {
		t.Fatalf("agentManageSelectEditing = false, want true")
	}
	if next.agentManageEditing {
		t.Fatalf("agentManageEditing = true, want false")
	}
	if got := next.agentManageSelectInput.Value(); got != "codex" {
		t.Fatalf("selector value = %q, want codex", got)
	}
	if cmd == nil {
		t.Fatalf("edit key should return selector blink cmd")
	}
}

func TestUpdateAgentManageHookBundleFieldUsesSelectorAndStoresMultipleIDs(t *testing.T) {
	dir := t.TempDir()
	bundleA := cockpit.HookBundle{ID: "diff-stat", Name: "Diff stat"}
	bundleB := cockpit.HookBundle{ID: "git-status", Name: "Git status"}
	preset := cockpit.LaunchPreset{
		ID:          "senior-dev",
		Name:        "Senior dev",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SaveHookBundle(dir, bundleA); err != nil {
		t.Fatalf("SaveHookBundle(bundleA): %v", err)
	}
	if err := cockpit.SaveHookBundle(dir, bundleB); err != nil {
		t.Fatalf("SaveHookBundle(bundleB): %v", err)
	}
	if err := cockpit.SavePreset(dir, preset); err != nil {
		t.Fatalf("SavePreset(preset): %v", err)
	}

	m := newModel(nil)
	m.mode = modeAgentManage
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, HooksDir: dir}
	m.cockpitHookBundles = []cockpit.HookBundle{bundleA, bundleB}
	m.cockpitPresets = []cockpit.LaunchPreset{preset}
	m.agentManageKind = "preset"
	m.agentManageFocus = 1
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "hook_bundle_id" {
			m.agentManageField = i
			break
		}
	}

	got, cmd := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	next := got.(model)
	if !next.agentManageSelectEditing {
		t.Fatalf("agentManageSelectEditing = false, want true")
	}
	if next.agentManageEditing {
		t.Fatalf("agentManageEditing = true, want false")
	}
	if cmd == nil {
		t.Fatalf("selector edit should return cursor blink cmd")
	}

	next.agentManageSelectInput.SetValue("Diff stat, Git status")
	got, _ = next.updateAgentManage(tea.KeyMsg{Type: tea.KeyEnter})
	next = got.(model)
	if next.agentManageSelectEditing {
		t.Fatalf("agentManageSelectEditing = true, want false after apply")
	}
	gotPreset := next.cockpitPresets[next.agentManageCursor]
	if len(gotPreset.HookBundleIDs) != 2 || gotPreset.HookBundleIDs[0] != "diff-stat" || gotPreset.HookBundleIDs[1] != "git-status" {
		t.Fatalf("HookBundleIDs = %#v, want diff-stat + git-status", gotPreset.HookBundleIDs)
	}
}

func TestBeginAgentManageEditSeedsEmptyHookJSONWithArray(t *testing.T) {
	m := newModel(nil)
	m.cockpitHookBundles = []cockpit.HookBundle{{ID: "hooks", Name: "Hooks"}}
	m.agentManageKind = "hookbundle"
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "prompt" {
			m.agentManageField = i
			break
		}
	}

	cmd := m.beginAgentManageEdit()
	if !m.agentManageEditing {
		t.Fatalf("agentManageEditing = false, want true")
	}
	if got := m.agentManageEditor.Value(); got != "[]" {
		t.Fatalf("editor value = %q, want [] seed for empty hook arrays", got)
	}
	if cmd == nil {
		t.Fatalf("beginAgentManageEdit should return blink cmd")
	}
}

func TestUpdateAgentManageOpensStructuredHookOverlay(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentManage
	m.agentManageKind = "hookbundle"
	m.agentManageFocus = 1
	m.cockpitHookBundles = []cockpit.HookBundle{{ID: "hooks", Name: "Hooks"}}
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "pre_shell" {
			m.agentManageField = i
			break
		}
	}

	got, _ := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if !next.agentManageHookEditing {
		t.Fatalf("agentManageHookEditing = false, want true")
	}
	if next.agentManageHookArrayKey != "pre_shell" {
		t.Fatalf("agentManageHookArrayKey = %q, want pre_shell", next.agentManageHookArrayKey)
	}
}

func TestStructuredHookOverlaySavesShellHookCommand(t *testing.T) {
	dir := t.TempDir()
	original := cockpit.HookBundle{
		ID:   "hooks",
		Name: "Hooks",
	}
	if err := cockpit.SaveHookBundle(dir, original); err != nil {
		t.Fatalf("SaveHookBundle(original): %v", err)
	}

	m := newModel(nil)
	m.mode = modeAgentManage
	m.cockpitPaths = cockpit.Paths{HooksDir: dir}
	m.cockpitHookBundles = []cockpit.HookBundle{original}
	m.agentManageKind = "hookbundle"
	m.agentManageCursor = 0
	m.agentManageHookEditing = true
	m.agentManageHookArrayKey = "pre_shell"
	m.addAgentManageHookRow()
	m.agentManageHookField = 1 // cmd

	cmd := m.beginAgentManageHookFieldEdit()
	if cmd == nil {
		t.Fatalf("beginAgentManageHookFieldEdit should return blink cmd")
	}
	m.agentManageEditor.SetValue("git status --short")
	m.endAgentManageHookFieldEdit(true)
	m.endAgentManageHookEdit(true)

	if len(m.cockpitHookBundles) != 1 {
		t.Fatalf("cockpitHookBundles len = %d, want 1", len(m.cockpitHookBundles))
	}
	if got := m.cockpitHookBundles[0].PreShell[0].Cmd; got != "git status --short" {
		t.Fatalf("PreShell[0].Cmd = %q, want git status --short", got)
	}
}

func TestStructuredHookOverlayCanReorderAndDuplicateRows(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentManage
	m.agentManageHookEditing = true
	m.agentManageHookArrayKey = "pre_shell"
	m.agentManageShellDraft = []cockpit.ShellHook{
		{Name: "First", Cmd: "first"},
		{Name: "Second", Cmd: "second"},
	}
	m.agentManageHookCursor = 1
	m.agentManageHookFocus = 0

	got, _ := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	next := got.(model)
	if next.agentManageShellDraft[0].Name != "Second" || next.agentManageHookCursor != 0 {
		t.Fatalf("reorder up failed: %#v cursor=%d", next.agentManageShellDraft, next.agentManageHookCursor)
	}

	got, _ = next.updateAgentManage(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	next = got.(model)
	if len(next.agentManageShellDraft) != 3 {
		t.Fatalf("len(agentManageShellDraft) = %d, want 3", len(next.agentManageShellDraft))
	}
	if next.agentManageShellDraft[1].Name != "Second" {
		t.Fatalf("duplicated row = %#v, want duplicate inserted after current row", next.agentManageShellDraft)
	}
}

func TestUpdateAgentManageEnterSavesSingleLineEdit(t *testing.T) {
	dir := t.TempDir()
	original := cockpit.LaunchPreset{
		ID:          "senior-dev",
		Name:        "Senior dev",
		LaunchMode:  cockpit.LaunchModeSingleJob,
		Permissions: "scoped-write",
	}
	if err := cockpit.SavePreset(dir, original); err != nil {
		t.Fatalf("SavePreset(original): %v", err)
	}

	m := newModel(nil)
	m.mode = modeAgentManage
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir, PromptsDir: dir, HooksDir: dir}
	m.cockpitPresets = []cockpit.LaunchPreset{original}
	m.agentManageKind = "preset"
	m.agentManageCursor = 0
	for i, spec := range m.agentManageFieldSpecs() {
		if spec.Key == "name" {
			m.agentManageField = i
			break
		}
	}
	m.agentManageEditing = true
	m.agentManageEditor.SetValue("Senior dev trimmed")

	got, _ := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if next.agentManageEditing {
		t.Fatalf("agentManageEditing = true, want false")
	}
	if got := next.cockpitPresets[next.agentManageCursor].Name; got != "Senior dev trimmed" {
		t.Fatalf("selected preset name = %q, want Senior dev trimmed", got)
	}
}

func TestOpenCurrentProjectPickerUsesSelectedProject(t *testing.T) {
	dir := t.TempDir()
	workFile := filepath.Join(dir, "WORK.md")
	if err := os.WriteFile(workFile, []byte("# WORK - demo\n\n## Current Tasks\n- first item\n- second item\n"), 0o644); err != nil {
		t.Fatalf("write WORK.md: %v", err)
	}

	m := newModel(nil)
	m.projects = []workmd.Project{{
		Name: "demo",
		Path: workFile,
		Dir:  dir,
	}}
	m.selected = 0

	if ok := m.openCurrentProjectPicker(); !ok {
		t.Fatalf("openCurrentProjectPicker returned false")
	}
	if m.pickerFile != workFile {
		t.Fatalf("pickerFile = %q, want %q", m.pickerFile, workFile)
	}
	if len(m.pickerItems) != 2 {
		t.Fatalf("pickerItems len = %d, want 2", len(m.pickerItems))
	}
}

func TestUpdateAgentListClampsCursorBeforeActions(t *testing.T) {
	m := newModel(nil)
	m.cockpitJobs = []cockpit.Job{
		{ID: "done", Status: cockpit.StatusCompleted},
		{ID: "live", Status: cockpit.StatusRunning},
	}
	m.agentFilter = "running"
	m.agentCursor = 1

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	next := got.(model)
	if next.agentCursor != 0 {
		t.Fatalf("agentCursor = %d, want 0 after clamp", next.agentCursor)
	}
}

func TestUpdateAgentListNStartsPickerAtStepOne(t *testing.T) {
	m := newModel(nil)
	m.projects = []workmd.Project{{Name: "demo"}}
	m.pickerFile = "/tmp/already-picked.md"
	m.pickerItems = []cockpit.PickerItem{{Line: 4, Text: "stale"}}
	m.pickerSelected = map[int]bool{0: true}
	m.launchSources = []cockpit.SourceTask{{File: "/tmp/already-picked.md", Line: 4, Text: "stale"}}
	m.launchRepo = "/tmp/repo"
	m.agentCursor = 3

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	next := got.(model)
	if next.mode != modeAgentPicker {
		t.Fatalf("mode = %v, want modeAgentPicker", next.mode)
	}
	if next.pickerFile != "" {
		t.Fatalf("pickerFile = %q, want empty", next.pickerFile)
	}
	if len(next.pickerItems) != 0 {
		t.Fatalf("pickerItems len = %d, want 0", len(next.pickerItems))
	}
	if countSelected(next.pickerSelected) != 0 {
		t.Fatalf("pickerSelected count = %d, want 0", countSelected(next.pickerSelected))
	}
	if len(next.launchSources) != 0 {
		t.Fatalf("launchSources len = %d, want 0", len(next.launchSources))
	}
	if next.agentCursor != 0 {
		t.Fatalf("agentCursor = %d, want 0", next.agentCursor)
	}
}

func TestAgentPickerBackClearsStepTwoState(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentPicker
	m.pickerFile = "/tmp/demo.md"
	m.pickerItems = []cockpit.PickerItem{{Line: 4, Text: "one"}}
	m.pickerSelected = map[int]bool{0: true}
	m.pickerProject = "demo"
	m.pickerRepo = "/tmp/repo"
	m.agentCursor = 2

	got, _ := m.updateAgentPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	next := got.(model)
	if next.pickerFile != "" {
		t.Fatalf("pickerFile = %q, want empty", next.pickerFile)
	}
	if len(next.pickerItems) != 0 {
		t.Fatalf("pickerItems len = %d, want 0", len(next.pickerItems))
	}
	if countSelected(next.pickerSelected) != 0 {
		t.Fatalf("pickerSelected count = %d, want 0", countSelected(next.pickerSelected))
	}
	if next.pickerProject != "" || next.pickerRepo != "" {
		t.Fatalf("picker project/repo = %q/%q, want empty", next.pickerProject, next.pickerRepo)
	}
	if next.agentCursor != 0 {
		t.Fatalf("agentCursor = %d, want 0", next.agentCursor)
	}
}

func TestHandleAgentMouseWheelUsesPickerAndLaunchLists(t *testing.T) {
	m := newModel(nil)
	m.projects = []workmd.Project{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.mode = modeAgentPicker
	m.agentCursor = 0
	next := m.handleAgentMouseWheel(1).(model)
	if next.agentCursor != 1 {
		t.Fatalf("picker step1 cursor = %d, want 1", next.agentCursor)
	}

	m.pickerFile = "/tmp/demo.md"
	m.pickerItems = []cockpit.PickerItem{{Text: "one"}, {Text: "two"}}
	m.agentCursor = 0
	next = m.handleAgentMouseWheel(1).(model)
	if next.agentCursor != 1 {
		t.Fatalf("picker step2 cursor = %d, want 1", next.agentCursor)
	}

	m.mode = modeAgentLaunch
	m.launchFocus = 0
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "one"}, {ID: "two"}}
	m.launchPresetIdx = 0
	next = m.handleAgentMouseWheel(1).(model)
	if next.launchPresetIdx != 1 {
		t.Fatalf("launchPresetIdx = %d, want 1", next.launchPresetIdx)
	}

	m.launchFocus = 1
	m.cockpitProviders = []cockpit.ProviderProfile{{Name: "codex"}, {Name: "claude"}}
	m.launchProviderIdx = 0
	next = m.handleAgentMouseWheel(1).(model)
	if next.launchProviderIdx != 1 {
		t.Fatalf("launchProviderIdx = %d, want 1", next.launchProviderIdx)
	}

	m.launchSources = []cockpit.SourceTask{{Text: "task"}}
	m.launchFocus = m.launchReviewFocus()
	m.launchReviewOffset = 0
	next = m.handleAgentMouseWheel(1).(model)
	if next.launchReviewOffset != 1 {
		t.Fatalf("sourced launchReviewOffset = %d, want 1", next.launchReviewOffset)
	}

	m.launchSources = nil
	m.projects = []workmd.Project{{Dir: "/tmp/a"}, {Dir: "/tmp/b"}}
	m.launchRepo = "/tmp/a"
	m.launchFocus = m.launchRepoFocus()
	next = m.handleAgentMouseWheel(1).(model)
	if next.launchRepo == "/tmp/a" {
		t.Fatalf("launchRepo = %q, want repo selection to advance", next.launchRepo)
	}

	m.launchFocus = m.launchReviewFocus()
	m.launchReviewOffset = 0
	next = m.handleAgentMouseWheel(1).(model)
	if next.launchReviewOffset != 1 {
		t.Fatalf("launchReviewOffset = %d, want 1", next.launchReviewOffset)
	}
}

func TestUpdateAgentLaunchEnterOnRepoStepAdvancesToNoteInsteadOfLaunching(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.cockpitClient = stubCockpitClient{}
	m.projects = []workmd.Project{{Dir: "/tmp/a"}, {Dir: "/tmp/b"}}
	m.launchSources = nil
	m.launchRepo = "/tmp/a"
	m.launchFocus = m.launchRepoFocus()

	got, cmd := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if next.mode != modeAgentLaunch {
		t.Fatalf("mode = %v, want modeAgentLaunch", next.mode)
	}
	if next.launchFocus != next.launchNoteFocus() {
		t.Fatalf("launchFocus = %d, want note focus %d", next.launchFocus, next.launchNoteFocus())
	}
	if next.launchRepo != "/tmp/a" {
		t.Fatalf("launchRepo = %q, want /tmp/a", next.launchRepo)
	}
	if cmd == nil {
		t.Fatalf("expected note-focus blink cmd")
	}
}

func TestUpdateAgentLaunchEnterOnRepoStepAppliesVisibleDefaultChoice(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.cockpitClient = stubCockpitClient{}
	m.projects = []workmd.Project{{Dir: "/tmp/a"}, {Dir: "/tmp/b"}}
	m.launchSources = nil
	m.launchRepo = ""
	m.launchFocus = m.launchRepoFocus()

	got, _ := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if next.launchRepo != "/tmp/a" {
		t.Fatalf("launchRepo = %q, want default repo /tmp/a even with custom-path row first", next.launchRepo)
	}
	if next.launchFocus != next.launchNoteFocus() {
		t.Fatalf("launchFocus = %d, want note focus %d", next.launchFocus, next.launchNoteFocus())
	}
}

func TestLaunchRepoChoicesPutCustomPathFirstWithoutChangingDefaultSelection(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.projects = []workmd.Project{{Dir: "/tmp/a"}, {Dir: "/tmp/b"}}
	m.launchSources = nil
	m.launchRepo = ""

	repos := m.launchRepoChoices()
	if len(repos) < 3 {
		t.Fatalf("launchRepoChoices len = %d, want at least 3", len(repos))
	}
	if repos[0] != repoSentinelCustom {
		t.Fatalf("launchRepoChoices[0] = %q, want custom-path sentinel first", repos[0])
	}
	if repos[1] != "/tmp/a" {
		t.Fatalf("launchRepoChoices[1] = %q, want default repo /tmp/a second", repos[1])
	}
	if got := indexOfLaunchRepo(repos, m.launchRepo); got != 1 {
		t.Fatalf("indexOfLaunchRepo(empty) = %d, want 1 so default selection starts on the second row", got)
	}
}

func TestUpdateAgentLaunchCustomRepoEnterSetsRepoAndAdvancesToNote(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.launchSources = nil
	m.launchFocus = m.launchRepoFocus()
	m.launchRepo = repoSentinelCustom
	m.launchRepoEditing = true
	m.launchRepoCustom.SetValue("/tmp/custom-repo")

	got, cmd := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	next := got.(model)

	if next.launchRepoEditing {
		t.Fatalf("launchRepoEditing = true, want false")
	}
	if next.launchRepo != "/tmp/custom-repo" {
		t.Fatalf("launchRepo = %q, want /tmp/custom-repo", next.launchRepo)
	}
	if next.launchFocus != next.launchNoteFocus() {
		t.Fatalf("launchFocus = %d, want note focus %d", next.launchFocus, next.launchNoteFocus())
	}
	if cmd == nil {
		t.Fatalf("expected note-focus blink cmd")
	}
}

func TestUpdateRoutesTypingIntoCustomRepoInput(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.cockpitClient = stubCockpitClient{}
	m.launchSources = nil
	m.launchFocus = m.launchRepoFocus()
	m.launchRepo = repoSentinelCustom
	m.launchRepoEditing = true
	m.launchRepoCustom.Focus()

	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/tmp/repo")})
	next := got.(model)

	if next.launchRepoCustom.Value() != "/tmp/repo" {
		t.Fatalf("launchRepoCustom = %q, want /tmp/repo", next.launchRepoCustom.Value())
	}
	if !next.launchRepoEditing {
		t.Fatalf("launchRepoEditing = false, want true")
	}
}

func TestLaunchRepoChoicesReachCustomPathWithOneMoveUpFromDefault(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.projects = []workmd.Project{
		{Dir: "/tmp/a"},
		{Dir: "/tmp/b"},
		{Dir: "/tmp/c"},
	}
	m.launchSources = nil
	m.launchRepo = ""
	m.launchFocus = m.launchRepoFocus()

	next := m.handleAgentMouseWheel(-1).(model)
	m = next

	if m.launchRepo != repoSentinelCustom {
		t.Fatalf("launchRepo = %q, want custom-path sentinel after one move up", m.launchRepo)
	}
}

func TestUpdateAgentListRLoadsRetrySetupIntoComposer(t *testing.T) {
	m := newModel(nil)
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Status:    cockpit.StatusIdle,
		Repo:      "/tmp/demo",
		Freeform:  "retry this",
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitJobs = []cockpit.Job{job}

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	next := got.(model)
	if next.mode != modeAgentLaunch {
		t.Fatalf("mode = %v, want modeAgentLaunch", next.mode)
	}
	if next.launchRepo != "/tmp/demo" {
		t.Fatalf("launchRepo = %q, want /tmp/demo", next.launchRepo)
	}
	if next.launchBrief.Value() != "retry this" {
		t.Fatalf("launchBrief = %q, want retry note", next.launchBrief.Value())
	}
}

func TestUpdateAgentListRLoadsForemanRetrySetupIntoComposer(t *testing.T) {
	m := newModel(nil)
	job := cockpit.Job{
		ID:             "job-queued",
		PresetID:       "senior-dev",
		Status:         cockpit.StatusQueued,
		WaitForForeman: true,
		ForemanManaged: true,
		CreatedAt:      time.Now().Add(-1 * time.Minute),
	}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitJobs = []cockpit.Job{job}

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	next := got.(model)
	if next.mode != modeAgentLaunch {
		t.Fatalf("mode = %v, want modeAgentLaunch", next.mode)
	}
	if !next.launchQueueOnly {
		t.Fatalf("launchQueueOnly = false, want true")
	}
}

func TestUpdateAgentListCtrlRArmsTakeoverConfirm(t *testing.T) {
	m := newModel(nil)
	job := cockpit.Job{
		ID:             "job-foreman",
		PresetID:       "senior-dev",
		Status:         cockpit.StatusIdle,
		Runner:         cockpit.RunnerTmux,
		TmuxTarget:     "sb-cockpit:@3",
		ForemanManaged: true,
		CreatedAt:      time.Now().Add(-1 * time.Minute),
	}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyCtrlR})
	next := got.(model)
	if !next.agentConfirmActive || next.agentConfirmKind != "takeover" || next.agentConfirmTarget != job.ID {
		t.Fatalf("takeover confirm not armed: %+v", next)
	}
	if !strings.Contains(next.statusMsg, "take over "+string(job.ID)) {
		t.Fatalf("statusMsg = %q, want takeover prompt", next.statusMsg)
	}
}

func TestGlobalCtrlRUsesPendingTmuxTakeoverTarget(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
case "$1" in
  show-environment)
    printf 'SB_TAKEOVER_TARGET=sb-cockpit:@3\n'
    ;;
  set-environment)
    ;;
esac
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	m := newModel(nil)
	job := cockpit.Job{
		ID:             "job-foreman",
		PresetID:       "senior-dev",
		Status:         cockpit.StatusIdle,
		Runner:         cockpit.RunnerTmux,
		TmuxTarget:     "sb-cockpit:@3",
		ForemanManaged: true,
		CreatedAt:      time.Now().Add(-1 * time.Minute),
	}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m.cockpitJobs = []cockpit.Job{job}
	m.page = pageDashboard
	m.mode = modeNormal

	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	next := got.(model)
	if next.page != pageAgent || next.mode != modeAgentList {
		t.Fatalf("page/mode = %v/%v, want agent list", next.page, next.mode)
	}
	if next.agentConfirmKind != "takeover" || next.agentConfirmTarget != job.ID {
		t.Fatalf("takeover confirm = %q %q", next.agentConfirmKind, next.agentConfirmTarget)
	}
}

func TestOpenAgentJobQueuedTmuxReportsQueueReasonInsteadOfMissingWindow(t *testing.T) {
	m := newModel(nil)
	job := cockpit.Job{
		ID:                "job-queued",
		PresetID:          "senior-dev",
		Runner:            cockpit.RunnerTmux,
		Status:            cockpit.StatusQueued,
		WaitForForeman:    true,
		EligibilityReason: "repo busy: abc123",
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	m.cockpitClient = stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}

	got, cmd := m.openAgentJob(job.ID, false)
	next := got.(model)
	if !strings.Contains(next.statusMsg, "job queued: repo busy: abc123") {
		t.Fatalf("statusMsg = %q, want queue reason", next.statusMsg)
	}
	if cmd == nil {
		t.Fatalf("expected refresh cmd for queued tmux job")
	}
}

func TestUpdateAgentListDetailPanePagingUsesNaturalDirection(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 12
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "transcript.txt")
	if err := os.WriteFile(transcriptPath, []byte(strings.Repeat("line\n", 200)), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	job := cockpit.Job{
		ID:             "job-1",
		PresetID:       "senior-dev",
		Status:         cockpit.StatusNeedsReview,
		CreatedAt:      time.Now().Add(-1 * time.Minute),
		TranscriptPath: transcriptPath,
	}
	m.cockpitJobs = []cockpit.Job{job}
	m.agentDetailOffset = 10

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyPgUp})
	next := got.(model)
	if next.agentDetailOffset != 5 {
		t.Fatalf("agentDetailOffset after pgup = %d, want 5", next.agentDetailOffset)
	}

	got, _ = next.updateAgentList(tea.KeyMsg{Type: tea.KeyPgDown})
	next = got.(model)
	if next.agentDetailOffset != 10 {
		t.Fatalf("agentDetailOffset after pgdown = %d, want 10", next.agentDetailOffset)
	}
}

func TestUpdateAgentListDetailPaneClampsOverscroll(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 18
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "transcript.txt")
	if err := os.WriteFile(transcriptPath, []byte(strings.Repeat("line\n", 200)), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	job := cockpit.Job{
		ID:             "job-1",
		PresetID:       "senior-dev",
		Status:         cockpit.StatusNeedsReview,
		CreatedAt:      time.Now().Add(-1 * time.Minute),
		TranscriptPath: transcriptPath,
	}
	m.cockpitJobs = []cockpit.Job{job}
	m.agentDetailOffset = 999

	got, _ := m.updateAgentList(tea.KeyMsg{Type: tea.KeyPgDown})
	next := got.(model)
	prefixLines := 2
	_, _, rightWidth, _ := next.agentListLayout(prefixLines)
	bodyWidth := rightWidth - 8
	if bodyWidth < 24 {
		bodyWidth = 24
	}
	maxOffset := clampDecoratedScrollOffset(999, len(jobPeekBody(job, bodyWidth)), next.agentDetailVisibleBody(job))
	if next.agentDetailOffset != maxOffset {
		t.Fatalf("agentDetailOffset = %d, want clamped %d", next.agentDetailOffset, maxOffset)
	}
}

func TestUpdateAgentLaunchReviewClampsOverscroll(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 16
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{
		{Project: "demo", File: "/tmp/demo/WORK.md", Line: 1, Text: "one"},
		{Project: "demo", File: "/tmp/demo/WORK.md", Line: 2, Text: "two"},
		{Project: "demo", File: "/tmp/demo/WORK.md", Line: 3, Text: "three"},
		{Project: "demo", File: "/tmp/demo/WORK.md", Line: 4, Text: "four"},
	}
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev"}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex"}}
	m.launchFocus = m.launchReviewFocus()
	m.launchReviewOffset = 999

	got, _ := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyPgDown})
	next := got.(model)
	maxOffset := clampDecoratedScrollOffset(999, len(launchReviewLines(next)), next.launchReviewVisibleRows())
	if next.launchReviewOffset != maxOffset {
		t.Fatalf("launchReviewOffset = %d, want clamped %d", next.launchReviewOffset, maxOffset)
	}
}

func TestClampAgentManageOffsetsUsesVisibleWindow(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 16
	m.mode = modeAgentManage
	m.agentManageKind = "preset"
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "a", Name: "A", Executor: cockpit.ExecutorSpec{Type: "codex"}}}
	m.agentManageListOffset = 999
	m.agentManageDetailOffset = 999

	m.clampAgentManageOffsets()

	if m.agentManageListOffset != 0 {
		t.Fatalf("agentManageListOffset = %d, want 0 with one visible item", m.agentManageListOffset)
	}
	maxDetail := clampDecoratedScrollOffset(999, len(m.agentManageFieldSpecs()), m.agentManageDetailVisibleRows())
	if m.agentManageDetailOffset != maxDetail {
		t.Fatalf("agentManageDetailOffset = %d, want clamped %d", m.agentManageDetailOffset, maxDetail)
	}
}

func TestHelpScrollClampsToVisibleWindow(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 16
	m.mode = modeHelp
	m.helpScroll = 999

	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	next := got.(model)
	maxOffset := clampScrollOffset(999, len(next.helpLines()), next.helpVisibleHeight())
	if next.helpScroll != maxOffset {
		t.Fatalf("helpScroll = %d, want clamped %d", next.helpScroll, maxOffset)
	}
}

func TestUpdateAgentManagePagesAndScrollsFieldList(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentManage
	m.width = 80
	m.height = 12
	m.agentManageKind = "preset"
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "a", Name: "A", Executor: cockpit.ExecutorSpec{Type: "codex"}}}
	m.agentManageFocus = 1
	m.agentManageField = 0

	got, _ := m.updateAgentManage(tea.KeyMsg{Type: tea.KeyPgDown})
	next := got.(model)
	if next.agentManageField <= 0 {
		t.Fatalf("agentManageField = %d, want > 0 after pgdown", next.agentManageField)
	}
	if next.agentManageDetailOffset < 0 {
		t.Fatalf("agentManageDetailOffset = %d, want non-negative after pgdown", next.agentManageDetailOffset)
	}
}

func TestHeaderTabAtFindsTopNavTabs(t *testing.T) {
	m := newModel(nil)

	if got, ok := m.headerTabAt(lipgloss.Width(titleStyle.Render("sb"))+2, 0); !ok || got != pageDashboard {
		t.Fatalf("dashboard hit = (%v, %v), want (%v, true)", got, ok, pageDashboard)
	}
	if got, ok := m.headerTabAt(lipgloss.Width(titleStyle.Render("sb"))+2+len("Dashboard")+len(" │ "), 0); !ok || got != pageDump {
		t.Fatalf("dump hit = (%v, %v), want (%v, true)", got, ok, pageDump)
	}
	if _, ok := m.headerTabAt(0, 1); ok {
		t.Fatalf("non-header row should not hit a tab")
	}
}

func TestJobListSummaryCompactsMultilineTask(t *testing.T) {
	j := cockpit.Job{
		Task: "first line\nsecond line\tthird bit",
	}
	if got := jobListSummary(j); got != "first line second line third bit" {
		t.Fatalf("jobListSummary() = %q", got)
	}
}

func TestMouseClickOnHeaderSwitchesPages(t *testing.T) {
	m := newModel(nil)
	m.page = pageProject
	m.mode = modeEdit
	m.projects = []workmd.Project{{Name: "demo", Content: "# demo"}}

	dumpX := lipgloss.Width(titleStyle.Render("sb")) + 2 + len("Dashboard") + len(" │ ")
	got, cmd := m.Update(tea.MouseMsg{
		X:      dumpX,
		Y:      0,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	next := got.(model)
	if next.page != pageDump {
		t.Fatalf("page = %v, want %v", next.page, pageDump)
	}
	if next.mode != modeDumpInput {
		t.Fatalf("mode = %v, want %v", next.mode, modeDumpInput)
	}
	if cmd == nil {
		t.Fatalf("dump tab click should return a focus cmd")
	}
}

func TestUpdateDashboardPageKeysMoveProjectCursor(t *testing.T) {
	m := newModel(nil)
	m.page = pageDashboard
	m.mode = modeNormal
	m.height = 20
	m.projects = []workmd.Project{
		{Name: "a", Content: "# a"},
		{Name: "b", Content: "# b"},
		{Name: "c", Content: "# c"},
		{Name: "d", Content: "# d"},
		{Name: "e", Content: "# e"},
		{Name: "f", Content: "# f"},
		{Name: "g", Content: "# g"},
		{Name: "h", Content: "# h"},
		{Name: "i", Content: "# i"},
	}
	m.viewport.SetContent("preview")

	got, _ := m.updateDashboard(tea.KeyMsg{Type: tea.KeyPgDown})
	next := got.(model)
	if next.cursor != 8 {
		t.Fatalf("cursor after pgdown = %d, want 8", next.cursor)
	}

	got, _ = next.updateDashboard(tea.KeyMsg{Type: tea.KeyHome})
	next = got.(model)
	if next.cursor != 0 {
		t.Fatalf("cursor after home = %d, want 0", next.cursor)
	}

	got, _ = next.updateDashboard(tea.KeyMsg{Type: tea.KeyEnd})
	next = got.(model)
	if next.cursor != len(next.projects)-1 {
		t.Fatalf("cursor after end = %d, want %d", next.cursor, len(next.projects)-1)
	}

	got, _ = next.updateDashboard(tea.KeyMsg{Type: tea.KeyPgUp})
	next = got.(model)
	if next.cursor != 0 {
		t.Fatalf("cursor after pgup = %d, want 0", next.cursor)
	}
}
