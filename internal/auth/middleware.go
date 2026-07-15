package auth

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// ContextKey is a type for context keys.
type ContextKey string

const (
	// UserContextKey is the context key for storing the username.
	UserContextKey ContextKey = "username"
)

// AuthMiddleware is a middleware that validates JWT tokens.
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
			log.Printf("No authentication token found for %s %s", strconv.Quote(r.Method), strconv.Quote(r.URL.Path))
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Validate session
		username, err := s.ValidateSession(token)
		if err != nil {
			log.Printf("Invalid session for %s %s: %v", strconv.Quote(r.Method), strconv.Quote(r.URL.Path), err)
			// Clear invalid cookie
			//nolint:gosec // G124: Secure is computed at runtime via isSecureRequest(r); see its doc comment for rationale.
			http.SetCookie(w, &http.Cookie{
				Name:     SessionCookieName,
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   isSecureRequest(r),
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Add username to context
		ctx := context.WithValue(r.Context(), UserContextKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUsernameFromContext retrieves the username from the request context.
func GetUsernameFromContext(r *http.Request) string {
	if username, ok := r.Context().Value(UserContextKey).(string); ok {
		return username
	}
	return ""
}

// OptionalAuthMiddleware is a middleware that adds user context if authenticated
// but doesn't redirect if not authenticated.
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

// isSecureRequest reports whether the request was received directly over
// TLS, mirroring the check SecurityHeadersMiddleware already uses (see
// internal/middleware) to decide whether to send Strict-Transport-Security.
// It is used to decide whether the session cookie should carry the Secure
// attribute: RunRun has no config-driven "am I deployed behind HTTPS" flag,
// so hardcoding Secure: true would break local HTTP development, and
// hardcoding Secure: false would send the session cookie in the clear over
// TLS. If RunRun is deployed behind a TLS-terminating reverse proxy,
// r.TLS is nil even though the origin request was HTTPS; making that case
// Secure too requires the proxy to set "X-Forwarded-Proto: https" and this
// package to trust it, which is a deployment-specific decision left for a
// future config option.
func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil
}
