package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/LFroesch/sb/internal/statusbar"
	"github.com/LFroesch/sb/internal/tui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tmux-status" {
		fmt.Print(statusbar.RenderTmuxLine())
		return
	}

	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "sb — Second Brain control plane for managing WORK.md files across projects\n\n")
		fmt.Fprintf(os.Stderr, "Usage: sb [flags] [tmux-status]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("sb " + version)
		os.Exit(0)
	}

	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sb: %v\n", err)
		os.Exit(1)
	}
}
