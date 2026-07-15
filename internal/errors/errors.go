// Package errors provides application-level HTTP error types plus helpers
// for logging them and writing standardized JSON error/success responses.
// It is conventionally imported under the alias apperrors to avoid
// shadowing the standard library errors package.
package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"

	"github.com/sgaunet/runrun/internal/ctxkeys"
)

// HTTP status thresholds used to classify AppError severity for logging and
// for deciding whether internal details may be exposed to clients.
const (
	// serverErrorThreshold is the lowest status code considered a server error (5xx).
	serverErrorThreshold = 500
	// clientErrorThreshold is the lowest status code considered a client error (4xx).
	clientErrorThreshold = 400
)

// AppError represents an application error with HTTP context.
type AppError struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	InternalID string `json:"-"` // Not exposed to clients
	Err        error  `json:"-"` // Original error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// ErrorResponse represents the JSON error response sent to clients.
type ErrorResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Code      int    `json:"code"`
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusBadRequest,
		Message: message,
		Err:     err,
	}
}

// Unauthorized creates a 401 Unauthorized error.
func Unauthorized(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: message,
	}
}

// Forbidden creates a 403 Forbidden error.
func Forbidden(message string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: message,
	}
}

// NotFound creates a 404 Not Found error.
func NotFound(message string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: message,
	}
}

// Conflict creates a 409 Conflict error.
func Conflict(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: message,
		Err:     err,
	}
}

// InternalError creates a 500 Internal Server Error.
func InternalError(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusInternalServerError,
		Message: message,
		Err:     err,
	}
}

// ServiceUnavailable creates a 503 Service Unavailable error.
func ServiceUnavailable(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusServiceUnavailable,
		Message: message,
		Err:     err,
	}
}

// HandleError sends a properly formatted error response to the client.
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *AppError

	// Check if it's already an AppError
	if !errors.As(err, &appErr) {
		// Not an AppError, wrap it as internal server error
		appErr = InternalError("An unexpected error occurred", err)
	}

	// Get request ID from context if available
	requestID := r.Context().Value(ctxkeys.RequestID)
	if requestID != nil {
		if rid, ok := requestID.(string); ok {
			appErr.RequestID = rid
		}
	}

	// Log the error with full details (including stack trace for 5xx errors).
	// The method and path are attacker-controlled (a percent-encoded path may
	// decode to contain control characters), so they are quoted via
	// strconv.Quote before being written to the log to prevent log injection.
	method, path := strconv.Quote(r.Method), strconv.Quote(r.URL.Path)
	switch {
	case appErr.Code >= serverErrorThreshold:
		log.Printf("[ERROR] %s %s - Status: %d, Message: %s, Error: %v\n%s",
			method, path, appErr.Code, appErr.Message, appErr.Err, debug.Stack())
	case appErr.Code >= clientErrorThreshold:
		log.Printf("[WARN] %s %s - Status: %d, Message: %s, Details: %s",
			method, path, appErr.Code, appErr.Message, appErr.Details)
	}

	// Prepare response
	response := ErrorResponse{
		Success:   false,
		Error:     http.StatusText(appErr.Code),
		Message:   appErr.Message,
		Details:   appErr.Details,
		RequestID: appErr.RequestID,
		Code:      appErr.Code,
	}

	// For internal server errors, don't expose internal details
	if appErr.Code >= serverErrorThreshold {
		response.Message = "An internal error occurred"
		response.Details = ""
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.Code)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[ERROR] Failed to encode error response: %v", err)
	}
}

// RespondJSON sends a successful JSON response.
func RespondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[ERROR] Failed to encode JSON response: %v", err)
		HandleError(w, &http.Request{}, InternalError("Failed to encode response", err))
	}
}

// SuccessResponse represents a successful API response.
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// RespondSuccess sends a standardized success response.
func RespondSuccess(w http.ResponseWriter, message string, data any) {
	RespondJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}
