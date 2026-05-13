//go:build windows

package workmd

func statFileKey(path string) (string, bool) {
	return "", false
}
