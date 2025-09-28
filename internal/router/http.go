package router

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bosley/txpx/internal/controllers"
	"github.com/bosley/txpx/internal/router/web"
	"github.com/bosley/txpx/internal/views"
)

type Router struct {
	logger      *slog.Logger
	viewManager *views.ViewManager
	controllers controllers.Controllers
}

func New(
	logger *slog.Logger,
	viewManager *views.ViewManager,
	controllers controllers.Controllers,
) *Router {
	return &Router{
		logger:      logger,
		viewManager: viewManager,
		controllers: controllers,
	}
}

func (x *Router) Bind(mux *http.ServeMux) {

	mux.HandleFunc("/", x.handleRootGet())
	mux.HandleFunc("/login", x.handleLoginGet())
	mux.HandleFunc("/dashboard", x.handleDashboardGet())
	mux.HandleFunc("/styles.css", x.handleStylesCssGet())
}

func (x *Router) handleRootGet() http.HandlerFunc {

	counter := 0
	return func(w http.ResponseWriter, r *http.Request) {

		defer func() {
			counter++
		}()

		lpd := views.LandingPageData{
			Message: fmt.Sprintf("Hello, World! %d", counter),
		}
		if err := x.viewManager.RenderLandingPage(lpd, w); err != nil {
			x.logger.Error("failed to render view", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (x *Router) handleLoginGet() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		lpd := views.LoginPageData{
			Message: "Please log in to continue",
			Tracker: "login-001",
		}
		if err := x.viewManager.RenderLoginPage(lpd, w); err != nil {
			x.logger.Error("failed to render login view", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (x *Router) handleDashboardGet() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		dpd := views.UserDashboardPageData{
			CSRFToken: "csrf-token-123",
		}
		if err := x.viewManager.RenderDashboardPage(dpd, w); err != nil {
			x.logger.Error("failed to render dashboard view", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (x *Router) handleStylesCssGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, err := web.GetWebAsset("styles.css")
		if err != nil {
			x.logger.Error("failed to get styles.css", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(http.StatusOK)
		w.Write(asset)
	}
}
