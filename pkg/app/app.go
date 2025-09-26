package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/bosley/txpx/pkg/beau"
	"github.com/bosley/txpx/pkg/pool"
	"github.com/bosley/txpx/pkg/procman"
	"github.com/bosley/txpx/pkg/xfs"
)

type ApplicationCandidate interface {
	Initialize(
		logger *slog.Logger,
		rt AppRuntimeSetup,
	) error
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
}

type runtimeImpl struct {
	ctx    context.Context
	cancel context.CancelFunc

	logger *slog.Logger

	workPool *pool.Pool

	candidate ApplicationCandidate

	cHttp         runtimeHttpConcern
	cFS           runtimeFSConcern
	cSideCar      runtimeSideCarConcern
	sideCarBinder AppExternalBinder
	cInsi         runtimeInsiConcern
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
}

var _ AppRuntimeSetup = &runtimeSetupImpl{}

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

	runtimeActual := &runtimeImpl{
		ctx:       appRtCtx,
		cancel:    appRtCancel,
		logger:    logger.With("runtime", "app"),
		candidate: candidate,
		cFS: runtimeFSConcern{
			fs:          xfs.NewDataStore(logger, appRtCtx, rts.installPath),
			installPath: rts.installPath,
		},
		workPool: pool.NewBuilder().WithLogger(logger).Build(appRtCtx),
		cHttp: runtimeHttpConcern{
			httpServerBinder: rts.httpServerBinder,
			currentAuthToken: "",
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
	r.cancel()
}
