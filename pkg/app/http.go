package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bosley/txpx/pkg/beau"
	"github.com/google/uuid"
)

const (
	HttpAuthorizationHeader = "txpx-app-authorization"
	HttpMaxRestartAttempts  = 3
	HttpRestartDelay        = 2 * time.Second
)

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

func (r *runtimeImpl) GetHttpPanel() AppHttpPanel {
	return &r.cHttp
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

		r.bindRuntimeApi(mux)

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

func (rt *runtimeImpl) bindRuntimeApi(mux *http.ServeMux) {

	mountPoint := rt.cHttp.httpServerBinder.GetApiMountPoint()

	if mountPoint == "" {
		mountPoint = "/_/api/v1"
		rt.cHttp.apiMountPoint = mountPoint
	}

	if strings.HasSuffix(mountPoint, "/") {
		mountPoint = mountPoint[:len(mountPoint)-1]
	}

	onRoute := func(x string) string {
		if strings.HasPrefix(x, "/") {
			x = x[1:]
		}
		return fmt.Sprintf("%s/%s", mountPoint, x)
	}

	mux.HandleFunc(onRoute("runtime/ping"), rt.handleRuntimePing)
}

func (rt *runtimeImpl) handleRuntimePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
