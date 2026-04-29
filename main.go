package main

import (
	"fmt"
	"github.com/LFroesch/sb/internal/statusbar"
	"github.com/LFroesch/sb/internal/tui"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tmux-status" {
		fmt.Print(statusbar.RenderTmuxLine())
		return
	}
	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sb: %v\n", err)
		os.Exit(1)
	}
}
