package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateToken creates a new JWT token for a user
func (s *Service) GenerateToken(username string) (string, error) {
	now := time.Now()
	expiresAt := now.Add(s.sessionTimeout)

	// Create claims
	claims := &Claims{
		Username:  username,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": claims.Username,
		"iat":      claims.IssuedAt,
		"exp":      claims.ExpiresAt,
	})

	// Sign token with secret
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	// Parse token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		username, _ := claims["username"].(string)
		iat, _ := claims["iat"].(float64)
		exp, _ := claims["exp"].(float64)

		c := &Claims{
			Username:  username,
			IssuedAt:  int64(iat),
			ExpiresAt: int64(exp),
		}

		// Validate claims
		if err := c.Valid(); err != nil {
			return nil, err
		}

		return c, nil
	}

	return nil, ErrTokenInvalid
}

// RefreshToken generates a new token for an existing valid token
func (s *Service) RefreshToken(oldToken string) (string, error) {
	// Validate the old token
	claims, err := s.ValidateToken(oldToken)
	if err != nil {
		return "", err
	}

	// Generate new token
	newToken, err := s.GenerateToken(claims.Username)
	if err != nil {
		return "", err
	}

	// Revoke old session and create new one
	s.RevokeSession(oldToken)
	s.sessionsMutex.Lock()
	s.sessions[newToken] = &Session{
		Username:  claims.Username,
		Token:     newToken,
		ExpiresAt: time.Now().Add(s.sessionTimeout),
		CreatedAt: time.Now(),
	}
	s.sessionsMutex.Unlock()

	return newToken, nil
}
