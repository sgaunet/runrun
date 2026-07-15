package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sgaunet/runrun/internal/ctxkeys"
	"github.com/stretchr/testify/assert"
)

func TestRequestIDMiddleware(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is in context
		requestID := r.Context().Value(ctxkeys.RequestID)
		assert.NotNil(t, requestID)

		requestIDStr, ok := requestID.(string)
		assert.True(t, ok)
		assert.NotEmpty(t, requestIDStr)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify X-Request-ID header was set
	requestIDHeader := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestIDHeader)

	// Verify it's a valid UUID format (36 characters with hyphens)
	assert.Len(t, requestIDHeader, 36)
	assert.Contains(t, requestIDHeader, "-")
}

func TestRequestIDMiddleware_UniqueIDs(t *testing.T) {
	requestIDs := make(map[string]bool)

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(ctxkeys.RequestID).(string)
		requestIDs[requestID] = true
		w.WriteHeader(http.StatusOK)
	}))

	// Make multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// All request IDs should be unique
	assert.Len(t, requestIDs, 10)
}

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

func TestRecoveryMiddleware_WithPanic(t *testing.T) {
	handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(w, req)

	// Should return 500 error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRecoveryMiddleware_WithNilPanic(t *testing.T) {
	handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ptr *string
		_ = *ptr // This will panic
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(w, req)

	// Should return 500 error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSecurityHeadersMiddleware_AllHeaders(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify all security headers are set
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	assert.NotEmpty(t, w.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "geolocation=(), microphone=(), camera=()", w.Header().Get("Permissions-Policy"))
}

func TestSecurityHeadersMiddleware_CSP(t *testing.T) {
	// Exercise SecurityHeadersMiddleware behind CSPNonceMiddleware, as it
	// always runs in production (see internal/server/server.go), so a real
	// per-request nonce is present in the emitted policy.
	var nonce string
	handler := CSPNonceMiddleware(SecurityHeadersMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nonce = NonceFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	assert.NotEmpty(t, nonce, "CSPNonceMiddleware must populate a nonce")

	// Current intended strict policy (post Tailwind -> Bulma/Alpine.js CSP
	// build migration): no 'unsafe-inline', no 'unsafe-eval', script-src
	// gated by a per-request nonce.
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "script-src 'self' 'nonce-"+nonce+"'")
	assert.Contains(t, csp, "style-src 'self'")
	assert.Contains(t, csp, "img-src 'self' data:")
	assert.Contains(t, csp, "font-src 'self'")
	assert.Contains(t, csp, "connect-src 'self'")
	assert.Contains(t, csp, "frame-ancestors 'none'")
	assert.Contains(t, csp, "base-uri 'self'")
	assert.Contains(t, csp, "form-action 'self'")
	assert.Contains(t, csp, "object-src 'none'")

	// Security-critical invariants: the strict CSP must never regress to
	// permitting inline scripts/styles or eval.
	assert.NotContains(t, csp, "'unsafe-inline'")
	assert.NotContains(t, csp, "'unsafe-eval'")
}

func TestSecurityHeadersMiddleware_CSPNoNonceInContext(t *testing.T) {
	// SecurityHeadersMiddleware used without CSPNonceMiddleware in front
	// (no nonce in context) must not emit a malformed, empty 'nonce-'
	// source. It should omit the nonce source entirely and fall back to
	// script-src 'self', which still blocks all inline scripts.
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "script-src 'self';", "script-src must fall back to 'self' only")
	assert.NotContains(t, csp, "nonce-", "no nonce source should be emitted when no nonce is in context")
}

func TestSecurityHeadersMiddleware_NoHSTS_HTTP(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No TLS, so HSTS should not be set
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Strict-Transport-Security"))
}

func TestLoggingMiddleware(t *testing.T) {
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxkeys.RequestID, "test-request-id"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "response")
}

func TestLoggingMiddleware_NoRequestID(t *testing.T) {
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic even without request ID in context
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLoggingMiddleware_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"NotFound", http.StatusNotFound},
		{"InternalError", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
		})
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	rw.WriteHeader(http.StatusCreated)

	assert.Equal(t, http.StatusCreated, rw.statusCode)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestResponseWriter_DefaultStatusCode(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Don't call WriteHeader, just write body
	rw.Write([]byte("test"))

	// Should use default status code
	assert.Equal(t, http.StatusOK, rw.statusCode)
}

func TestResponseWriter_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Should not panic
	rw.Flush()
}

func TestResponseWriter_Hijack_NotSupported(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// httptest.ResponseRecorder doesn't support Hijack
	conn, buf, err := rw.Hijack()

	assert.Nil(t, conn)
	assert.Nil(t, buf)
	assert.Equal(t, http.ErrNotSupported, err)
}

func TestTimeoutMiddleware_NoTimeout(t *testing.T) {
	middleware := TimeoutMiddleware(5 * time.Second)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

func TestTimeoutMiddleware_WithTimeout(t *testing.T) {
	middleware := TimeoutMiddleware(100 * time.Millisecond)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow operation
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should timeout and return 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestTimeoutMiddleware_FastRequest(t *testing.T) {
	middleware := TimeoutMiddleware(1 * time.Second)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fast operation
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fast"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "fast")
}

func TestMiddlewareChain(t *testing.T) {
	// Test multiple middleware together
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is present
		requestID := r.Context().Value(ctxkeys.RequestID)
		assert.NotNil(t, requestID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("chained"))
	})

	// Chain multiple middleware
	chained := RequestIDMiddleware(
		RecoveryMiddleware(
			SecurityHeadersMiddleware(
				LoggingMiddleware(handler),
			),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	chained.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "chained")

	// Verify headers from different middleware
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
}

func TestMiddlewareOrder(t *testing.T) {
	var executionOrder []string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "handler")
		w.WriteHeader(http.StatusOK)
	})

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "middleware1-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "middleware1-after")
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "middleware2-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "middleware2-after")
		})
	}

	chained := middleware1(middleware2(handler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	chained.ServeHTTP(w, req)

	expected := []string{
		"middleware1-before",
		"middleware2-before",
		"handler",
		"middleware2-after",
		"middleware1-after",
	}

	assert.Equal(t, expected, executionOrder)
}

func TestRecoveryMiddleware_PreservesStatusCode(t *testing.T) {
	handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "created")
}

func TestLoggingMiddleware_DifferentMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(method, "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestTimeoutMiddleware_ContextCancellation(t *testing.T) {
	middleware := TimeoutMiddleware(100 * time.Millisecond)

	var contextCancelled atomic.Bool
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			contextCancelled.Store(true)
			return
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Give time for context cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	assert.True(t, contextCancelled.Load(), "Context should be cancelled on timeout")
}

func TestSecurityHeadersMiddleware_MultipleRequests(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make multiple requests to ensure headers are consistent
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.NotEmpty(t, w.Header().Get("Content-Security-Policy"))
	}
}

func TestResponseWriter_MultipleWriteHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// First write
	rw.WriteHeader(http.StatusCreated)

	// Second write (should be ignored by HTTP spec)
	rw.WriteHeader(http.StatusBadRequest)

	// Should keep first status code
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestRequestIDMiddleware_Integration(t *testing.T) {
	var capturedRequestID string

	handler := RequestIDMiddleware(
		LoggingMiddleware(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestID := r.Context().Value(ctxkeys.RequestID)
				if rid, ok := requestID.(string); ok {
					capturedRequestID = rid
				}
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Request ID should be captured and set in header
	assert.NotEmpty(t, capturedRequestID)
	assert.Equal(t, capturedRequestID, w.Header().Get("X-Request-ID"))
}

func TestTimeoutMiddleware_VeryShortTimeout(t *testing.T) {
	// Very short timeout with slow handler
	middleware := TimeoutMiddleware(1 * time.Millisecond)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should timeout
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestLoggingMiddleware_WithRequestIDInvalid(t *testing.T) {
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Set invalid type for request_id
	req = req.WithContext(context.WithValue(req.Context(), ctxkeys.RequestID, 12345))
	w := httptest.NewRecorder()

	// Should not panic with invalid type
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// captureNonce wraps CSPNonceMiddleware and writes the per-request nonce
// observed inside the handler into a string pointer for the test to assert
// on.
func captureNonce(out *string) http.Handler {
	return CSPNonceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*out = NonceFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCSPNonceMiddlewareGeneratesFreshNoncePerRequest(t *testing.T) {
	var first, second string
	handler := captureNonce(&first)

	req1 := httptest.NewRequest(http.MethodGet, "/page", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Re-bind output pointer for a second request
	handler = captureNonce(&second)
	req2 := httptest.NewRequest(http.MethodGet, "/page", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.NotEmpty(t, first, "nonce should be populated on the request context")
	assert.NotEmpty(t, second)
	assert.NotEqual(t, first, second, "each request must receive a distinct nonce")

	// base64-url encoding of 16 bytes yields 22 chars (no padding). At minimum
	// the nonce MUST have enough entropy to round-trip to ≥128 bits.
	assert.GreaterOrEqual(t, len(first), 22)
	assert.GreaterOrEqual(t, len(second), 22)
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]+$`), first,
		"nonce should be base64url-encoded (no padding)")
}

func TestNonceFromContextReturnsEmptyWhenAbsent(t *testing.T) {
	assert.Equal(t, "", NonceFromContext(context.Background()))
}

// runSecurityHeaders runs SecurityHeadersMiddleware inside CSPNonceMiddleware
// and returns the resulting CSP header on the response.
func runSecurityHeaders(t *testing.T) (string, string) {
	t.Helper()
	chain := CSPNonceMiddleware(SecurityHeadersMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)
	csp := w.Header().Get("Content-Security-Policy")
	return csp, w.Header().Get("X-Frame-Options")
}

func TestSecurityHeadersCSPExcludesUnsafeInline(t *testing.T) {
	csp, _ := runSecurityHeaders(t)
	assert.NotEmpty(t, csp, "Content-Security-Policy must be set")
	assert.NotContains(t, csp, "'unsafe-inline'",
		"strict CSP MUST NOT contain 'unsafe-inline'")
}

func TestSecurityHeadersCSPExcludesUnsafeEval(t *testing.T) {
	csp, _ := runSecurityHeaders(t)
	assert.NotContains(t, csp, "'unsafe-eval'",
		"strict CSP MUST NOT contain 'unsafe-eval'")
}

func TestSecurityHeadersCSPContainsRequestNonce(t *testing.T) {
	csp, xfo := runSecurityHeaders(t)
	assert.Contains(t, csp, "script-src 'self' 'nonce-",
		"script-src must use a nonce, not 'unsafe-inline'")
	// Sanity check: other expected directives are present.
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "object-src 'none'")
	assert.Contains(t, csp, "frame-ancestors 'none'")
	assert.Equal(t, "DENY", xfo)
}

func TestSecurityHeadersCSPNonceMatchesContextValue(t *testing.T) {
	var observed string
	chain := CSPNonceMiddleware(SecurityHeadersMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			observed = NonceFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	assert.NotEmpty(t, observed)
	assert.True(t,
		strings.Contains(csp, "'nonce-"+observed+"'"),
		"the nonce in the CSP header must equal the one stored on the request context (got header %q, ctx %q)",
		csp, observed,
	)
}
