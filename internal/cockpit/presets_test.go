package cockpit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedTestLibraries() ([]PromptTemplate, []HookBundle, []ProviderProfile) {
	return defaultPrompts(), defaultHookBundles(), defaultProviders()
}

func TestWriteDefaultPresets_SeedsThenNoops(t *testing.T) {
	dir := t.TempDir()
	n, err := WriteDefaultPresets(dir)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n != DefaultPresetCount {
		t.Fatalf("expected %d seeds, got %d", DefaultPresetCount, n)
	}

	prompts, bundles, providers := seedTestLibraries()
	presets, err := LoadPresets(dir, prompts, bundles, providers)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(presets) != DefaultPresetCount {
		t.Fatalf("expected %d loaded presets, got %d", DefaultPresetCount, len(presets))
	}

	n2, err := WriteDefaultPresets(dir)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("expected no-op on second seed, wrote %d", n2)
	}
}

func TestWriteDefaultPresets_FillsMissingSeeds(t *testing.T) {
	dir := t.TempDir()
	if err := SavePreset(dir, LaunchPreset{ID: "senior-dev", Name: "Senior dev"}); err != nil {
		t.Fatalf("preseed: %v", err)
	}
	n, err := WriteDefaultPresets(dir)
	if err != nil {
		t.Fatalf("seed missing presets: %v", err)
	}
	if n != DefaultPresetCount-1 {
		t.Fatalf("expected %d missing seeds, got %d", DefaultPresetCount-1, n)
	}
}

func TestWriteDefaultProviders_FillsMissingSeeds(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProvider(dir, ProviderProfile{ID: "claude", Name: "Claude Code"}); err != nil {
		t.Fatalf("preseed provider: %v", err)
	}
	n, err := WriteDefaultProviders(dir)
	if err != nil {
		t.Fatalf("seed missing providers: %v", err)
	}
	if n != DefaultProviderCount-1 {
		t.Fatalf("expected %d missing providers, got %d", DefaultProviderCount-1, n)
	}
}

func TestWriteDefaultPrompts_FillsMissingSeeds(t *testing.T) {
	dir := t.TempDir()
	if err := SavePrompt(dir, PromptTemplate{ID: "senior-dev", Name: "Senior dev", Body: "x"}); err != nil {
		t.Fatalf("preseed prompt: %v", err)
	}
	n, err := WriteDefaultPrompts(dir)
	if err != nil {
		t.Fatalf("seed missing prompts: %v", err)
	}
	if n != DefaultPromptCount-1 {
		t.Fatalf("expected %d missing prompts, got %d", DefaultPromptCount-1, n)
	}
}

func TestWriteDefaultHookBundles_FillsMissingSeeds(t *testing.T) {
	dir := t.TempDir()
	if err := SaveHookBundle(dir, HookBundle{ID: "diff-stat", Name: "Post: git diff --stat"}); err != nil {
		t.Fatalf("preseed hook bundle: %v", err)
	}
	n, err := WriteDefaultHookBundles(dir)
	if err != nil {
		t.Fatalf("seed missing hook bundles: %v", err)
	}
	if n != DefaultHookBundleCount-1 {
		t.Fatalf("expected %d missing hook bundles, got %d", DefaultHookBundleCount-1, n)
	}
}

func TestLoadPresets_SortsCoreBeforeUtility(t *testing.T) {
	dir := t.TempDir()
	prompts, bundles, providers := seedTestLibraries()
	for _, p := range []LaunchPreset{
		{ID: "summarize", Name: "Summarize", PromptID: "scaffold", EngineID: "ollama-qwen"},
		{ID: "senior-dev", Name: "Senior dev", PromptID: "senior-dev", HookBundleIDs: []string{"diff-stat"}, EngineID: "claude"},
		{ID: "custom-role", Name: "Custom role", PromptID: "scaffold", EngineID: "codex"},
	} {
		if err := SavePreset(dir, p); err != nil {
			t.Fatalf("SavePreset(%s): %v", p.ID, err)
		}
	}

	got, err := LoadPresets(dir, prompts, bundles, providers)
	if err != nil {
		t.Fatalf("LoadPresets: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].ID != "custom-role" || got[1].ID != "senior-dev" || got[2].ID != "summarize" {
		t.Fatalf("unexpected order: %q, %q, %q", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestSavePreset_RoundtripFields(t *testing.T) {
	dir := t.TempDir()
	prompts, bundles, providers := seedTestLibraries()
	p := LaunchPreset{
		ID:       "custom",
		Name:     "Custom",
		PromptID: "scaffold",
		EngineID: "ollama-qwen",
	}
	if err := SavePreset(dir, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom.json")); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	loaded, err := LoadPresets(dir, prompts, bundles, providers)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != "custom" || loaded[0].Executor.Type != "ollama" {
		t.Fatalf("roundtrip mismatch: %+v", loaded)
	}
}

func TestSavePreset_StripsRuntimeFieldsOnDisk(t *testing.T) {
	dir := t.TempDir()
	p := LaunchPreset{
		ID: "x", Name: "X",
		PromptID: "senior-dev", HookBundleIDs: []string{"diff-stat"}, EngineID: "claude",
		// runtime fields populated as if just resolved
		SystemPrompt: "should-not-be-on-disk",
		Executor:     ExecutorSpec{Type: "claude"},
		Hooks:        HookSpec{PostShell: []ShellHook{{Cmd: "ghost"}}},
	}
	if err := SavePreset(dir, p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "x.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, leaked := range []string{"should-not-be-on-disk", "ghost", "system_prompt", `"executor"`, `"hooks"`} {
		if strings.Contains(body, leaked) {
			t.Fatalf("disk file should not contain %q; got:\n%s", leaked, body)
		}
	}
}

func TestLoadPresets_SkipsUnresolvableRefs(t *testing.T) {
	dir := t.TempDir()
	prompts, bundles, providers := seedTestLibraries()
	good := LaunchPreset{ID: "good", Name: "G", PromptID: "senior-dev", HookBundleIDs: []string{"diff-stat"}, EngineID: "claude"}
	bad := LaunchPreset{ID: "bad", Name: "B", PromptID: "ghost", HookBundleIDs: []string{"diff-stat"}, EngineID: "claude"}
	if err := SavePreset(dir, good); err != nil {
		t.Fatal(err)
	}
	if err := SavePreset(dir, bad); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPresets(dir, prompts, bundles, providers)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("expected only 'good' to load, got %d entries: %+v", len(got), got)
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
	out := ComposeBrief(preset, sources, "extra freeform", false, ExecutorSpec{Type: "claude"})
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
	if !strings.Contains(out, SupervisorWaitingHumanMarker) || !strings.Contains(out, SupervisorReadyReviewMarker) {
		t.Fatalf("missing supervisor markers:\n%s", out)
	}
}

func TestComposeBrief_OmitsForemanProtocolBlock(t *testing.T) {
	preset := LaunchPreset{}
	out := ComposeBrief(preset, nil, "", true, ExecutorSpec{Type: "claude"})
	if strings.Contains(out, "FOREMAN PROTOCOL") {
		t.Fatalf("foreman protocol block should be gone; perms enum drives unattended behaviour now:\n%s", out)
	}
}

func TestComposeBrief_OmitsSupervisorProtocolForOllama(t *testing.T) {
	preset := LaunchPreset{SystemPrompt: "Keep it short."}
	out := ComposeBrief(preset, nil, "", false, ExecutorSpec{Type: "ollama"})
	if strings.Contains(out, SupervisorWaitingHumanMarker) || strings.Contains(out, SupervisorReadyReviewMarker) {
		t.Fatalf("ollama brief should not include supervisor markers:\n%s", out)
	}
	if strings.Contains(out, "### Supervisor Protocol") {
		t.Fatalf("ollama brief should not include supervisor protocol block:\n%s", out)
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
