package cockpit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// SocketClient is the sb-side half of the NDJSON protocol. It satisfies
// the Client interface, so TUI code treats it the same as an in-proc
// Manager. One goroutine reads the connection and demuxes replies to
// pending callers; event envelopes fan out to every Subscribe() caller.
type SocketClient struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder

	writeMu sync.Mutex
	nextID  int64

	pendingMu sync.Mutex
	pending   map[string]chan Envelope

	subsMu   sync.RWMutex
	subs     map[int]chan Event
	nextSub  int
	subStart sync.Once
	subErr   error

	closed atomic.Bool
}

var _ Client = (*SocketClient)(nil)

// Dial connects to the foreman unix socket and starts the read pump.
// Returns an error if the socket is missing or unreachable.
func Dial(socket string) (*SocketClient, error) {
	c, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	sc := &SocketClient{
		conn:    c,
		enc:     json.NewEncoder(c),
		dec:     json.NewDecoder(bufio.NewReaderSize(c, 64*1024)),
		pending: map[string]chan Envelope{},
		subs:    map[int]chan Event{},
	}
	go sc.readLoop()
	return sc, nil
}

// EnsureDaemon dials the socket; if nothing is listening it exec's
// the foreman binary, waits for the socket, and redials. binary is the
// path to sb-foreman (empty = resolve via PATH). Returns a live client
// or the first error encountered.
func EnsureDaemon(paths Paths, binary string) (*SocketClient, error) {
	if c, err := Dial(paths.Socket); err == nil {
		return c, nil
	}
	if binary == "" {
		if p, err := exec.LookPath("sb-foreman"); err == nil {
			binary = p
		} else {
			return nil, fmt.Errorf("sb-foreman not found on PATH; build it with `go build ./cmd/foreman`")
		}
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "-serve")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Detach so the daemon outlives sb.
	cmd.SysProcAttr = detachSysProcAttr()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start foreman: %w", err)
	}
	_ = cmd.Process.Release()

	// Wait up to 3s for the socket to appear and accept a connection.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(paths.Socket); err == nil {
			if c, err := Dial(paths.Socket); err == nil {
				return c, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("foreman failed to come up at %s", paths.Socket)
}

// Close tears down the connection. Safe to call multiple times.
func (c *SocketClient) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	err := c.conn.Close()
	// Close all subscriber channels so watchers unblock.
	c.subsMu.Lock()
	for id, ch := range c.subs {
		close(ch)
		delete(c.subs, id)
	}
	c.subsMu.Unlock()
	return err
}

func (c *SocketClient) readLoop() {
	defer c.Close()
	for {
		var env Envelope
		if err := c.dec.Decode(&env); err != nil {
			if errors.Is(err, io.EOF) || c.closed.Load() {
				return
			}
			return
		}
		if env.Kind == "event" && env.Event != nil {
			c.fanout(*env.Event)
			continue
		}
		if env.ID != "" {
			c.deliver(env)
		}
	}
}

func (c *SocketClient) fanout(e Event) {
	c.subsMu.RLock()
	for _, ch := range c.subs {
		select {
		case ch <- e:
		default:
		}
	}
	c.subsMu.RUnlock()
}

func (c *SocketClient) deliver(env Envelope) {
	c.pendingMu.Lock()
	ch, ok := c.pending[env.ID]
	if ok {
		delete(c.pending, env.ID)
	}
	c.pendingMu.Unlock()
	if ok {
		ch <- env
	}
}

// call sends a request and waits for the matching reply.
func (c *SocketClient) call(method Method, params any, out any) error {
	id := fmt.Sprintf("r-%d", atomic.AddInt64(&c.nextID, 1))
	req := Envelope{ID: id, Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}
	ch := make(chan Envelope, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	err := c.enc.Encode(req)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return err
	}

	select {
	case env := <-ch:
		if env.Error != "" {
			return errors.New(env.Error)
		}
		if out != nil && len(env.Result) > 0 {
			return json.Unmarshal(env.Result, out)
		}
		return nil
	case <-time.After(30 * time.Second):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return fmt.Errorf("%s timed out", method)
	}
}

// --- Client interface implementation ---

func (c *SocketClient) ListJobs() []Job {
	var out ListJobsResult
	if err := c.call(MethodListJobs, nil, &out); err != nil {
		return nil
	}
	return out.Jobs
}

func (c *SocketClient) GetJob(id JobID) (Job, bool) {
	var out GetJobResult
	if err := c.call(MethodGetJob, GetJobParams{ID: id}, &out); err != nil {
		return Job{}, false
	}
	return out.Job, out.OK
}

func (c *SocketClient) GetForemanState() ForemanState {
	var out GetForemanStateResult
	if err := c.call(MethodGetForeman, nil, &out); err != nil {
		return ForemanState{}
	}
	return out.State
}

func (c *SocketClient) SetForemanEnabled(enabled bool) (ForemanState, error) {
	var out GetForemanStateResult
	err := c.call(MethodSetForeman, SetForemanEnabledParams{Enabled: enabled}, &out)
	return out.State, err
}

func (c *SocketClient) LaunchJob(req LaunchRequest) (Job, error) {
	var out LaunchJobResult
	err := c.call(MethodLaunchJob, req, &out)
	return out.Job, err
}

func (c *SocketClient) StartJob(id JobID) (Job, error) {
	var out LaunchJobResult
	err := c.call(MethodStartJob, StartJobParams{ID: id}, &out)
	return out.Job, err
}

func (c *SocketClient) SoftStopJob(id JobID) error {
	return c.call(MethodSoftStopJob, StopJobParams{ID: id}, nil)
}

func (c *SocketClient) ContinueJob(id JobID) error {
	return c.call(MethodContinueJob, StopJobParams{ID: id}, nil)
}

func (c *SocketClient) StopJob(id JobID) error {
	return c.call(MethodStopJob, StopJobParams{ID: id}, nil)
}

func (c *SocketClient) SkipJob(id JobID) error {
	return c.call(MethodSkipJob, SkipJobParams{ID: id}, nil)
}

func (c *SocketClient) SkipCampaign(id JobID) error {
	return c.call(MethodSkipCampaign, SkipCampaignParams{ID: id}, nil)
}

func (c *SocketClient) DeleteJob(id JobID) error {
	return c.call(MethodDeleteJob, DeleteJobParams{ID: id}, nil)
}

func (c *SocketClient) ApproveJob(id JobID, devlogPath string) error {
	return c.call(MethodApproveJob, ApproveJobParams{ID: id, DevlogPath: devlogPath}, nil)
}

func (c *SocketClient) RetryJob(id JobID, presets []LaunchPreset) (Job, error) {
	var out RetryJobResult
	err := c.call(MethodRetryJob, RetryJobParams{ID: id, Presets: presets}, &out)
	return out.Job, err
}

func (c *SocketClient) TakeOverJob(id JobID, presets []LaunchPreset) (Job, error) {
	var out TakeOverJobResult
	err := c.call(MethodTakeOverJob, TakeOverJobParams{ID: id, Presets: presets}, &out)
	return out.Job, err
}

func (c *SocketClient) SendInput(id JobID, data []byte) error {
	return c.call(MethodSendInput, SendInputParams{ID: id, Data: data}, nil)
}

func (c *SocketClient) ReadTranscript(id JobID) (string, error) {
	var out ReadTranscriptResult
	if err := c.call(MethodReadTranscript, ReadTranscriptParams{ID: id}, &out); err != nil {
		return "", err
	}
	return out.Body, nil
}

func (c *SocketClient) AttachTmux(id JobID) error {
	return c.call(MethodAttachTmux, AttachTmuxParams{ID: id}, nil)
}

// Subscribe asks the server to start streaming events (once) and returns
// a local fan-out channel. Cancel the returned func to stop receiving.
func (c *SocketClient) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 128)
	c.subsMu.Lock()
	id := c.nextSub
	c.nextSub++
	c.subs[id] = ch
	c.subsMu.Unlock()

	c.subStart.Do(func() {
		c.subErr = c.call(MethodSubscribe, nil, nil)
	})

	return ch, func() {
		c.subsMu.Lock()
		if s, ok := c.subs[id]; ok {
			delete(c.subs, id)
			close(s)
		}
		c.subsMu.Unlock()
	}
}
