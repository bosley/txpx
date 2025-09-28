package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/bosley/txpx/pkg/app/internal/api/v1"
	"github.com/bosley/txpx/pkg/beau"
	"github.com/bosley/txpx/pkg/events"
	"github.com/bosley/txpx/pkg/pool"
	"github.com/bosley/txpx/pkg/procman"
	"github.com/bosley/txpx/pkg/xfs"
)

type AppMetaStat interface {
	GetIdentifier() string // Must be unique to all instances across all processes - this is how we specificly event-to-from each other
	GetUptime() time.Duration

	APIHeaderExpectation() [4]byte
}

type ApplicationCandidate interface {
	Initialize(
		logger *slog.Logger,
		rt AppRuntimeSetup,
	) error

	GetAppMeta() AppMetaStat

	GetAppSecret() string

	/*
		A TX Event is a transaction event from the app backend.
		This is required, even if no http insi fs et al are used
		as this will be the async way that the app backend interfaces
		and communicates with the app components.

		The topic-locked publisher is provi
	*/
	OnApiEvent(event events.Event)

	Main(ctx context.Context, rap AppRuntime)
}

/*
The actuall application runtime that offers controls over the various operational stacks
*/
type AppRuntime interface {
	GetLogger(component string) *slog.Logger

	GetWorkPool() *pool.Pool
	OnError(err error)

	GetHttpPanel() AppHttpPanel

	GetFSPanel() AppFSPanel

	GetSideCarPanel() AppSideCarPanel

	GetInsiPanel() AppInsiPanel

	GetEventsPanel() AppEventsPanel

	Launch()

	Shutdown()
}

/*
This is handed to the appliction candidate during initialization.
This is how the candidate informs the app runtime of its needs.
before launch
*/
type AppRuntimeSetup interface {
	WithLogLevel(level slog.Level)

	// On init app impl informs us where they want to be installed/ or are installed
	SetInstallPath(path string)

	// Indicate we need hjttp server so we can get a call to bind routes
	RequireHttpServer(
		binder AppHTTPBinder,
	)

	RequireSideCar(
		sideCar AppExternalBinder,
	)

	RequireInsi(
		insi AppInsiBinder,
	)

	ListenOn(topic string, handler events.EventHandler)
}

type AppEventsPanel interface {
	GetApi() api.API
	GetTopicPublisher(topic string) (events.TopicPublisher, error)
}

type runtimeEventsConcern struct {
	api    api.API
	events *events.EventManager

	apiHttpSubmitter api.APISubmitter
	apiInsiSubmitter api.APISubmitter
	activeTopics     map[string]string // topic handler id -> topic name
}

var _ AppEventsPanel = &runtimeEventsConcern{}

type runtimeImpl struct {
	ctx         context.Context
	installPath string
	cancel      context.CancelFunc

	logger *slog.Logger

	workPool *pool.Pool

	candidate ApplicationCandidate

	cHttp         runtimeHttpConcern
	cFS           runtimeFSConcern
	cSideCar      runtimeSideCarConcern
	sideCarBinder AppExternalBinder
	cInsi         runtimeInsiConcern
	cEvents       runtimeEventsConcern

	isPerformingApiTriggeredShutdown atomic.Bool
}

var _ AppRuntime = &runtimeImpl{}

type runtimeSetupImpl struct {
	installPath      string
	httpServerBinder AppHTTPBinder
	sideCarBinder    AppExternalBinder
	insiBinder       AppInsiBinder
	logLevel         slog.Level
	certPath         string
	keyPath          string
	requestedTopics  map[string]events.EventHandler
}

var _ AppRuntimeSetup = &runtimeSetupImpl{}

type dummyEventsPanel struct {
}

func (d *dummyEventsPanel) GetTopicPublisher(topic string) (events.TopicPublisher, error) {
	return nil, fmt.Errorf("events panel not initialized; were any topics requested?")
}

func (d *dummyEventsPanel) GetApi() api.API {
	return nil
}

func (r *runtimeImpl) GetEventsPanel() AppEventsPanel {
	if r.cEvents.events == nil {
		return &dummyEventsPanel{}
	}
	return &r.cEvents
}

func (r *runtimeSetupImpl) ListenOn(topic string, handler events.EventHandler) {
	if r.requestedTopics == nil {
		r.requestedTopics = make(map[string]events.EventHandler)
	}
	if _, exists := r.requestedTopics[topic]; exists {
		panic(fmt.Errorf("topic already requested: %s", topic))
	}
	r.requestedTopics[topic] = handler
}

func (r *runtimeImpl) GetLogger(component string) *slog.Logger {
	return r.logger.With("component", component)
}

func (r *runtimeImpl) GetWorkPool() *pool.Pool {
	return r.workPool
}

func (r *runtimeSetupImpl) WithLogLevel(level slog.Level) {
	r.logLevel = level
}

func (r *runtimeImpl) OnError(err error) {
	if err != nil {
		r.logger.Error("Application runtime error", "error", err)
	}
}

func (r *runtimeSetupImpl) SetInstallPath(path string) {
	r.installPath = path
}

func (r *runtimeSetupImpl) RequireHttpServer(binder AppHTTPBinder) {
	r.httpServerBinder = binder
}

func (r *runtimeSetupImpl) RequireSideCar(sideCar AppExternalBinder) {
	r.sideCarBinder = sideCar
}

func (r *runtimeSetupImpl) RequireInsi(insi AppInsiBinder) {
	r.insiBinder = insi
}

func New(candidate ApplicationCandidate) beau.Optional[AppRuntime] {

	if candidate == nil {
		return beau.None[AppRuntime]()
	}

	rts := &runtimeSetupImpl{}

	if err := candidate.Initialize(slog.Default(), rts); err != nil {
		return beau.None[AppRuntime]()
	}

	if rts.installPath == "" {
		rts.installPath = os.TempDir()
	}

	if rts.logLevel == 0 {
		rts.logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: rts.logLevel}))

	appRtCtx, appRtCancel := context.WithCancel(context.Background())

	eventConfig := events.EventManagerConfig{
		MaxTopics:           1024,
		MaxHandlersPerTopic: 64,
		WorkerPoolSize:      256,
		Logger:              logger.WithGroup("events"),
	}
	eventsManager := events.NewEventManager(appRtCtx, &eventConfig)

	if err := eventsManager.RegisterTopic(api.EventTopic); err != nil {
		panic(err)
	}
	apiPublisher, err := eventsManager.GetTopicPublisher(api.EventTopic)
	if err != nil {
		panic(err)
	}

	appApi := api.New(
		logger.With("runtime", "api"),
		apiPublisher,
	)

	runtimeActual := &runtimeImpl{
		ctx:         appRtCtx,
		cancel:      appRtCancel,
		logger:      logger.With("runtime", "app"),
		installPath: rts.installPath,
		candidate:   candidate,
		workPool:    pool.NewBuilder().WithLogger(logger).Build(appRtCtx),
		// Note: CFS postponed until Launch to ensure we can have multiple instances of pre-launch + launched
		// for cli driving
		cHttp: runtimeHttpConcern{
			httpServerBinder: rts.httpServerBinder,
			currentAuthToken: "",
			api:              appApi,
			installDir:       rts.installPath,
			secret:           candidate.GetAppSecret(),
		},
		cSideCar: runtimeSideCarConcern{
			hostedApps: make(map[string]*procman.HostedApp),
		},
		sideCarBinder: rts.sideCarBinder,
		cInsi: runtimeInsiConcern{
			insiApiKey:    "",
			insiEndpoints: nil,
			logger:        logger.WithGroup("insi"),
			skipVerify:    false,
		},
		cEvents: runtimeEventsConcern{
			events:       eventsManager,
			activeTopics: make(map[string]string),
			api:          appApi,
			apiHttpSubmitter: appApi.NewSubmiter(
				candidate.GetAppMeta().APIHeaderExpectation(),
				api.DataOriginHTTP,
			),
			apiInsiSubmitter: appApi.NewSubmiter(
				candidate.GetAppMeta().APIHeaderExpectation(),
				api.DataOriginInsi,
			),
		},
	}

	runtimeActual.cHttp.rt = runtimeActual

	// Always listen on the tx topic to forward tx events to the candidate
	rts.ListenOn(api.EventTopic, runtimeActual)

	if rts.requestedTopics != nil {
		for topic, handler := range rts.requestedTopics {
			if err := eventsManager.RegisterTopic(topic); err != nil && err != events.ErrTopicAlreadyRegistered {
				panic(err)
			}
			handlerID, err := eventsManager.AddHandler(topic, handler)
			if err != nil {
				panic(err)
			}
			runtimeActual.cEvents.activeTopics[handlerID] = topic
		}
	}

	if rts.insiBinder != nil {
		runtimeActual.cInsi.insiApiKey = rts.insiBinder.GetInsiApiKey()
		runtimeActual.cInsi.insiEndpoints = rts.insiBinder.GetInsiEndpoints()
		runtimeActual.cInsi.skipVerify = rts.insiBinder.GetInsiSkipVerify()
	}

	opt := beau.Some(AppRuntime(runtimeActual))

	return opt
}

func (r *runtimeImpl) Launch() {

	r.cFS = runtimeFSConcern{
		fs:          xfs.NewDataStore(r.logger, r.ctx, r.installPath),
		installPath: r.installPath,
	}

	if r.cHttp.httpServerBinder != nil {
		r.internalSetupHttpServer()
	}
	if r.sideCarBinder != nil {
		r.internalSetupSideCar()
	}
	if r.cInsi.insiApiKey != "" && r.cInsi.insiEndpoints != nil {
		r.internalSetupInsi()
	}
	r.candidate.Main(r.ctx, r)
}

func (r *runtimeImpl) Shutdown() {
	r.logger.Info("Shutting down application runtime")

	if r.cSideCar.host != nil {
		r.cSideCar.host.StopApp("*")
	}
	r.cHttp.cleanAuthToken()
	r.cancel()
}

func (r *runtimeEventsConcern) GetTopicPublisher(topic string) (events.TopicPublisher, error) {
	return r.events.GetTopicPublisher(topic)
}

func (r *runtimeEventsConcern) GetApi() api.API {
	return r.api
}

var _ events.EventHandler = &runtimeImpl{}

/*
runtimeImpl implements the events.EventHandler interface
to forward tx events to the candidate
*/
func (r *runtimeImpl) OnEvent(event events.Event) {
	if r.candidate != nil {
		r.candidate.OnApiEvent(event)
	}
}

type txHandlerObject struct {
	app *runtimeImpl
}

var _ events.EventHandler = &txHandlerObject{}

func (t *txHandlerObject) OnEvent(event events.Event) {
	t.app.OnEvent(event)
}
