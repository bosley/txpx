package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bosley/txpx/cmd/txpx/config"
	"github.com/bosley/txpx/internal/router"
	"github.com/bosley/txpx/internal/views"
	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/datascape"
	"github.com/bosley/txpx/pkg/events"
)

type AppTxPxHttpServer struct {
	logger *slog.Logger

	binding      string
	certPath     string
	keyPath      string
	config       *config.Config
	eventHandler *AppTxPxHttpServerEventHandler

	app *AppTxPx

	serverRouter *router.Router
	viewManager  *views.ViewManager
}

func NewAppTxPxHttpServer(
	logger *slog.Logger,
	config *config.Config,
	viewManager *views.ViewManager,
) *AppTxPxHttpServer {

	binding := fmt.Sprintf(":%d", config.Port)
	if !config.Prod {
		binding = "localhost" + binding
	}

	return &AppTxPxHttpServer{
		logger:   logger,
		binding:  binding,
		certPath: config.CertPath,
		keyPath:  config.KeyPath,
		config:   config,
		eventHandler: &AppTxPxHttpServerEventHandler{
			app: nil,
		},
		app:         nil,
		viewManager: viewManager,
	}
}

var _ app.AppHTTPBinder = &AppTxPxHttpServer{}

func (a *AppTxPxHttpServer) Initialize(rt app.AppRuntimeSetup) error {

	// Self-register as an http server binder so the main app doesn't have to
	rt.RequireHttpServer(a)

	// listen on the http-server topic so we can receive events from other
	// app modules that might relate to us at runtime
	rt.ListenOn(config.EventTopicTxPxHttpServer, a.eventHandler)

	return nil
}

func (a *AppTxPxHttpServer) setApp(app *AppTxPx) {
	a.eventHandler.app = a
	a.app = app
}

func (a *AppTxPxHttpServer) GetBinding() string {
	return a.binding
}

func (a *AppTxPxHttpServer) GetTLSConfig() app.TLSConfig {
	return app.TLSConfig{
		Enabled:  a.config.HTTPS,
		CertPath: a.certPath,
		KeyPath:  a.keyPath,
	}
}

func (a *AppTxPxHttpServer) BindPublicRoutes(mux *http.ServeMux, controllers datascape.Controllers) {
	a.serverRouter = router.New(
		a.logger,
		a.config.Prod,
		a.config.URL,
		a.viewManager,
		controllers,
	)
	a.serverRouter.Bind(mux)
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
