package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/statusbar"
	"github.com/LFroesch/sb/internal/tui"
	"github.com/LFroesch/sb/internal/workmd"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tmux-status" {
		fmt.Print(statusbar.RenderTmuxLine())
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "audit-taskfiles" {
		cfg := config.Load()
		issues, err := workmd.AuditDiscoveredFiles(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sb audit-taskfiles: %v\n", err)
			os.Exit(1)
		}
		if len(issues) == 0 {
			fmt.Println("all discovered task files match the canonical schema")
			return
		}
		for _, issue := range issues {
			fmt.Println(issue.Path)
			for _, msg := range issue.Issues {
				fmt.Println("  - " + msg)
			}
		}
		os.Exit(1)
	}

	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "sb — Second Brain control plane for managing WORK.md files across projects\n\n")
		fmt.Fprintf(os.Stderr, "Usage: sb [flags] [tmux-status|audit-taskfiles]\n\n")
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
