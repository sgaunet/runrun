package validation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	apperrors "github.com/sgaunet/runrun/internal/errors"
)

var (
	// Common validation patterns
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)
	taskNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	uuidRegex     = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
)

// ValidateUsername validates a username
func ValidateUsername(username string) error {
	if username == "" {
		return apperrors.BadRequest("Username is required", nil)
	}

	if !usernameRegex.MatchString(username) {
		return apperrors.BadRequest("Username must be 3-32 characters and contain only letters, numbers, hyphens, and underscores", nil)
	}

	return nil
}

// ValidatePassword validates a password
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return apperrors.BadRequest("Password must be at least 8 characters long", nil)
	}

	if len(password) > 128 {
		return apperrors.BadRequest("Password must be less than 128 characters", nil)
	}

	// Check for at least one letter and one number
	hasLetter := false
	hasNumber := false
	for _, char := range password {
		if unicode.IsLetter(char) {
			hasLetter = true
		}
		if unicode.IsNumber(char) {
			hasNumber = true
		}
	}

	if !hasLetter || !hasNumber {
		return apperrors.BadRequest("Password must contain at least one letter and one number", nil)
	}

	return nil
}

// ValidateTaskName validates a task name
func ValidateTaskName(taskName string) error {
	if taskName == "" {
		return apperrors.BadRequest("Task name is required", nil)
	}

	if !taskNameRegex.MatchString(taskName) {
		return apperrors.BadRequest("Task name must be 1-128 characters and contain only letters, numbers, hyphens, and underscores", nil)
	}

	return nil
}

// ValidateUUID validates a UUID string
func ValidateUUID(id string) error {
	if id == "" {
		return apperrors.BadRequest("ID is required", nil)
	}

	if !uuidRegex.MatchString(id) {
		return apperrors.BadRequest("Invalid ID format", nil)
	}

	return nil
}

// SanitizePath prevents path traversal attacks
func SanitizePath(path string) (string, error) {
	// Remove any path traversal attempts
	cleaned := filepath.Clean(path)

	// Check for path traversal attempts
	if strings.Contains(cleaned, "..") {
		return "", apperrors.BadRequest("Invalid path: path traversal detected", nil)
	}

	// Ensure path doesn't start with /
	if strings.HasPrefix(cleaned, "/") {
		return "", apperrors.BadRequest("Invalid path: absolute paths not allowed", nil)
	}

	return cleaned, nil
}

// SanitizeString removes potentially dangerous characters
func SanitizeString(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	return input
}

// SanitizeHTML escapes HTML special characters
func SanitizeHTML(input string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(input)
}

// ValidateContentType checks if content type is allowed
func ValidateContentType(contentType string, allowed []string) error {
	contentType = strings.ToLower(strings.TrimSpace(contentType))

	// Handle charset in content type
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = contentType[:idx]
	}

	for _, a := range allowed {
		if contentType == strings.ToLower(a) {
			return nil
		}
	}

	return apperrors.BadRequest(
		fmt.Sprintf("Content-Type %s not allowed", contentType),
		nil,
	)
}

// ValidateStringLength validates string length constraints
func ValidateStringLength(value, fieldName string, min, max int) error {
	length := len(value)

	if length < min {
		return apperrors.BadRequest(
			fmt.Sprintf("%s must be at least %d characters", fieldName, min),
			nil,
		)
	}

	if max > 0 && length > max {
		return apperrors.BadRequest(
			fmt.Sprintf("%s must be less than %d characters", fieldName, max),
			nil,
		)
	}

	return nil
}

// ValidateRequired checks if a value is not empty
func ValidateRequired(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return apperrors.BadRequest(
			fmt.Sprintf("%s is required", fieldName),
			nil,
		)
	}
	return nil
}

// ValidateEnum checks if value is in allowed list
func ValidateEnum(value string, allowed []string, fieldName string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}

	return apperrors.BadRequest(
		fmt.Sprintf("%s must be one of: %s", fieldName, strings.Join(allowed, ", ")),
		nil,
	)
}

// ValidateExecutionID validates an execution ID format
func ValidateExecutionID(id string) error {
	return ValidateUUID(id)
}

// ValidateQueryParam validates and sanitizes a query parameter
func ValidateQueryParam(param, fieldName string, maxLength int) (string, error) {
	// Sanitize
	param = SanitizeString(param)

	// Validate length
	if err := ValidateStringLength(param, fieldName, 0, maxLength); err != nil {
		return "", err
	}

	return param, nil
}
