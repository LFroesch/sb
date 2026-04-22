package cockpit

// Client is the surface the sb TUI uses to drive the cockpit. Both the
// in-process Manager and the SocketClient implement it; sb decides at
// startup whether to run in-proc (no daemon) or dial a unix socket into
// sb-foreman.
//
// Keep this small. Anything that needs the full Registry / internals
// belongs inside the daemon, not in the TUI.
type Client interface {
	ListJobs() []Job
	GetJob(id JobID) (Job, bool)
	LaunchJob(req LaunchRequest) (Job, error)
	StopJob(id JobID) error
	DeleteJob(id JobID) error
	ApproveJob(id JobID, devlogPath string) error
	RetryJob(id JobID, presets []LaunchPreset) (Job, error)
	SendInput(id JobID, data []byte) error
	ReadTranscript(id JobID) (string, error)

	Subscribe() (<-chan Event, func())
	Close() error
}

// Manager satisfies Client.
var _ Client = (*Manager)(nil)

// ListJobs wraps Registry.List so Manager satisfies Client.
func (m *Manager) ListJobs() []Job { return m.Registry.List() }

// GetJob wraps Registry.Get so Manager satisfies Client.
func (m *Manager) GetJob(id JobID) (Job, bool) { return m.Registry.Get(id) }

// Close is a no-op for the in-proc manager; kept for Client parity.
func (m *Manager) Close() error { return nil }
