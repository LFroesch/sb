package cockpit

import (
	"path/filepath"
	"testing"
)

func TestRegistryCreateRehydrate(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     dir,
		JobsDir:      filepath.Join(dir, "jobs"),
		CampaignDir:  filepath.Join(dir, "campaigns"),
		PresetsDir:   filepath.Join(dir, "presets"),
		ProvidersDir: filepath.Join(dir, "providers"),
		LogFile:      filepath.Join(dir, "foreman.log"),
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(paths)
	j, err := r.Create(Job{PresetID: "p", Brief: "hi", Repo: "/tmp", Status: StatusQueued})
	if err != nil {
		t.Fatal(err)
	}
	if j.ID == "" {
		t.Fatal("expected id assigned")
	}
	if j.TranscriptPath == "" || j.EventLogPath == "" || j.ArtifactsDir == "" {
		t.Fatalf("paths not wired: %+v", j)
	}

	// Simulate a running job — rehydrate should flip it to failed.
	_ = r.Update(j.ID, func(jj *Job) { jj.Status = StatusRunning })

	r2 := NewRegistry(paths)
	if err := r2.Rehydrate(); err != nil {
		t.Fatal(err)
	}
	got, ok := r2.Get(j.ID)
	if !ok {
		t.Fatal("rehydrate missed job")
	}
	if got.Status != StatusFailed {
		t.Fatalf("expected StatusFailed, got %s", got.Status)
	}
}
