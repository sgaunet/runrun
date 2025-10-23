package server

import (
	"log"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/executor"
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
}

// New creates a new server instance
func New(cfg *config.Config) *Server {
	s := &Server{
		config: cfg,
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
	r.Use(middleware.RequestID)        // Inject request ID into context
	r.Use(middleware.RealIP)           // Set RemoteAddr to real IP
	r.Use(middleware.Logger)           // Log requests
	r.Use(middleware.Recoverer)        // Recover from panics
	r.Use(middleware.Compress(5))      // Compress responses
	r.Use(middleware.Timeout(60 * time.Second)) // Request timeout

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
