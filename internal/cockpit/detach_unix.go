//go:build !windows

package cockpit

import "syscall"

// detachSysProcAttr puts the spawned foreman in its own session so SIGHUP
// from the parent shell doesn't kill it when sb exits.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
