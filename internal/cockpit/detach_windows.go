//go:build windows

package cockpit

import "syscall"

func detachSysProcAttr() *syscall.SysProcAttr {
	return nil
}
