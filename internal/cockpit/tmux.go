// Package cockpit tmux.go: thin CLI wrapper around `tmux` pinned to an
// isolated server via `-L sb`. Every call shells out — no persistent
// control-mode connection. Tests inject a fake binary via SB_TMUX_BIN.
//
// Isolation rationale: by always passing `-L sb` we speak to our own
// tmux server under $TMUX_TMPDIR/sb-*.sock, so the user's default server,
// config, and key bindings stay untouched. No risk of surprising the
// user's personal tmux setup.

package cockpit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// TmuxServerLabel is the -L value we pin every tmux call to. Keep in sync
// with the bootstrap exec and any user-facing docs.
const TmuxServerLabel = "sb"

// CockpitSession is the session name used by the bootstrap and runner.
// Window 0 of this session is the sb TUI itself; windows 1..N are jobs.
const CockpitSession = "sb-cockpit"

// WindowInfo is the parsed result of `list-panes` / `list-windows`.
type WindowInfo struct {
	Target   string // "sb-cockpit:@3"
	WindowID string // "@3"
	Name     string
	PaneID   string // "%7"
	PanePID  int
	Dead     bool
}

// TmuxBin returns the binary path to invoke. SB_TMUX_BIN overrides so
// tests can swap in a shim.
func TmuxBin() string {
	if v := strings.TrimSpace(os.Getenv("SB_TMUX_BIN")); v != "" {
		return v
	}
	return "tmux"
}

// HasTmux reports whether tmux is available. Defined as: `tmux -L sb
// kill-server` returns (any exit) — we only care that the binary runs,
// not that a server exists. We actually just exec `tmux -V` which is
// the cheapest liveness probe.
func HasTmux() bool {
	bin := TmuxBin()
	if _, err := exec.LookPath(bin); err != nil {
		// If SB_TMUX_BIN is set, it may be an absolute path we can't look
		// up; try running it directly.
		if os.Getenv("SB_TMUX_BIN") == "" {
			return false
		}
	}
	cmd := exec.Command(bin, "-V")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// InsideTmux reports whether the current process is already running
// inside a tmux pane (used to decide whether `attach` vs `switch-client`
// is the right UX primitive).
func InsideTmux() bool { return os.Getenv("TMUX") != "" }

// tmuxArgs prepends the -L flag to every call so we always hit our
// isolated server. All public helpers below funnel through this.
func tmuxArgs(args ...string) []string {
	out := make([]string, 0, len(args)+2)
	out = append(out, "-L", TmuxServerLabel)
	out = append(out, args...)
	return out
}

func runTmux(args ...string) (string, error) {
	full := tmuxArgs(args...)
	cmd := exec.Command(TmuxBin(), full...)
	cmd.Env = tmuxCmdEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("tmux %s: %w: %s", strings.Join(full, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func tmuxSessionExists(name string) (bool, error) {
	_, err := runTmux("has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	msg := err.Error()
	if strings.Contains(msg, "can't find session") || strings.Contains(msg, "no server") {
		return false, nil
	}
	return false, err
}

func tmuxCmdEnv() []string {
	env := os.Environ()
	for i, kv := range env {
		if strings.HasPrefix(kv, "TERM=") {
			if kv != "TERM=" {
				return env
			}
			out := append([]string{}, env...)
			out[i] = "TERM=xterm-256color"
			return out
		}
	}
	// Detached daemon launches may not inherit TERM, but tmux still
	// expects a terminal type for session/window creation. Use a sane
	// default so the foreman can manage the isolated cockpit server even
	// when it was started without an attached TTY.
	return append(env, "TERM=xterm-256color")
}

// EnsureSession creates the session if it does not yet exist. Uses
// an explicit `has-session` probe first so detached daemon launches do
// not accidentally hit tmux's attach/reuse path.
func EnsureSession(name string) error {
	exists, err := tmuxSessionExists(name)
	if err != nil {
		return err
	}
	if !exists {
		// -d: don't attach. -x/-y: give it a sane default size; tmux clamps
		// to the attaching client's terminal once one connects.
		if _, err = runTmux("new-session", "-d", "-s", name, "-n", "sb", "-x", "200", "-y", "50"); err != nil {
			return err
		}
	}
	return ConfigureSession(name)
}

// NewWindow creates a window inside `session` running `cmd` in `cwd`.
// Returns the created window's target + metadata. If tmux can't emit
// the new window info itself, we list-windows and pick the newest.
func NewWindow(session, name string, cmd []string, env []string, cwd string) (WindowInfo, error) {
	if len(cmd) == 0 {
		return WindowInfo{}, errors.New("tmux: empty command for new-window")
	}
	// -P prints the window's target. -F sets the format.
	args := []string{"new-window", "-t", session + ":", "-n", name, "-P", "-F", "#{session_name}:#{window_id}"}
	// Pass cwd explicitly so `claude` / `codex` see the right repo.
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	// Env is passed via `-e KEY=VAL`.
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	// Separator between tmux flags and the command-to-run.
	args = append(args, "--")
	args = append(args, cmd...)

	out, err := runTmux(args...)
	if err != nil {
		return WindowInfo{}, err
	}
	target := strings.TrimSpace(out)
	parts := strings.SplitN(target, ":", 2)
	info := WindowInfo{Target: target, Name: name}
	if len(parts) == 2 {
		info.WindowID = parts[1]
	}
	return info, nil
}

// PipePane starts streaming the pane's output to logPath. -o toggles
// pipe-on if off; we call it once per window right after creation.
// Appending via `cat >> path` ensures we don't truncate if the pane
// restarts.
func PipePane(target, logPath string) error {
	_, err := runTmux("pipe-pane", "-t", target, "-o", fmt.Sprintf("cat >> %s", shellQuote(logPath)))
	return err
}

// SelectWindow jumps the current client to the target window. Only
// meaningful when a tmux client is attached to the sb-cockpit session —
// inside the bootstrap re-exec we always are.
func SelectWindow(target string) error {
	_, err := runTmux("select-window", "-t", target)
	return err
}

// SwitchClient swaps the active client's session/window. Useful only if
// called while running inside a tmux client.
func SwitchClient(target string) error {
	_, err := runTmux("switch-client", "-t", target)
	return err
}

// DetachClient detaches the current client from the tmux server. Called
// when the user quits sb; the cockpit session + jobs keep running in
// the background.
func DetachClient() error {
	_, err := runTmux("detach-client")
	return err
}

// KillWindow kills a single window.
func KillWindow(target string) error {
	_, err := runTmux("kill-window", "-t", target)
	return err
}

// KillSession kills the entire cockpit session. Currently unused — the
// cockpit does not offer a global shutdown path in v2; exposed here for
// tests and future work.
func KillSession(name string) error {
	_, err := runTmux("kill-session", "-t", name)
	return err
}

// WindowAlive checks whether the given target still exists and its pane
// hasn't died. A killed-window target returns (false, nil) — that's the
// expected lifecycle signal for a finished job.
func WindowAlive(target string) (bool, error) {
	out, err := runTmux("list-panes", "-t", target, "-F", "#{pane_dead}")
	if err != nil {
		// tmux prints "can't find window" when the window is gone;
		// treat that as a clean "not alive" rather than an error.
		if strings.Contains(err.Error(), "can't find") || strings.Contains(err.Error(), "no server") {
			return false, nil
		}
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "0" {
			return true, nil
		}
	}
	return false, nil
}

// ListWindows returns every window in session with enough info for the
// poller to diff alive-sets. Format fields separated by tabs.
func ListWindows(session string) ([]WindowInfo, error) {
	out, err := runTmux("list-windows", "-t", session, "-F",
		"#{session_name}:#{window_id}\t#{window_id}\t#{window_name}\t#{pane_id}\t#{pane_pid}\t#{pane_dead}")
	if err != nil {
		// No server yet → empty list.
		if strings.Contains(err.Error(), "no server") {
			return nil, nil
		}
		return nil, err
	}
	var wins []WindowInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(fields[4])
		wins = append(wins, WindowInfo{
			Target:   fields[0],
			WindowID: fields[1],
			Name:     fields[2],
			PaneID:   fields[3],
			PanePID:  pid,
			Dead:     fields[5] == "1",
		})
	}
	return wins, nil
}

// SendKeys is the programmatic input path. Reserved for future use; the
// v2 cockpit returns an error from SendInput on tmux-backed jobs and
// asks the user to attach instead.
func SendKeys(target, keys string) error {
	_, err := runTmux("send-keys", "-t", target, keys, "Enter")
	return err
}

// BindKey binds a key at the `root` table (no prefix needed) so F1 etc.
// work globally inside the cockpit session. tmuxCmd is the raw tmux
// command (e.g. "select-window -t sb-cockpit:0").
func BindKey(key, tmuxCmd string) error {
	args := append([]string{"bind-key", "-T", "root", key}, strings.Fields(tmuxCmd)...)
	_, err := runTmux(args...)
	return err
}

func bindKeyArgs(table, key string, args ...string) error {
	cmd := []string{"bind-key"}
	if table != "" {
		cmd = append(cmd, "-T", table)
	}
	cmd = append(cmd, key)
	cmd = append(cmd, args...)
	_, err := runTmux(cmd...)
	return err
}

// UnbindKey removes a root-table binding. Useful for test cleanup.
func UnbindKey(key string) error {
	_, err := runTmux("unbind-key", "-T", "root", key)
	return err
}

func setOption(target, key, value string) error {
	args := []string{"set-option", "-g"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", target)
	}
	args = append(args, key, value)
	_, err := runTmux(args...)
	return err
}

func setWindowOption(target, key, value string) error {
	args := []string{"set-window-option", "-g"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", target)
	}
	args = append(args, key, value)
	_, err := runTmux(args...)
	return err
}

// ConfigureSession applies a deliberate operator-focused look/feel to the
// isolated sb tmux session. Because the cockpit always uses its own `-L sb`
// server, these settings never touch the user's personal tmux setup.
func ConfigureSession(target string) error {
	options := []struct {
		key   string
		value string
	}{
		{"status", "on"},
		{"status-position", "bottom"},
		{"mouse", "on"},
		{"history-limit", "100000"},
		{"status-left-length", "32"},
		{"status-right-length", "80"},
		{"status-style", "bg=#0f172a,fg=#cbd5e1"},
		{"status-left", "#[bold,bg=#0b5cad,fg=#f8fafc] sb #[default]"},
		{"status-right", "#[fg=#94a3b8]#S #[fg=#475569]· #[fg=#e2e8f0]#I:#W #[fg=#475569]· #[fg=#94a3b8]%H:%M "},
		{"window-status-format", "#[fg=#94a3b8] #I:#W "},
		{"window-status-current-format", "#[bold,bg=#e2e8f0,fg=#0f172a] #I:#W #[default]"},
		{"window-status-separator", ""},
		{"message-style", "bg=#e2e8f0,fg=#0f172a"},
		{"message-command-style", "bg=#dbeafe,fg=#0f172a"},
		{"pane-border-style", "fg=#334155"},
		{"pane-active-border-style", "fg=#0b5cad"},
	}
	for _, opt := range options {
		if err := setOption(target, opt.key, opt.value); err != nil {
			return err
		}
	}
	windowOptions := []struct {
		key   string
		value string
	}{
		{"mode-keys", "vi"},
		{"clock-mode-colour", "#0b5cad"},
	}
	for _, opt := range windowOptions {
		if err := setWindowOption(target, opt.key, opt.value); err != nil {
			return err
		}
	}
	// Make wheel/page scroll behave more like normal terminal scrollback:
	// scrolling up enters copy-mode automatically, then continued wheel
	// events/page keys keep moving through history with no tmux prefix.
	scrollBinds := []struct {
		table string
		key   string
		args  []string
	}{
		{
			table: "root",
			key:   "WheelUpPane",
			args: []string{
				"if-shell", "-F", "#{==:#{window_index},0}",
				"send-keys -M",
				`if-shell -F "#{pane_in_mode}" "send-keys -M" "copy-mode -eu"`,
			},
		},
		{
			table: "root",
			key:   "WheelDownPane",
			args: []string{
				"if-shell", "-F", "#{==:#{window_index},0}",
				"send-keys -M",
				`if-shell -F "#{pane_in_mode}" "send-keys -M" ""`,
			},
		},
		{
			table: "root",
			key:   "PageUp",
			args: []string{
				"if-shell", "-F", "#{==:#{window_index},0}",
				"send-keys PageUp",
				"copy-mode -eu",
			},
		},
		{
			table: "root",
			key:   "PageDown",
			args: []string{
				"if-shell", "-F", "#{==:#{window_index},0}",
				"send-keys PageDown",
				`if-shell -F "#{pane_in_mode}" "send-keys -X page-down" ""`,
			},
		},
	}
	for _, bind := range scrollBinds {
		if err := bindKeyArgs(bind.table, bind.key, bind.args...); err != nil {
			return err
		}
	}
	return nil
}

// shellQuote single-quotes a string for embedding inside a tmux
// pipe-pane command. tmux runs pipe-pane commands via /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
