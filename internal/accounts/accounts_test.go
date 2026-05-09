package accounts

import (
	"os"
	"path/filepath"
	"testing"
)

func codexAuthJSON(accountID, refreshToken, label string) []byte {
	return []byte(`{"auth_mode":"chatgpt","last_refresh":"` + label + `","tokens":{"account_id":"` + accountID + `","refresh_token":"` + refreshToken + `"}}`)
}

func TestSaveAndUseClaudeSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude-live"))

	live := filepath.Join(home, "claude-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, ".credentials.json"), []byte(`{"account":"a"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := SaveCurrent("claude", "work"); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, ".credentials.json"), []byte(`{"account":"b"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := SaveCurrent("claude", "personal"); err != nil {
		t.Fatalf("SaveCurrent personal: %v", err)
	}
	if err := Use("claude", "work"); err != nil {
		t.Fatalf("Use: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(live, ".credentials.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got, want := string(data), `{"account":"a"}`; got != want {
		t.Fatalf("claude creds = %q, want %q", got, want)
	}

	status, err := Show()
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if got := status.Active["claude"]; got != "work" {
		t.Fatalf("active claude = %q, want work", got)
	}
}

func TestSaveCurrentCodexSnapshotsWholeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-live"))

	live := filepath.Join(home, "codex-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "auth.json"), []byte(`{"account":"a"}`), 0o600); err != nil {
		t.Fatalf("WriteFile auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-a"), 0o600); err != nil {
		t.Fatalf("WriteFile db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "state_5.sqlite"), []byte("state-a"), 0o600); err != nil {
		t.Fatalf("WriteFile state: %v", err)
	}

	if err := SaveCurrent("codex", "work"); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}

	auth, err := os.ReadFile(filepath.Join(home, ".config", "sb", "accounts", "codex", "work", "auth.json"))
	if err != nil {
		t.Fatalf("ReadFile saved auth: %v", err)
	}
	if got, want := string(auth), `{"account":"a"}`; got != want {
		t.Fatalf("codex auth = %q, want %q", got, want)
	}
	db, err := os.ReadFile(filepath.Join(home, ".config", "sb", "accounts", "codex", "work", "logs_2.sqlite"))
	if err != nil {
		t.Fatalf("ReadFile saved db: %v", err)
	}
	if got, want := string(db), "db-a"; got != want {
		t.Fatalf("codex db = %q, want %q", got, want)
	}
	state, err := os.ReadFile(filepath.Join(home, ".config", "sb", "accounts", "codex", "work", "state_5.sqlite"))
	if err != nil {
		t.Fatalf("ReadFile saved state: %v", err)
	}
	if got, want := string(state), "state-a"; got != want {
		t.Fatalf("codex state = %q, want %q", got, want)
	}
}

func TestUseCodexMarksSelectedSlotActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-live"))

	live := filepath.Join(home, "codex-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "auth.json"), []byte(`{"account":"a"}`), 0o600); err != nil {
		t.Fatalf("WriteFile auth a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-a"), 0o600); err != nil {
		t.Fatalf("WriteFile db a: %v", err)
	}
	if err := SaveCurrent("codex", "work"); err != nil {
		t.Fatalf("SaveCurrent work: %v", err)
	}

	if err := os.WriteFile(filepath.Join(live, "auth.json"), []byte(`{"account":"b"}`), 0o600); err != nil {
		t.Fatalf("WriteFile auth b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-b"), 0o600); err != nil {
		t.Fatalf("WriteFile db b: %v", err)
	}
	if err := SaveCurrent("codex", "personal"); err != nil {
		t.Fatalf("SaveCurrent personal: %v", err)
	}

	if err := Use("codex", "personal"); err != nil {
		t.Fatalf("Use personal: %v", err)
	}
	status, err := Show()
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if got := status.Active["codex"]; got != "personal" {
		t.Fatalf("active codex = %q, want personal", got)
	}
}

func TestActiveEnvReturnsSelectedCodexHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-live"))

	live := filepath.Join(home, "codex-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "auth.json"), codexAuthJSON("acct-a", "rt-a", "ta"), 0o600); err != nil {
		t.Fatalf("WriteFile auth: %v", err)
	}

	if err := SaveCurrent("codex", "work"); err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}
	if err := Use("codex", "work"); err != nil {
		t.Fatalf("Use: %v", err)
	}

	env, err := ActiveEnv("codex")
	if err != nil {
		t.Fatalf("ActiveEnv: %v", err)
	}
	if len(env) != 1 {
		t.Fatalf("len(env) = %d, want 1", len(env))
	}
	want := "CODEX_HOME=" + filepath.Join(home, ".config", "sb", "accounts", "codex", "work")
	if env[0] != want {
		t.Fatalf("env[0] = %q, want %q", env[0], want)
	}
}

func TestUseCodexLeavesSavedSlotContentsUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-live"))

	live := filepath.Join(home, "codex-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "auth.json"), codexAuthJSON("acct-a", "rt-a", "ta"), 0o600); err != nil {
		t.Fatalf("WriteFile auth a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-a"), 0o600); err != nil {
		t.Fatalf("WriteFile db a: %v", err)
	}
	if err := SaveCurrent("codex", "work"); err != nil {
		t.Fatalf("SaveCurrent work: %v", err)
	}

	if err := os.WriteFile(filepath.Join(live, "auth.json"), codexAuthJSON("acct-b", "rt-b", "tb"), 0o600); err != nil {
		t.Fatalf("WriteFile auth b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-b"), 0o600); err != nil {
		t.Fatalf("WriteFile db b: %v", err)
	}
	if err := SaveCurrent("codex", "personal"); err != nil {
		t.Fatalf("SaveCurrent personal: %v", err)
	}

	if err := Use("codex", "work"); err != nil {
		t.Fatalf("Use work: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "auth.json"), codexAuthJSON("acct-b", "rt-b2", "tb2"), 0o600); err != nil {
		t.Fatalf("WriteFile auth b2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-b2"), 0o600); err != nil {
		t.Fatalf("WriteFile db b2: %v", err)
	}

	if err := Use("codex", "personal"); err != nil {
		t.Fatalf("Use personal: %v", err)
	}

	workAuth, err := os.ReadFile(filepath.Join(home, ".config", "sb", "accounts", "codex", "work", "auth.json"))
	if err != nil {
		t.Fatalf("ReadFile work auth: %v", err)
	}
	if got, want := string(workAuth), string(codexAuthJSON("acct-a", "rt-a", "ta")); got != want {
		t.Fatalf("work auth = %q, want %q", got, want)
	}
	status, err := Show()
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if got := status.Active["codex"]; got != "personal" {
		t.Fatalf("active codex = %q, want personal", got)
	}
}

func TestUseCodexRequiresSavedSlot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-live"))

	live := filepath.Join(home, "codex-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "auth.json"), codexAuthJSON("acct-a", "rt-a", "ta"), 0o600); err != nil {
		t.Fatalf("WriteFile auth a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-a"), 0o600); err != nil {
		t.Fatalf("WriteFile db a: %v", err)
	}
	if err := SaveCurrent("codex", "work"); err != nil {
		t.Fatalf("SaveCurrent work: %v", err)
	}

	if err := os.WriteFile(filepath.Join(live, "auth.json"), codexAuthJSON("acct-b", "rt-b", "tb"), 0o600); err != nil {
		t.Fatalf("WriteFile auth b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, "logs_2.sqlite"), []byte("db-b"), 0o600); err != nil {
		t.Fatalf("WriteFile db b: %v", err)
	}
	if err := Use("codex", "personal"); err == nil {
		t.Fatal("Use personal = nil, want missing saved slot error")
	}
}

func TestListMarksActiveSlots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude-live"))

	live := filepath.Join(home, "claude-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, ".credentials.json"), []byte(`{"account":"a"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := SaveCurrent("claude", "a"); err != nil {
		t.Fatalf("SaveCurrent a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(live, ".credentials.json"), []byte(`{"account":"b"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := SaveCurrent("claude", "b"); err != nil {
		t.Fatalf("SaveCurrent b: %v", err)
	}
	if err := Use("claude", "a"); err != nil {
		t.Fatalf("Use: %v", err)
	}

	slots, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(slots) != 2 {
		t.Fatalf("len(slots) = %d, want 2", len(slots))
	}
	if !slots[0].Active {
		t.Fatalf("slot %q active = false, want true", slots[0].Name)
	}
	if slots[1].Active {
		t.Fatalf("slot %q active = true, want false", slots[1].Name)
	}
}

func TestSaveCurrentRequiresLiveAuthFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-live"))

	if err := SaveCurrent("codex", "missing"); err == nil {
		t.Fatal("SaveCurrent() = nil, want missing auth error")
	}
}
