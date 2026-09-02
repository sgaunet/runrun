package server

import (
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/csrf"
	"github.com/sgaunet/runrun/internal/executor"
	customMiddleware "github.com/sgaunet/runrun/internal/middleware"
	"github.com/sgaunet/runrun/internal/ratelimit"
	"github.com/sgaunet/runrun/internal/security"
	"github.com/sgaunet/runrun/internal/websocket"
)

// maxLoginAttempts is the number of failed login attempts allowed within
// loginRateLimitWindow before the rate limiter blocks further attempts.
const maxLoginAttempts = 5

// loginRateLimitWindow is the sliding window over which maxLoginAttempts is enforced.
const loginRateLimitWindow = 15 * time.Minute

// sessionCleanupInterval is how often expired sessions are purged.
const sessionCleanupInterval = 5 * time.Minute

// assetVersion returns a short build identifier used to cache-bust the
// embedded static assets in <link> and <script> URLs. It is derived from
// the VCS revision recorded in the binary by the Go build pipeline. When
// no VCS info is available (e.g. `go run`), it falls back to "dev".
func assetVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return "dev"
}

// Server represents the HTTP server.
type Server struct {
	router        *chi.Mux
	authService   *auth.Service
	executor      *executor.TaskExecutor
	config        *config.Config
	wsHub         *websocket.Hub
	wsHandler     *websocket.Handler
	wsBroadcaster *websocket.Broadcaster
	startTime     time.Time
	rateLimiter   *ratelimit.Limiter
	csrf          *csrf.Protection
	auditLogger   *security.Logger
	assetVersion  string
}

// New creates a new server instance.
func New(cfg *config.Config) *Server {
	s := &Server{
		config:       cfg,
		startTime:    time.Now(),
		assetVersion: assetVersion(),
	}

	// Initialize authentication service
	s.authService = auth.NewService(cfg.Auth.JWTSecret, cfg.Server.SessionTimeout)

	// Add users from config
	for _, user := range cfg.Auth.Users {
		s.authService.AddUser(user.Username, user.Password)
	}

	// Initialize task executor
	s.executor = executor.NewTaskExecutor(
		cfg.Server.MaxConcurrentTasks,
		cfg.Server.LogDirectory,
		cfg.Server.ShutdownTimeout,
	)

	// Initialize WebSocket hub
	wsConfig := websocket.DefaultConfig()
	s.wsHub = websocket.NewHub(wsConfig)
	s.wsHandler = websocket.NewHandler(s.wsHub, wsConfig)
	s.wsBroadcaster = websocket.NewBroadcaster(s.wsHub)

	// Wire broadcaster to executor for real-time log streaming
	s.executor.SetBroadcaster(s.wsBroadcaster)

	// Initialize rate limiter
	s.rateLimiter = ratelimit.NewLimiter(maxLoginAttempts, loginRateLimitWindow)

	// Initialize CSRF protection
	s.csrf = csrf.New()

	// Initialize security audit logger
	s.auditLogger = security.NewLogger()

	// Start WebSocket hub
	go s.wsHub.Run()

	// Set up router
	s.setupRouter()

	// Start session cleanup goroutine
	go s.sessionCleanupWorker()

	return s
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown() error {
	log.Println("Shutting down server...")

	// Shutdown WebSocket hub
	s.wsHub.Shutdown()

	// Shutdown executor
	if err := s.executor.Shutdown(); err != nil {
		log.Printf("Executor shutdown error: %v", err)
		return fmt.Errorf("shutdown executor: %w", err)
	}
	return nil
}

// Router returns the configured Chi router.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// AuthService returns the authentication service.
func (s *Server) AuthService() *auth.Service {
	return s.authService
}

// GetWebSocketBroadcaster returns the WebSocket broadcaster.
func (s *Server) GetWebSocketBroadcaster() *websocket.Broadcaster {
	return s.wsBroadcaster
}

// setupRouter configures the Chi router with middleware and routes.
func (s *Server) setupRouter() {
	r := chi.NewRouter()

	// Middleware stack (order matters!)
	// Apply these middleware to ALL routes
	r.Use(customMiddleware.RequestIDMiddleware)       // Custom request ID with UUID
	r.Use(customMiddleware.RecoveryMiddleware)        // Custom panic recovery
	r.Use(customMiddleware.CSPNonceMiddleware)        // Per-request CSP nonce (must precede SecurityHeaders)
	r.Use(customMiddleware.SecurityHeadersMiddleware) // Security headers (reads nonce from context)
	r.Use(customMiddleware.LoggingMiddleware)         // Custom logging
	// NOTE: chi's middleware.RealIP is deliberately NOT used. It rewrites
	// r.RemoteAddr from unauthenticated X-Forwarded-For / X-Real-IP /
	// True-Client-IP headers, so any client can forge the address the access
	// log records (GHSA-3fxj-6jh8-hvhx). r.RemoteAddr therefore stays the real
	// TCP peer; components that need the proxied client IP resolve it
	// themselves (see internal/ratelimit and internal/security).
	// NOTE: Compression and Timeout middleware are applied selectively in SetupRoutes
	// because they wrap the response writer and break WebSocket upgrades (http.Hijacker)

	s.router = r
}

// sessionCleanupWorker periodically cleans up expired sessions.
func (s *Server) sessionCleanupWorker() {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.authService.CleanupExpiredSessions()
		log.Println("Cleaned up expired sessions")
	}
}
