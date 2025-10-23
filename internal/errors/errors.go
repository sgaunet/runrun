package errors

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

// AppError represents an application error with HTTP context
type AppError struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	InternalID string `json:"-"` // Not exposed to clients
	Err        error  `json:"-"` // Original error
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// ErrorResponse represents the JSON error response sent to clients
type ErrorResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Code      int    `json:"code"`
}

// Common error constructors
func BadRequest(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusBadRequest,
		Message: message,
		Err:     err,
	}
}

func Unauthorized(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: message,
	}
}

func Forbidden(message string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: message,
	}
}

func NotFound(message string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: message,
	}
}

func Conflict(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: message,
		Err:     err,
	}
}

func InternalError(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusInternalServerError,
		Message: message,
		Err:     err,
	}
}

func ServiceUnavailable(message string, err error) *AppError {
	return &AppError{
		Code:    http.StatusServiceUnavailable,
		Message: message,
		Err:     err,
	}
}

// HandleError sends a properly formatted error response to the client
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *AppError
	var ok bool

	// Check if it's already an AppError
	if appErr, ok = err.(*AppError); !ok {
		// Not an AppError, wrap it as internal server error
		appErr = InternalError("An unexpected error occurred", err)
	}

	// Get request ID from context if available
	requestID := r.Context().Value("request_id")
	if requestID != nil {
		if rid, ok := requestID.(string); ok {
			appErr.RequestID = rid
		}
	}

	// Log the error with full details (including stack trace for 5xx errors)
	if appErr.Code >= 500 {
		log.Printf("[ERROR] %s %s - Status: %d, Message: %s, Error: %v\n%s",
			r.Method, r.URL.Path, appErr.Code, appErr.Message, appErr.Err, debug.Stack())
	} else if appErr.Code >= 400 {
		log.Printf("[WARN] %s %s - Status: %d, Message: %s, Details: %s",
			r.Method, r.URL.Path, appErr.Code, appErr.Message, appErr.Details)
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
	if appErr.Code >= 500 {
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

// RespondJSON sends a successful JSON response
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[ERROR] Failed to encode JSON response: %v", err)
		HandleError(w, &http.Request{}, InternalError("Failed to encode response", err))
	}
}

// SuccessResponse represents a successful API response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// RespondSuccess sends a standardized success response
func RespondSuccess(w http.ResponseWriter, message string, data interface{}) {
	RespondJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}
