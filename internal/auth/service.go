package auth

import (
	"sync"
	"time"
)

// Service handles authentication operations
type Service struct {
	users         map[string]*User // username -> User
	sessions      map[string]*Session // token -> Session
	sessionsMutex sync.RWMutex
	jwtSecret     string
	sessionTimeout time.Duration
}

// NewService creates a new authentication service
func NewService(jwtSecret string, sessionTimeout time.Duration) *Service {
	return &Service{
		users:          make(map[string]*User),
		sessions:       make(map[string]*Session),
		jwtSecret:      jwtSecret,
		sessionTimeout: sessionTimeout,
	}
}

// AddUser adds a user to the service (from configuration)
func (s *Service) AddUser(username, passwordHash string) {
	s.users[username] = &User{
		Username:     username,
		PasswordHash: passwordHash,
	}
}

// GetUser retrieves a user by username
func (s *Service) GetUser(username string) (*User, error) {
	user, exists := s.users[username]
	if !exists {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// Authenticate validates credentials and creates a session
func (s *Service) Authenticate(username, password string) (string, error) {
	// Get user
	user, err := s.GetUser(username)
	if err != nil {
		return "", ErrInvalidCredentials
	}

	// Verify password
	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return "", ErrInvalidCredentials
	}

	// Generate JWT token
	token, err := s.GenerateToken(username)
	if err != nil {
		return "", err
	}

	// Create session
	s.sessionsMutex.Lock()
	s.sessions[token] = &Session{
		Username:  username,
		Token:     token,
		ExpiresAt: time.Now().Add(s.sessionTimeout),
		CreatedAt: time.Now(),
	}
	s.sessionsMutex.Unlock()

	return token, nil
}

// ValidateSession validates a token and returns the username
func (s *Service) ValidateSession(token string) (string, error) {
	// Validate JWT token
	claims, err := s.ValidateToken(token)
	if err != nil {
		return "", err
	}

	// Check session exists and is not expired
	s.sessionsMutex.RLock()
	session, exists := s.sessions[token]
	s.sessionsMutex.RUnlock()

	if !exists {
		return "", ErrTokenInvalid
	}

	if session.IsExpired() {
		s.RevokeSession(token)
		return "", ErrTokenExpired
	}

	return claims.Username, nil
}

// RevokeSession removes a session
func (s *Service) RevokeSession(token string) {
	s.sessionsMutex.Lock()
	delete(s.sessions, token)
	s.sessionsMutex.Unlock()
}

// CleanupExpiredSessions removes expired sessions (should be called periodically)
func (s *Service) CleanupExpiredSessions() {
	s.sessionsMutex.Lock()
	defer s.sessionsMutex.Unlock()

	for token, session := range s.sessions {
		if session.IsExpired() {
			delete(s.sessions, token)
		}
	}
}
