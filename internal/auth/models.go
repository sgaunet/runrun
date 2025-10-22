package auth

import (
	"errors"
	"time"
)

// Common authentication errors
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrTokenExpired       = errors.New("token has expired")
	ErrTokenInvalid       = errors.New("token is invalid")
	ErrUserNotFound       = errors.New("user not found")
	ErrUnauthorized       = errors.New("unauthorized")
)

// User represents an authenticated user
type User struct {
	Username     string
	PasswordHash string
}

// Session represents an active user session
type Session struct {
	Username  string
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// IsExpired checks if the session has expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// Claims represents JWT token claims
type Claims struct {
	Username string `json:"username"`
	IssuedAt int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// Valid checks if claims are valid (not expired)
func (c *Claims) Valid() error {
	now := time.Now().Unix()
	if c.ExpiresAt < now {
		return ErrTokenExpired
	}
	return nil
}
