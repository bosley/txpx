package main

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"

	"github.com/InsulaLabs/insi/runtime"
	"github.com/fatih/color"
)

func main() {

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	args := os.Args[1:]

	if len(args) == 0 {
		color.HiRed("no arguments provided")
		os.Exit(1)
	}

	// Process app-sepcific args (if any) and then ensure all are removed if not related to insid

	go insid(appCtx, args)

	select {
	case <-appCtx.Done():
		return
	}
}

func showHelp() {
}

// This is the entire main for insid. We need to start our app with http and an insi client,
// but before we even define the app we should start insid in a different thread.
// Then, we can tie our insi client to the insi cluster running in the background.
// This will provide a stable distributed cache/kv store for the application to use
// We can ALSO (later) hookup the insi event system to the application event system
// and then leverage the insi raft eventing system to coordinate with other instances of the application
// That will be once we get this up and running.
func insid(ctx context.Context, args []string) {
	// misconfigured logger in raft bbolt backend
	// we use slog so this only silences the bbolt backend
	// to raft snapshot store that isn't directly used by us
	log.SetOutput(io.Discard)

	// The default config file path can be set here
	// It's passed to the runtime, which handles flag parsing for --config override.
	rt, err := runtime.New(ctx, args, "cluster.yaml")
	if err != nil {
		slog.Error("Failed to initialize runtime", "error", err)
		os.Exit(1)
	}

	if err := rt.Run(); err != nil {
		slog.Error("Runtime exited with error", "error", err)
		os.Exit(1)
	}

	rt.Wait()
	slog.Info("Application exiting.")
}
