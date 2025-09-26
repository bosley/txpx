package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bosley/txpx/app"
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
}

var _ app.ApplicationCandidate = &DemoHttpsApp{}
var _ app.AppHTTPBinder = &DemoHttpsApp{}

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
}

func (d *DemoHttpsApp) GetApiMountPoint() string {
	return "/api"
}

func (d *DemoHttpsApp) Initialize(logger *slog.Logger, rt app.AppRuntimeSetup) error {

	logger.Info("This is just the default logger. It wont be used in the app.")

	// indicate we want to use Default log level
	rt.WithLogLevel(slog.LevelDebug)

	// indicate we want to use the install directory
	rt.SetInstallPath(d.installDir)

	// indicate we want to use the http server binder
	rt.RequireHttpServer(d)
	return nil
}

func (d *DemoHttpsApp) Main(ctx context.Context, rap app.AppRuntime) {

	d.rt = rap

	d.logger = rap.GetLogger("demo-https-app")

	d.appCtx, d.appCancel = context.WithCancel(ctx)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go func() {
		time.Sleep(5 * time.Second)
		color.HiCyan("setting system setting")

		d.logger.Info("setting system setting")
		result, err := d.rt.GetFSPanel().GetDataStore().SystemSettings().Set([]byte("hello"), []byte("world!"))
		if err != nil {
			d.logger.Error("error setting system setting", "error", err)
		}
		d.logger.Info("result", "result", result)

		time.Sleep(5 * time.Second)
		color.HiGreen("getting system setting")

		d.logger.Info("getting system setting")
		value, err := d.rt.GetFSPanel().GetDataStore().SystemSettings().Get([]byte("hello"))
		if err != nil {
			d.logger.Error("error getting system setting", "error", err)
		}
		d.logger.Info("value", "value", string(value))
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
			d.logger.Debug("tick")
		}
	}
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

func main() {
	opt := app.New(NewDemoHttpsApp(":443"))

	app := opt.UnwrapOrElse(func() app.AppRuntime {
		color.HiRed("failed to create app")
		os.Exit(1)
		return nil
	})

	app.Launch()
}
