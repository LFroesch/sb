package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry is the persistence + in-memory index layer beneath Manager.
// It owns no runtime state (PTYs, hooks) — those live in the Manager
// session map. On startup, Rehydrate loads every job.json into memory
// and reconciles statuses (anything running → failed, since no live PTY
// can be attached after a restart).
type Registry struct {
	paths Paths

	mu   sync.RWMutex
	jobs map[JobID]*Job
}

func NewRegistry(paths Paths) *Registry {
	return &Registry{paths: paths, jobs: map[JobID]*Job{}}
}

// Rehydrate walks JobsDir and loads every job.json. Exec-backed
// running/paused jobs are marked failed with a note so the UI shows
// them as stale after a crash or daemon restart. Tmux-backed jobs are
// left alone so the tmux runner can reconcile them against live panes.
func (r *Registry) Rehydrate() error {
	if err := os.MkdirAll(r.paths.JobsDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(r.paths.JobsDir)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(r.paths.JobsDir, e.Name(), "job.json")
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var j Job
		if err := json.Unmarshal(b, &j); err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: bad job file %s: %v\n", path, err)
			continue
		}
		if (j.Status == StatusRunning || j.Status == StatusPaused) && j.Runner != RunnerTmux {
			j.Status = StatusFailed
			j.Note = "interrupted by restart"
			j.FinishedAt = time.Now()
			_ = writeJob(r.paths.JobDir(j.ID), &j)
		}
		r.jobs[j.ID] = &j
	}
	return nil
}

// Create registers a fresh job, writes its directory, and persists
// job.json. The caller fills in the returned pointer before any state
// transitions.
func (r *Registry) Create(j Job) (*Job, error) {
	if j.ID == "" {
		j.ID = NewJobID()
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now()
	}
	if j.Status == "" {
		j.Status = StatusQueued
	}
	jobDir := r.paths.JobDir(j.ID)
	artifactsDir := filepath.Join(jobDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return nil, err
	}
	j.TranscriptPath = filepath.Join(jobDir, "transcript.log")
	j.EventLogPath = filepath.Join(jobDir, "events.jsonl")
	j.ArtifactsDir = artifactsDir
	if j.SyncBackState == "" {
		j.SyncBackState = SyncBackPending
	}
	if err := os.WriteFile(filepath.Join(jobDir, "brief.md"), []byte(j.Brief), 0o644); err != nil {
		return nil, err
	}
	if prompt := strings.TrimSpace(j.Prompt); prompt != "" {
		if err := os.WriteFile(filepath.Join(jobDir, "prompt.md"), []byte(j.Prompt), 0o644); err != nil {
			return nil, err
		}
	}
	r.mu.Lock()
	r.jobs[j.ID] = &j
	r.mu.Unlock()
	return &j, writeJob(jobDir, &j)
}

// Save persists the in-memory Job back to disk.
func (r *Registry) Save(id JobID) error {
	r.mu.RLock()
	j, ok := r.jobs[id]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown job %s", id)
	}
	return writeJob(r.paths.JobDir(id), j)
}

// Update applies fn under the mutex, then persists the result.
func (r *Registry) Update(id JobID, fn func(*Job)) error {
	r.mu.Lock()
	j, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("unknown job %s", id)
	}
	fn(j)
	r.mu.Unlock()
	return r.Save(id)
}

// Get returns a shallow copy safe to hand to UI code.
func (r *Registry) Get(id JobID) (Job, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	j, ok := r.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// Delete removes a job from memory and its on-disk directory.
// Safe to call on unknown IDs (returns nil).
func (r *Registry) Delete(id JobID) error {
	r.mu.Lock()
	_, ok := r.jobs[id]
	if ok {
		delete(r.jobs, id)
	}
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return os.RemoveAll(r.paths.JobDir(id))
}

// List returns a shallow copy of every known job, sorted newest first.
func (r *Registry) List() []Job {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, *j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func writeJob(dir string, j *Job) error {
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "job.json"), append(b, '\n'), 0o644)
}

// NewJobID returns a millisecond-precision timestamp id. Good enough for
// V0: jobs are never created faster than the filesystem clock ticks on
// realistic hardware; if we ever hit a collision the second call simply
// bumps to the next millisecond.
var lastID int64

var idMu sync.Mutex

func NewJobID() JobID {
	idMu.Lock()
	defer idMu.Unlock()
	now := time.Now().UnixMilli()
	if now <= lastID {
		now = lastID + 1
	}
	lastID = now
	return JobID(fmt.Sprintf("j-%d", now))
}
