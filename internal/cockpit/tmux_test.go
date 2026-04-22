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
		"set-option -g -t sb-cockpit status-left #[bold,bg=#0b5cad,fg=#f8fafc] sb #[default]",
		"set-window-option -g -t sb-cockpit mode-keys vi",
		"bind-key -T root WheelUpPane if-shell -F #{==:#{window_index},0} send-keys -M if-shell -F \"#{pane_in_mode}\" \"send-keys -M\" \"copy-mode -eu\"",
		"bind-key -T root PageUp if-shell -F #{==:#{window_index},0} send-keys PageUp copy-mode -eu",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing tmux option %q in log:\n%s", want, got)
		}
	}
}
