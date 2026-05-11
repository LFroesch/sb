package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/statusbar"
	"github.com/LFroesch/sb/internal/tui"
	"github.com/LFroesch/sb/internal/workmd"
)

var version = "dev"

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "tmux-status" {
		fmt.Fprint(stdout, statusbar.RenderTmuxLine())
		return 0
	}
	if len(args) > 0 && args[0] == "audit-taskfiles" {
		cfg := config.Load()
		issues, err := workmd.AuditDiscoveredFiles(cfg)
		if err != nil {
			fmt.Fprintf(stderr, "sb audit-taskfiles: %v\n", err)
			return 1
		}
		if len(issues) == 0 {
			fmt.Fprintln(stdout, "all discovered task files match the canonical schema")
			return 0
		}
		for _, issue := range issues {
			fmt.Fprintln(stdout, issue.Path)
			for _, msg := range issue.Issues {
				fmt.Fprintln(stdout, "  - "+msg)
			}
		}
		return 1
	}

	fs := flag.NewFlagSet("sb", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "sb — Second Brain control plane for managing WORK.md files across projects\n\n")
		fmt.Fprintf(stderr, "Usage: sb [flags] [tmux-status|audit-taskfiles]\n\n")
		fs.PrintDefaults()
	}
	showVersion := fs.Bool("version", false, "Print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "sb "+version)
		return 0
	}

	if err := tui.Run(); err != nil {
		fmt.Fprintf(stderr, "sb: %v\n", err)
		return 1
	}
	return 0
}
