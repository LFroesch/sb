//go:build !windows

package cockpit

import "syscall"

func processAlivePID(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
