package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bosley/txpx/pkg/app/internal/api/v1"
	"github.com/bosley/txpx/pkg/beau"
	"github.com/bosley/txpx/pkg/datascape"
	"github.com/bosley/txpx/pkg/security"
	"github.com/google/uuid"
)

const (
	HttpAuthorizationHeader = "txpx-app-authorization"
	HttpMaxRestartAttempts  = 3
	HttpRestartDelay        = 2 * time.Second
	HttpTxPxApiMountPoint   = "/txpx/api/v1"
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
	BindPublicRoutes(mux *http.ServeMux, controllers datascape.Controllers)
}

type ApiClient interface {
	Ping() error
	Status() string
	Shutdown() (string, error)
	ListUsers() (string, error)
	CreateUser(email, password string) (string, error)
	DeleteUser(userUUID string) (string, error)
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

	GetApiClient(skipTLSVerify bool) ApiClient // http backend client
}

type runtimeHttpConcern struct {
	currentAuthToken string

	rt *runtimeImpl

	httpServerBinder AppHTTPBinder
	httpServer       *http.Server
	serverMu         sync.RWMutex

	certPath string
	keyPath  string

	installDir string

	secret string
	api    api.API
}

var _ AppHttpPanel = &runtimeHttpConcern{}

func (r *runtimeHttpConcern) cleanAuthToken() {
	os.Remove(filepath.Join(r.installDir, "auth.token"))
}

func (r *runtimeHttpConcern) GetAuthorizationToken() string {
	return r.currentAuthToken
}

func (r *runtimeHttpConcern) BumpAuthToken() string {
	newToken := uuid.New().String()
	r.currentAuthToken = newToken

	go func() {
		encryptedToken, err := security.Encrypt([]byte(newToken), r.secret)
		if err != nil {
			r.rt.logger.Error("Error encrypting auth token", "error", err)
			return
		}

		if err := os.WriteFile(filepath.Join(r.installDir, "auth.token"), []byte(encryptedToken), 0600); err != nil {
			r.rt.logger.Error("Error writing auth token file", "error", err)
		}
	}()

	return newToken
}

func (r *runtimeHttpConcern) GetApiMountPoint() string {
	return HttpTxPxApiMountPoint
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

		// Since we are using the http endpoint, we need to ensure we have a token to secure the endpoint
		// that can be cycled over time
		r.cHttp.BumpAuthToken()

		r.cHttp.serverMu.Lock()
		defer r.cHttp.serverMu.Unlock()

		if r.cHttp.httpServer != nil {
			r.cHttp.httpServer.Shutdown(r.ctx)
			r.cHttp.httpServer = nil
		}

		mux := http.NewServeMux()

		r.cHttp.certPath = r.cHttp.httpServerBinder.GetCertPath()
		r.cHttp.keyPath = r.cHttp.httpServerBinder.GetKeyPath()

		r.bindRuntimeApi(mux)

		r.cHttp.httpServerBinder.BindPublicRoutes(mux, r.dataControllers)

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
				if r.cHttp.httpServer != nil {
					addr := r.cHttp.httpServer.Addr
					server := r.cHttp.httpServer
					r.cHttp.serverMu.RUnlock()

					r.logger.Info("Shutting down http server", "address", addr)
					if err := server.Shutdown(r.ctx); err != nil {
						r.logger.Error("Error shutting down http server", "error", err)
					}
				} else {
					r.cHttp.serverMu.RUnlock()
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

	onRoute := func(x string) string {
		if strings.HasPrefix(x, "/") {
			x = x[1:]
		}
		return fmt.Sprintf("%s/%s", HttpTxPxApiMountPoint, x)
	}

	mux.HandleFunc(onRoute("/ping"), rt.httpApiHandleRuntimePing)

	/*
		These routes are protected by the UUID token that the app can
		cycle by using BumpAuthToken() ensuting that only the owner
		of the runtime interface can access these routes (from go)
	*/
	mux.Handle(onRoute("/status"), rt.httpTokenMiddleware(
		http.HandlerFunc(rt.httpApiHandleRuntimeStatus),
	))

	// 10 second countdown before shutdown - events once per second
	// to informt he app runtime that it should prepare for shutdown
	mux.Handle(onRoute("/shutdown"), rt.httpTokenMiddleware(
		http.HandlerFunc(rt.httpApiHandleRuntimeShutdown),
	))

	mux.Handle(onRoute("/users/list"), rt.httpTokenMiddleware(
		http.HandlerFunc(rt.httpApiHandleRuntimeUsersList),
	))

	mux.Handle(onRoute("/users/create"), rt.httpTokenMiddleware(
		http.HandlerFunc(rt.httpApiHandleRuntimeUsersCreate),
	))

	mux.Handle(onRoute("/users/delete"), rt.httpTokenMiddleware(
		http.HandlerFunc(rt.httpApiHandleRuntimeUsersDelete),
	))
}

/*
Api Handlers for application runtime
*/

func (rt *runtimeImpl) httpTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(HttpAuthorizationHeader)

		if rt.cHttp.currentAuthToken == "" {
			rt.logger.Error("Unauthorized request; no token established to secure authentication")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if token != rt.cHttp.currentAuthToken {
			rt.logger.Error("Unauthorized request", "token", token)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rt *runtimeImpl) httpApiHandleRuntimePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))

}

func (rt *runtimeImpl) httpApiHandleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":     "running",
		"uptime":     rt.candidate.GetAppMeta().GetUptime(),
		"identifier": rt.candidate.GetAppMeta().GetIdentifier(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(status); err != nil {
		rt.logger.Error("Failed to encode status response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (rt *runtimeImpl) httpApiHandleRuntimeShutdown(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	if rt.isPerformingApiTriggeredShutdown.Load() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "shutdown already in progress"}`))
		return
	}

	rt.isPerformingApiTriggeredShutdown.Store(true)

	now := time.Now()
	deadline := now.Add(10 * time.Second)

	// Send event indicating shutdown with countdown once a second for 10 seconds
	// then call rt.Shutdown()
	ctx, cancel := context.WithDeadline(rt.ctx, deadline)

	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				rt.Shutdown()
				return
			case <-time.After(1 * time.Second):
				timeRemaining := deadline.Sub(time.Now())
				if timeRemaining <= 0 {
					rt.Shutdown()
					return
				}
				if err := rt.cEvents.apiHttpSubmitter.Submit(api.Msg{
					UUID:   uuid.New().String(),
					Origin: api.DataOriginHTTP,
					Request: api.ApiRequest{
						Id:   api.ApiCommandIdRuntimeShutdown,
						Data: timeRemaining,
					},
				}); err != nil {
					rt.logger.Error("Failed to submit shutdown event", "error", err)
				}
			}
		}
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "shutdown initiated"}`))
}

/*

HTTP Client to talk to the api iteself

*/

func (r *runtimeHttpConcern) GetApiClient(skipTLSVerify bool) ApiClient {
	return newHttpApiClient(r.rt, r, skipTLSVerify)
}

type httpApiClientImpl struct {
	rt         *runtimeImpl
	r          *runtimeHttpConcern
	httpClient *http.Client
}

func newHttpApiClient(rt *runtimeImpl, r *runtimeHttpConcern, skipTLSVerify bool) *httpApiClientImpl {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
	}

	return &httpApiClientImpl{
		rt: rt,
		r:  r,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (h *httpApiClientImpl) doFetch(url string, method string, body io.Reader) (*http.Response, error) {
	authToken, err := os.ReadFile(filepath.Join(h.r.installDir, "auth.token"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("auth.token file not found; is the server running?")
		}
		return nil, err
	}

	decryptedToken, err := security.Decrypt(string(authToken), h.r.secret)
	if err != nil {
		return nil, err
	}

	attemptRequest := func(targetUrl string) (*http.Response, error) {
		req, err := http.NewRequest(method, targetUrl, body)
		if err != nil {
			h.rt.logger.Error("Failed to create request", "error", err)
			return nil, err
		}

		req.Header.Set(HttpAuthorizationHeader, string(decryptedToken))
		return h.httpClient.Do(req)
	}

	return attemptRequest(url)
}

func (h *httpApiClientImpl) makeUrl(path string) string {
	binding := h.rt.cHttp.httpServerBinder.GetBinding()
	keyPathSet := h.rt.cHttp.httpServerBinder.GetKeyPath() != ""
	certPathSet := h.rt.cHttp.httpServerBinder.GetCertPath() != ""

	protocol := "http"
	if keyPathSet && certPathSet {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s%s%s", protocol, binding, HttpTxPxApiMountPoint, path)
}

func (h *httpApiClientImpl) Ping() error {

	url := h.makeUrl("/ping")

	resp, err := h.doFetch(url, "GET", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status: %d", resp.StatusCode)
	}

	return nil
}

func (h *httpApiClientImpl) Status() string {
	url := h.makeUrl("/status")

	resp, err := h.doFetch(url, "GET", nil)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.rt.logger.Error("Failed to read status response body", "error", err)
		return ""
	}

	return string(body)
}

func (h *httpApiClientImpl) Shutdown() (string, error) {
	url := h.makeUrl("/shutdown")

	resp, err := h.doFetch(url, "POST", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shutdown failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.rt.logger.Error("Failed to read shutdown response body", "error", err)
		return "", err
	}

	return string(body), nil
}

func (h *httpApiClientImpl) ListUsers() (string, error) {
	url := h.makeUrl("/users/list")

	resp, err := h.doFetch(url, "GET", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list users failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.rt.logger.Error("Failed to read list users response body", "error", err)
		return "", err
	}

	return string(body), nil
}

func (h *httpApiClientImpl) CreateUser(email, password string) (string, error) {
	url := h.makeUrl("/users/create")

	payload := map[string]string{
		"email":    email,
		"password": password,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := h.doFetch(url, "POST", strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create user failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.rt.logger.Error("Failed to read create user response body", "error", err)
		return "", err
	}

	return string(body), nil
}

func (h *httpApiClientImpl) DeleteUser(userUUID string) (string, error) {
	url := h.makeUrl("/users/delete")

	payload := map[string]string{
		"user_uuid": userUUID,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := h.doFetch(url, "POST", strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("delete user failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.rt.logger.Error("Failed to read delete user response body", "error", err)
		return "", err
	}

	return string(body), nil
}

func (rt *runtimeImpl) httpApiHandleRuntimeUsersList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	users, err := rt.dataControllers.GetUserController().ListAllUsers()
	if err != nil {
		rt.logger.Error("Failed to list users", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(users); err != nil {
		rt.logger.Error("Failed to encode users list response", "error", err)
	}
}

func (rt *runtimeImpl) httpApiHandleRuntimeUsersCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rt.logger.Error("Failed to decode create user request", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Email == "" || req.Password == "" {
		rt.logger.Error("Missing required fields for user creation")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "email and password are required"})
		return
	}

	user, err := rt.dataControllers.GetUserController().CreateUser(req.Email, req.Password)
	if err != nil {
		rt.logger.Error("Failed to create user", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(user); err != nil {
		rt.logger.Error("Failed to encode create user response", "error", err)
	}
}

func (rt *runtimeImpl) httpApiHandleRuntimeUsersDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		UserUUID string `json:"user_uuid"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rt.logger.Error("Failed to decode delete user request", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.UserUUID == "" {
		rt.logger.Error("Missing user_uuid for user deletion")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "user_uuid is required"})
		return
	}

	if err := rt.dataControllers.GetUserController().DeleteUser(req.UserUUID); err != nil {
		rt.logger.Error("Failed to delete user", "error", err, "user_uuid", req.UserUUID)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "user deleted"})
}
