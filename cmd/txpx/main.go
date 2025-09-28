package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/InsulaLabs/insi/config"
	"github.com/bosley/txpx/cmd/txpx/assets"
	"github.com/bosley/txpx/internal/insid"
	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/events"
	"github.com/fatih/color"
	"github.com/google/uuid"
)

const (
	VERSION_MAJOR = 0
	VERSION_MINOR = 1
	VERSION_PATCH = 0
)

var (
	VERSION = fmt.Sprintf("%d.%d.%d", VERSION_MAJOR, VERSION_MINOR, VERSION_PATCH)
)

func init() {
	/*
		We will be using a lot of uuids, so we need to enable the rand pool
		so we don't have to wait for the first call to uuid.New() to initialize
		the pool.
	*/
	uuid.EnableRandPool()
}

/*
This application comes up and helps us boostrap insi clusters/ utilize them for cross-
application coordination.
Each application instance can be configured to host the entire cluster itself, or simply
start a node of a cluster so it can bind with its peers.

The goal is to make txpx an app to quickly launch and utilize the insi network for
distributed coordination and web services.
*/
func main() {
	// Insi Specific Flags
	var (
		usingInsi  = flag.Bool("insi", false, "Run the insi cluster itself.")
		nodeID     = flag.String("as", "", "Node ID to run as (e.g., node0). Mutually exclusive with --host.")
		hostAll    = flag.Bool("host", false, "Run instances for all nodes in the config. Mutually exclusive with --as.")
		configPath = flag.String("config", "", "Path to the cluster configuration file.")
	)

	// TxPx Specific Flags
	var (
		version = flag.Bool("version", false, "Print the version and exit.")
	)

	flag.Parse()

	// txpx
	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	instanceId := uuid.New().String()

	txpxApp := NewAppTxPx(fmt.Sprintf("txpx-%s", instanceId), *usingInsi)

	// insi
	if *usingInsi {

		if *hostAll && *nodeID != "" {
			color.HiRed("Error: --host and --as flags are mutually exclusive")
			os.Exit(1)
		}

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

		txpxApp.SetupInsidPreLaunch(insidArgs)

	}

	/*
		Before we launch the app, we check to see if there are any cli arguments specific to
		the app that we need to handle
	*/
	remainingArgs := flag.Args()

	if len(remainingArgs) > 0 {
		if err := executeCliArguments(remainingArgs, txpxApp); err != nil {
			color.HiRed("Error: %v", err)
			os.Exit(1)
		}
	}

	opt := app.New(txpxApp)

	application := opt.UnwrapOrElse(func() app.AppRuntime {
		color.HiRed("failed to create app")
		os.Exit(1)
		return nil
	})

	application.Launch()

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

func (a *AppMetaTxPx) APIHeaderExpectation() [4]byte {
	return [4]byte{
		HeaderValImportanceHighest,
		HeaderValThreadOriginTxPxApi,
		HeaderValSpecificEventTxPxApiToMainApp,
		HeaderValFilterAcceptAll,
	}
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

	insidArgs       []string
	insidController insid.Controller
	insiCfg         *config.Cluster
}

func NewAppTxPx(
	identifier string, usingInsi bool) *AppTxPx {

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

	var insiCfg *config.Cluster
	if usingInsi {
		/*
			This is the insi cluster configuration that the application is hosting.
			From this we derive the root insi api key for full control of the cluster or
			node (depending on --host or --as)

			With this insi as a node, we can briddge multiple instances of the application
			across the insi event system
		*/
		cfg, err := config.LoadConfig(filepath.Join(installDir, ConfigFileClusterDefault))
		if err != nil {
			panic(err)
		}
		insiCfg = cfg
	}

	return &AppTxPx{
		appMeta: AppMetaTxPx{
			identifier: identifier,
			startTime:  time.Now(),
		},
		config:     appCfg,
		installDir: installDir,
		insiCfg:    insiCfg,
	}
}

func (a *AppTxPx) SetupInsidPreLaunch(args []string) {
	a.insidArgs = args
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

	// with insi
	if a.insiCfg != nil {
		a.logger.Info("setting up insi controller")
		a.insidController = insid.NewController(
			a.logger.With("component", "insid"),
			a.insiCfg,
			a,
		)
		rt.RequireInsi(a.insidController)
	}

	return nil
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

	if a.insidController != nil {
		go a.insidController.Start(a.appCtx, a.insidArgs, 10*time.Second, a.rt)
	}
}

func (a *AppTxPx) OnApiEvent(event events.Event) {
	/*
		A TX event from teh app - an api message of some sort
	*/
	a.logger.Info("received  event", "event", event)
}

func (a *AppTxPx) OnEvent(event events.Event) {
	/*
		A normal event from somewhere in this app
	*/
	a.logger.Info("received event", "event", event)
}

func (a *AppTxPx) InsidOnline(insiPingData map[string]string) {
	systemPublisher, err := a.rt.GetEventsPanel().GetTopicPublisher(EventTopicTxPxSystem)
	if err != nil {
		a.logger.Error("Failed to get system publisher", "error", err)
		return
	}
	systemPublisher.Publish(events.Event{
		Header: ConstructEventHeader(
			HeaderValImportanceHighest,
			HeaderValThreadOriginTxPxInsi,
			int(SystemEventIdentifierInsiOnline),
		),
		Body: insiPingData,
	})
}

func (a *AppTxPx) InsidOffline() {
	systemPublisher, err := a.rt.GetEventsPanel().GetTopicPublisher(EventTopicTxPxSystem)
	if err != nil {
		a.logger.Error("Failed to get system publisher", "error", err)
		return
	}
	systemPublisher.Publish(events.Event{
		Header: ConstructEventHeader(
			HeaderValImportanceHighest,
			HeaderValThreadOriginTxPxInsi,
			int(SystemEventIdentifierInsiOffline),
		),
	})
}
