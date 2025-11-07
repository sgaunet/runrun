package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)
	require.NotNil(t, limiter)

	assert.Equal(t, 5, limiter.rate)
	assert.Equal(t, 15*time.Minute, limiter.window)
	assert.NotNil(t, limiter.visitors)
	assert.Len(t, limiter.visitors, 0)
}

func TestGetVisitor_NewVisitor(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)

	ip := "192.168.1.1"
	visitor := limiter.getVisitor(ip)

	require.NotNil(t, visitor)
	assert.Equal(t, 0, visitor.count)
	assert.False(t, visitor.lastSeen.IsZero())
	assert.False(t, visitor.resetTime.IsZero())

	// Verify visitor was stored
	limiter.mu.RLock()
	storedVisitor, exists := limiter.visitors[ip]
	limiter.mu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, visitor, storedVisitor)
}

func TestGetVisitor_ExistingVisitor(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)

	ip := "192.168.1.2"

	// Get visitor first time
	visitor1 := limiter.getVisitor(ip)
	visitor1.count = 3

	// Get same visitor again
	visitor2 := limiter.getVisitor(ip)

	// Should be the same visitor instance
	assert.Equal(t, visitor1, visitor2)
	assert.Equal(t, 3, visitor2.count)
}

func TestAllow_WithinLimit(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)
	ip := "192.168.1.3"

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		allowed := limiter.Allow(ip)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Check visitor count
	visitor := limiter.getVisitor(ip)
	visitor.mu.Lock()
	count := visitor.count
	visitor.mu.Unlock()
	assert.Equal(t, 5, count)
}

func TestAllow_ExceedsLimit(t *testing.T) {
	limiter := NewLimiter(3, 15*time.Minute)
	ip := "192.168.1.4"

	// First 3 requests allowed
	for i := 0; i < 3; i++ {
		allowed := limiter.Allow(ip)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 4th request should be denied
	allowed := limiter.Allow(ip)
	assert.False(t, allowed, "Request 4 should be denied")

	// 5th request should also be denied
	allowed = limiter.Allow(ip)
	assert.False(t, allowed, "Request 5 should be denied")
}

func TestAllow_WindowReset(t *testing.T) {
	// Use very short window for testing
	limiter := NewLimiter(2, 100*time.Millisecond)
	ip := "192.168.1.5"

	// Use up the limit
	assert.True(t, limiter.Allow(ip))
	assert.True(t, limiter.Allow(ip))
	assert.False(t, limiter.Allow(ip), "Should be rate limited")

	// Wait for window to reset
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	assert.True(t, limiter.Allow(ip), "Should be allowed after window reset")
}

func TestAllow_MultipleIPs(t *testing.T) {
	limiter := NewLimiter(2, 15*time.Minute)

	ip1 := "192.168.1.6"
	ip2 := "192.168.1.7"

	// Each IP should have independent limits
	assert.True(t, limiter.Allow(ip1))
	assert.True(t, limiter.Allow(ip1))
	assert.False(t, limiter.Allow(ip1), "IP1 should be limited")

	assert.True(t, limiter.Allow(ip2))
	assert.True(t, limiter.Allow(ip2))
	assert.False(t, limiter.Allow(ip2), "IP2 should be limited")
}

func TestGetRetryAfter_NotExceeded(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)
	ip := "192.168.1.8"

	// Make one request
	limiter.Allow(ip)

	// Get retry after
	retryAfter := limiter.GetRetryAfter(ip)

	// Should be close to the window duration
	assert.Greater(t, retryAfter, 14*time.Minute)
	assert.LessOrEqual(t, retryAfter, 15*time.Minute)
}

func TestGetRetryAfter_AfterReset(t *testing.T) {
	limiter := NewLimiter(2, 50*time.Millisecond)
	ip := "192.168.1.9"

	// Use up limit
	limiter.Allow(ip)
	limiter.Allow(ip)

	// Wait for reset
	time.Sleep(100 * time.Millisecond)

	retryAfter := limiter.GetRetryAfter(ip)
	assert.Equal(t, time.Duration(0), retryAfter)
}

func TestMiddleware_Allowed(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.10:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

func TestMiddleware_RateLimited(t *testing.T) {
	limiter := NewLimiter(2, 15*time.Minute)

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	ip := "192.168.1.11:12345"

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i+1)
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestMiddleware_RetryAfterHeader(t *testing.T) {
	limiter := NewLimiter(1, 1*time.Minute)

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "192.168.1.12:12345"

	// Use up limit
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = ip
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Next request should be limited with Retry-After header
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = ip
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusTooManyRequests, w2.Code)

	retryAfter := w2.Header().Get("Retry-After")
	assert.NotEmpty(t, retryAfter)
	// Should be approximately 60 seconds
	assert.Contains(t, []string{"59", "60"}, retryAfter)
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	ip := getClientIP(req)
	assert.Equal(t, "192.168.1.100:12345", ip)
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")

	ip := getClientIP(req)
	assert.Equal(t, "203.0.113.1, 198.51.100.1", ip)
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Real-IP", "203.0.113.2")

	ip := getClientIP(req)
	assert.Equal(t, "203.0.113.2", ip)
}

func TestGetClientIP_PriorityOrder(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.Header.Set("X-Real-IP", "203.0.113.2")

	ip := getClientIP(req)
	// X-Forwarded-For should take priority
	assert.Equal(t, "203.0.113.1", ip)
}

func TestFormatDuration_Seconds(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "1"}, // Minimum is 1 second
		{"subsecond", 500 * time.Millisecond, "1"},
		{"one second", 1 * time.Second, "1"},
		{"five seconds", 5 * time.Second, "5"},
		{"one minute", 60 * time.Second, "60"},
		{"five minutes", 5 * time.Minute, "300"},
		{"one hour", 1 * time.Hour, "3600"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	limiter := NewLimiter(100, 1*time.Minute)

	var wg sync.WaitGroup
	numGoroutines := 10
	requestsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			ip := "192.168.1.200"
			for j := 0; j < requestsPerGoroutine; j++ {
				limiter.Allow(ip)
			}
		}(i)
	}

	wg.Wait()

	// Verify total count
	visitor := limiter.getVisitor("192.168.1.200")
	visitor.mu.Lock()
	count := visitor.count
	visitor.mu.Unlock()

	assert.Equal(t, numGoroutines*requestsPerGoroutine, count)
}

func TestConcurrentMultipleIPs(t *testing.T) {
	limiter := NewLimiter(10, 1*time.Minute)

	var wg sync.WaitGroup
	numIPs := 5
	requestsPerIP := 5

	for i := 0; i < numIPs; i++ {
		wg.Add(1)
		go func(ipID int) {
			defer wg.Done()

			ip := "192.168.1." + string(rune('1'+ipID))
			for j := 0; j < requestsPerIP; j++ {
				limiter.Allow(ip)
			}
		}(i)
	}

	wg.Wait()

	// Verify each IP has correct count
	for i := 0; i < numIPs; i++ {
		ip := "192.168.1." + string(rune('1'+i))
		visitor := limiter.getVisitor(ip)
		visitor.mu.Lock()
		count := visitor.count
		visitor.mu.Unlock()
		assert.Equal(t, requestsPerIP, count, "IP %s should have %d requests", ip, requestsPerIP)
	}
}

func TestVisitorLastSeenUpdate(t *testing.T) {
	limiter := NewLimiter(5, 15*time.Minute)
	ip := "192.168.1.20"

	// Make first request
	limiter.Allow(ip)

	visitor := limiter.getVisitor(ip)
	visitor.mu.Lock()
	firstSeen := visitor.lastSeen
	visitor.mu.Unlock()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Make another request
	limiter.Allow(ip)

	visitor.mu.Lock()
	secondSeen := visitor.lastSeen
	visitor.mu.Unlock()

	// lastSeen should be updated
	assert.True(t, secondSeen.After(firstSeen))
}

func TestResetTimeCalculation(t *testing.T) {
	window := 10 * time.Minute
	limiter := NewLimiter(5, window)
	ip := "192.168.1.21"

	// Make a request
	limiter.Allow(ip)

	visitor := limiter.getVisitor(ip)
	visitor.mu.Lock()
	resetTime := visitor.resetTime
	lastSeen := visitor.lastSeen
	visitor.mu.Unlock()

	// Reset time should be approximately lastSeen + window
	expectedReset := lastSeen.Add(window)
	assert.WithinDuration(t, expectedReset, resetTime, 100*time.Millisecond)
}

func TestMiddleware_DifferentIPs(t *testing.T) {
	limiter := NewLimiter(1, 15*time.Minute)

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request from IP1 - should succeed
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.168.1.30:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Another request from IP1 - should be limited
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.168.1.30:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)

	// Request from IP2 - should succeed (different IP)
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.168.1.31:12345"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
}
