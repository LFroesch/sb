package cockpit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDefaultPresets_SeedsThenNoops(t *testing.T) {
	dir := t.TempDir()
	n, err := WriteDefaultPresets(dir)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n != DefaultPresetCount {
		t.Fatalf("expected %d seeds, got %d", DefaultPresetCount, n)
	}

	// Verify roundtrip
	presets, err := LoadPresets(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(presets) != DefaultPresetCount {
		t.Fatalf("expected %d loaded presets, got %d", DefaultPresetCount, len(presets))
	}

	// Second call is a no-op when any preset file exists
	n2, err := WriteDefaultPresets(dir)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("expected no-op on second seed, wrote %d", n2)
	}
}

func TestSavePreset_RoundtripFields(t *testing.T) {
	dir := t.TempDir()
	p := LaunchPreset{
		ID:   "custom",
		Name: "Custom",
		Role: "xp",
		Executor: ExecutorSpec{
			Type:  "shell",
			Cmd:   "bash",
			Args:  []string{"-c"},
			Model: "",
		},
		Hooks: HookSpec{
			Iteration: IterationPolicy{Mode: IterationOneShot},
		},
	}
	if err := SavePreset(dir, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom.json")); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	loaded, err := LoadPresets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != "custom" || loaded[0].Executor.Cmd != "bash" {
		t.Fatalf("roundtrip mismatch: %+v", loaded)
	}
}

func TestComposeBrief_Order(t *testing.T) {
	preset := LaunchPreset{
		SystemPrompt: "persona",
		Hooks: HookSpec{
			Prompt: []PromptHook{
				{Kind: "literal", Placement: "before", Body: "BEFORE"},
				{Kind: "literal", Placement: "after", Body: "AFTER"},
			},
		},
	}
	sources := []SourceTask{{Text: "do thing"}}
	out := ComposeBrief(preset, sources, "extra freeform")
	// Simple order check: persona, then BEFORE, then Tasks/do thing, then extra, then AFTER.
	wantSeq := []string{"persona", "BEFORE", "do thing", "extra freeform", "AFTER"}
	last := -1
	for _, w := range wantSeq {
		idx := indexOf(out, w)
		if idx <= last {
			t.Fatalf("order broken for %q; brief:\n%s", w, out)
		}
		last = idx
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
