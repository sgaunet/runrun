package server

import (
	"log"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/csrf"
	"github.com/sgaunet/runrun/internal/executor"
	customMiddleware "github.com/sgaunet/runrun/internal/middleware"
	"github.com/sgaunet/runrun/internal/ratelimit"
	"github.com/sgaunet/runrun/internal/security"
	"github.com/sgaunet/runrun/internal/websocket"
)

// Server represents the HTTP server
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
}

// New creates a new server instance
func New(cfg *config.Config) *Server {
	s := &Server{
		config:    cfg,
		startTime: time.Now(),
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
	s.wsHub = websocket.NewHub()
	s.wsHandler = websocket.NewHandler(s.wsHub, websocket.DefaultConfig())
	s.wsBroadcaster = websocket.NewBroadcaster(s.wsHub)

	// Initialize rate limiter (5 login attempts per 15 minutes)
	s.rateLimiter = ratelimit.NewLimiter(5, 15*time.Minute)

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

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	log.Println("Shutting down server...")

	// Shutdown WebSocket hub
	s.wsHub.Shutdown()

	// Shutdown executor
	if err := s.executor.Shutdown(); err != nil {
		log.Printf("Executor shutdown error: %v", err)
		return err
	}
	return nil
}

// setupRouter configures the Chi router with middleware and routes
func (s *Server) setupRouter() {
	r := chi.NewRouter()

	// Middleware stack (order matters!)
	// Apply these middleware to ALL routes
	r.Use(customMiddleware.RequestIDMiddleware)       // Custom request ID with UUID
	r.Use(customMiddleware.RecoveryMiddleware)        // Custom panic recovery
	r.Use(customMiddleware.SecurityHeadersMiddleware) // Security headers
	r.Use(customMiddleware.LoggingMiddleware)         // Custom logging
	r.Use(middleware.RealIP)                          // Set RemoteAddr to real IP
	// NOTE: Compression middleware is applied selectively in SetupRoutes
	// because it wraps the response writer and breaks WebSocket upgrades
	r.Use(customMiddleware.TimeoutMiddleware(60 * time.Second)) // Request timeout with custom handling

	s.router = r
}

// Router returns the configured Chi router
func (s *Server) Router() *chi.Mux {
	return s.router
}

// AuthService returns the authentication service
func (s *Server) AuthService() *auth.Service {
	return s.authService
}

// sessionCleanupWorker periodically cleans up expired sessions
func (s *Server) sessionCleanupWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.authService.CleanupExpiredSessions()
		log.Println("Cleaned up expired sessions")
	}
}

// GetWebSocketBroadcaster returns the WebSocket broadcaster
func (s *Server) GetWebSocketBroadcaster() *websocket.Broadcaster {
	return s.wsBroadcaster
}
