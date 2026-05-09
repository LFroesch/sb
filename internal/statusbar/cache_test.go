package statusbar

import "testing"

func TestCacheFileReturnsEmptyWhenNoStateDirAvailable(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")

	if got := cacheFile("demo.json"); got != "" {
		t.Fatalf("cacheFile = %q, want empty path", got)
	}
}
