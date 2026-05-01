// Package cockpit bootstrap.go: self-exec sb inside the cockpit's
// isolated tmux session so window 0 of sb-cockpit is sb itself, and
// windows 1..N can hold per-job claude/codex panes the user can jump
// between with F1.

package cockpit

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

const cockpitDashboardTarget = CockpitSession + ":" + CockpitDashboardWindow

func dashboardShellCommand(self string) string {
	return fmt.Sprintf("export %s=1; exec %s", EnvInCockpit, shellQuote(self))
}

func createDashboardSession(self string) error {
	if _, err := runTmux(
		"new-session", "-d", "-s", CockpitSession,
		"-n", CockpitDashboardWindow,
		"-x", "200", "-y", "50",
		dashboardShellCommand(self),
	); err != nil {
		return err
	}
	return ConfigureSession(CockpitSession)
}

func dashboardWindowAlive(self string) (bool, error) {
	out, err := runTmux("list-panes", "-t", cockpitDashboardTarget, "-F", "#{pane_pid}\t#{pane_dead}\t#{pane_current_command}")
	if err != nil {
		if isTmuxMissingResourceError(err) {
			return false, nil
		}
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		if fields[1] == "0" && processAlive(pid) && sameExecutableName(fields[2], self) {
			return true, nil
		}
	}
	return false, nil
}

func ensureDashboardWindow(self string) error {
	alive, err := dashboardWindowAlive(self)
	if err != nil {
		return err
	}
	if alive {
		return nil
	}

	out, err := runTmux("list-panes", "-t", cockpitDashboardTarget, "-F", "#{pane_dead}")
	if err != nil {
		if isTmuxMissingResourceError(err) {
			_, err = runTmux(
				"new-window", "-d",
				"-t", CockpitSession+":",
				"-n", CockpitDashboardWindow,
				dashboardShellCommand(self),
			)
			return err
		}
		return err
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("dashboard target %s exists but has no panes", cockpitDashboardTarget)
	}
	_, err = runTmux("respawn-window", "-k", "-t", cockpitDashboardTarget, dashboardShellCommand(self))
	return err
}

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

	exists, err := tmuxSessionExists(CockpitSession)
	if err != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("ensure cockpit session: %w", err)
	}
	if !exists {
		if err := createDashboardSession(self); err != nil {
			ExecFallback = true
			return false, true, fmt.Errorf("create cockpit session: %w", err)
		}
	} else if err := ensureDashboardWindow(self); err != nil {
		ExecFallback = true
		return false, true, fmt.Errorf("ensure dashboard window: %w", err)
	}

	// Root-table bindings for getting back to the shared dashboard window.
	// F1 is nice when terminals pass it through, but VS Code / Cursor
	// often intercept function keys, so bind plain-key fallbacks too.
	_ = BindKey("F1", "select-window -t "+cockpitDashboardTarget)
	_ = BindKey("C-g", "select-window -t "+cockpitDashboardTarget)
	_ = BindKey("F12", "select-window -t "+cockpitDashboardTarget)
	// Make Ctrl+C safer in attached job windows: on the shared dashboard,
	// detach just the current client when multiple terminals are attached
	// so one operator does not kill sb for everyone else. Everywhere else,
	// treat Ctrl+C as "back to sb" instead of sending SIGINT into the
	// agent process.
	_, _ = runTmux(
		"bind-key", "-T", "root", "C-c",
		"if-shell", "-F", dashboardWindowCondition(),
		`if-shell -F "#{>:#{session_attached},1}" "detach-client" "send-keys C-c"`,
		"select-window -t "+cockpitDashboardTarget,
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
	args := []string{tmux, "-L", TmuxServerLabel, "attach", "-t", CockpitSession}
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
