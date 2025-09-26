package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bosley/txpx/beau"
	"github.com/bosley/txpx/utils/pool"
	"github.com/bosley/txpx/xfs"
	"github.com/google/uuid"
)

const (
	HttpAuthorizationHeader = "txpx-app-authorization"
	HttpMaxRestartAttempts  = 3
	HttpRestartDelay        = 2 * time.Second
)

type ApplicationCandidate interface {
	Initialize(
		logger *slog.Logger,
		rt AppRuntimeSetup,
	) error
	Run(ctx context.Context, rap AppRuntime)
	Shutdown()
}

/*
If an application needs HTTP/S they can indicate as such with the AppRuntimeSetup
and they provide this. These function calls MUST be idempotent.

If the server fails and needs to restart it will recreate the server, so
the state of the callee of AppHTTPBinder MUST take this into account.
*/
type AppHTTPBinder interface {
	GetBinding() string
	GetCertPath() string
	GetKeyPath() string
	BindPublicRoutes(mux *http.ServeMux)
	GetApiMountPoint() string // where they want the application api to be mounted
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

	Launch()
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
}

/*
The specific concerns for FS users
When FS is indicated the panel can be offered to inform the application
instance of the current install path.
*/
type AppFSPanel interface {
	GetInstallPath() string
	GetFS() xfs.FileStore
}

type runtimeFSConcern struct {
	fs          xfs.FileStore
	installPath string
}

var _ AppFSPanel = &runtimeFSConcern{}

func (r *runtimeFSConcern) GetInstallPath() string {
	return r.installPath
}

func (r *runtimeFSConcern) GetFS() xfs.FileStore {
	return r.fs
}

/*
The specific concerns for HTTP users
When HTTP/s is indicated the panel can be offered to inform the application
instance of the current token that the api endpoint will require to be used.
*/
type AppHttpPanel interface {
	GetAuthorizationToken() string
	BumpAuthToken() string
	GetApiMountPoint() string
}

type runtimeHttpConcern struct {
	currentAuthToken string
	httpServerBinder AppHTTPBinder
	apiMountPoint    string
	httpServer       *http.Server
	serverMu         sync.RWMutex

	certPath string
	keyPath  string
}

var _ AppHttpPanel = &runtimeHttpConcern{}

func (r *runtimeHttpConcern) GetAuthorizationToken() string {
	return r.currentAuthToken
}

func (r *runtimeHttpConcern) BumpAuthToken() string {
	newToken := uuid.New().String()
	r.currentAuthToken = newToken
	return newToken
}

func (r *runtimeHttpConcern) GetApiMountPoint() string {
	return r.apiMountPoint
}

func (r *runtimeHttpConcern) GetCertPath() string {
	return r.certPath
}

func (r *runtimeHttpConcern) GetKeyPath() string {
	return r.keyPath
}

func (r *runtimeHttpConcern) useTLS() (bool, error) {
	r.serverMu.RLock()
	defer r.serverMu.RUnlock()
	if r.httpServer == nil {
		return false, nil
	}
	if r.certPath == "" && r.keyPath != "" ||
		r.certPath != "" && r.keyPath == "" {
		return false, fmt.Errorf("certPath and keyPath must both be set or both be empty")
	}
	return r.certPath != "" && r.keyPath != "", nil
}

type runtimeImpl struct {
	ctx    context.Context
	cancel context.CancelFunc

	logger *slog.Logger

	workPool *pool.Pool

	candidate ApplicationCandidate

	cHttp runtimeHttpConcern
	cFS   runtimeFSConcern
}

var _ AppRuntime = &runtimeImpl{}

type runtimeSetupImpl struct {
	installPath      string
	httpServerBinder AppHTTPBinder
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

func (r *runtimeImpl) GetHttpPanel() AppHttpPanel {
	return &r.cHttp
}

func (r *runtimeImpl) GetFSPanel() AppFSPanel {
	return &r.cFS
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
			fs:          xfs.NewFileStore(rts.installPath),
			installPath: rts.installPath,
		},
		workPool: pool.NewBuilder().WithLogger(logger).Build(appRtCtx),
		cHttp: runtimeHttpConcern{
			httpServerBinder: rts.httpServerBinder,
			currentAuthToken: "",
		},
	}

	opt := beau.Some(AppRuntime(runtimeActual))

	return opt
}

func (r *runtimeImpl) Launch() {
	if r.cHttp.httpServerBinder != nil {
		r.internalSetupHttpServer()
	}
	r.candidate.Run(r.ctx, r)
}

func (r *runtimeImpl) Shutdown() {
	r.logger.Info("Shutting down application runtime")
	r.cancel()
	r.candidate.Shutdown()
}

func (r *runtimeImpl) internalSetupHttpServer() {

	buildServer := func() {
		r.cHttp.serverMu.Lock()
		defer r.cHttp.serverMu.Unlock()

		if r.cHttp.httpServer != nil {
			r.cHttp.httpServer.Shutdown(r.ctx)
			r.cHttp.httpServer = nil
		}

		mux := http.NewServeMux()

		r.cHttp.apiMountPoint = r.cHttp.httpServerBinder.GetApiMountPoint()

		r.cHttp.certPath = r.cHttp.httpServerBinder.GetCertPath()
		r.cHttp.keyPath = r.cHttp.httpServerBinder.GetKeyPath()

		r.cHttp.httpServerBinder.BindPublicRoutes(mux)

		r.cHttp.httpServer = &http.Server{
			Addr:    r.cHttp.httpServerBinder.GetBinding(),
			Handler: mux,
		}
	}

	startServer := func() error {
		servingTLS, err := r.cHttp.useTLS()
		if err != nil {
			r.logger.Error("Error building http server; invalid TLS configuration", "error", err)
			return nil
		}

		r.cHttp.serverMu.RLock()
		defer r.cHttp.serverMu.RUnlock()

		if r.cHttp.httpServer == nil {
			return fmt.Errorf("http server not built")
		}

		if servingTLS {
			return r.cHttp.httpServer.ListenAndServeTLS(r.cHttp.certPath, r.cHttp.keyPath)
		}
		return r.cHttp.httpServer.ListenAndServe()
	}

	/*
		Build the server, and then start it
	*/
	buildServer()

	go func() {

		r.cHttp.serverMu.RLock()
		addr := r.cHttp.httpServer.Addr
		r.cHttp.serverMu.RUnlock()

		// Here we know TLS was good once if it was ever good. If it changes thats bad and Mfn takes care of that
		r.logger.Info("Starting http server", "address", addr, "tls", beau.MFn[bool](r.cHttp.useTLS))

		restartCount := 0

		for {
			select {
			case <-r.ctx.Done():
				r.cHttp.serverMu.RLock()
				addr := r.cHttp.httpServer.Addr
				server := r.cHttp.httpServer
				r.cHttp.serverMu.RUnlock()

				r.logger.Info("Shutting down http server", "address", addr)
				if err := server.Shutdown(r.ctx); err != nil {
					r.logger.Error("Error shutting down http server", "error", err)
				}
				return
			default:
				err := startServer()
				if r.ctx.Err() != nil {
					r.cHttp.serverMu.RLock()
					addr := r.cHttp.httpServer.Addr
					r.cHttp.serverMu.RUnlock()
					r.logger.Info("Http server stopped due to context cancellation", "address", addr)
					return
				}

				r.cHttp.serverMu.RLock()
				addr := r.cHttp.httpServer.Addr
				r.cHttp.serverMu.RUnlock()
				r.logger.Error("Http server died unexpectedly", "error", err, "address", addr)

				restartCount++
				if restartCount >= HttpMaxRestartAttempts {
					r.cHttp.serverMu.RLock()
					addr := r.cHttp.httpServer.Addr
					r.cHttp.serverMu.RUnlock()
					r.logger.Error("Http server failed to restart after maximum attempts", "attempts", restartCount, "address", addr)
					r.OnError(fmt.Errorf("http server failed after %d restart attempts: %w", restartCount, err))
					return
				}

				r.cHttp.serverMu.RLock()
				addr = r.cHttp.httpServer.Addr
				r.cHttp.serverMu.RUnlock()
				r.logger.Info("Attempting to restart http server", "attempt", restartCount, "max_attempts", HttpMaxRestartAttempts, "delay", HttpRestartDelay)

				select {
				case <-time.After(HttpRestartDelay):
					// Continue to restart
				case <-r.ctx.Done():
					r.cHttp.serverMu.RLock()
					addr = r.cHttp.httpServer.Addr
					r.cHttp.serverMu.RUnlock()
					r.logger.Info("Aborting server restart due to context cancellation", "address", addr)
					return
				}

				/*
					Rebuild the server
				*/
				buildServer()
			}
		}
	}()
}
