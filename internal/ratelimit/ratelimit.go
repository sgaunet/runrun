package ratelimit

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	apperrors "github.com/sgaunet/runrun/internal/errors"
)

// Limiter tracks rate limiting for IP addresses
type Limiter struct {
	visitors map[string]*Visitor
	mu       sync.RWMutex
	rate     int           // requests per window
	window   time.Duration // time window for rate limiting
}

// Visitor tracks request counts for an IP address
type Visitor struct {
	count     int
	lastSeen  time.Time
	resetTime time.Time
	mu        sync.Mutex
}

// NewLimiter creates a new rate limiter
// rate: number of requests allowed
// window: time window for the rate limit (e.g., 15 minutes)
func NewLimiter(rate int, window time.Duration) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*Visitor),
		rate:     rate,
		window:   window,
	}

	// Start cleanup goroutine
	go l.cleanupVisitors()

	return l
}

// getVisitor retrieves or creates a visitor entry for an IP
func (l *Limiter) getVisitor(ip string) *Visitor {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, exists := l.visitors[ip]
	if !exists {
		v = &Visitor{
			count:     0,
			lastSeen:  time.Now(),
			resetTime: time.Now().Add(l.window),
		}
		l.visitors[ip] = v
	}

	return v
}

// Allow checks if a request from the given IP should be allowed
func (l *Limiter) Allow(ip string) bool {
	v := l.getVisitor(ip)

	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	v.lastSeen = now

	// Check if we need to reset the counter
	if now.After(v.resetTime) {
		v.count = 0
		v.resetTime = now.Add(l.window)
	}

	// Check if limit exceeded
	if v.count >= l.rate {
		return false
	}

	v.count++
	return true
}

// GetRetryAfter returns the duration until the rate limit resets for an IP
func (l *Limiter) GetRetryAfter(ip string) time.Duration {
	v := l.getVisitor(ip)

	v.mu.Lock()
	defer v.mu.Unlock()

	if time.Now().After(v.resetTime) {
		return 0
	}

	return time.Until(v.resetTime)
}

// cleanupVisitors periodically removes old visitor entries
func (l *Limiter) cleanupVisitors() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		for ip, v := range l.visitors {
			v.mu.Lock()
			// Remove visitors that haven't been seen in 2x the window duration
			if time.Since(v.lastSeen) > l.window*2 {
				delete(l.visitors, ip)
			}
			v.mu.Unlock()
		}
		l.mu.Unlock()

		log.Printf("[RATELIMIT] Cleaned up old visitor entries")
	}
}

// Middleware creates an HTTP middleware for rate limiting
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !l.Allow(ip) {
			retryAfter := l.GetRetryAfter(ip)

			log.Printf("[RATELIMIT] Rate limit exceeded for IP: %s, retry after: %v", ip, retryAfter)

			// Set Retry-After header
			w.Header().Set("Retry-After", formatDuration(retryAfter))

			err := apperrors.AppError{
				Code:    http.StatusTooManyRequests,
				Message: "Rate limit exceeded. Please try again later.",
				Details: "Too many requests from your IP address. Please wait before trying again.",
			}

			apperrors.HandleError(w, r, &err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (when behind a proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP if there are multiple
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// formatDuration formats a duration for the Retry-After header (in seconds)
func formatDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%d", seconds)
}
