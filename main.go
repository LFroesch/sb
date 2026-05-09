package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/LFroesch/sb/internal/accounts"
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
	if len(args) > 0 && args[0] == "account" {
		return runAccount(args[1:], stdout, stderr)
	}
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
		fmt.Fprintf(stderr, "Usage: sb [flags] [tmux-status|audit-taskfiles|account]\n\n")
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

func runAccount(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printAccountUsage(stderr)
		return 2
	}
	switch args[0] {
	case "list":
		slots, err := accounts.List()
		if err != nil {
			fmt.Fprintf(stderr, "sb account list: %v\n", err)
			return 1
		}
		if len(slots) == 0 {
			fmt.Fprintln(stdout, "no saved accounts")
			return 0
		}
		for _, slot := range slots {
			active := ""
			if slot.Active {
				active = " *"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s%s\n", slot.Provider, slot.Name, slot.SavedAt.Local().Format(timeFormat), active)
		}
		return 0
	case "show":
		status, err := accounts.Show()
		if err != nil {
			fmt.Fprintf(stderr, "sb account show: %v\n", err)
			return 1
		}
		for _, provider := range []string{"claude", "codex"} {
			name := strings.TrimSpace(status.Active[provider])
			if name == "" {
				name = "-"
			}
			fmt.Fprintf(stdout, "%s\t%s\n", provider, name)
		}
		return 0
	case "save", "use", "login":
		if len(args) != 3 {
			printAccountUsage(stderr)
			return 2
		}
		var err error
		switch args[0] {
		case "save":
			err = accounts.SaveCurrent(args[1], args[2])
		case "use":
			err = accounts.Use(args[1], args[2])
		case "login":
			err = runAccountLogin(args[1], args[2])
		}
		if err != nil {
			fmt.Fprintf(stderr, "sb account %s: %v\n", args[0], err)
			return 1
		}
		fmt.Fprintf(stdout, "%s %s %s\n", args[0], args[1], args[2])
		return 0
	default:
		printAccountUsage(stderr)
		return 2
	}
}

const timeFormat = "2006-01-02 15:04:05"

func printAccountUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sb account list")
	fmt.Fprintln(w, "  sb account show")
	fmt.Fprintln(w, "  sb account save <claude|codex> <name>")
	fmt.Fprintln(w, "  sb account use <claude|codex> <name>")
	fmt.Fprintln(w, "  sb account login codex <name>")
}

func runAccountLogin(provider, name string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "codex" {
		return fmt.Errorf("account login currently supports codex only")
	}
	dir, err := accounts.PrepareLogin(provider, name)
	if err != nil {
		return err
	}
	cmd := exec.Command("codex", "login")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+dir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return accounts.Use(provider, name)
}
