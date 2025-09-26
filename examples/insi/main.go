package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/InsulaLabs/insi/client"
	"github.com/bosley/txpx/pkg/app"
	"github.com/fatih/color"
)

type DemoInsiApp struct {
	appCtx    context.Context
	appCancel context.CancelFunc

	installDir string

	logger *slog.Logger

	rt app.AppRuntime

	apiKey     string
	endpoints  []client.Endpoint
	skipVerify bool
}

var _ app.ApplicationCandidate = &DemoInsiApp{}
var _ app.AppInsiBinder = &DemoInsiApp{}

func (d *DemoInsiApp) GetInsiApiKey() string {
	return d.apiKey
}

func (d *DemoInsiApp) GetInsiEndpoints() []client.Endpoint {
	return d.endpoints
}

func (d *DemoInsiApp) GetInsiSkipVerify() bool {
	return d.skipVerify
}

func (d *DemoInsiApp) Initialize(logger *slog.Logger, rt app.AppRuntimeSetup) error {

	logger.Info("Initializing Insi demo app")

	rt.WithLogLevel(slog.LevelDebug)

	rt.SetInstallPath(d.installDir)

	rt.RequireInsi(d)
	return nil
}

func (d *DemoInsiApp) Main(ctx context.Context, rap app.AppRuntime) {

	d.rt = rap

	d.logger = rap.GetLogger("demo-insi-app")

	d.appCtx, d.appCancel = context.WithCancel(ctx)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		time.Sleep(2 * time.Second)
		color.HiCyan("Getting Insi client and pinging cluster")

		insiClient := d.rt.GetInsiPanel().GetInsiClient()
		if insiClient == nil {
			d.logger.Error("Failed to get Insi client")
			return
		}

		d.logger.Info("Pinging Insi cluster...")
		pingData, err := insiClient.Ping()
		if err != nil {
			d.logger.Error("Failed to ping Insi cluster", "error", err)
		} else {
			d.logger.Info("Successfully pinged Insi cluster",
				"cluster_id", pingData["cluster_id"],
				"cluster_name", pingData["cluster_name"],
				"cluster_region", pingData["cluster_region"],
				"cluster_version", pingData["cluster_version"])
		}

		color.HiGreen("Demonstrating filesystem integration")
		d.logger.Info("Setting a value in system settings")
		result, err := d.rt.GetFSPanel().GetDataStore().SystemSettings().Set([]byte("insi-demo"), []byte("connected!"))
		if err != nil {
			d.logger.Error("error setting system setting", "error", err)
		}
		d.logger.Info("Set result", "result", result)

		time.Sleep(2 * time.Second)

		d.logger.Info("Getting value from system settings")
		value, err := d.rt.GetFSPanel().GetDataStore().SystemSettings().Get([]byte("insi-demo"))
		if err != nil {
			d.logger.Error("error getting system setting", "error", err)
		} else {
			d.logger.Info("Retrieved value", "value", string(value))
		}
	}()

	for {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
		select {
		case <-d.appCtx.Done():
			return
		case <-ctx.Done():
			d.appCancel()
			return
		case <-interrupt:
			d.appCancel()
			d.logger.Info("shutting down")
			d.rt.Shutdown()
			return
		case <-ticker.C:
			d.logger.Debug("App heartbeat")
		}
	}
}

func NewDemoInsiApp() *DemoInsiApp {

	installDir := os.TempDir()

	apiKey := os.Getenv("INSI_API_KEY")
	if apiKey == "" {
		color.HiRed("INSI_API_KEY environment variable not set")
		panic("INSI_API_KEY is required")
	}

	skipVerify := false
	if os.Getenv("INSI_SKIP_VERIFY") == "true" {
		skipVerify = true
		color.HiYellow("WARNING: TLS certificate verification is disabled")
	}

	var endpoints []client.Endpoint
	if os.Getenv("INSI_IS_LOCAL") == "true" {
		endpoints = []client.Endpoint{
			{PublicBinding: "127.0.0.1:8443", PrivateBinding: "127.0.0.1:8443", ClientDomain: "127.0.0.1"},
			{PublicBinding: "127.0.0.1:8444", PrivateBinding: "127.0.0.1:8444", ClientDomain: "127.0.0.1"},
			{PublicBinding: "127.0.0.1:8445", PrivateBinding: "127.0.0.1:8445", ClientDomain: "127.0.0.1"},
		}
	} else {
		endpoints = []client.Endpoint{
			{PublicBinding: "red.insulalabs.io:443", PrivateBinding: "red.insulalabs.io:443", ClientDomain: "red.insulalabs.io"},
			{PublicBinding: "green.insulalabs.io:443", PrivateBinding: "green.insulalabs.io:443", ClientDomain: "green.insulalabs.io"},
			{PublicBinding: "blue.insulalabs.io:443", PrivateBinding: "blue.insulalabs.io:443", ClientDomain: "blue.insulalabs.io"},
		}
	}

	return &DemoInsiApp{
		installDir: installDir,
		apiKey:     apiKey,
		endpoints:  endpoints,
		skipVerify: skipVerify,
	}
}

func main() {
	color.HiYellow("Starting Insi Demo Application")
	color.HiYellow("Make sure INSI_API_KEY is set in your environment")
	color.HiYellow("Set INSI_IS_LOCAL=true for local development")
	color.HiYellow("Set INSI_SKIP_VERIFY=true to skip TLS certificate verification (local dev only)\n")

	opt := app.New(NewDemoInsiApp())

	app := opt.UnwrapOrElse(func() app.AppRuntime {
		color.HiRed("failed to create app")
		os.Exit(1)
		return nil
	})

	app.Launch()
}
