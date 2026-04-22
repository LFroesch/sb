package cockpit

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSocketRoundtrip boots a server + client pair over a tempdir unix
// socket and exercises the Client interface end-to-end: launch a shell
// job, observe events, read the transcript, list jobs.
func TestSocketRoundtrip(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		StateDir:     dir,
		JobsDir:      filepath.Join(dir, "jobs"),
		CampaignDir:  filepath.Join(dir, "campaigns"),
		PresetsDir:   filepath.Join(dir, "presets"),
		ProvidersDir: filepath.Join(dir, "providers"),
		Socket:       filepath.Join(dir, "sock"),
		PIDFile:      filepath.Join(dir, "pid"),
		LogFile:      filepath.Join(dir, "log"),
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(paths)
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	l, err := ListenUnix(paths.Socket)
	if err != nil {
		if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("unix sockets unavailable in this environment: %v", err)
		}
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan struct{})
	go func() {
		_ = Serve(ctx, l, mgr)
		close(serveDone)
	}()

	cli, err := Dial(paths.Socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()

	events, unsub := cli.Subscribe()
	defer unsub()

	preset := LaunchPreset{
		ID:       "test",
		Name:     "test",
		Executor: ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}},
		Hooks:    HookSpec{Iteration: IterationPolicy{Mode: IterationOneShot}},
	}
	j, err := cli.LaunchJob(LaunchRequest{
		Preset:   preset,
		Repo:     dir,
		Freeform: "echo hello-from-job",
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if j.ID == "" {
		t.Fatal("empty job id")
	}

	// Wait for the first turn to settle. Shell one-shot exits quickly, but
	// the manager now leaves the job in StatusIdle (ready for a follow-up
	// turn) until the user explicitly approves it.
	deadline := time.Now().Add(5 * time.Second)
	sawStdout := false
	for time.Now().Before(deadline) {
		select {
		case e := <-events:
			if e.Kind == EventStdout {
				sawStdout = true
			}
			if e.Kind == EventStatusChanged {
				got, ok := cli.GetJob(j.ID)
				if ok && (got.Status == StatusIdle || got.Status == StatusFailed) {
					if !sawStdout {
						t.Fatal("never saw stdout event")
					}
					if got.Status != StatusIdle {
						t.Fatalf("expected idle, got %s", got.Status)
					}
					jobs := cli.ListJobs()
					if len(jobs) != 1 || jobs[0].ID != j.ID {
						t.Fatalf("ListJobs mismatch: %+v", jobs)
					}
					body, err := cli.ReadTranscript(j.ID)
					if err != nil {
						t.Fatalf("read transcript: %v", err)
					}
					if body == "" {
						t.Fatal("empty transcript")
					}
					return
				}
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("timed out waiting for first turn to settle")
}
