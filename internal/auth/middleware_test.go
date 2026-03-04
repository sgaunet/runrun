package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Test handler that checks if username is in context
func testHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := GetUsernameFromContext(r)
		if username != "" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Authenticated: " + username))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Not authenticated"))
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	service := setupTestAuthService(t)

	// Create a valid token
	validToken, err := service.Authenticate("testuser", "testPassword123")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	tests := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedStatus int
		expectedBody   string
		expectRedirect bool
	}{
		{
			name: "Valid cookie auth",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: validToken,
				})
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "Authenticated: testuser",
			expectRedirect: false,
		},
		{
			name: "Valid bearer token auth",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+validToken)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "Authenticated: testuser",
			expectRedirect: false,
		},
		{
			name: "Cookie takes precedence over header",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: validToken,
				})
				r.Header.Set("Authorization", "Bearer invalid-token")
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "Authenticated: testuser",
			expectRedirect: false,
		},
		{
			name:           "No authentication",
			setupRequest:   func(r *http.Request) {},
			expectedStatus: http.StatusSeeOther,
			expectRedirect: true,
		},
		{
			name: "Invalid cookie token",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: "invalid-token",
				})
			},
			expectedStatus: http.StatusSeeOther,
			expectRedirect: true,
		},
		{
			name: "Invalid bearer token",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer invalid-token")
			},
			expectedStatus: http.StatusSeeOther,
			expectRedirect: true,
		},
		{
			name: "Malformed authorization header",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "InvalidFormat")
			},
			expectedStatus: http.StatusSeeOther,
			expectRedirect: true,
		},
		{
			name: "Authorization header without Bearer",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", validToken)
			},
			expectedStatus: http.StatusSeeOther,
			expectRedirect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/protected", nil)
			tt.setupRequest(req)

			rr := httptest.NewRecorder()

			// Wrap test handler with middleware
			handler := service.AuthMiddleware(testHandler())
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectRedirect {
				location := rr.Header().Get("Location")
				if location != "/login" {
					t.Errorf("Expected redirect to /login, got %s", location)
				}
			} else if tt.expectedBody != "" {
				body := rr.Body.String()
				if body != tt.expectedBody {
					t.Errorf("Expected body %q, got %q", tt.expectedBody, body)
				}
			}
		})
	}
}

func TestOptionalAuthMiddleware(t *testing.T) {
	service := setupTestAuthService(t)

	// Create a valid token
	validToken, err := service.Authenticate("testuser", "testPassword123")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	tests := []struct {
		name         string
		setupRequest func(*http.Request)
		expectedBody string
	}{
		{
			name: "Valid cookie auth",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: validToken,
				})
			},
			expectedBody: "Authenticated: testuser",
		},
		{
			name: "Valid bearer token auth",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+validToken)
			},
			expectedBody: "Authenticated: testuser",
		},
		{
			name:         "No authentication - should still allow access",
			setupRequest: func(r *http.Request) {},
			expectedBody: "Not authenticated",
		},
		{
			name: "Invalid token - should still allow access",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: "invalid-token",
				})
			},
			expectedBody: "Not authenticated",
		},
		{
			name: "Malformed bearer token - should still allow access",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Malformed")
			},
			expectedBody: "Not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/optional", nil)
			tt.setupRequest(req)

			rr := httptest.NewRecorder()

			// Wrap test handler with optional middleware
			handler := service.OptionalAuthMiddleware(testHandler())
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
			}

			body := rr.Body.String()
			if body != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, body)
			}
		})
	}
}

func TestGetUsernameFromContext(t *testing.T) {
	service := setupTestAuthService(t)

	// Create a valid token
	validToken, err := service.Authenticate("testuser", "testPassword123")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	tests := []struct {
		name             string
		setupRequest     func(*http.Request)
		expectedUsername string
	}{
		{
			name: "Valid authentication - username in context",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: validToken,
				})
			},
			expectedUsername: "testuser",
		},
		{
			name:             "No authentication - no username",
			setupRequest:     func(r *http.Request) {},
			expectedUsername: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			tt.setupRequest(req)

			rr := httptest.NewRecorder()

			// Wrap handler that uses GetUsernameFromContext
			handler := service.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username := GetUsernameFromContext(r)
				if username != tt.expectedUsername {
					t.Errorf("Expected username %q, got %q", tt.expectedUsername, username)
				}
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			// AuthMiddleware redirects if no auth, so check for that
			if tt.expectedUsername == "" && rr.Code != http.StatusSeeOther {
				t.Errorf("Expected redirect for unauthenticated request")
			}
		})
	}
}

func TestGetUsernameFromContextDirect(t *testing.T) {
	// Test GetUsernameFromContext without middleware
	req := httptest.NewRequest("GET", "/test", nil)

	// No username in context
	username := GetUsernameFromContext(req)
	if username != "" {
		t.Errorf("Expected empty username, got %q", username)
	}

	// Add username to context
	ctx := req.Context()
	ctx = context.WithValue(ctx, UserContextKey, "directuser")
	req = req.WithContext(ctx)

	username = GetUsernameFromContext(req)
	if username != "directuser" {
		t.Errorf("Expected username %q, got %q", "directuser", username)
	}
}
