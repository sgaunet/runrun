package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/executor"
)

// setupTestServer creates a test server with authentication
func setupTestServer(t *testing.T) (*Server, *auth.Service) {
	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:               8080,
			LogLevel:           "info",
			MaxConcurrentTasks: 5,
			SessionTimeout:     24 * time.Hour,
			LogDirectory:       t.TempDir(),
			ShutdownTimeout:    5 * time.Minute,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret-key-for-jwt-signing-at-least-32-characters-long",
			Users:     []config.User{},
		},
		Tasks: []config.Task{},
	}

	// Create auth service
	authService := auth.NewService(cfg.Auth.JWTSecret, cfg.Server.SessionTimeout)
	authService.AddUser("testuser", "$2a$10$test") // Add test user

	// Create executor
	exec := executor.NewTaskExecutor(
		cfg.Server.MaxConcurrentTasks,
		cfg.Server.LogDirectory,
		cfg.Server.ShutdownTimeout,
	)

	// Create server
	server := &Server{
		config:      cfg,
		authService: authService,
		executor:    exec,
		router:      chi.NewRouter(),
	}

	return server, authService
}

func TestWebSocketAuth_ValidSessionCookie(t *testing.T) {
	server, authService := setupTestServer(t)

	// Create valid session
	token, err := authService.GenerateToken("testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	authService.CreateSessionForTesting(token, "testuser")

	// Create test execution
	executionID := "test-execution-id"
	exec := &executor.Execution{
		ID:          executionID,
		TaskName:    "test-task",
		Status:      executor.StatusRunning,
		StartedAt:   time.Now(),
		LogFilePath: "",
	}
	server.executor.AddTestExecution(executionID, exec) // We'll need to add this helper method

	// Create test request with session cookie
	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/ws/%s", executionID), nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: token,
	})

	// Record response
	w := httptest.NewRecorder()

	// Call handler
	server.wsLogsHandler(w, req)

	// Check response - WebSocket upgrade returns 101 Switching Protocols
	// But in tests without actual WebSocket connection, we won't get 101
	// Instead, we should verify that authentication passed (no 401)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Expected authentication to succeed, got 401 Unauthorized")
	}
}

func TestWebSocketAuth_ValidAuthorizationHeader(t *testing.T) {
	server, authService := setupTestServer(t)

	// Create valid session
	token, err := authService.GenerateToken("testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	authService.CreateSessionForTesting(token, "testuser")

	// Create test execution
	executionID := "test-execution-id"
	exec := &executor.Execution{
		ID:          executionID,
		TaskName:    "test-task",
		Status:      executor.StatusRunning,
		StartedAt:   time.Now(),
		LogFilePath: "",
	}
	server.executor.AddTestExecution(executionID, exec)

	// Create test request with Authorization header
	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/ws/%s", executionID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Record response
	w := httptest.NewRecorder()

	// Call handler
	server.wsLogsHandler(w, req)

	// Check response - should not be 401
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Expected authentication to succeed, got 401 Unauthorized")
	}
}

func TestWebSocketAuth_NoAuthentication(t *testing.T) {
	server, _ := setupTestServer(t)

	// Create test request without any authentication
	req := httptest.NewRequest("GET", "/logs/ws/test-execution-id", nil)
	w := httptest.NewRecorder()

	// Call handler
	server.wsLogsHandler(w, req)

	// Should return 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %d", w.Code)
	}

	// Check error message
	body := w.Body.String()
	if body != "Unauthorized: no session token\n" {
		t.Errorf("Expected 'Unauthorized: no session token', got %q", body)
	}
}

func TestWebSocketAuth_InvalidToken(t *testing.T) {
	server, _ := setupTestServer(t)

	// Create test request with invalid token
	req := httptest.NewRequest("GET", "/logs/ws/test-execution-id", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: "invalid-token-12345",
	})
	w := httptest.NewRecorder()

	// Call handler
	server.wsLogsHandler(w, req)

	// Should return 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %d", w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !contains(body, "Unauthorized: invalid session") {
		t.Errorf("Expected error message about invalid session, got %q", body)
	}
}

func TestWebSocketAuth_MalformedAuthorizationHeader(t *testing.T) {
	server, _ := setupTestServer(t)

	tests := []struct {
		name   string
		header string
	}{
		{
			name:   "Missing Bearer prefix",
			header: "some-token",
		},
		{
			name:   "Wrong auth type",
			header: "Basic some-token",
		},
		{
			name:   "Empty token after Bearer",
			header: "Bearer ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/logs/ws/test-execution-id", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			server.wsLogsHandler(w, req)

			// Should return 401 Unauthorized
			if w.Code != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized, got %d", w.Code)
			}
		})
	}
}

func TestWebSocketAuth_ExpiredToken(t *testing.T) {
	// Create auth service with very short session timeout
	authService := auth.NewService("test-secret-key-for-jwt-signing-at-least-32-characters-long", 1*time.Millisecond)
	authService.AddUser("testuser", "$2a$10$test")

	// Generate token
	token, err := authService.GenerateToken("testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Create server
	server, _ := setupTestServer(t)
	server.authService = authService

	// Create test request with expired token
	req := httptest.NewRequest("GET", "/logs/ws/test-execution-id", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: token,
	})
	w := httptest.NewRecorder()

	// Call handler
	server.wsLogsHandler(w, req)

	// Should return 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for expired token, got %d", w.Code)
	}
}

func TestWebSocketAuth_NonExistentExecution(t *testing.T) {
	server, authService := setupTestServer(t)

	// Create valid session
	token, err := authService.GenerateToken("testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	authService.CreateSessionForTesting(token, "testuser")

	// Create test request for non-existent execution
	req := httptest.NewRequest("GET", "/logs/ws/non-existent-id", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: token,
	})
	w := httptest.NewRecorder()

	// Call handler
	server.wsLogsHandler(w, req)

	// Should return 404 Not Found (after successful authentication)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found for non-existent execution, got %d", w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !contains(body, "Execution not found") {
		t.Errorf("Expected 'Execution not found' error, got %q", body)
	}
}

func TestWebSocketAuth_MultipleAuthMethods(t *testing.T) {
	server, authService := setupTestServer(t)

	// Create valid tokens
	validToken, err := authService.GenerateToken("testuser")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	authService.CreateSessionForTesting(validToken, "testuser")

	// Create test execution
	executionID := "test-execution-id"
	exec := &executor.Execution{
		ID:          executionID,
		TaskName:    "test-task",
		Status:      executor.StatusRunning,
		StartedAt:   time.Now(),
		LogFilePath: "",
	}
	server.executor.AddTestExecution(executionID, exec)

	// Test: Both cookie and header present - cookie should be used first
	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/ws/%s", executionID), nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: validToken,
	})
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	server.wsLogsHandler(w, req)

	// Should succeed because cookie is valid (cookie takes precedence)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Expected authentication to succeed with valid cookie, got 401")
	}
}

func TestWebSocketCheckOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		requestHost    string
		expectedResult bool
	}{
		{
			name:           "Same origin",
			origin:         "http://localhost:8080",
			requestHost:    "localhost:8080",
			expectedResult: true,
		},
		{
			name:           "Different origin",
			origin:         "http://evil.com",
			requestHost:    "localhost:8080",
			expectedResult: false,
		},
		{
			name:           "No origin header",
			origin:         "",
			requestHost:    "localhost:8080",
			expectedResult: true, // Should allow when no origin
		},
		{
			name:           "Same host different scheme",
			origin:         "https://localhost:8080",
			requestHost:    "localhost:8080",
			expectedResult: true, // Host matches, scheme doesn't matter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/logs/ws/test", nil)
			req.Host = tt.requestHost
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			// Get the upgrader from handlers.go
			result := upgrader.CheckOrigin(req)

			if result != tt.expectedResult {
				t.Errorf("CheckOrigin() = %v, expected %v", result, tt.expectedResult)
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
