package middleware

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sgaunet/runrun/internal/ctxkeys"
	apperrors "github.com/sgaunet/runrun/internal/errors"
)

// cspNonceKey is the unexported context key under which the per-request
// CSP nonce is stored. Using a distinct unexported type avoids context
// key collisions with other packages.
type cspNonceKey struct{}

// cspNonceBytes is the number of random bytes used to generate the
// per-request nonce. 16 bytes = 128 bits of entropy, the OWASP minimum
// recommendation for CSP nonces.
const cspNonceBytes = 16

// CSPNonceMiddleware generates a fresh, cryptographically random nonce
// for each request and stores it on the request context. Downstream
// handlers retrieve it via NonceFromContext; SecurityHeadersMiddleware
// uses it to populate the script-src directive of the Content-Security-
// Policy header.
func CSPNonceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, cspNonceBytes)
		if _, err := rand.Read(buf); err != nil {
			// rand.Read should not fail; if it does, fail closed.
			apperrors.HandleError(w, r,
				apperrors.InternalError("failed to generate CSP nonce", err))
			return
		}
		nonce := base64.RawURLEncoding.EncodeToString(buf)
		ctx := context.WithValue(r.Context(), cspNonceKey{}, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// NonceFromContext returns the CSP nonce stored on ctx by
// CSPNonceMiddleware, or "" if no nonce is present.
func NonceFromContext(ctx context.Context) string {
	if v := ctx.Value(cspNonceKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RequestIDMiddleware adds a unique request ID to each request context
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		ctx := context.WithValue(r.Context(), ctxkeys.RequestID, requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RecoveryMiddleware recovers from panics and returns a 500 error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC] Recovered from panic: %v\n%s", err, debug.Stack())

				// Send error response
				appErr := apperrors.InternalError("Internal server error", nil)
				apperrors.HandleError(w, r, appErr)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware adds security-related HTTP headers
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Enable XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Strict Content Security Policy. Per specs/001-replace-tailwind-bulma:
		//   - no 'unsafe-inline' in script-src or style-src
		//   - no 'unsafe-eval' in script-src
		//   - script-src uses a per-request nonce supplied by CSPNonceMiddleware
		// If CSPNonceMiddleware has not run for this request (e.g. a handler
		// wired up outside the standard middleware chain), no nonce is
		// available. The nonce source is omitted entirely in that case rather
		// than emitted as an empty, spec-invalid 'nonce-' token; script-src
		// then falls back to 'self' only, which still blocks all inline
		// scripts.
		nonce := NonceFromContext(r.Context())
		scriptSrc := "script-src 'self'"
		if nonce != "" {
			scriptSrc += " 'nonce-" + nonce + "'"
		}
		csp := "default-src 'self'; " +
			scriptSrc + "; " +
			"style-src 'self'; " +
			"img-src 'self' data:; " +
			"font-src 'self'; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'; " +
			"object-src 'none'"
		w.Header().Set("Content-Security-Policy", csp)

		// Permissions Policy (formerly Feature-Policy)
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Only allow HTTPS in production (check if request is HTTPS)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs HTTP requests with duration and status
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Get request ID from context
		requestID := ""
		if rid := r.Context().Value(ctxkeys.RequestID); rid != nil {
			if ridStr, ok := rid.(string); ok {
				requestID = ridStr
			}
		}

		log.Printf("[%s] %s %s - Status: %d, Duration: %v, RequestID: %s",
			r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, duration, requestID)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker interface for WebSocket support
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Flush implements http.Flusher interface for streaming responses
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// timeoutWriter wraps http.ResponseWriter to prevent concurrent writes
type timeoutWriter struct {
	w        http.ResponseWriter
	mu       sync.Mutex
	written  bool
	timedOut bool
}

func (tw *timeoutWriter) Header() http.Header {
	return tw.w.Header()
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return 0, http.ErrHandlerTimeout
	}
	if !tw.written {
		tw.written = true
	}
	return tw.w.Write(b)
}

func (tw *timeoutWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return
	}
	if !tw.written {
		tw.written = true
		tw.w.WriteHeader(code)
	}
}

func (tw *timeoutWriter) timeout() bool {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.written {
		return false
	}
	tw.timedOut = true
	return true
}

// TimeoutMiddleware adds a timeout to requests
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			tw := &timeoutWriter{w: w}
			done := make(chan struct{})

			go func() {
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// Request completed successfully
				return
			case <-ctx.Done():
				// Request timed out - only write timeout response if handler hasn't written yet
				if tw.timeout() {
					err := apperrors.ServiceUnavailable("Request timeout", ctx.Err())
					apperrors.HandleError(w, r, err)
				}
				return
			}
		})
	}
}
