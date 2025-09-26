// Package main demonstrates a combined HTTP + Sidecar application using txpx.
// This example shows:
// - HTTPS server with self-signed certificates
// - Sidecar process management that adapts to the operating system
// - HTTP endpoints to control sidecar processes
// - Runtime OS detection for cross-platform compatibility
// Note: AI generated this based off of the basic example (grok code) - bosley
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/bosley/txpx/app"
	"github.com/bosley/txpx/pkg/procman"
	"github.com/bosley/txpx/pkg/security"
	"github.com/fatih/color"
)

type DemoHttpsApp struct {
	appCtx    context.Context
	appCancel context.CancelFunc

	binding    string
	keysDir    string
	installDir string

	logger *slog.Logger

	rt app.AppRuntime

	sidecarAppName string
}

var _ app.ApplicationCandidate = &DemoHttpsApp{}
var _ app.AppHTTPBinder = &DemoHttpsApp{}
var _ app.AppExternalBinder = &DemoHttpsApp{}

// HTTP
func (d *DemoHttpsApp) GetBinding() string {
	return d.binding
}

func (d *DemoHttpsApp) GetCertPath() string {
	return filepath.Join(d.keysDir, "cert.pem")
}

func (d *DemoHttpsApp) GetKeyPath() string {
	return filepath.Join(d.keysDir, "key.pem")
}

func (d *DemoHttpsApp) BindPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", d.handleRootGet())
	mux.HandleFunc("/sidecar/start", d.handleSidecarStart())
	mux.HandleFunc("/sidecar/stop", d.handleSidecarStop())
	mux.HandleFunc("/sidecar/status", d.handleSidecarStatus())
}

func (d *DemoHttpsApp) GetApiMountPoint() string {
	return "/api"
}

// SideCar
func (d *DemoHttpsApp) BindProcman() *procman.Host {
	return procman.NewHost(slog.Default().With("component", "procman"))
}

func (d *DemoHttpsApp) Initialize(logger *slog.Logger, rt app.AppRuntimeSetup) error {

	logger.Info("This is just the default logger. It wont be used in the app.")

	// indicate we want to use Default log level
	rt.WithLogLevel(slog.LevelDebug)

	// indicate we want to use the install directory
	rt.SetInstallPath(d.installDir)

	// indicate we want to use the http server binder
	rt.RequireHttpServer(d)

	// indicate we want to use the sidecar binder
	rt.RequireSideCar(d)
	return nil
}

func (d *DemoHttpsApp) Run(ctx context.Context, rap app.AppRuntime) {

	d.rt = rap

	d.logger = rap.GetLogger("demo-https-app")

	d.appCtx, d.appCancel = context.WithCancel(ctx)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.appCtx.Done():
			return
		case <-ctx.Done():
			d.appCancel()
			return
		case <-ticker.C:
			d.logger.Debug("tick")
		}
	}
}

func (d *DemoHttpsApp) Shutdown() {
}

func NewDemoHttpsApp(binding string) *DemoHttpsApp {

	installDir := os.TempDir()

	keysDir := filepath.Join(installDir, "keys")
	err := os.MkdirAll(keysDir, 0755)
	if err != nil {
		panic(err)
	}

	err = security.GenerateSelfSignedCert(keysDir)
	if err != nil {
		panic(err)
	}

	return &DemoHttpsApp{
		binding:    binding,
		keysDir:    keysDir,
		installDir: installDir,
	}
}

func (d *DemoHttpsApp) handleRootGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World! - Current Auth Token: " + d.rt.GetHttpPanel().BumpAuthToken()))
	}
}

func (d *DemoHttpsApp) handleSidecarStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if sidecar is already running
		if d.sidecarAppName != "" {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("Sidecar is already running"))
			return
		}

		host := d.rt.GetSideCarPanel().GetHost()

		// Choose program based on OS - use long-running commands for demo
		var targetPath string
		var cmdArgs []string

		switch runtime.GOOS {
		case "windows":
			targetPath = "cmd.exe"
			cmdArgs = []string{"/c", "for /L %i in (1,0,1) do (echo Sidecar running on", runtime.GOOS, "& timeout /t 5 /nobreak >nul)"}
		default: // unix-like systems
			targetPath = "sh"
			cmdArgs = []string{"-c", "while true; do echo 'Sidecar running on " + runtime.GOOS + "'; sleep 5; done"}
		}

		app := procman.NewHostedApp(procman.HostedAppOpt{
			Name: "demo-sidecar",
			StartStopCb: func(name string, running bool) {
				status := "stopped"
				if running {
					status = "running"
				}
				d.logger.Info("Sidecar status changed", "name", name, "status", status)
				if !running {
					d.sidecarAppName = "" // Clear the name when stopped
				}
			},
			TargetPath: targetPath,
			CmdArgs:    cmdArgs,
			CmdOptions: procman.DefaultOptions(),
		})

		host = host.WithApp(app)
		d.sidecarAppName = "demo-sidecar"

		err := host.StartApp("demo-sidecar")
		if err != nil {
			d.logger.Error("Failed to start sidecar", "error", err)
			d.sidecarAppName = "" // Clear on failure
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("Failed to start sidecar: %v", err)))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Sidecar started successfully on %s!", runtime.GOOS)))
	}
}

func (d *DemoHttpsApp) handleSidecarStop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.sidecarAppName == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("No sidecar is running"))
			return
		}

		host := d.rt.GetSideCarPanel().GetHost()
		err := host.StopApp(d.sidecarAppName)
		if err != nil {
			d.logger.Error("Failed to stop sidecar", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("Failed to stop sidecar: %v", err)))
			return
		}

		// sidecarAppName will be cleared by the callback when stopped
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Sidecar stop initiated successfully!"))
	}
}

func (d *DemoHttpsApp) handleSidecarStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := fmt.Sprintf("No sidecar running on %s", runtime.GOOS)
		if d.sidecarAppName != "" {
			status = fmt.Sprintf("Sidecar '%s' is running on %s (prints status every 5 seconds)", d.sidecarAppName, runtime.GOOS)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(status))
	}
}

// main starts the HTTPS + Sidecar demo application.
// Usage:
//  1. Run: go run main.go
//  2. Visit https://localhost:443/ (accept self-signed cert)
//  3. Visit https://localhost:443/sidecar/start to start sidecar
//  4. Visit https://localhost:443/sidecar/status to check status
//  5. Visit https://localhost:443/sidecar/stop to stop sidecar
func main() {
	opt := app.New(NewDemoHttpsApp(":443"))

	app := opt.UnwrapOrElse(func() app.AppRuntime {
		color.HiRed("failed to create app")
		os.Exit(1)
		return nil
	})

	app.Launch()
}
