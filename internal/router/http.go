package router

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bosley/txpx/internal/router/web"
	"github.com/bosley/txpx/internal/views"
	"github.com/bosley/txpx/pkg/datascape"
	"github.com/bosley/txpx/pkg/datascape/users"
	"github.com/bosley/txpx/pkg/middleware"
	"github.com/google/uuid"
)

type Router struct {
	logger       *slog.Logger
	viewManager  *views.ViewManager
	controllers  datascape.Controllers
	inProduction bool
	url          string
}

func New(
	logger *slog.Logger,
	inProduction bool,
	url string,
	viewManager *views.ViewManager,
	controllers datascape.Controllers,
) *Router {
	// Create a test user for development
	testUser, err := controllers.GetUserController().CreateUser("test@example.com", "password123")
	if err != nil {
		logger.Info("test user might already exist", "error", err)
	} else {
		logger.Info("created test user", "email", testUser.Email)
	}

	return &Router{
		logger:       logger,
		viewManager:  viewManager,
		controllers:  controllers,
		inProduction: inProduction,
		url:          url,
	}
}

func (x *Router) Bind(mux *http.ServeMux) {
	/*
		The router for the site utilizes the generalized middleware in pkg
		to handle all aspects of the site. The middleware controller I made
		can me configured for public/private routes with various redirect
		and rate limiting mechanics.

		Authenticated users are "more" trusted than the plebs so we increase
		their web session rate limits

		All "non public" routes that require posting data are protected by
		a CSRF token that is generated per-session and validated against
		the user's session.

		CORS Examples:
		- middleware.CORSSameOrigin() - Default, only same-origin requests
		- middleware.CORSAllowAll() - Allow all origins (NOT for production)
		- middleware.CORSForDevelopment() - Common dev server ports
		- middleware.CORSForDevelopment("http://localhost:4200") - Custom dev ports
		- middleware.CORSForStripe("https://myapp.com") - Stripe checkout + your origins
		- middleware.NewCORSConfig().WithOrigins("https://app.example.com").WithCredentials(true)
		- middleware.NewCORSConfig().WithOrigins("https://app1.example.com", "https://app2.example.com")
	*/
	publicPaths := []string{"/", "/login", "/styles.css"}
	redirectIfAuthenticated := map[string]string{
		"/login": "/dashboard",
	}

	internalMux := http.NewServeMux()
	internalMux.HandleFunc("/", x.handleRootGet())
	internalMux.HandleFunc("/styles.css", x.handleStylesCssGet())
	internalMux.HandleFunc("/login", x.handleLogin())
	internalMux.HandleFunc("/dashboard", x.handleDashboardGet())
	internalMux.HandleFunc("/logout", x.handleLogout())

	var corsConfig middleware.CORSConfig
	if x.inProduction {
		corsConfig = middleware.CORSForStripe(x.url)
	} else {
		corsConfig = middleware.CORSForDevelopment()
	}

	handler := middleware.New(
		x.logger.WithGroup("middleware"),
		x.controllers,
		internalMux,
	).WithCORSMiddleware(corsConfig).
		WithRateLimitPerOriginMiddleware(100, time.Minute).
		WithWebSessionMiddleware(24*time.Hour, publicPaths, "/login", redirectIfAuthenticated).
		WithCSRFMiddleware().
		WithRateLimitPerUserMiddleware(500, time.Minute).
		Build()

	// Register the single handler for all paths
	mux.Handle("/", handler)
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

func (x *Router) handleLogin() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			lpd := views.LoginPageData{
				Message: "Please log in to continue",
				Tracker: uuid.New().String(),
			}
			if err := x.viewManager.RenderLoginPage(lpd, w); err != nil {
				x.logger.Error("failed to render login view", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost {
			email := r.FormValue("email")
			password := r.FormValue("password")

			user, err := x.controllers.GetUserController().GetUserByEmail(email)
			if err != nil || !users.VerifyUserPassword(password, user.Password) {
				lpd := views.LoginPageData{
					Message: "Invalid email or password",
					Tracker: uuid.New().String(),
				}
				if err := x.viewManager.RenderLoginPage(lpd, w); err != nil {
					x.logger.Error("failed to render login view", "error", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			session, err := x.controllers.GetWebsessionController().NewWebSession(
				user.UUID,
				time.Now().Add(24*time.Hour),
			)
			if err != nil {
				x.logger.Error("failed to create session", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			session.BumpCSRF()
			session.SetCookie(w)

			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (x *Router) handleDashboardGet() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := middleware.GetSession(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, ok := middleware.GetUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		dpd := views.UserDashboardPageData{
			CSRFToken: session.NextCSRF,
			UserEmail: user.Email,
		}
		if err := x.viewManager.RenderDashboardPage(dpd, w); err != nil {
			x.logger.Error("failed to render dashboard view", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (x *Router) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		session, ok := middleware.GetSession(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session.DeleteCookie(w)

		err := x.controllers.GetWebsessionController().DeleteWebSession(session.UUID)
		if err != nil {
			x.logger.Error("failed to delete session", "error", err)
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
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
