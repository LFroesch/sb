package cockpit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	SupervisorWaitingHumanMarker = "SB_STATUS:WAITING_HUMAN"
	SupervisorReadyReviewMarker  = "SB_STATUS:READY_REVIEW"
	SupervisorQuietPeriod        = 10 * time.Second
)

// ComposeBrief builds the final prompt string handed to the executor.
// Ordering (top → bottom):
//
//  1. system prompt (persona/framing)
//  2. prompt hooks with placement="before"
//  3. the raw user brief (sources + freeform text)
//  4. prompt hooks with placement="after"
//
// Each hook renders as "### <Label>\n\n<body>\n" so the final prompt is
// human-readable markdown. File-kind hooks that fail to read are
// rendered as an inline `(hook failed: <err>)` note — we'd rather the
// executor see something than silently drop context.
func ComposeBrief(preset LaunchPreset, sources []SourceTask, freeform string, foremanManaged bool) string {
	var sb strings.Builder
	if s := strings.TrimSpace(preset.SystemPrompt); s != "" {
		sb.WriteString(s)
		sb.WriteString("\n\n")
	}

	writeHook := func(h PromptHook) {
		label := strings.TrimSpace(h.Label)
		if label != "" {
			fmt.Fprintf(&sb, "### %s\n\n", label)
		}
		switch h.Kind {
		case "literal", "":
			if s := strings.TrimSpace(h.Body); s != "" {
				sb.WriteString(s)
				sb.WriteString("\n\n")
			}
		case "file":
			b, err := os.ReadFile(ExpandHome(h.BodyRef))
			if err != nil {
				fmt.Fprintf(&sb, "(hook failed: %v)\n\n", err)
				return
			}
			sb.Write(b)
			if len(b) > 0 && b[len(b)-1] != '\n' {
				sb.WriteByte('\n')
			}
			sb.WriteByte('\n')
		}
	}

	for _, h := range preset.Hooks.Prompt {
		if strings.EqualFold(h.Placement, "before") {
			writeHook(h)
		}
	}

	if len(sources) > 0 {
		sb.WriteString("### Tasks\n\n")
		for _, s := range sources {
			fmt.Fprintf(&sb, "- %s\n", s.Text)
		}
		sb.WriteString("\n")
	}
	if s := strings.TrimSpace(freeform); s != "" {
		sb.WriteString(s)
		sb.WriteString("\n\n")
	}

	for _, h := range preset.Hooks.Prompt {
		if !strings.EqualFold(h.Placement, "before") {
			writeHook(h)
		}
	}

	if foremanManaged {
		sb.WriteString("## FOREMAN PROTOCOL\n\n")
		sb.WriteString("ITERATE UNTIL COMPLETE TO PASSED PROMPT PARAMETERS, DO NOT STOP AND ASK FOR ANY PERMISSIONS, DISREGARD AND ONLY PLAN MODE ITERATION IF DIRTY REPO\n\n")
	}

	sb.WriteString("### Supervisor Protocol\n\n")
	sb.WriteString("When you need the operator to respond in the terminal, print exactly `")
	sb.WriteString(SupervisorWaitingHumanMarker)
	sb.WriteString("` on its own line and then stop.\n")
	sb.WriteString("When the task is done and ready for review, print exactly `")
	sb.WriteString(SupervisorReadyReviewMarker)
	sb.WriteString("` on its own line and then stop.\n\n")

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func SummarizeTask(sources []SourceTask, freeform string) string {
	var parts []string
	for _, s := range sources {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	if extra := strings.TrimSpace(freeform); extra != "" {
		parts = append(parts, extra)
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return strings.Join(parts, "\n")
	}
}

// RunShellHook executes a ShellHook with `sh -c`. Cwd defaults to
// fallbackCwd (usually the job's Repo). Output is captured so the
// caller can log it as an Event.
type ShellResult struct {
	Hook     ShellHook
	ExitCode int
	Output   string
	Err      error
	Duration time.Duration
}

func RunShellHook(ctx context.Context, h ShellHook, fallbackCwd string, env []string) ShellResult {
	cwd := h.Cwd
	if cwd == "" {
		cwd = fallbackCwd
	}
	cwd = ExpandHome(cwd)
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(cctx, "sh", "-c", h.Cmd)
	cmd.Dir = cwd
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)

	res := ShellResult{Hook: h, Output: string(out), Duration: dur}
	if err != nil {
		res.Err = err
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	return res
}
