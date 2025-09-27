package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bosley/txpx/cmd/txpx/assets"
	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/events"
	"github.com/fatih/color"
)

type AppTxPx struct {
	logger *slog.Logger

	appCtx    context.Context
	appCancel context.CancelFunc

	config     *Config
	installDir string

	rt app.AppRuntime

	appMeta            AppMetaTxPx
	httpServer         *AppTxPxHttpServer
	systemEventHandler *AppTxPxSystemEventHandler
}

func NewAppTxPx(
	config *Config,
	identifier string) *AppTxPx {

	home, err := os.UserHomeDir()
	if err != nil {
		color.Red("error getting user home directory: %s", err)
		home = "/tmp"
	}

	installDir := fmt.Sprintf(
		home,
		".config",
		fmt.Sprintf("txpx-%s", config.Domain),
	)

	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		err = os.MkdirAll(installDir, 0755)
		panic(err)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	ensureFileInstalled := func(destExpected string, srcExpected string) {
		defer wg.Done()
		if _, err := os.Stat(destExpected); os.IsNotExist(err) {
			data, err := assets.GetConfiguration(srcExpected)
			if err != nil {
				panic(err)
			}
			err = os.WriteFile(destExpected, data, 0644)
			if err != nil {
				panic(err)
			}
			color.Green("created file: %s", destExpected)
			time.Sleep(1 * time.Second)
		}
	}

	go ensureFileInstalled(
		filepath.Join(installDir, ConfigFileClusterDefault),
		ConfigFileClusterDefault,
	)

	go ensureFileInstalled(
		filepath.Join(installDir, ConfigFileSiteDefault),
		ConfigFileSiteDefault,
	)

	wg.Wait()

	return &AppTxPx{
		appMeta: AppMetaTxPx{
			identifier: identifier,
			startTime:  time.Now(),
		},
		config:     config,
		installDir: installDir,
	}
}

var _ app.ApplicationCandidate = &AppTxPx{}

var _ events.EventHandler = &AppTxPx{}

func (a *AppTxPx) GetAppMeta() app.AppMetaStat {
	return &a.appMeta
}

func (a *AppTxPx) Initialize(logger *slog.Logger, rt app.AppRuntimeSetup) error {

	a.logger = logger

	a.httpServer = NewAppTxPxHttpServer(logger, a.config)
	a.httpServer.Initialize(rt)

	a.systemEventHandler = &AppTxPxSystemEventHandler{
		logger: logger.With("component", EventTopicTxPxSystem),
		app:    a,
	}

	rt.ListenOn(EventTopicTxPxSystem, a.systemEventHandler)

	rt.ListenOn(EventTopicTxPxMainApp, a)

	return nil
}

func (a *AppTxPx) OnEvent(event events.Event) {
	a.logger.Info("received event", "event", event)
}

func (a *AppTxPx) Main(ctx context.Context, rap app.AppRuntime) {

	a.appCtx, a.appCancel = context.WithCancel(ctx)
	a.rt = rap

	a.logger = rap.GetLogger("txpx-main-app")

	/*
		The app backend runs the server for us, so we don't need to do anything here
		other than inform the http server of the app instance so it can operate on
		it directly.
	*/
	a.httpServer.setApp(a)

}
