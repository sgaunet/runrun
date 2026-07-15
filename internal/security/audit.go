// Package security provides structured audit logging for
// security-relevant HTTP events (authentication, authorization,
// CSRF/rate-limit violations, and suspicious activity).
package security

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/sgaunet/runrun/internal/ctxkeys"
)

// EventType represents different types of security events.
type EventType string

// Event type constants for security audit events.
const (
	// EventLoginSuccess marks a successful authentication.
	EventLoginSuccess EventType = "login_success"
	// EventLoginFailure marks a failed authentication attempt.
	EventLoginFailure EventType = "login_failure"
	// EventLogout marks a user-initiated logout.
	EventLogout EventType = "logout"
	// EventUnauthorized marks a request rejected for lacking valid auth.
	EventUnauthorized EventType = "unauthorized_access"
	// EventRateLimitExceed marks a request rejected by the rate limiter.
	EventRateLimitExceed EventType = "rate_limit_exceeded"
	// EventCSRFViolation marks a request rejected for failing CSRF validation.
	EventCSRFViolation EventType = "csrf_violation"
	// EventInvalidInput marks a request rejected for failing input validation.
	EventInvalidInput EventType = "invalid_input"
	// EventPermissionDenied marks a request rejected for insufficient permissions.
	EventPermissionDenied EventType = "permission_denied"
	// EventTaskExecution marks the start of a task execution.
	EventTaskExecution EventType = "task_execution"
	// EventSuspiciousActivity marks behavior flagged as potentially malicious.
	EventSuspiciousActivity EventType = "suspicious_activity"
)

// Event represents a security audit event.
type Event struct {
	Timestamp  time.Time      `json:"timestamp"`
	Type       EventType      `json:"type"`
	Username   string         `json:"username,omitempty"`
	IP         string         `json:"ip"`
	UserAgent  string         `json:"user_agent,omitempty"`
	Path       string         `json:"path"`
	Method     string         `json:"method"`
	StatusCode int            `json:"status_code,omitempty"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
}

// Logger handles security event logging.
type Logger struct {
	// In a production environment, this could write to a dedicated security log file,
	// send to a SIEM system, or store in a database
}

// NewLogger creates a new security audit logger.
func NewLogger() *Logger {
	return &Logger{}
}

// Log logs a security event.
func (l *Logger) Log(event Event) {
	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Convert to JSON for structured logging
	jsonData, err := json.Marshal(event)
	if err != nil {
		log.Printf("[AUDIT ERROR] Failed to marshal security event: %v", err)
		return
	}

	// Log to standard logger with [AUDIT] prefix
	log.Printf("[AUDIT] %s", string(jsonData))
}

// LogFromRequest creates and logs an event from an HTTP request.
func (l *Logger) LogFromRequest(r *http.Request, eventType EventType, message string, details map[string]any) {
	// Get username from context if available
	username := ""
	if usernameCtx := r.Context().Value("username"); usernameCtx != nil {
		if u, ok := usernameCtx.(string); ok {
			username = u
		}
	}

	// Get request ID from context
	requestID := ""
	if ridCtx := r.Context().Value(ctxkeys.RequestID); ridCtx != nil {
		if rid, ok := ridCtx.(string); ok {
			requestID = rid
		}
	}

	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Username:  username,
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Path:      r.URL.Path,
		Method:    r.Method,
		Message:   message,
		Details:   details,
		RequestID: requestID,
	}

	l.Log(event)
}

// LogLogin logs a login attempt.
func (l *Logger) LogLogin(r *http.Request, username string, success bool, reason string) {
	eventType := EventLoginSuccess
	message := "User logged in successfully"

	if !success {
		eventType = EventLoginFailure
		message = "Login failed: " + reason
	}

	details := map[string]any{
		"username": username,
		"success":  success,
	}

	if !success {
		details["reason"] = reason
	}

	l.LogFromRequest(r, eventType, message, details)
}

// LogLogout logs a logout event.
func (l *Logger) LogLogout(r *http.Request, username string) {
	l.LogFromRequest(r, EventLogout, "User logged out", map[string]any{
		"username": username,
	})
}

// LogUnauthorized logs an unauthorized access attempt.
func (l *Logger) LogUnauthorized(r *http.Request, reason string) {
	l.LogFromRequest(r, EventUnauthorized, "Unauthorized access attempt: "+reason, map[string]any{
		"reason": reason,
	})
}

// LogRateLimitExceeded logs when rate limit is exceeded.
func (l *Logger) LogRateLimitExceeded(r *http.Request, limit int, window time.Duration) {
	l.LogFromRequest(r, EventRateLimitExceed, "Rate limit exceeded", map[string]any{
		"limit":  limit,
		"window": window.String(),
	})
}

// LogCSRFViolation logs a CSRF violation.
func (l *Logger) LogCSRFViolation(r *http.Request, reason string) {
	l.LogFromRequest(r, EventCSRFViolation, "CSRF violation: "+reason, map[string]any{
		"reason": reason,
	})
}

// LogTaskExecution logs task execution events.
func (l *Logger) LogTaskExecution(r *http.Request, taskName string, executionID string) {
	l.LogFromRequest(r, EventTaskExecution, "Task execution initiated", map[string]any{
		"task_name":    taskName,
		"execution_id": executionID,
	})
}

// LogSuspiciousActivity logs suspicious behavior.
func (l *Logger) LogSuspiciousActivity(r *http.Request, reason string, details map[string]any) {
	if details == nil {
		details = make(map[string]any)
	}
	details["reason"] = reason

	l.LogFromRequest(r, EventSuspiciousActivity, "Suspicious activity detected: "+reason, details)
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (when behind a proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}
