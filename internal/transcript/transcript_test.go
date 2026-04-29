package transcript

import "testing"

func TestSanitizeNormalizesTerminalControlOutput(t *testing.T) {
	raw := "draft\rfinal\x1b[31m output\x1b[0m\nspin\b!\x07\n"
	got := Sanitize(raw)
	want := "final output\nspi!"
	if got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}

func TestSanitizeDropsBoxDrawingChrome(t *testing.T) {
	raw := "╭────────────╮\nactual output\n│            │\nnext line\n╰────────────╯\n"
	got := Sanitize(raw)
	want := "actual output\nnext line"
	if got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}
