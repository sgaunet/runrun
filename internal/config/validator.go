package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

// Sentinel errors returned while validating a configuration.
var (
	// ErrDuplicateTaskName indicates two or more tasks share the same name.
	ErrDuplicateTaskName = errors.New("duplicate task name found")
	// ErrConfigValidation wraps one or more field-level validation failures.
	ErrConfigValidation = errors.New("configuration validation errors")
)

func init() {
	validate = validator.New()
}

// validateConfig validates the configuration using go-playground/validator.
func validateConfig(config *Config) error {
	err := validate.Struct(config)
	if err != nil {
		// Format validation errors into a readable message
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			return formatValidationErrors(validationErrors)
		}
		return fmt.Errorf("validation failed: %w", err)
	}

	// Additional custom validations
	if err := validateTaskNames(config.Tasks); err != nil {
		return err
	}

	return nil
}

// validateTaskNames ensures task names are unique.
func validateTaskNames(tasks []Task) error {
	seen := make(map[string]bool)
	for _, task := range tasks {
		if seen[task.Name] {
			return fmt.Errorf("%w: %s", ErrDuplicateTaskName, task.Name)
		}
		seen[task.Name] = true
	}
	return nil
}

// formatValidationErrors converts validator errors into user-friendly messages.
func formatValidationErrors(errs validator.ValidationErrors) error {
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		var msg string
		switch err.Tag() {
		case "required":
			msg = err.Field() + " is required"
		case "min":
			msg = fmt.Sprintf("%s must be at least %s", err.Field(), err.Param())
		case "max":
			msg = fmt.Sprintf("%s must be at most %s", err.Field(), err.Param())
		case "oneof":
			msg = fmt.Sprintf("%s must be one of: %s", err.Field(), err.Param())
		default:
			msg = fmt.Sprintf("%s failed validation: %s", err.Field(), err.Tag())
		}
		messages = append(messages, msg)
	}
	return fmt.Errorf("%w:\n  - %s", ErrConfigValidation, strings.Join(messages, "\n  - "))
}
