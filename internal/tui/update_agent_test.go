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
	"github.com/LFroesch/sb/internal/config"
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
	m.cockpitForeman = cockpit.ForemanState{Enabled: true}
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

func TestUpdateAgentLaunchCtrlTWarnsWhenForemanIsDisabled(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentLaunch
	m.launchSources = []cockpit.SourceTask{{Text: "keep draft state"}}
	m.launchFocus = m.launchNoteFocus()
	m.launchRepo = "/tmp/demo"
	m.launchBrief.Focus()
	m.cockpitForeman = cockpit.ForemanState{Enabled: false}

	got, _ := m.updateAgentLaunch(tea.KeyMsg{Type: tea.KeyCtrlT})
	next := got.(model)
	if !next.launchQueueOnly {
		t.Fatalf("launchQueueOnly = false, want true")
	}
	want := "Foreman is off: this run will queue until you enable it with F"
	if next.statusMsg != want {
		t.Fatalf("statusMsg = %q, want %q", next.statusMsg, want)
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

func TestUpdateAgentLaunchEditKeyAllowsTypedNoPreset(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.launchFocus = launchFocusRole
	m.launchRepo = "/tmp/demo"
	m.cockpitPresets = []cockpit.LaunchPreset{{ID: "senior-dev", Name: "Senior dev", Executor: cockpit.ExecutorSpec{Type: "claude"}}}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}}
	m.launchPresetIdx = 0
	m.launchProviderIdx = -1

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
	if next.launchPresetIdx != -1 {
		t.Fatalf("launchPresetIdx = %d, want -1 after blank role selection", next.launchPresetIdx)
	}
	if next.launchProviderIdx != 0 {
		t.Fatalf("launchProviderIdx = %d, want default provider after clearing preset", next.launchProviderIdx)
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

func TestSingleCleanupDiscardReturnsToDashboard(t *testing.T) {
	m := newModel(nil)
	m.page = pageCleanup
	m.mode = modeNormal
	m.cleanupReturn = pageDashboard
	m.projects = []workmd.Project{{Name: "demo", Content: "# WORK - demo\nsummary\n\n## Current Phase\nbuild\n"}}
	m.selected = 0
	m.cursor = 0

	got, _ := m.updateCleanup(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	next := got.(model)
	if next.page != pageDashboard {
		t.Fatalf("page = %v, want dashboard", next.page)
	}
}

func TestSingleCleanupDiscardReturnsToProject(t *testing.T) {
	m := newModel(nil)
	m.page = pageCleanup
	m.mode = modeNormal
	m.cleanupReturn = pageProject
	m.width = 100
	m.projects = []workmd.Project{{Name: "demo", Content: "# WORK - demo\nsummary\n\n## Current Phase\nbuild\n"}}
	m.selected = 0
	m.viewport.SetContent("stale diff")

	got, _ := m.updateCleanup(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	next := got.(model)
	if next.page != pageProject {
		t.Fatalf("page = %v, want project", next.page)
	}
	if strings.Contains(next.viewport.View(), "stale diff") {
		t.Fatalf("project viewport kept stale cleanup diff")
	}
}

func TestCleanupErrorReturnsToOriginPage(t *testing.T) {
	m := newModel(nil)
	m.page = pageDashboard
	m.mode = modeCleanupWait
	m.cleanupReturn = pageDashboard
	m.projects = []workmd.Project{{Name: "demo", Content: "# WORK - demo\nsummary\n\n## Current Phase\nbuild\n"}}
	m.selected = 0
	m.cursor = 0
	m.width = 100

	got, _ := m.Update(cleanupDoneMsg{err: os.ErrInvalid})
	next := got.(model)
	if next.page != pageDashboard {
		t.Fatalf("page = %v, want dashboard", next.page)
	}
	if next.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", next.mode)
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

func TestDoLaunchExplainsDisabledForemanQueue(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.launchQueueOnly = true
	m.cockpitForeman = cockpit.ForemanState{Enabled: false}
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:          "senior-dev",
		Name:        "Senior dev",
		Permissions: "scoped-write",
		Executor:    cockpit.ExecutorSpec{Type: "codex"},
		Hooks:       cockpit.HookSpec{Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot}},
	}}
	m.launchPresetIdx = 0
	m.launchRepo = "/tmp/demo"
	m.launchBrief.SetValue("tighten the launcher")
	launchReq := cockpit.LaunchRequest{}
	m.cockpitClient = stubCockpitClient{
		launchJob: cockpit.Job{ID: "job-1", PresetID: "senior-dev", ForemanManaged: true, WaitForForeman: true},
		launchReq: &launchReq,
	}

	got, _ := m.doLaunch()
	next := got.(model)

	if !launchReq.QueueOnly {
		t.Fatalf("launchReq.QueueOnly = false, want true")
	}
	want := "queued for disabled Foreman: Senior dev (press F in the list to start dispatching)"
	if next.statusMsg != want {
		t.Fatalf("statusMsg = %q, want %q", next.statusMsg, want)
	}
	if next.mode != modeAgentList {
		t.Fatalf("mode = %v, want modeAgentList", next.mode)
	}
}

func TestLaunchRepoChoicesPutCustomPathFirstWithoutChangingDefaultSelection(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.projects = []workmd.Project{{Dir: "/tmp/a"}, {Dir: "/tmp/b"}}
	m.launchSources = nil
	m.launchRepo = ""

	repos := m.launchRepoChoices()
	if len(repos) < 4 {
		t.Fatalf("launchRepoChoices len = %d, want at least 4", len(repos))
	}
	if repos[0] != "/tmp/a" {
		t.Fatalf("launchRepoChoices[0] = %q, want default repo /tmp/a first", repos[0])
	}
	if repos[1] != repoSentinelNone {
		t.Fatalf("launchRepoChoices[1] = %q, want no-repo sentinel second", repos[1])
	}
	if repos[2] != repoSentinelCustom {
		t.Fatalf("launchRepoChoices[2] = %q, want custom-path sentinel third", repos[2])
	}
	if got := indexOfLaunchRepo(repos, m.launchRepo); got != 0 {
		t.Fatalf("indexOfLaunchRepo(empty) = %d, want 0 so default selection starts on the default repo row", got)
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

func TestLaunchRepoChoicesReachNoRepoWithOneMoveDownFromDefault(t *testing.T) {
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

	next := m.handleAgentMouseWheel(1).(model)
	m = next

	if m.launchRepo != repoSentinelNone {
		t.Fatalf("launchRepo = %q, want no-repo sentinel after one move down", m.launchRepo)
	}
}

func TestPrepareRetryLaunchPreservesNoRepoFreeform(t *testing.T) {
	m := newModel(nil)
	job := cockpit.Job{
		PresetID: "senior-dev",
		Freeform: "freeform",
	}

	m.prepareRetryLaunch(job)
	if m.launchRepo != repoSentinelNone {
		t.Fatalf("launchRepo = %q, want no-repo sentinel", m.launchRepo)
	}
}

func TestLaunchListMoveAllowsNoPresetSelection(t *testing.T) {
	m := newModel(nil)
	m.mode = modeAgentLaunch
	m.cockpitPresets = []cockpit.LaunchPreset{
		{ID: "senior-dev", Name: "Senior dev", Executor: cockpit.ExecutorSpec{Type: "claude"}},
		{ID: "bug-fixer", Name: "Bug fixer", Executor: cockpit.ExecutorSpec{Type: "claude"}},
	}
	m.cockpitProviders = []cockpit.ProviderProfile{{ID: "codex", Name: "Codex", Executor: cockpit.ExecutorSpec{Type: "codex"}}}
	m.launchPresetIdx = 0
	m.launchFocus = launchFocusRole

	m.launchListMove(-1)
	if m.launchPresetIdx != -1 {
		t.Fatalf("launchPresetIdx = %d, want -1 after moving above first preset", m.launchPresetIdx)
	}
	if m.launchProviderIdx != 0 {
		t.Fatalf("launchProviderIdx = %d, want default provider in no-preset mode", m.launchProviderIdx)
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
	t.Setenv("SB_TMUX_SOCKET", filepath.Join(dir, "tmux.sock"))
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
	if [ "$1" = "-S" ]; then
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

func TestHelpMouseWheelMovesSingleStep(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 16
	m.mode = modeHelp

	got := m.handleMouseWheel(tea.MouseMsg{}, 1)
	next := got.(model)
	if next.helpScroll != 1 {
		t.Fatalf("helpScroll = %d, want 1 after a single wheel notch", next.helpScroll)
	}
}

func TestDashboardMouseWheelOverLeftPaneMovesCursor(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 20
	m.page = pageDashboard
	m.mode = modeNormal
	m.projects = []workmd.Project{
		{Name: "one", Hydrated: true, Content: "one"},
		{Name: "two", Hydrated: true, Content: "two"},
	}

	got := m.handleMouseWheel(tea.MouseMsg{X: m.dashboardLeftPanelWidth() - 1}, 1)
	next := got.(model)
	if next.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after wheel in left pane", next.cursor)
	}
}

func TestDashboardMouseWheelOverRightPaneScrollsPreview(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 20
	m.page = pageDashboard
	m.mode = modeNormal
	m.projects = []workmd.Project{{Name: "one", Hydrated: true, Content: strings.Repeat("line\n", 40)}}
	m.viewport.Width = m.rightPanelWidth() - 4
	m.viewport.Height = 6
	m.setViewportProjectContent(0, m.rightPanelWidth())

	got := m.handleMouseWheel(tea.MouseMsg{X: m.dashboardLeftPanelWidth()}, 1)
	next := got.(model)
	if next.cursor != 0 {
		t.Fatalf("cursor = %d, want unchanged when wheel is in right pane", next.cursor)
	}
	if next.viewport.YOffset <= 0 {
		t.Fatalf("viewport.YOffset = %d, want > 0 after wheel in right pane", next.viewport.YOffset)
	}
}

func TestHydrationContinuesAfterFailedProject(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	missing := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(first, []byte("# WORK - first\nsummary\n\n## Current Phase\nx\n\n## Current Tasks\n- a\n\n## Backlog / Future Features\n- b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(first): %v", err)
	}
	if err := os.WriteFile(second, []byte("# WORK - second\nsummary\n\n## Current Phase\nx\n\n## Current Tasks\n- a\n\n## Backlog / Future Features\n- b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(second): %v", err)
	}

	m := newModel(config.Load())
	m.projects = []workmd.Project{
		{Name: "first", Path: first},
		{Name: "missing", Path: missing},
		{Name: "second", Path: second},
	}
	m.hydrateQueue = []string{first, missing, second}

	cmd := m.nextHydrateCmd()
	msg := cmd()
	hydrated, ok := msg.(projectHydratedMsg)
	if !ok || hydrated.project.Path != first {
		t.Fatalf("first hydration msg = %#v, want projectHydratedMsg for first", msg)
	}

	got, cmd := m.Update(msg)
	next := got.(model)
	msg = cmd()
	failed, ok := msg.(projectHydrationFailedMsg)
	if !ok || failed.path != missing {
		t.Fatalf("second hydration msg = %#v, want projectHydrationFailedMsg for missing", msg)
	}

	got, cmd = next.Update(msg)
	next = got.(model)
	msg = cmd()
	hydrated, ok = msg.(projectHydratedMsg)
	if !ok || hydrated.project.Path != second {
		t.Fatalf("third hydration msg = %#v, want projectHydratedMsg for second", msg)
	}
}

func TestMergeDiscoveredProjectsKeepsCachedHydratedPreview(t *testing.T) {
	now := time.Now()
	discovered := []workmd.Project{{
		Name:         "fallback",
		Path:         "/tmp/demo/WORK.md",
		RelativePath: "demo/WORK.md",
		FileName:     "WORK.md",
		ModTime:      now,
	}}
	cached := []workmd.Project{{
		Name:         "demo",
		Path:         "/tmp/demo/WORK.md",
		RelativePath: "demo/WORK.md",
		FileName:     "WORK.md",
		Content:      "# WORK - demo\nsummary",
		CurrentCount: 2,
		BacklogCount: 1,
		ModTime:      now,
		Hydrated:     true,
	}}

	merged := mergeDiscoveredProjects(cached, discovered)
	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1", len(merged))
	}
	if merged[0].Name != "demo" {
		t.Fatalf("merged name = %q, want cached hydrated name", merged[0].Name)
	}
	if merged[0].Content == "" || !merged[0].Hydrated {
		t.Fatalf("merged project lost hydrated content: %+v", merged[0])
	}
	if merged[0].CurrentCount != 2 || merged[0].BacklogCount != 1 {
		t.Fatalf("merged counts = %d/%d, want 2/1", merged[0].CurrentCount, merged[0].BacklogCount)
	}
}

func TestHydrationOrderSkipsUnchangedCachedProjects(t *testing.T) {
	now := time.Now()
	m := newModel(nil)
	m.projects = []workmd.Project{
		{Name: "cached", Path: "/tmp/cached/WORK.md", Content: "# WORK - cached\nsummary", Hydrated: true, ModTime: now},
		{Name: "pending", Path: "/tmp/pending/WORK.md"},
	}

	queue := m.hydrationOrder()
	if len(queue) != 1 || queue[0] != "/tmp/pending/WORK.md" {
		t.Fatalf("hydrationOrder = %v, want only pending project", queue)
	}
}

func TestCachedProjectsLoadedMsgPaintsDashboardImmediately(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 20
	m.page = pageDashboard

	got, _ := m.Update(cachedProjectsLoadedMsg{projects: []workmd.Project{{
		Name:     "demo",
		Path:     "/tmp/demo/WORK.md",
		Content:  "# WORK - demo\nsummary",
		Hydrated: true,
	}}})
	next := got.(model)
	if next.loading {
		t.Fatalf("loading = true, want false after cached projects load")
	}
	if len(next.projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(next.projects))
	}
	if !strings.Contains(next.viewport.View(), "demo") {
		t.Fatalf("viewport did not render cached project content: %q", next.viewport.View())
	}
}

func TestProjectsLoadedMsgKeepsCurrentDashboardPreview(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 20
	m.page = pageDashboard
	m.cursor = 1
	m.projects = []workmd.Project{
		{Name: "a", Path: "/tmp/a/WORK.md", Content: "# WORK - a\nsummary", Hydrated: true},
		{Name: "b", Path: "/tmp/b/WORK.md", Content: "# WORK - b\nsummary", Hydrated: true},
	}
	m.setViewportProjectContent(m.cursor, m.rightPanelWidth())

	got, _ := m.Update(projectsLoadedMsg{projects: []workmd.Project{
		{Name: "a", Path: "/tmp/a/WORK.md"},
		{Name: "b", Path: "/tmp/b/WORK.md"},
	}})
	next := got.(model)
	if next.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", next.cursor)
	}
	if !strings.Contains(next.viewport.View(), "b") {
		t.Fatalf("viewport switched away from current selection: %q", next.viewport.View())
	}
}

func TestProjectHydratedMsgDoesNotRepaintUnrelatedDashboardPreview(t *testing.T) {
	m := newModel(nil)
	m.width = 100
	m.height = 20
	m.page = pageDashboard
	m.cursor = 0
	m.projects = []workmd.Project{
		{Name: "a", Path: "/tmp/a/WORK.md", Content: "# WORK - a\nsummary", Hydrated: true},
		{Name: "b", Path: "/tmp/b/WORK.md"},
	}
	m.setViewportProjectContent(0, m.rightPanelWidth())
	before := m.viewport.View()

	got, _ := m.Update(projectHydratedMsg{project: workmd.Project{
		Name:     "b",
		Path:     "/tmp/b/WORK.md",
		Content:  "# WORK - b\nsummary",
		Hydrated: true,
	}})
	next := got.(model)
	after := next.viewport.View()
	if before != after {
		t.Fatalf("viewport changed for unrelated hydration\nbefore: %q\nafter: %q", before, after)
	}
}

func TestDashboardRefreshStartsStagedDiscovery(t *testing.T) {
	cfg := config.Load()
	m := newModel(cfg)
	m.page = pageDashboard
	m.projects = []workmd.Project{{Name: "demo", Path: "/tmp/demo/WORK.md", Hydrated: true, Content: "# WORK - demo\nsummary"}}
	m.hydrateQueue = []string{"/tmp/demo/WORK.md"}

	got, cmd := m.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	next := got.(model)
	if !next.loading {
		t.Fatalf("loading = false, want true after refresh")
	}
	if len(next.hydrateQueue) != 0 {
		t.Fatalf("hydrateQueue = %v, want cleared before rediscovery", next.hydrateQueue)
	}
	if next.statusMsg != "refreshing..." {
		t.Fatalf("statusMsg = %q, want refreshing...", next.statusMsg)
	}
	if cmd == nil {
		t.Fatal("refresh should return discover command")
	}
	if _, ok := cmd().(projectsLoadedMsg); !ok {
		t.Fatalf("refresh cmd should return projectsLoadedMsg")
	}
}

func TestUpdateEditSaveRefreshesProjectCacheImmediately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectPath := filepath.Join(home, "demo", "WORK.md")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	original := "# WORK - demo\nold summary\n\n## Current Phase\nold phase\n\n## Current Tasks\n- old task\n\n## Backlog / Future Features\n- old backlog\n"
	if err := os.WriteFile(projectPath, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &config.Config{}
	project, ok := workmd.HydrateProject(workmd.Project{Path: projectPath}, cfg)
	if !ok {
		t.Fatalf("HydrateProject failed for seed project")
	}

	m := newModel(cfg)
	m.selected = 0
	m.projects = []workmd.Project{project}
	m.mode = modeEdit
	m.editArea.SetValue("# WORK - demo\nnew summary\n\n## Current Phase\nnew phase\n\n## Current Tasks\n- new task\n\n## Backlog / Future Features\n- new backlog\n")

	got, _ := m.updateEdit(tea.KeyMsg{Type: tea.KeyCtrlS})
	next := got.(model)

	cached, ok, err := loadProjectCache(cfg)
	if err != nil {
		t.Fatalf("loadProjectCache error: %v", err)
	}
	if !ok || len(cached) != 1 {
		t.Fatalf("cache load = ok:%v len:%d, want true and 1 project", ok, len(cached))
	}
	if !strings.Contains(cached[0].Content, "new summary") {
		t.Fatalf("cached content = %q, want updated summary", cached[0].Content)
	}
	if next.projects[0].Phase != "new phase" {
		t.Fatalf("phase = %q, want hydrated updated phase", next.projects[0].Phase)
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
	headerX := lipgloss.Width(m.headerTitle()) + 2

	if got, ok := m.headerTabAt(headerX, 0); !ok || got != pageDashboard {
		t.Fatalf("dashboard hit = (%v, %v), want (%v, true)", got, ok, pageDashboard)
	}
	if got, ok := m.headerTabAt(headerX+len("Dashboard")+len(" │ "), 0); !ok || got != pageDump {
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

	dumpX := lipgloss.Width(m.headerTitle()) + 2 + len("Dashboard") + len(" │ ")
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

func TestUpdateEditCtrlHomeAndEndJumpFileBounds(t *testing.T) {
	m := newModel(nil)
	m.mode = modeEdit
	m.editArea.SetWidth(40)
	m.editArea.SetHeight(5)
	m.editArea.SetValue("one\ntwo\nthree\nfour")
	m.editArea.CursorDown()
	m.editArea.CursorDown()
	m.editArea.CursorEnd()

	got, _ := m.updateEdit(tea.KeyMsg{Type: tea.KeyCtrlEnd})
	next := got.(model)
	if next.editArea.Line() != next.editArea.LineCount()-1 {
		t.Fatalf("line after ctrl+end = %d, want %d", next.editArea.Line(), next.editArea.LineCount()-1)
	}

	got, _ = next.updateEdit(tea.KeyMsg{Type: tea.KeyCtrlHome})
	next = got.(model)
	if next.editArea.Line() != 0 {
		t.Fatalf("line after ctrl+home = %d, want 0", next.editArea.Line())
	}
	if next.editArea.LineInfo().ColumnOffset != 0 {
		t.Fatalf("column after ctrl+home = %d, want 0", next.editArea.LineInfo().ColumnOffset)
	}
}

func TestDashboardCondensesPinnedSectionOnShortTerminal(t *testing.T) {
	m := newModel(nil)
	m.page = pageDashboard
	m.mode = modeNormal
	m.width = 80
	m.height = 12
	m.loading = false
	m.projects = []workmd.Project{
		{Name: "alpha", Path: "/alpha", Content: "# alpha"},
		{Name: "bravo", Path: "/bravo", Content: "# bravo"},
		{Name: "charlie", Path: "/charlie", Content: "# charlie"},
		{Name: "delta", Path: "/delta", Content: "# delta"},
		{Name: "echo", Path: "/echo", Content: "# echo"},
		{Name: "foxtrot", Path: "/foxtrot", Content: "# foxtrot"},
	}
	m.favorites["/alpha"] = true
	m.favorites["/bravo"] = true
	m.favorites["/charlie"] = true
	m.favorites["/delta"] = true

	if !m.dashboardCondensePinned() {
		t.Fatalf("dashboardCondensePinned() = false, want true")
	}

	out := m.renderDashboard()
	if strings.Contains(out, "★ Pinned") {
		t.Fatalf("renderDashboard should collapse pinned section on short terminals: %q", out)
	}
	if !strings.Contains(out, "pinned condensed") {
		t.Fatalf("renderDashboard missing condensed pinned hint: %q", out)
	}
}

func TestDashboardCondensedPinnedKeepsCursorVisible(t *testing.T) {
	m := newModel(nil)
	m.page = pageDashboard
	m.mode = modeNormal
	m.width = 80
	m.height = 12
	m.loading = false
	m.projects = []workmd.Project{
		{Name: "alpha", Path: "/alpha", Content: "# alpha"},
		{Name: "bravo", Path: "/bravo", Content: "# bravo"},
		{Name: "charlie", Path: "/charlie", Content: "# charlie"},
		{Name: "delta", Path: "/delta", Content: "# delta"},
		{Name: "echo", Path: "/echo", Content: "# echo"},
		{Name: "foxtrot", Path: "/foxtrot", Content: "# foxtrot"},
		{Name: "golf", Path: "/golf", Content: "# golf"},
	}
	m.favorites["/alpha"] = true
	m.favorites["/bravo"] = true
	m.favorites["/charlie"] = true
	m.favorites["/delta"] = true
	m.cursor = 5

	out := m.renderDashboard()
	if !strings.Contains(out, "foxtrot") {
		t.Fatalf("renderDashboard should keep cursor row visible when pinned condenses: %q", out)
	}
}

func TestRefreshAttachedViewportKeepsBottomDistanceWhenContentGrows(t *testing.T) {
	now := time.Now()
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "ollama-qwen",
		Status:    cockpit.StatusIdle,
		CreatedAt: now.Add(-2 * time.Minute),
		Turns: []cockpit.Turn{
			{Role: cockpit.TurnUser, Content: "show me the full reply"},
			{Role: cockpit.TurnAssistant, Content: strings.Join([]string{
				"line 01", "line 02", "line 03", "line 04", "line 05",
				"line 06", "line 07", "line 08", "line 09", "line 10",
				"line 11", "line 12", "line 13", "line 14", "line 15",
			}, "\n")},
		},
	}

	client := stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 72
	m.height = 14
	m.cockpitClient = client
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(true)

	m.viewport.LineUp(2)
	beforeOffset := m.viewport.YOffset
	beforeDistance := m.viewport.TotalLineCount() - (m.viewport.YOffset + m.viewport.Height)

	job.Turns[1].Content += "\nline 16\nline 17\nline 18"
	client.jobs[job.ID] = job
	m.cockpitClient = client
	m.cockpitJobs[0] = job
	m.attachedTurns = append([]cockpit.Turn(nil), job.Turns...)
	m.refreshAttachedViewport(false)

	afterDistance := m.viewport.TotalLineCount() - (m.viewport.YOffset + m.viewport.Height)
	if afterDistance != beforeDistance {
		t.Fatalf("bottom distance = %d, want %d", afterDistance, beforeDistance)
	}
	if m.viewport.YOffset <= beforeOffset {
		t.Fatalf("expected offset to advance with new lines; got %d <= %d", m.viewport.YOffset, beforeOffset)
	}
}

func TestUpdateAgentAttachedInterruptUpdatesLocalDashboardStatus(t *testing.T) {
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Runner:    cockpit.RunnerTmux,
		Status:    cockpit.StatusRunning,
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}
	client := stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.cockpitClient = client
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID

	got, _ := m.updateAgentAttached(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	next := got.(model)

	if next.cockpitJobs[0].Status != cockpit.StatusIdle {
		t.Fatalf("status = %s, want idle", next.cockpitJobs[0].Status)
	}
	if next.cockpitJobs[0].Note != "interrupted" {
		t.Fatalf("note = %q, want interrupted", next.cockpitJobs[0].Note)
	}
}

func TestUpdateAgentListSoftStopUpdatesLocalDashboardNote(t *testing.T) {
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Runner:    cockpit.RunnerTmux,
		Status:    cockpit.StatusRunning,
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}
	client := stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentList
	m.cockpitClient = client
	m.cockpitJobs = []cockpit.Job{job}

	got, _ := m.updateAgent(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	next := got.(model)

	if next.cockpitJobs[0].Note != "sent Esc" {
		t.Fatalf("note = %q, want sent Esc", next.cockpitJobs[0].Note)
	}
}

func TestRenderFooterClarifiesAttachedViewVsRunControls(t *testing.T) {
	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.attachedFocus = 0

	out := m.renderFooter()
	if !strings.Contains(out, "leave view") {
		t.Fatalf("renderFooter missing leave-view hint: %q", out)
	}
	if !strings.Contains(out, "send Escape to run") {
		t.Fatalf("renderFooter missing run Escape hint: %q", out)
	}
}

func TestRenderAgentAttachedTmuxShowsControlHint(t *testing.T) {
	job := cockpit.Job{
		ID:        "job-1",
		PresetID:  "senior-dev",
		Runner:    cockpit.RunnerTmux,
		Status:    cockpit.StatusRunning,
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}
	client := stubCockpitClient{jobs: map[cockpit.JobID]cockpit.Job{job.ID: job}}

	m := newModel(nil)
	m.page = pageAgent
	m.mode = modeAgentAttached
	m.width = 120
	m.height = 30
	m.cockpitClient = client
	m.cockpitJobs = []cockpit.Job{job}
	m.attachedJobID = job.ID
	m.refreshAttachedViewport(true)

	out := m.renderAgentAttached()
	if !strings.Contains(out, "leave view") {
		t.Fatalf("renderAgentAttached missing leave-view hint: %q", out)
	}
	if !strings.Contains(out, "send Escape to run") {
		t.Fatalf("renderAgentAttached missing run Escape hint: %q", out)
	}
}
