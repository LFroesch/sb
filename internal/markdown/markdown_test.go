package markdown

import (
	"strings"
	"testing"
)

func TestRenderListItemUsesHangingIndent(t *testing.T) {
	out := Render("- this is a long list item that should wrap onto another line cleanly", 22)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("Render() produced %d lines, want wrapped output", len(lines))
	}
	if !strings.HasPrefix(lines[0], "    · ") {
		t.Fatalf("first line = %q, want bullet prefix", lines[0])
	}
	if !strings.HasPrefix(lines[1], "      ") {
		t.Fatalf("wrapped line = %q, want hanging indent", lines[1])
	}
}
