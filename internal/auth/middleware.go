package auth

import (
	"context"
	"log"
	"net/http"
	"strings"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// UserContextKey is the context key for storing the username
	UserContextKey ContextKey = "username"
)

// AuthMiddleware is a middleware that validates JWT tokens
func (s *Service) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get token from cookie first
		token := ""
		cookie, err := r.Cookie(SessionCookieName)
		if err == nil {
			token = cookie.Value
		}

		// If no cookie, try Authorization header
		if token == "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				// Expected format: "Bearer <token>"
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					token = parts[1]
				}
			}
		}

		// No token found
		if token == "" {
			log.Printf("No authentication token found for %s %s", r.Method, r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Validate session
		username, err := s.ValidateSession(token)
		if err != nil {
			log.Printf("Invalid session for %s %s: %v", r.Method, r.URL.Path, err)
			// Clear invalid cookie
			http.SetCookie(w, &http.Cookie{
				Name:   SessionCookieName,
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Add username to context
		ctx := context.WithValue(r.Context(), UserContextKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUsernameFromContext retrieves the username from the request context
func GetUsernameFromContext(r *http.Request) string {
	if username, ok := r.Context().Value(UserContextKey).(string); ok {
		return username
	}
	return ""
}

// OptionalAuthMiddleware is a middleware that adds user context if authenticated
// but doesn't redirect if not authenticated
func (s *Service) OptionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get token from cookie
		token := ""
		cookie, err := r.Cookie(SessionCookieName)
		if err == nil {
			token = cookie.Value
		}

		// If no cookie, try Authorization header
		if token == "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					token = parts[1]
				}
			}
		}

		// If token exists, validate and add to context
		if token != "" {
			username, err := s.ValidateSession(token)
			if err == nil {
				ctx := context.WithValue(r.Context(), UserContextKey, username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// No valid auth, continue without user context
		next.ServeHTTP(w, r)
	})
}
