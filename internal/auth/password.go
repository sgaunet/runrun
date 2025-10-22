package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultCost is the default bcrypt cost factor
	DefaultCost = 10
	// MinPasswordLength is the minimum password length
	MinPasswordLength = 8
)

// HashPassword generates a bcrypt hash from a plain text password
func HashPassword(password string) (string, error) {
	return HashPasswordWithCost(password, DefaultCost)
}

// HashPasswordWithCost generates a bcrypt hash with a custom cost factor
func HashPasswordWithCost(password string, cost int) (string, error) {
	if len(password) < MinPasswordLength {
		return "", fmt.Errorf("password must be at least %d characters long", MinPasswordLength)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hash), nil
}

// VerifyPassword compares a bcrypt hash with a plain text password
func VerifyPassword(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("failed to verify password: %w", err)
	}
	return nil
}

// ValidatePasswordStrength checks if a password meets minimum requirements
func ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters long", MinPasswordLength)
	}

	// Add more strength requirements as needed
	// - Must contain uppercase
	// - Must contain lowercase
	// - Must contain numbers
	// - Must contain special characters

	return nil
}
