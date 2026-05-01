package cockpit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxCmdEnvAddsTERMWhenMissing(t *testing.T) {
	t.Setenv("TERM", "")

	env := tmuxCmdEnv()
	found := false
	for _, kv := range env {
		if kv == "TERM=xterm-256color" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected default TERM in env, got %q", strings.Join(env, " "))
	}
}

func TestTmuxSessionExistsRecognizesMissingSession(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
if [ "$1" = "has-session" ]; then
  echo "can't find session: $4" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	ok, err := tmuxSessionExists("missing")
	if err != nil {
		t.Fatalf("tmuxSessionExists: %v", err)
	}
	if ok {
		t.Fatal("expected missing session to return false")
	}
}

func TestTmuxSessionExistsRecognizesMissingSocket(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
if [ "$1" = "has-session" ]; then
  echo "error connecting to /tmp/tmux-1000/sb (No such file or directory)" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	ok, err := tmuxSessionExists("missing")
	if err != nil {
		t.Fatalf("tmuxSessionExists: %v", err)
	}
	if ok {
		t.Fatal("expected missing socket to return false")
	}
}

func TestConfigureSessionSetsCockpitOptions(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "` + logPath + `"
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	if err := ConfigureSession("sb-cockpit"); err != nil {
		t.Fatalf("ConfigureSession: %v", err)
	}
	out, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"set-option -g -t sb-cockpit mouse on",
		"set-option -g -t sb-cockpit status-interval 15",
		"set-option -g -t sb-cockpit status-left #[bold,bg=#0b5cad,fg=#f8fafc] sb #[default]",
		"set-option -g -t sb-cockpit status-right #(",
		"set-option -g -t sb-cockpit window-status-format #[fg=#94a3b8] #W ",
		"set-option -g -t sb-cockpit window-status-current-format #[bold,bg=#e2e8f0,fg=#0f172a] #W #[default]",
		"set-window-option -g -t sb-cockpit mode-keys vi",
		"bind-key -T root WheelUpPane if-shell -F #{==:#{window_name},main} send-keys -M if-shell -F \"#{pane_in_mode}\" \"send-keys -M\" \"copy-mode -eu\"",
		"bind-key -T root PageUp if-shell -F #{==:#{window_name},main} send-keys PageUp copy-mode -eu",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing tmux option %q in log:\n%s", want, got)
		}
	}
	var statusRightLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, " set-option -g -t sb-cockpit status-right ") {
			statusRightLine = line
			break
		}
	}
	if statusRightLine == "" {
		t.Fatalf("missing status-right line in log:\n%s", got)
	}
	for _, unwanted := range []string{"#S ", "#I:#W"} {
		if strings.Contains(statusRightLine, unwanted) {
			t.Fatalf("unexpected redundant tmux status-right content %q in log line:\n%s", unwanted, statusRightLine)
		}
	}
}

func TestCapturePaneUsesSnapshotCommand(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$3" = "capture-pane" ]; then
  printf 'snapshot line\n'
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	out, err := CapturePane("sb-cockpit:@3")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if !strings.Contains(out, "snapshot line") {
		t.Fatalf("CapturePane output = %q", out)
	}
	cmds, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(cmds), "capture-pane -p -J -t sb-cockpit:@3") {
		t.Fatalf("missing capture-pane command in log:\n%s", string(cmds))
	}
}

func TestShowEnvironmentReadsGlobalValue(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
if [ "$1" = "show-environment" ]; then
  printf 'SB_TAKEOVER_TARGET=sb-cockpit:@3\n'
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	got, ok, err := ShowEnvironment("SB_TAKEOVER_TARGET")
	if err != nil {
		t.Fatalf("ShowEnvironment: %v", err)
	}
	if !ok || got != "sb-cockpit:@3" {
		t.Fatalf("ShowEnvironment = (%q, %v), want target", got, ok)
	}
}

func TestEnsureDashboardWindowRespawnsPlaceholderShell(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$1" = "list-panes" ]; then
  printf '1\t0\tzsh\n'
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	if err := ensureDashboardWindow("/tmp/sb"); err != nil {
		t.Fatalf("ensureDashboardWindow: %v", err)
	}
	out, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "respawn-window -k -t sb-cockpit:main") {
		t.Fatalf("expected respawn-window in log:\n%s", got)
	}
}

func TestEnsureDashboardWindowRecreatesMissingMainWindowInSession(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$1" = "list-panes" ]; then
  echo "can't find window: main" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	if err := ensureDashboardWindow("/tmp/sb"); err != nil {
		t.Fatalf("ensureDashboardWindow: %v", err)
	}
	out, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "new-window -d -t sb-cockpit: -n main") {
		t.Fatalf("expected new-window in session root, got:\n%s", got)
	}
}

func TestEnsureDashboardWindowLeavesLiveDashboardAlone(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	shim := filepath.Join(dir, "tmux-shim.sh")
	body := `#!/bin/sh
if [ "$1" = "-L" ]; then
  shift 2
fi
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$1" = "list-panes" ]; then
  printf '1\t0\tsb\n'
fi
exit 0
`
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("SB_TMUX_BIN", shim)

	if err := ensureDashboardWindow("/tmp/sb"); err != nil {
		t.Fatalf("ensureDashboardWindow: %v", err)
	}
	out, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "respawn-window") || strings.Contains(got, "new-window") {
		t.Fatalf("expected no dashboard repair commands, got:\n%s", got)
	}
}
