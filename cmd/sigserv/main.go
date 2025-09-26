package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/security"
	"github.com/jellydator/ttlcache/v3"
)

const (
	SERVER_SECRET_ENV_STR         = "SERVER_SECRET"
	SRE_ENV_STR                   = "SECRET_ROTATION_ENABLED"
	SERVER_SECRET_UPDATE_INTERVAL = 1 * time.Minute
	SECRET_ROTATION_ENABLED       = "true"

	EPHEMERAL_CACHE_TTL      = 5 * time.Minute  // How long to keep data in cache after it is posted
	EPHEMERAL_CACHE_CAPACITY = 1024             // How many items to keep in cache
	EPHEMERAL_CACHE_MAX_SIZE = 1024 * 1024 * 10 // 10MB (of data, not items)
)

type Payload struct {
	StoredAt     time.Time     `json:"stored_at"`
	RemainingTTL time.Duration `json:"remaining_ttl"`
	Data         []byte        `json:"data"`
	Size         uint64        `json:"size"`
}

type SigServApp struct {
	appCtx    context.Context
	appCancel context.CancelFunc

	binding    string
	keysDir    string
	installDir string

	logger *slog.Logger
	rt     app.AppRuntime

	// Server state
	serverSecret               string
	secretRotationEnabled      bool
	serverSecretUpdateInterval time.Duration
	cacheDuration              time.Duration
	serverDataCache            *ttlcache.Cache[string, Payload]
	currentDataInMemory        atomic.Int64

	// Client flags
	clientKey           string
	clientPost          string
	clientGet           bool
	clientSkipPrintData bool
	clientPrintMetadata bool

	// App state
	logLevel string
}

var _ app.ApplicationCandidate = &SigServApp{}
var _ app.AppHTTPBinder = &SigServApp{}

// HTTP binder implementation
func (s *SigServApp) GetBinding() string {
	return s.binding
}

func (s *SigServApp) GetCertPath() string {
	return filepath.Join(s.keysDir, "cert.pem")
}

func (s *SigServApp) GetKeyPath() string {
	return filepath.Join(s.keysDir, "key.pem")
}

func (s *SigServApp) BindPublicRoutes(mux *http.ServeMux) {
	mux.Handle("/", s.authMiddleware(http.HandlerFunc(s.handleRequest)))
}

func (s *SigServApp) GetApiMountPoint() string {
	return "/api"
}

func (s *SigServApp) Initialize(logger *slog.Logger, rt app.AppRuntimeSetup) error {
	logger.Info("Initializing SigServ application")

	// Parse command line flags
	flag.StringVar(&s.binding, "at", "", "Globally reachable host:port")
	flag.StringVar(&s.logLevel, "log", "info", "Log level")
	flag.DurationVar(&s.serverSecretUpdateInterval, "ssi", SERVER_SECRET_UPDATE_INTERVAL, "Server-secret update interval")
	flag.BoolVar(&s.secretRotationEnabled, "sre", os.Getenv(SRE_ENV_STR) == SECRET_ROTATION_ENABLED, "Secret rotation enabled")
	flag.DurationVar(&s.cacheDuration, "cd", EPHEMERAL_CACHE_TTL, "Cache duration")
	flag.StringVar(&s.clientKey, "key", "", "Key for client operations")
	flag.StringVar(&s.clientPost, "post", "", "File path to post data from")
	flag.BoolVar(&s.clientGet, "get", false, "Get data for the specified key")
	flag.BoolVar(&s.clientSkipPrintData, "no-data", s.clientSkipPrintData, "Do not print data")
	flag.BoolVar(&s.clientPrintMetadata, "meta", s.clientPrintMetadata, "Print metadata")
	flag.Parse()

	// Set log level
	switch s.logLevel {
	case "debug":
		rt.WithLogLevel(slog.LevelDebug)
	case "info":
		rt.WithLogLevel(slog.LevelInfo)
	case "warn":
		rt.WithLogLevel(slog.LevelWarn)
	case "error":
		rt.WithLogLevel(slog.LevelError)
	default:
		rt.WithLogLevel(slog.LevelInfo)
	}

	// Require HTTP server only if not in client mode
	if s.clientKey == "" {
		// Set install path
		s.installDir = os.TempDir()
		rt.SetInstallPath(s.installDir)

		// Create keys directory and generate certs
		s.keysDir = filepath.Join(s.installDir, "keys")
		if err := os.MkdirAll(s.keysDir, 0755); err != nil {
			return fmt.Errorf("failed to create keys directory: %w", err)
		}
		if err := security.GenerateSelfSignedCert(s.keysDir); err != nil {
			return fmt.Errorf("failed to generate self-signed cert: %w", err)
		}

		rt.RequireHttpServer(s)
	} else {
		// In client mode, set a unique install path to avoid conflicts
		s.installDir = filepath.Join(os.TempDir(), "sigserv-client")
		rt.SetInstallPath(s.installDir)
	}

	return nil
}

func (s *SigServApp) Main(ctx context.Context, rap app.AppRuntime) {
	s.rt = rap
	s.logger = rap.GetLogger("sigserv")

	s.appCtx, s.appCancel = context.WithCancel(ctx)

	// Get initial server secret
	s.getServerSecret(false)

	// Check if this is client mode
	if s.clientKey != "" {
		s.handleClientOperations()
		return
	}

	// Server mode
	if s.binding == "" {
		s.logger.Error("Server 'at' binding is required")
		os.Exit(1)
	}

	s.setupCache()
	s.startSecretRotation()

	s.logger.Info("Starting cords server", "serve_at", s.binding, "cache_duration", s.cacheDuration, "secret_rotation_enabled", s.secretRotationEnabled)

	// Wait for shutdown
	<-s.appCtx.Done()
	s.logger.Info("SigServ application shutting down")
}

func (s *SigServApp) getServerSecret(isUpdate bool) {
	newSecret := os.Getenv(SERVER_SECRET_ENV_STR)
	if isUpdate && newSecret == "" && s.secretRotationEnabled {
		s.logger.Warn(SERVER_SECRET_ENV_STR + " is not set - Ignoring update")
		return
	}
	s.serverSecret = newSecret
}

func (s *SigServApp) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != s.serverSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *SigServApp) handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		s.handleGet(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *SigServApp) handlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Missing key parameter", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, EPHEMERAL_CACHE_MAX_SIZE))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	if uint64(len(body)) > EPHEMERAL_CACHE_MAX_SIZE {
		http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	currentMemory := s.currentDataInMemory.Load()
	if currentMemory+int64(len(body)) > EPHEMERAL_CACHE_MAX_SIZE {
		http.Error(w, "Cache capacity exceeded", http.StatusInsufficientStorage)
		return
	}

	payload := Payload{
		StoredAt:     time.Now(),
		RemainingTTL: s.cacheDuration,
		Data:         body,
		Size:         uint64(len(body)),
	}

	s.serverDataCache.Set(key, payload, s.cacheDuration)
	s.currentDataInMemory.Add(int64(len(body)))

	s.logger.Info("Data stored", "key", key, "size", len(body))
	w.WriteHeader(http.StatusCreated)
}

func (s *SigServApp) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Missing key parameter", http.StatusBadRequest)
		return
	}

	item := s.serverDataCache.Get(key)
	if item == nil {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	payload := item.Value()
	payload.RemainingTTL = time.Until(item.ExpiresAt())

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Stored-At", payload.StoredAt.Format(time.RFC3339))
	w.Header().Set("X-Remaining-TTL", payload.RemainingTTL.String())
	w.Header().Set("X-Size", fmt.Sprintf("%d", payload.Size))

	s.logger.Info("Data retrieved", "key", key, "size", payload.Size, "remaining_ttl", payload.RemainingTTL)
	w.Write(payload.Data)
}

func (s *SigServApp) setupCache() {
	s.currentDataInMemory.Store(int64(0))
	s.serverDataCache = ttlcache.New[string, Payload](
		ttlcache.WithTTL[string, Payload](s.cacheDuration),
		ttlcache.WithCapacity[string, Payload](EPHEMERAL_CACHE_CAPACITY),
		ttlcache.WithDisableTouchOnHit[string, Payload](),
	)

	s.serverDataCache.OnEviction(func(ctx context.Context, reason ttlcache.EvictionReason, item *ttlcache.Item[string, Payload]) {
		if item != nil {
			// Subtract the size of the data from the current data in memory
			s.currentDataInMemory.Add(
				-1 * int64(item.Value().Size),
			)
			check := s.currentDataInMemory.Load()
			if check < 0 {
				s.logger.Error("currentDataInMemory is less than 0", "check", check)
				// It should never get less than 0, but if it does, we need to try to replace it immediately
				// We do a swap because there is technically a possiblity that the value is being accessed by another thread during our
				// conditional above. VERY UNLIKELY, but we need to be safe and avoid locking here.
				if !s.currentDataInMemory.CompareAndSwap(check, 0) {
					s.logger.Error("currentDataInMemory is less than 0 and failed to swap - forcing (may cause corruption)", "check", check)
					s.currentDataInMemory.Store(0)
				}
			}
		}
	})

	go s.serverDataCache.Start()
}

func (s *SigServApp) startSecretRotation() {
	if !s.secretRotationEnabled {
		s.logger.Info("Secret rotation disabled")
		return
	}

	go func() {
		ticker := time.NewTicker(s.serverSecretUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.getServerSecret(true)
			case <-s.appCtx.Done():
				s.logger.Info("Secret rotation stopped")
				return
			}
		}
	}()
	s.logger.Info("Secret rotation started", "interval", s.serverSecretUpdateInterval)
}

func (s *SigServApp) handleClientOperations() {
	if s.clientPost != "" && s.clientGet {
		fmt.Println("Error: Cannot specify both --post and --get")
		os.Exit(1)
	}
	if s.clientPost == "" && !s.clientGet {
		fmt.Println("Error: When --key is specified, either --post or --get must be provided")
		os.Exit(1)
	}

	if s.binding == "" {
		fmt.Println("Error: --at (server URL) is required for client operations")
		os.Exit(1)
	}

	serverURL := s.binding
	if !strings.HasPrefix(serverURL, "https://") {
		serverURL = "https://" + serverURL
	}

	if s.clientPost != "" {
		if err := s.clientPostData(serverURL, s.clientKey, s.clientPost); err != nil {
			s.logger.Error("Failed to post data", "error", err)
			os.Exit(1)
		}
	} else if s.clientGet {
		if err := s.clientGetData(serverURL, s.clientKey); err != nil {
			s.logger.Error("Failed to get data", "error", err)
			os.Exit(1)
		}
	}
}

func (s *SigServApp) makeHTTPSClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func (s *SigServApp) clientPostData(serverURL, key, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	u, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	q := u.Query()
	q.Set("key", key)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", s.serverSecret)
	req.Header.Set("Content-Type", "application/octet-stream")

	client := s.makeHTTPSClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server responded with status %d: %s", resp.StatusCode, string(body))
	}

	s.logger.Info("Data posted successfully", "key", key, "file", filePath, "size", len(data))
	return nil
}

func (s *SigServApp) clientGetData(serverURL, key string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	q := u.Query()
	q.Set("key", key)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", s.serverSecret)

	client := s.makeHTTPSClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server responded with status %d: %s", resp.StatusCode, string(body))
	}

	storedAt := resp.Header.Get("X-Stored-At")
	remainingTTL := resp.Header.Get("X-Remaining-TTL")
	size := resp.Header.Get("X-Size")

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if s.clientPrintMetadata {
		fmt.Printf("Key: %s\n", key)
		fmt.Printf("Stored At: %s\n", storedAt)
		fmt.Printf("Remaining TTL: %s\n", remainingTTL)
		fmt.Printf("Size: %s bytes\n", size)
		fmt.Printf("Received: %d bytes\n", len(data))
	}
	if !s.clientSkipPrintData {
		os.Stdout.Write(data)
	}

	return nil
}

func main() {
	opt := app.New(&SigServApp{})

	app := opt.UnwrapOrElse(func() app.AppRuntime {
		fmt.Println("Failed to create SigServ app")
		os.Exit(1)
		return nil
	})

	app.Launch()
}
