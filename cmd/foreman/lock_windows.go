//go:build windows

package main

import "os"

func nonBlockingLock(f *os.File) error {
	return nil
}
