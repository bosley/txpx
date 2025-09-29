package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/bosley/txpx/pkg/datascape"
	"github.com/bosley/txpx/pkg/datascape/models"
)

const (
	csrfHeaderName = "X-CSRF-Token"
)

type contextKey string

const (
	sessionContextKey contextKey = "session"
	userContextKey    contextKey = "user"
)

func GetSession(r *http.Request) (*models.UserSession, bool) {
	session, ok := r.Context().Value(sessionContextKey).(*models.UserSession)
	return session, ok
}

func GetUser(r *http.Request) (*models.User, bool) {
	user, ok := r.Context().Value(userContextKey).(*models.User)
	return user, ok
}

type MiddlewareBuilder interface {
	WithLogger(logger *slog.Logger) MiddlewareBuilder
	WithRateLimitPerOriginMiddleware(maxRequests int, per time.Duration) MiddlewareBuilder // for non-logged in users
	WithCSRFMiddleware() MiddlewareBuilder                                                 // get the csrf from the request header and validate it agaisnt the web session user (in an http only cookie)
	WithWebSessionMiddleware(sessionDuration time.Duration, publicPaths []string, loginPath string, redirectIfAuthenticated map[string]string) MiddlewareBuilder
	WithRateLimitPerUserMiddleware(maxRequests int, per time.Duration) MiddlewareBuilder // rate limit per user, per endpoint
	WithRateLimitPerRefererMiddleware(maxRequests int, per time.Duration, trustedProxies []string) MiddlewareBuilder
	Build() http.Handler
}

type middlewareBuilder struct {
	logger     *slog.Logger
	datascape  datascape.Controllers
	middleware []func(http.Handler) http.Handler
	handler    http.Handler
}

type rateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
}

func New(
	logger *slog.Logger,
	datascape datascape.Controllers,
	handler http.Handler,
) MiddlewareBuilder {
	return &middlewareBuilder{
		logger:     logger,
		datascape:  datascape,
		middleware: make([]func(http.Handler) http.Handler, 0),
		handler:    handler,
	}
}

func (m *middlewareBuilder) WithLogger(logger *slog.Logger) MiddlewareBuilder {
	m.logger = logger
	return m
}

func (m *middlewareBuilder) WithRateLimitPerOriginMiddleware(maxRequests int, per time.Duration) MiddlewareBuilder {
	m.middleware = append(m.middleware, m.rateLimitPerOriginMiddleware(maxRequests, per))
	return m
}

func (m *middlewareBuilder) WithCSRFMiddleware() MiddlewareBuilder {
	m.middleware = append(m.middleware, m.csrfMiddleware())
	return m
}

func (m *middlewareBuilder) WithWebSessionMiddleware(sessionDuration time.Duration, publicPaths []string, loginPath string, redirectIfAuthenticated map[string]string) MiddlewareBuilder {
	m.middleware = append(m.middleware, m.webSessionMiddleware(sessionDuration, publicPaths, loginPath, redirectIfAuthenticated))
	return m
}

func (m *middlewareBuilder) WithRateLimitPerUserMiddleware(maxRequests int, per time.Duration) MiddlewareBuilder {
	m.middleware = append(m.middleware, m.rateLimitPerUserMiddleware(maxRequests, per))
	return m
}

func (m *middlewareBuilder) WithRateLimitPerRefererMiddleware(maxRequests int, per time.Duration, trustedProxies []string) MiddlewareBuilder {
	m.middleware = append(m.middleware, m.rateLimitPerRefererMiddleware(maxRequests, per, trustedProxies))
	return m
}

func (m *middlewareBuilder) Build() http.Handler {
	handler := m.handler
	for i := len(m.middleware) - 1; i >= 0; i-- {
		handler = m.middleware[i](handler)
	}
	return handler
}

func (m *middlewareBuilder) rateLimitPerOriginMiddleware(maxRequests int, per time.Duration) func(http.Handler) http.Handler {
	limiter := &rateLimiter{
		requests: make(map[string][]time.Time),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if user has a valid session - authenticated users bypass origin limits
			if cookie, err := r.Cookie(models.SessionCookieName); err == nil && cookie.Value != "" {
				if session, err := m.datascape.GetWebsessionController().GetWebSession(cookie.Value); err == nil {
					if session.ExpiresAt.After(time.Now()) {
						// Valid session found, skip origin rate limiting
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			origin := r.RemoteAddr
			if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
				origin = forwardedFor
			}

			limiter.mu.Lock()
			now := time.Now()
			if _, exists := limiter.requests[origin]; !exists {
				limiter.requests[origin] = []time.Time{}
			}

			validRequests := []time.Time{}
			for _, t := range limiter.requests[origin] {
				if now.Sub(t) < per {
					validRequests = append(validRequests, t)
				}
			}

			if len(validRequests) >= maxRequests {
				limiter.mu.Unlock()
				m.logger.Warn("rate limit exceeded", "origin", origin, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			limiter.requests[origin] = append(validRequests, now)
			limiter.mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}

func (m *middlewareBuilder) csrfMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			session, ok := r.Context().Value(sessionContextKey).(*models.UserSession)
			if !ok {
				/*
					CSRF is not required for pages that do not contain a session of
					some sort.
					Our CSRFs are hard-tied to specific web sessions.
					If we have a public form to post and need to add protection,
					then an ephemral session for non-logged in users should be created
					to ensure we can help mitigate abuse.
				*/
				next.ServeHTTP(w, r)
				return
			}

			csrfToken := r.Header.Get(csrfHeaderName)
			if csrfToken == "" {
				if err := r.ParseForm(); err == nil {
					csrfToken = r.FormValue("csrf_token")
				}
			}

			if csrfToken == "" {
				m.logger.Error("CSRF validation failed: no CSRF token in header or form")
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			if err := session.ValidateCSRF(csrfToken); err != nil {
				m.logger.Error("CSRF validation failed", "error", err, "session_uuid", session.UUID)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			session.BumpCSRF()
			w.Header().Set(csrfHeaderName, session.NextCSRF)

			next.ServeHTTP(w, r)
		})
	}
}

func (m *middlewareBuilder) webSessionMiddleware(sessionDuration time.Duration, publicPaths []string, loginPath string, redirectIfAuthenticated map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(models.SessionCookieName)
			if err == nil && cookie.Value != "" {
				session, err := m.datascape.GetWebsessionController().GetWebSession(cookie.Value)
				if err == nil && session.ExpiresAt.After(time.Now()) {
					user, err := m.datascape.GetUserController().GetUserByUUID(session.UserUUID)
					if err == nil {
						ctx := context.WithValue(r.Context(), sessionContextKey, session)
						ctx = context.WithValue(ctx, userContextKey, user)

						if time.Until(session.ExpiresAt) < sessionDuration/2 {
							newSession, err := m.datascape.GetWebsessionController().NewWebSession(user.UUID, time.Now().Add(sessionDuration))
							if err == nil {
								newSession.SetCookie(w)
								ctx = context.WithValue(ctx, sessionContextKey, newSession)
							}
						}

						if redirectTo, shouldRedirect := redirectIfAuthenticated[r.URL.Path]; shouldRedirect {
							http.Redirect(w, r, redirectTo, http.StatusSeeOther)
							return
						}

						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			for _, publicPath := range publicPaths {
				if r.URL.Path == publicPath {
					next.ServeHTTP(w, r)
					return
				}
			}

			m.logger.Warn("no valid session", "path", r.URL.Path)
			http.Redirect(w, r, loginPath, http.StatusSeeOther)
		})
	}
}

func (m *middlewareBuilder) rateLimitPerUserMiddleware(maxRequests int, per time.Duration) func(http.Handler) http.Handler {
	limiter := &rateLimiter{
		requests: make(map[string][]time.Time),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(userContextKey).(*models.User)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			key := user.UUID + ":" + r.URL.Path

			limiter.mu.Lock()
			now := time.Now()
			if _, exists := limiter.requests[key]; !exists {
				limiter.requests[key] = []time.Time{}
			}

			validRequests := []time.Time{}
			for _, t := range limiter.requests[key] {
				if now.Sub(t) < per {
					validRequests = append(validRequests, t)
				}
			}

			if len(validRequests) >= maxRequests {
				limiter.mu.Unlock()
				m.logger.Warn("user rate limit exceeded", "user_uuid", user.UUID, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			limiter.requests[key] = append(validRequests, now)
			limiter.mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}

func (m *middlewareBuilder) rateLimitPerRefererMiddleware(maxRequests int, per time.Duration, trustedProxies []string) func(http.Handler) http.Handler {
	limiter := &rateLimiter{
		requests: make(map[string][]time.Time),
	}

	trustedProxyMap := make(map[string]bool)
	for _, proxy := range trustedProxies {
		trustedProxyMap[proxy] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			referer := r.Header.Get("Referer")
			if referer == "" {
				referer = "no-referer"
			}

			origin := r.RemoteAddr
			if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
				origin = forwardedFor
			}

			if len(trustedProxies) > 0 && !trustedProxyMap[origin] {
				m.logger.Warn("referer from untrusted proxy", "referer", referer, "origin", origin, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}

			limiter.mu.Lock()
			now := time.Now()
			if _, exists := limiter.requests[referer]; !exists {
				limiter.requests[referer] = []time.Time{}
			}

			validRequests := []time.Time{}
			for _, t := range limiter.requests[referer] {
				if now.Sub(t) < per {
					validRequests = append(validRequests, t)
				}
			}

			if len(validRequests) >= maxRequests {
				limiter.mu.Unlock()
				m.logger.Warn("referer rate limit exceeded", "referer", referer, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			limiter.requests[referer] = append(validRequests, now)
			limiter.mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}
