package cockpit

import "testing"

func TestDefaultPathsDoNotFallBackToRelativeDirsWithoutHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if got := xdgStateHome(""); got != "" {
		t.Fatalf("xdgStateHome(\"\") = %q, want empty", got)
	}
	if got := xdgDataHome(""); got != "" {
		t.Fatalf("xdgDataHome(\"\") = %q, want empty", got)
	}
	if got := xdgConfigHome(""); got != "" {
		t.Fatalf("xdgConfigHome(\"\") = %q, want empty", got)
	}

	if err := (Paths{}).EnsureDirs(); err == nil {
		t.Fatalf("EnsureDirs() = nil, want resolve error for missing user dirs")
	}
}
