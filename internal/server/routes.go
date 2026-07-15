package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	customMiddleware "github.com/sgaunet/runrun/internal/middleware"
)

// compressionLevel is the gzip compression level applied to non-WebSocket
// responses (see middleware.Compress).
const compressionLevel = 5

// requestTimeout bounds how long a non-WebSocket request may run before
// the server aborts it.
const requestTimeout = 60 * time.Second

// SetupRoutes configures all application routes.
func (s *Server) SetupRoutes() {
	r := s.router

	// WebSocket routes - NO compression or timeout middleware (both break Hijacker interface)
	r.Get("/logs/ws/{executionID}", s.wsLogsHandler)

	// Apply compression and timeout middleware to all non-WebSocket routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.Compress(compressionLevel))
		r.Use(customMiddleware.TimeoutMiddleware(requestTimeout))

		// Public routes (no authentication required)
		r.Group(func(r chi.Router) {
			r.Get("/login", s.loginPageHandlerTempl)

			// Apply rate limiting to login POST endpoint
			r.Group(func(r chi.Router) {
				r.Use(s.rateLimiter.Middleware)
				r.Post("/login", s.authService.LoginHandler)
			})

			r.Post("/logout", s.authService.LogoutHandler)

			// Health check endpoints
			r.Get("/health", s.healthCheckHandler)
			r.Get("/health/ready", s.readinessHandler)
			r.Get("/health/live", s.livenessHandler)

			// Static assets
			r.Handle("/static/*", http.StripPrefix("/static/", s.serveStaticFiles()))
		})

		// Protected routes (authentication required)
		r.Group(func(r chi.Router) {
			// Apply authentication middleware
			r.Use(s.authService.AuthMiddleware)

			// Dashboard
			r.Get("/", s.dashboardHandlerTempl)

			// Task routes
			r.Route("/tasks", func(r chi.Router) {
				r.Get("/{taskName}", s.taskDetailHandlerTempl)

				// Apply CSRF protection to POST requests
				r.Group(func(r chi.Router) {
					r.Use(s.csrf.Middleware)
					r.Post("/{taskName}/execute", s.executeTaskHandler)
				})
			})

			// API routes
			r.Route("/api", func(r chi.Router) {
				r.Get("/status", s.statusAPIHandler)
			})

			// Log routes
			r.Route("/logs", func(r chi.Router) {
				r.Get("/{executionID}", s.viewLogsHandlerTempl)
				r.Get("/{executionID}/download", s.downloadLogsHandler)
				r.Get("/{executionID}/poll", s.pollLogsHandler)
				r.Get("/{executionID}/segment", s.segmentLogsHandler)
			})
		})
	})
}
