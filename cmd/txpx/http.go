package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/events"
)

type AppTxPxHttpServer struct {
	logger *slog.Logger

	binding      string
	certPath     string
	keyPath      string
	config       *Config
	eventHandler *AppTxPxHttpServerEventHandler

	app *AppTxPx
}

func NewAppTxPxHttpServer(
	logger *slog.Logger,
	config *Config,
) *AppTxPxHttpServer {

	binding := fmt.Sprintf(":%d", config.Port)

	return &AppTxPxHttpServer{
		logger:   logger,
		binding:  binding,
		certPath: config.CertPath,
		keyPath:  config.KeyPath,
		config:   config,
		eventHandler: &AppTxPxHttpServerEventHandler{
			app: nil,
		},
		app: nil,
	}
}

var _ app.AppHTTPBinder = &AppTxPxHttpServer{}

func (a *AppTxPxHttpServer) Initialize(rt app.AppRuntimeSetup) error {

	// Self-register as an http server binder so the main app doesn't have to
	rt.RequireHttpServer(a)

	// listen on the http-server topic so we can receive events from other
	// app modules that might relate to us at runtime
	rt.ListenOn(EventTopicTxPxHttpServer, a.eventHandler)

	return nil
}

func (a *AppTxPxHttpServer) setApp(app *AppTxPx) {
	a.eventHandler.app = a
	a.app = app
}

func (a *AppTxPxHttpServer) GetBinding() string {
	return a.binding
}

func (a *AppTxPxHttpServer) GetCertPath() string {
	return a.certPath
}

func (a *AppTxPxHttpServer) GetKeyPath() string {
	return a.keyPath
}

func (a *AppTxPxHttpServer) GetApiMountPoint() string {
	return "/api"
}

func (a *AppTxPxHttpServer) BindPublicRoutes(mux *http.ServeMux) {
	a.bindHandlers(mux)
}

/*
HTTP Event Handler
*/

type AppTxPxHttpServerEventHandler struct {
	app *AppTxPxHttpServer
}

var _ events.EventHandler = &AppTxPxHttpServerEventHandler{}

func (a *AppTxPxHttpServerEventHandler) OnEvent(event events.Event) {
	a.app.logger.Info("received event", "event", event)
}

/*
HTTP Handlers
*/

func (a *AppTxPxHttpServer) bindHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/", a.handleRootGet())
}

func (a *AppTxPxHttpServer) handleRootGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}
}
