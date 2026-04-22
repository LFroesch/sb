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
