package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/InsulaLabs/insi/config"
	"github.com/bosley/txpx/cmd/txpx/assets"
	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/events"
	"github.com/fatih/color"
)

/*
By design the app event system only permits one handler per topic.
The idea is that each thing is only concerned with what its doing.

The main app is the exception in that it is responsible for handling
its own, as the others do, but also system events.

System events are what will bridge the application as a whole to
external intances of the application via insi et all.

Whenever http or someone gets something that is determined to be
a system event FROM an external instance, they will publish it to
the system event topic.

The main app will then handle it and determine what to do with it.
*/
type AppTxPxSystemEventHandler struct {
	logger *slog.Logger
	app    *AppTxPx
}

var _ events.EventHandler = &AppTxPxSystemEventHandler{}

func (a *AppTxPxSystemEventHandler) OnEvent(event events.Event) {

	switch event.Header[3] {
	case byte(SystemEventIdentifierInsiOnline):
		color.HiGreen("Insi is online %+v", event.Body)
	case byte(SystemEventIdentifierInsiOffline):
		color.HiRed("Insi is offline %+v", event.Body)
	}
}

/*
The app meta identifies the instance of the application as we will
be operating in multiple processes and potentially across multiple nodes.
*/
type AppMetaTxPx struct {
	identifier string
	startTime  time.Time
}

var _ app.AppMetaStat = &AppMetaTxPx{}

func (a *AppMetaTxPx) GetIdentifier() string {
	return a.identifier
}

func (a *AppMetaTxPx) GetUptime() time.Duration {
	return time.Since(a.startTime)
}

/*
The primary set of application data. This holds all of the items
registered to the loose runtime framework.

The app is setup to be event driven. Once the application is initialized,
it will listen on its various server mediums and either handle them directly
or emit an event to the appropriate topic for handling.
*/
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
	insiProvider       *AppTxPxInsiProvider
}

func NewAppTxPx(
	identifier string) *AppTxPx {

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	installDir := filepath.Join(home, ".config", "txpx")

	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		err = os.MkdirAll(installDir, 0755)
		if err != nil {
			panic(err)
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	// We install the two configuration files to the install directory,
	// but if we do that (first run) we need to wait for the file to "settle"
	// so we do this in parallel and wait for them to settle.
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

	/*
		Load the web application configuration, that which the http
		routes of the "site" will be, as the cluster is going to be
		bound privately, the http server will act as a medium for
		the txpx application sets to interact with eachothers clusters,
		even if they aren't part of the same cluster network.
	*/
	appCfg, err := LoadConfig(filepath.Join(installDir, ConfigFileSiteDefault))
	if err != nil {
		panic(err)
	}

	/*
		This is the insi cluster configuration that the application is hosting.
		From this we derive the root insi api key for full control of the cluster or
		node (depending on --host or --as)

		With this insi as a node, we can briddge multiple instances of the application
		across the insi event system
	*/
	insiCfg, err := config.LoadConfig(filepath.Join(installDir, ConfigFileClusterDefault))
	if err != nil {
		panic(err)
	}

	return &AppTxPx{
		appMeta: AppMetaTxPx{
			identifier: identifier,
			startTime:  time.Now(),
		},
		config:       appCfg,
		installDir:   installDir,
		insiProvider: NewAppTxPxInsiProvider(insiCfg),
	}
}

func (a *AppTxPx) GetInstallDir() string {
	return a.installDir
}

var _ app.ApplicationCandidate = &AppTxPx{}

var _ events.EventHandler = &AppTxPx{}

func (a *AppTxPx) GetAppMeta() app.AppMetaStat {
	return &a.appMeta
}

func (a *AppTxPx) Initialize(logger *slog.Logger, rt app.AppRuntimeSetup) error {

	a.logger = logger

	// http
	a.httpServer = NewAppTxPxHttpServer(logger, a.config)
	a.httpServer.Initialize(rt)

	// events
	a.systemEventHandler = &AppTxPxSystemEventHandler{
		logger: logger.With("component", EventTopicTxPxSystem),
		app:    a,
	}

	rt.ListenOn(EventTopicTxPxSystem, a.systemEventHandler)

	rt.ListenOn(EventTopicTxPxMainApp, a)

	// kv datastore
	rt.SetInstallPath(a.installDir)

	// insi
	rt.RequireInsi(a.insiProvider)

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

	// insi
	a.insiProvider.SetApp(a)

	go a.insiProvider.MonitorForInsiStartup()
}
