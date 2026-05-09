package logs

import "testing"

func TestXDGDataHomeReturnsEmptyWithoutResolvableHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	if got := xdgDataHome(); got != "" {
		t.Fatalf("xdgDataHome() = %q, want empty", got)
	}
}
