// sb-foreman is the agent-cockpit daemon. It owns Manager + Registry
// and serves the NDJSON cockpit protocol on a unix socket so the sb TUI
// can connect, disconnect, and reconnect without losing running jobs.
//
// Flags:
//
//	-list    print known jobs and exit (no server)
//	-serve   start the daemon and block (default when no other flag set)
//	-socket  override the socket path
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/logs"
)

func main() {
	var (
		listCmd  = flag.Bool("list", false, "list jobs and exit")
		serveCmd = flag.Bool("serve", false, "start the socket server (default)")
		sockPath = flag.String("socket", "", "override socket path")
	)
	flag.Parse()

	slog.SetDefault(logs.Open("sb-foreman", "info"))
	paths := cockpit.DefaultPaths()
	if *sockPath != "" {
		paths.Socket = *sockPath
	}

	mgr, err := cockpit.NewManager(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "foreman: %v\n", err)
		os.Exit(1)
	}

	if *listCmd {
		for _, j := range mgr.ListJobs() {
			fmt.Printf("%s  %-20s %-14s %s\n", j.ID, j.PresetID, j.Status, j.Repo)
		}
		return
	}

	// Default mode is serve. -serve is accepted for explicitness.
	_ = serveCmd

	l, err := cockpit.ListenUnix(paths.Socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "foreman: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(paths.Socket)

	if err := writePID(paths.PIDFile); err != nil {
		slog.Warn("foreman: write pid", "err", err)
	}
	defer os.Remove(paths.PIDFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		slog.Info("foreman: shutting down")
		cancel()
	}()

	slog.Info("foreman: serving", "socket", paths.Socket, "jobs", len(mgr.ListJobs()))
	fmt.Fprintf(os.Stderr, "sb-foreman: serving on %s (%d jobs)\n", paths.Socket, len(mgr.ListJobs()))
	if err := cockpit.Serve(ctx, l, mgr); err != nil {
		fmt.Fprintf(os.Stderr, "foreman: serve: %v\n", err)
		os.Exit(1)
	}
}

func writePID(path string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644)
}
