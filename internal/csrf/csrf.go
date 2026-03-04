package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"sync"

	apperrors "github.com/sgaunet/runrun/internal/errors"
)

const (
	// TokenLength is the length of the CSRF token in bytes
	TokenLength = 32

	// TokenHeader is the header name for CSRF tokens
	TokenHeader = "X-CSRF-Token"

	// TokenFormField is the form field name for CSRF tokens
	TokenFormField = "csrf_token"

	// TokenCookie is the cookie name for CSRF tokens
	TokenCookie = "csrf_token"
)

// Protection provides CSRF protection functionality
type Protection struct {
	tokens map[string]string // session ID -> token
	mu     sync.RWMutex
}

// New creates a new CSRF protection instance
func New() *Protection {
	return &Protection{
		tokens: make(map[string]string),
	}
}

// GenerateToken generates a new CSRF token for a session
func (p *Protection) GenerateToken(sessionID string) (string, error) {
	// Generate random bytes
	bytes := make([]byte, TokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode to base64
	token := base64.URLEncoding.EncodeToString(bytes)

	// Store token for session
	p.mu.Lock()
	p.tokens[sessionID] = token
	p.mu.Unlock()

	return token, nil
}

// GetToken retrieves the CSRF token for a session
func (p *Protection) GetToken(sessionID string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tokens[sessionID]
}

// ValidateToken checks if a token is valid for a session
func (p *Protection) ValidateToken(sessionID, token string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	expectedToken, exists := p.tokens[sessionID]
	if !exists {
		return false
	}

	return token == expectedToken
}

// DeleteToken removes a token for a session
func (p *Protection) DeleteToken(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tokens, sessionID)
}

// Middleware creates HTTP middleware for CSRF protection
func (p *Protection) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF check for safe methods
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Get session ID from cookie
		sessionCookie, err := r.Cookie("session")
		if err != nil {
			log.Printf("[CSRF] No session cookie found")
			apperrors.HandleError(w, r, apperrors.Forbidden("CSRF validation failed: no session"))
			return
		}

		sessionID := sessionCookie.Value

		// Get token from request (check header first, then form)
		token := r.Header.Get(TokenHeader)
		if token == "" {
			// Try to get from form
			if err := r.ParseForm(); err == nil {
				token = r.FormValue(TokenFormField)
			}
		}

		if token == "" {
			log.Printf("[CSRF] No CSRF token provided in request")
			apperrors.HandleError(w, r, apperrors.Forbidden("CSRF validation failed: no token"))
			return
		}

		// Validate token
		if !p.ValidateToken(sessionID, token) {
			log.Printf("[CSRF] Invalid CSRF token for session: %s", sessionID)
			apperrors.HandleError(w, r, apperrors.Forbidden("CSRF validation failed: invalid token"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isSafeMethod returns true if the HTTP method is considered safe (no side effects)
func isSafeMethod(method string) bool {
	return method == http.MethodGet ||
		method == http.MethodHead ||
		method == http.MethodOptions ||
		method == http.MethodTrace
}

// SetTokenCookie sets a CSRF token cookie
func (p *Protection) SetTokenCookie(w http.ResponseWriter, sessionID string) error {
	token, err := p.GenerateToken(sessionID)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     TokenCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JavaScript needs to read this
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
	})

	return nil
}
