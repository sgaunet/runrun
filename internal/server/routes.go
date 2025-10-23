package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// SetupRoutes configures all application routes
func (s *Server) SetupRoutes() {
	r := s.router

	// Public routes (no authentication required)
	r.Group(func(r chi.Router) {
		r.Get("/login", s.loginPageHandlerTempl)
		r.Post("/login", s.authService.LoginHandler)
		r.Post("/logout", s.authService.LogoutHandler)

		// Health check
		r.Get("/health", s.healthCheckHandler)

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
			r.Post("/{taskName}/execute", s.executeTaskHandler)
		})

		// API routes
		r.Route("/api", func(r chi.Router) {
			r.Get("/status", s.statusAPIHandler)
			r.Get("/logs/{executionID}/poll", s.pollLogsHandler)
		})

		// Log routes
		r.Route("/logs", func(r chi.Router) {
			r.Get("/{executionID}", s.viewLogsHandlerTempl)
			r.Get("/{executionID}/download", s.downloadLogsHandler)
			r.Get("/ws/{executionID}", s.wsHandler.ServeWS)
		})
	})
}
