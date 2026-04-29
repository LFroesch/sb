// Package cockpit protocol.go: NDJSON wire format for sb ↔ sb-foreman.
//
// One TCP-like connection per TUI. Each line is a single JSON message.
// Request messages carry a method + payload; the server responds with a
// matching ID. Events from the manager are pushed out of band (ID="").
//
// This is deliberately boring: no JSON-RPC framing, no MCP, no gRPC.
// Everything is `encoding/json` in both directions.

package cockpit

import "encoding/json"

// Method enumerates requests the client can send. Add new methods here
// and dispatch them in server.go.
type Method string

const (
	MethodListJobs       Method = "list_jobs"
	MethodGetJob         Method = "get_job"
	MethodGetForeman     Method = "get_foreman_state"
	MethodSetForeman     Method = "set_foreman_enabled"
	MethodLaunchJob      Method = "launch_job"
	MethodStartJob       Method = "start_job"
	MethodSoftStopJob    Method = "soft_stop_job"
	MethodContinueJob    Method = "continue_job"
	MethodStopJob        Method = "stop_job"
	MethodSkipJob        Method = "skip_job"
	MethodSkipCampaign   Method = "skip_campaign"
	MethodDeleteJob      Method = "delete_job"
	MethodApproveJob     Method = "approve_job"
	MethodRetryJob       Method = "retry_job"
	MethodSendInput      Method = "send_input"
	MethodReadTranscript Method = "read_transcript"
	MethodAttachTmux     Method = "attach_tmux"
	MethodSubscribe      Method = "subscribe" // server starts pushing events on this conn
	MethodPing           Method = "ping"
)

// Envelope is the single line format in both directions. Exactly one of
// {Method, Result, Event} is populated per line.
//
//   - Request:  {id, method, params}
//   - Response: {id, result | error}
//   - Event:    {kind:"event", event:{...}}
type Envelope struct {
	ID     string          `json:"id,omitempty"`
	Method Method          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`

	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`

	Kind  string `json:"kind,omitempty"` // "event"
	Event *Event `json:"event,omitempty"`
}

// --- Request/response payloads ---

type GetJobParams struct {
	ID JobID `json:"id"`
}
type GetJobResult struct {
	Job Job  `json:"job"`
	OK  bool `json:"ok"`
}

type GetForemanStateResult struct {
	State ForemanState `json:"state"`
}

type SetForemanEnabledParams struct {
	Enabled bool `json:"enabled"`
}

type LaunchJobResult struct {
	Job Job `json:"job"`
}

type StartJobParams struct {
	ID JobID `json:"id"`
}

type StopJobParams struct {
	ID JobID `json:"id"`
}

type SkipJobParams struct {
	ID JobID `json:"id"`
}

type SkipCampaignParams struct {
	ID JobID `json:"id"`
}

type DeleteJobParams struct {
	ID JobID `json:"id"`
}

type ApproveJobParams struct {
	ID         JobID  `json:"id"`
	DevlogPath string `json:"devlog_path"`
}

type RetryJobParams struct {
	ID      JobID          `json:"id"`
	Presets []LaunchPreset `json:"presets"`
}
type RetryJobResult struct {
	Job Job `json:"job"`
}

type SendInputParams struct {
	ID   JobID  `json:"id"`
	Data []byte `json:"data"`
}

type ReadTranscriptParams struct {
	ID JobID `json:"id"`
}
type ReadTranscriptResult struct {
	Body string `json:"body"`
}

type AttachTmuxParams struct {
	ID JobID `json:"id"`
}

type ListJobsResult struct {
	Jobs []Job `json:"jobs"`
}
