// Package cockpit bootstrap.go: self-exec sb inside the cockpit's
// isolated tmux session so window 0 of sb-cockpit is sb itself, and
// windows 1..N can hold per-job claude/codex panes the user can jump
// between with F1.

package cockpit

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ExecFallback is set by MaybeReExecIntoTmux when tmux is unavailable
// (or the user opted out via SB_NO_TMUX). The TUI reads this to show a
// header badge and skip tmux-only affordances.
var ExecFallback bool

// Env keys we set/read to keep the bootstrap from looping.
const (
	EnvInCockpit = "SB_IN_COCKPIT" // set to 1 by the re-exec child
	EnvNoTmux    = "SB_NO_TMUX"    // user opt-out
)

// MaybeReExecIntoTmux re-execs sb inside the cockpit tmux session if
// conditions are right. Call early in main() before any TTY setup.
//
// Returns:
//   - reExeced: true on the parent side when exec succeeded (the caller
//     should consider itself replaced; in practice Go keeps running if
//     syscall.Exec returns, indicating an error — we handle that).
//   - fallback: true when we intentionally skipped tmux (no binary,
//     env opt-out, or we're already the child). Set ExecFallback too.
func MaybeReExecIntoTmux() (reExeced bool, fallback bool, err error) {
	if os.Getenv(EnvInCockpit) == "1" {
		return false, false, nil
	}
	if v := os.Getenv(EnvNoTmux); v == "1" || v == "true" {
		ExecFallback = true
		return false, true, nil
	}
	if !HasTmux() {
		ExecFallback = true
		return false, true, nil
	}

	self, err := os.Executable()
	if err != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("resolve self: %w", err)
	}

	// Ensure the cockpit session exists.
	if err := EnsureSession(CockpitSession); err != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("ensure cockpit session: %w", err)
	}

	// Kill any stale sb window from a previous crashed run. We look for
	// the first window named "sb" and replace its command.
	// Simpler: respawn-kill-respawn. We send select-window + respawn-window.
	// For v2 we assume the user runs one sb per host; if there is a
	// previous sb window, just reuse its slot.
	//
	// Easiest correct path: try kill-window on sb-cockpit:sb (ignore
	// error if missing), then create a new one running this binary.
	_, _ = runTmux("kill-window", "-t", CockpitSession+":sb")

	_, err = runTmux(
		"new-window", "-t", CockpitSession+":0",
		"-n", "sb",
		"-e", EnvInCockpit+"=1",
		"--",
		self,
	)
	if err != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("new-window sb: %w", err)
	}

	// Root-table bindings for getting back to window 0 (sb itself).
	// F1 is nice when terminals pass it through, but VS Code / Cursor
	// often intercept function keys, so bind plain-key fallbacks too.
	_ = BindKey("F1", "select-window -t "+CockpitSession+":0")
	_ = BindKey("C-g", "select-window -t "+CockpitSession+":0")
	_ = BindKey("F12", "select-window -t "+CockpitSession+":0")
	// Make Ctrl+C safer in attached job windows: when you're on window 0
	// (sb itself), preserve the normal Ctrl+C behavior by forwarding it
	// into the pane. Everywhere else, treat Ctrl+C as "back to sb"
	// instead of sending SIGINT to the agent process.
	_, _ = runTmux(
		"bind-key", "-T", "root", "C-c",
		"if-shell", "-F", "#{==:#{window_index},0}",
		"send-keys C-c",
		"select-window -t "+CockpitSession+":0",
	)

	// Finally, exec tmux attach so the user lands in the session. We
	// prefer syscall.Exec so our parent process is replaced — tmux
	// becomes the foreground process. If that fails (e.g. not a TTY),
	// fall through to fallback.
	tmux, err := exec.LookPath(TmuxBin())
	if err != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("lookpath tmux: %w", err)
	}
	args := []string{tmux, "-L", TmuxServerLabel, "attach", "-t", CockpitSession + ":sb"}
	env := os.Environ()
	if execErr := syscall.Exec(tmux, args, env); execErr != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("exec tmux attach: %w", execErr)
	}
	// Unreachable on success — exec replaced us.
	return true, false, nil
}

// ShouldDetachOnQuit reports whether the current sb process is running
// as window 0 inside the isolated cockpit tmux session. In that case a
// user-triggered quit should detach the client rather than just killing
// the dashboard process and leaving the tmux client attached.
func ShouldDetachOnQuit() bool {
	return os.Getenv(EnvInCockpit) == "1" && InsideTmux() && !ExecFallback
}
