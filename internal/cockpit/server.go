package cockpit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// Serve accepts connections on l and handles cockpit requests against
// mgr. Blocks until l is closed. Safe to call concurrently with other
// Manager consumers (the Manager itself is already goroutine-safe).
func Serve(ctx context.Context, l net.Listener, mgr *Manager) error {
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	var wg sync.WaitGroup
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				wg.Wait()
				return nil
			}
			return err
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			handleConn(ctx, c, mgr)
		}(conn)
	}
}

// ListenUnix prepares a unix socket at path, removing any stale socket
// file first. The caller owns the returned listener.
func ListenUnix(path string) (net.Listener, error) {
	if conn, err := net.DialTimeout("unix", path, 150*time.Millisecond); err == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("listen unix %s: socket already active", path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

func handleConn(ctx context.Context, c net.Conn, mgr *Manager) {
	defer c.Close()

	// One writer goroutine per connection so event push + response
	// writes never interleave mid-line.
	writes := make(chan Envelope, 64)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		enc := json.NewEncoder(c)
		for env := range writes {
			if err := enc.Encode(env); err != nil {
				return
			}
		}
	}()
	send := func(env Envelope) {
		select {
		case writes <- env:
		case <-ctx.Done():
		}
	}

	var (
		subMu     sync.Mutex
		subCancel func()
	)
	unsubscribe := func() {
		subMu.Lock()
		if subCancel != nil {
			subCancel()
			subCancel = nil
		}
		subMu.Unlock()
	}
	defer unsubscribe()
	defer close(writes)

	scanner := bufio.NewScanner(c)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var req Envelope
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			send(Envelope{Error: "parse: " + err.Error()})
			continue
		}
		res := dispatch(ctx, mgr, req, func() {
			// subscribe activation: start forwarding events on this conn
			subMu.Lock()
			defer subMu.Unlock()
			if subCancel != nil {
				return // already subscribed
			}
			ch, cancel := mgr.Subscribe()
			subCancel = cancel
			go func() {
				for e := range ch {
					ev := e
					send(Envelope{Kind: "event", Event: &ev})
				}
			}()
		})
		send(res)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		slog.Debug("foreman: conn read", "err", err)
	}
}

// dispatch runs a single request against mgr. activateSub is called
// lazily if the method is MethodSubscribe.
func dispatch(_ context.Context, mgr *Manager, req Envelope, activateSub func()) Envelope {
	reply := Envelope{ID: req.ID}
	switch req.Method {
	case MethodPing:
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodListJobs:
		reply.Result = mustJSON(ListJobsResult{Jobs: mgr.ListJobs()})

	case MethodGetJob:
		var p GetJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		j, ok := mgr.GetJob(p.ID)
		reply.Result = mustJSON(GetJobResult{Job: j, OK: ok})

	case MethodGetForeman:
		reply.Result = mustJSON(GetForemanStateResult{State: mgr.GetForemanState()})

	case MethodSetForeman:
		var p SetForemanEnabledParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		state, err := mgr.SetForemanEnabled(p.Enabled)
		if err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(GetForemanStateResult{State: state})

	case MethodLaunchJob:
		var p LaunchRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		j, err := mgr.LaunchJob(p)
		if err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(LaunchJobResult{Job: j})

	case MethodStartJob:
		var p StartJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		j, err := mgr.StartJob(p.ID)
		if err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(LaunchJobResult{Job: j})

	case MethodSoftStopJob:
		var p StopJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.SoftStopJob(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodContinueJob:
		var p StopJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.ContinueJob(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodStopJob:
		var p StopJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.StopJob(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodDeleteJob:
		var p DeleteJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.DeleteJob(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodSkipJob:
		var p SkipJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.SkipJob(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodSkipCampaign:
		var p SkipCampaignParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.SkipCampaign(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodApproveJob:
		var p ApproveJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.ApproveJob(p.ID, p.DevlogPath); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodRetryJob:
		var p RetryJobParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		j, err := mgr.RetryJob(p.ID, p.Presets)
		if err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(RetryJobResult{Job: j})

	case MethodSendInput:
		var p SendInputParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.SendInput(p.ID, p.Data); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodReadTranscript:
		var p ReadTranscriptParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		body, err := mgr.ReadTranscript(p.ID)
		if err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(ReadTranscriptResult{Body: body})

	case MethodAttachTmux:
		var p AttachTmuxParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply.Error = err.Error()
			return reply
		}
		if err := mgr.AttachTmux(p.ID); err != nil {
			reply.Error = err.Error()
			return reply
		}
		reply.Result = mustJSON(map[string]any{"ok": true})

	case MethodSubscribe:
		activateSub()
		reply.Result = mustJSON(map[string]any{"ok": true})

	default:
		reply.Error = "unknown method: " + string(req.Method)
	}
	return reply
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
}
