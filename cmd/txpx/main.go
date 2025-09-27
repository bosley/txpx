package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/InsulaLabs/insi/runtime"
	"github.com/bosley/txpx/pkg/app"
	"github.com/fatih/color"
	"github.com/google/uuid"
)

func main() {
	var (
		nodeID     = flag.String("as", "", "Node ID to run as (e.g., node0). Mutually exclusive with --host.")
		hostAll    = flag.Bool("host", false, "Run instances for all nodes in the config. Mutually exclusive with --as.")
		configPath = flag.String("config", "", "Path to the cluster configuration file.")
	)
	flag.Parse()

	if *hostAll && *nodeID != "" {
		color.HiRed("Error: --host and --as flags are mutually exclusive")
		os.Exit(1)
	}

	instanceId := uuid.New().String()

	txpxApp := NewAppTxPx(fmt.Sprintf("txpx-%s", instanceId))

	var insidArgs []string

	if *configPath != "" {
		insidArgs = append(insidArgs, "--config", *configPath)
	} else {
		insidArgs = append(insidArgs, "--config", filepath.Join(
			txpxApp.GetInstallDir(),
			ConfigFileClusterDefault,
		))
	}

	if *hostAll {
		insidArgs = append(insidArgs, "--host")
	} else if *nodeID != "" {
		insidArgs = append(insidArgs, "--as", *nodeID)
	} else {
		color.HiRed("Error: --host or --as flag is required")
		os.Exit(1)
	}

	if txpxApp.config.Prod {
		insidArgs = append(insidArgs, "--prod")
	}

	opt := app.New(txpxApp)

	application := opt.UnwrapOrElse(func() app.AppRuntime {
		color.HiRed("failed to create app")
		os.Exit(1)
		return nil
	})

	application.Launch()

	go insid(txpxApp.appCtx, insidArgs)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-txpxApp.appCtx.Done():
		if application == nil {
			return
		}
		application.Shutdown()
		return
	case <-signalChan:
		if application == nil {
			return
		}
		application.Shutdown()
		return
	}
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
