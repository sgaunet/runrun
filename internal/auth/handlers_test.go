package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoginHandler(t *testing.T) {
	service := setupTestAuthService(t)

	tests := []struct {
		name           string
		method         string
		contentType    string
		body           string
		expectedStatus int
		checkCookie    bool
	}{
		{
			name:           "Valid JSON login",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"username":"testuser","password":"testPassword123"}`,
			expectedStatus: http.StatusOK,
			checkCookie:    true,
		},
		{
			name:           "Valid form login",
			method:         "POST",
			contentType:    "application/x-www-form-urlencoded",
			body:           "username=testuser&password=testPassword123",
			expectedStatus: http.StatusSeeOther, // Form login redirects
			checkCookie:    true,
		},
		{
			name:           "Invalid credentials",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"username":"testuser","password":"wrongpassword"}`,
			expectedStatus: http.StatusUnauthorized,
			checkCookie:    false,
		},
		{
			name:           "Wrong method",
			method:         "GET",
			contentType:    "application/json",
			body:           "",
			expectedStatus: http.StatusMethodNotAllowed,
			checkCookie:    false,
		},
		{
			name:           "Invalid JSON",
			method:         "POST",
			contentType:    "application/json",
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
			checkCookie:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.contentType == "application/x-www-form-urlencoded" {
				req = httptest.NewRequest(tt.method, "/login", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/login", bytes.NewBufferString(tt.body))
			}
			req.Header.Set("Content-Type", tt.contentType)

			rr := httptest.NewRecorder()
			service.LoginHandler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.checkCookie {
				cookies := rr.Result().Cookies()
				found := false
				for _, cookie := range cookies {
					if cookie.Name == SessionCookieName {
						found = true
						if cookie.Value == "" {
							t.Error("Session cookie is empty")
						}
						break
					}
				}
				if !found {
					t.Error("Session cookie not set")
				}
			}
		})
	}
}

func TestLogoutHandler(t *testing.T) {
	service := setupTestAuthService(t)

	// Create a valid session
	token, err := service.Authenticate("testuser", "testPassword123")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	tests := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedStatus int
		checkCleared   bool
	}{
		{
			name: "Valid logout",
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: token,
				})
			},
			expectedStatus: http.StatusOK,
			checkCleared:   true,
		},
		{
			name:           "Logout without session",
			setupRequest:   func(r *http.Request) {},
			expectedStatus: http.StatusOK,
			checkCleared:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/logout", nil)
			tt.setupRequest(req)

			rr := httptest.NewRecorder()
			service.LogoutHandler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.checkCleared {
				cookies := rr.Result().Cookies()
				for _, cookie := range cookies {
					if cookie.Name == SessionCookieName {
						if cookie.MaxAge != -1 {
							t.Error("Session cookie MaxAge should be -1 (deleted)")
						}
					}
				}
			}
		})
	}
}

func TestLoginPageHandler(t *testing.T) {
	service := setupTestAuthService(t)

	tests := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedStatus int
		checkRedirect  bool
	}{
		{
			name:           "Not logged in - show login page",
			setupRequest:   func(r *http.Request) {},
			expectedStatus: http.StatusOK,
			checkRedirect:  false,
		},
		{
			name: "Already logged in - redirect to home",
			setupRequest: func(r *http.Request) {
				token, _ := service.Authenticate("testuser", "testPassword123")
				r.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: token,
				})
			},
			expectedStatus: http.StatusSeeOther,
			checkRedirect:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/login", nil)
			tt.setupRequest(req)

			rr := httptest.NewRecorder()
			service.LoginPageHandler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.checkRedirect {
				location := rr.Header().Get("Location")
				if location != "/" {
					t.Errorf("Expected redirect to /, got %s", location)
				}
			}
		})
	}
}

func TestLoginHandlerJSONResponse(t *testing.T) {
	service := setupTestAuthService(t)

	// Test successful login with JSON response
	reqBody := `{"username":"testuser","password":"testPassword123"}`
	req := httptest.NewRequest("POST", "/login", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	service.LoginHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp LoginResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true in JSON response")
	}
}

func TestLogoutHandlerJSONResponse(t *testing.T) {
	service := setupTestAuthService(t)

	// Create a valid session
	token, err := service.Authenticate("testuser", "testPassword123")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: token,
	})
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	service.LogoutHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify session was revoked
	_, err = service.ValidateSession(token)
	if err == nil {
		t.Error("Session should be invalid after logout")
	}
}

func TestLoginHandlerFormURLEncoded(t *testing.T) {
	service := setupTestAuthService(t)

	form := url.Values{}
	form.Set("username", "testuser")
	form.Set("password", "testPassword123")

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	service.LoginHandler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("Expected status %d, got %d", http.StatusSeeOther, rr.Code)
	}

	// Verify redirect to /
	location := rr.Header().Get("Location")
	if location != "/" {
		t.Errorf("Expected redirect to /, got %s", location)
	}

	// Verify session cookie was set
	cookies := rr.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == SessionCookieName && cookie.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Session cookie not set after form login")
	}
}
