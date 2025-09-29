package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bosley/txpx/pkg/datascape"
	"github.com/bosley/txpx/pkg/datascape/models"
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/time/rate"
)

const (
	csrfHeaderName = "X-CSRF-Token"
)

type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

func NewCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization", csrfHeaderName},
		ExposeHeaders:    []string{csrfHeaderName},
		AllowCredentials: false,
		MaxAge:           86400,
	}
}

func (c *CORSConfig) WithOrigins(origins ...string) *CORSConfig {
	c.AllowOrigins = origins
	return c
}

func (c *CORSConfig) WithMethods(methods ...string) *CORSConfig {
	c.AllowMethods = methods
	return c
}

func (c *CORSConfig) WithHeaders(headers ...string) *CORSConfig {
	c.AllowHeaders = headers
	return c
}

func (c *CORSConfig) WithCredentials(allow bool) *CORSConfig {
	c.AllowCredentials = allow
	return c
}

func (c *CORSConfig) WithMaxAge(seconds int) *CORSConfig {
	c.MaxAge = seconds
	return c
}

func CORSAllowAll() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"*"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: false,
		MaxAge:           86400,
	}
}

func CORSSameOrigin() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization", csrfHeaderName, "Stripe-Signature"},
		ExposeHeaders:    []string{csrfHeaderName, "Request-Id", "Stripe-Request-Id"},
		AllowCredentials: true,
		MaxAge:           86400,
	}
}

func CORSForDevelopment(devOrigins ...string) CORSConfig {
	if len(devOrigins) == 0 {
		devOrigins = []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:8080"}
	}
	return CORSConfig{
		AllowOrigins:     devOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{csrfHeaderName},
		AllowCredentials: true,
		MaxAge:           3600,
	}
}

func CORSForStripe(allowedOrigins ...string) CORSConfig {
	// Stripe Connect might redirect from https://connect.stripe.com
	origins := append([]string{"https://checkout.stripe.com", "https://js.stripe.com"}, allowedOrigins...)
	return CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization", csrfHeaderName, "Stripe-Signature"},
		ExposeHeaders:    []string{csrfHeaderName, "Request-Id", "Stripe-Request-Id"},
		AllowCredentials: true,
		MaxAge:           86400,
	}
}

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

func (m *middlewareBuilder) corsMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Skip CORS for requests without Origin header (server-to-server like webhooks)
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if origin is allowed
			originAllowed := false
			if len(config.AllowOrigins) == 0 {
				// Empty AllowOrigins means same-origin only
				// Browser will handle same-origin, we just don't set CORS headers
				next.ServeHTTP(w, r)
				return
			}

			for _, allowedOrigin := range config.AllowOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					originAllowed = true
					break
				}
			}

			if !originAllowed {
				// Origin not allowed, don't set CORS headers
				next.ServeHTTP(w, r)
				return
			}

			// Set CORS headers
			if origin != "" && originAllowed {
				if len(config.AllowOrigins) > 0 && config.AllowOrigins[0] == "*" && !config.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if len(config.ExposeHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				if len(config.AllowMethods) > 0 {
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
				}
				if len(config.AllowHeaders) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
				}
				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", config.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type MiddlewareBuilder interface {
	WithLogger(logger *slog.Logger) MiddlewareBuilder
	WithRateLimitPerOriginMiddleware(maxRequests int, per time.Duration) MiddlewareBuilder // for non-logged in users
	WithCSRFMiddleware() MiddlewareBuilder                                                 // get the csrf from the request header and validate it agaisnt the web session user (in an http only cookie)
	WithWebSessionMiddleware(sessionDuration time.Duration, publicPaths []string, loginPath string, redirectIfAuthenticated map[string]string) MiddlewareBuilder
	WithRateLimitPerUserMiddleware(maxRequests int, per time.Duration) MiddlewareBuilder // rate limit per user, per endpoint
	WithRateLimitPerRefererMiddleware(maxRequests int, per time.Duration, trustedProxies []string) MiddlewareBuilder
	WithCORSMiddleware(corsConfig CORSConfig) MiddlewareBuilder
	Build() http.Handler
}

type middlewareBuilder struct {
	logger     *slog.Logger
	datascape  datascape.Controllers
	middleware []func(http.Handler) http.Handler
	handler    http.Handler
}

type rateLimiter struct {
	limiters sync.Map
	cache    *ttlcache.Cache[string, *rate.Limiter]
	rate     rate.Limit
	burst    int
}

func newRateLimiter(maxRequests int, per time.Duration) *rateLimiter {
	rl := &rateLimiter{
		rate:  rate.Limit(float64(maxRequests) / per.Seconds()),
		burst: maxRequests,
	}
	rl.cache = ttlcache.New[string, *rate.Limiter](
		ttlcache.WithTTL[string, *rate.Limiter](10*time.Minute),
		ttlcache.WithDisableTouchOnHit[string, *rate.Limiter](),
	)
	rl.cache.OnEviction(func(ctx context.Context, reason ttlcache.EvictionReason, item *ttlcache.Item[string, *rate.Limiter]) {
		if reason == ttlcache.EvictionReasonExpired {
			rl.limiters.Delete(item.Key())
		}
	})
	go rl.cache.Start()
	return rl
}

func (rl *rateLimiter) getLimiter(key string) *rate.Limiter {
	if limiter, ok := rl.limiters.Load(key); ok {
		rl.cache.Set(key, limiter.(*rate.Limiter), ttlcache.DefaultTTL)
		return limiter.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters.Store(key, limiter)
	rl.cache.Set(key, limiter, ttlcache.DefaultTTL)
	return limiter
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

func (m *middlewareBuilder) WithCORSMiddleware(corsConfig CORSConfig) MiddlewareBuilder {
	m.middleware = append(m.middleware, m.corsMiddleware(corsConfig))
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
	rl := newRateLimiter(maxRequests, per)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(models.SessionCookieName); err == nil && cookie.Value != "" {
				if session, err := m.datascape.GetWebsessionController().GetWebSession(cookie.Value); err == nil {
					if session.ExpiresAt.After(time.Now()) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			origin := r.RemoteAddr
			if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
				origin = forwardedFor
			}

			limiter := rl.getLimiter(origin)
			if !limiter.Allow() {
				m.logger.Warn("rate limit exceeded", "origin", origin, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

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
	rl := newRateLimiter(maxRequests, per)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(userContextKey).(*models.User)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			key := user.UUID + ":" + r.URL.Path
			limiter := rl.getLimiter(key)
			if !limiter.Allow() {
				m.logger.Warn("user rate limit exceeded", "user_uuid", user.UUID, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (m *middlewareBuilder) rateLimitPerRefererMiddleware(maxRequests int, per time.Duration, trustedProxies []string) func(http.Handler) http.Handler {
	rl := newRateLimiter(maxRequests, per)

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

			limiter := rl.getLimiter(referer)
			if !limiter.Allow() {
				m.logger.Warn("referer rate limit exceeded", "referer", referer, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
