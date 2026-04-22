package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/workmd"
)

func TestSetAgentManageFieldValueParsesPresetHookJSON(t *testing.T) {
	dir := t.TempDir()
	m := newModel(nil)
	m.cockpitPaths = cockpit.Paths{PresetsDir: dir, ProvidersDir: dir}
	m.cockpitPresets = []cockpit.LaunchPreset{{
		ID:   "senior-dev",
		Name: "Senior dev",
		Executor: cockpit.ExecutorSpec{
			Type: "codex",
		},
		Hooks: cockpit.HookSpec{
			Iteration: cockpit.IterationPolicy{Mode: cockpit.IterationOneShot},
		},
	}}
	m.agentManageKind = "preset"

	if err := m.setAgentManageFieldValue(0, 10, `[{"kind":"literal","label":"extra","body":"ctx"}]`); err != nil {
		t.Fatalf("setAgentManageFieldValue(prompt hooks): %v", err)
	}
	if got := len(m.cockpitPresets[0].Hooks.Prompt); got != 1 {
		t.Fatalf("prompt hooks len = %d, want 1", got)
	}
	if got := m.cockpitPresets[0].Hooks.Prompt[0].Body; got != "ctx" {
		t.Fatalf("prompt hook body = %q, want ctx", got)
	}
	if _, err := cockpit.LoadPresets(dir); err != nil {
		t.Fatalf("LoadPresets after save: %v", err)
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
