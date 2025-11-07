package csrf

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	protection := New()
	require.NotNil(t, protection)
	assert.NotNil(t, protection.tokens)
}

func TestGenerateToken(t *testing.T) {
	protection := New()

	token1, err := protection.GenerateToken("session1")
	require.NoError(t, err)
	assert.NotEmpty(t, token1)
	assert.Greater(t, len(token1), 20, "Token should be sufficiently long")

	// Generate another token for different session
	token2, err := protection.GenerateToken("session2")
	require.NoError(t, err)
	assert.NotEmpty(t, token2)
	assert.NotEqual(t, token1, token2, "Tokens for different sessions should be different")
}

func TestGetToken(t *testing.T) {
	protection := New()

	// Generate a token
	sessionID := "test-session"
	generatedToken, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	// Retrieve the token
	retrievedToken := protection.GetToken(sessionID)
	assert.Equal(t, generatedToken, retrievedToken)

	// Try to get non-existent token
	nonExistentToken := protection.GetToken("non-existent-session")
	assert.Empty(t, nonExistentToken)
}

func TestValidateToken_ValidToken(t *testing.T) {
	protection := New()

	sessionID := "test-session"
	token, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	// Validate correct token
	valid := protection.ValidateToken(sessionID, token)
	assert.True(t, valid)
}

func TestValidateToken_InvalidToken(t *testing.T) {
	protection := New()

	sessionID := "test-session"
	_, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	// Validate incorrect token
	valid := protection.ValidateToken(sessionID, "wrong-token")
	assert.False(t, valid)
}

func TestValidateToken_NonExistentSession(t *testing.T) {
	protection := New()

	// Validate token for session that doesn't exist
	valid := protection.ValidateToken("non-existent", "some-token")
	assert.False(t, valid)
}

func TestDeleteToken(t *testing.T) {
	protection := New()

	sessionID := "test-session"
	token, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	// Verify token exists
	assert.True(t, protection.ValidateToken(sessionID, token))

	// Delete the token
	protection.DeleteToken(sessionID)

	// Verify token no longer exists
	assert.False(t, protection.ValidateToken(sessionID, token))
	assert.Empty(t, protection.GetToken(sessionID))
}

func TestMiddleware_SafeMethod(t *testing.T) {
	protection := New()

	handler := protection.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	safeMethods := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodTrace,
	}

	for _, method := range safeMethods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "success")
		})
	}
}

func TestMiddleware_NoSessionCookie(t *testing.T) {
	protection := New()

	handler := protection.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestMiddleware_NoCSRFToken(t *testing.T) {
	protection := New()

	handler := protection.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: "test-session-id",
	})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestMiddleware_ValidTokenInHeader(t *testing.T) {
	protection := New()

	sessionID := "test-session"
	token, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	handler := protection.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: sessionID,
	})
	req.Header.Set(TokenHeader, token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

func TestMiddleware_ValidTokenInForm(t *testing.T) {
	protection := New()

	sessionID := "test-session"
	token, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	handler := protection.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	formData := url.Values{}
	formData.Set(TokenFormField, token)

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: sessionID,
	})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

func TestMiddleware_InvalidToken(t *testing.T) {
	protection := New()

	sessionID := "test-session"
	_, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	handler := protection.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: sessionID,
	})
	req.Header.Set(TokenHeader, "invalid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSetTokenCookie(t *testing.T) {
	protection := New()

	w := httptest.NewRecorder()
	sessionID := "test-session"

	err := protection.SetTokenCookie(w, sessionID)
	require.NoError(t, err)

	// Check cookie was set
	cookies := w.Result().Cookies()
	assert.Len(t, cookies, 1)

	cookie := cookies[0]
	assert.Equal(t, TokenCookie, cookie.Name)
	assert.NotEmpty(t, cookie.Value)
	assert.Equal(t, "/", cookie.Path)
	assert.False(t, cookie.HttpOnly, "JavaScript needs to read this")
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)

	// Verify the token was stored
	storedToken := protection.GetToken(sessionID)
	assert.Equal(t, cookie.Value, storedToken)
}

func TestIsSafeMethod(t *testing.T) {
	tests := []struct {
		method string
		safe   bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodOptions, true},
		{http.MethodTrace, true},
		{http.MethodPost, false},
		{http.MethodPut, false},
		{http.MethodPatch, false},
		{http.MethodDelete, false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := isSafeMethod(tt.method)
			assert.Equal(t, tt.safe, result)
		})
	}
}

func TestConcurrentTokenOperations(t *testing.T) {
	protection := New()

	// Test concurrent token generation
	sessions := []string{"session1", "session2", "session3", "session4", "session5"}
	tokens := make(map[string]string)

	// Generate tokens concurrently
	done := make(chan bool, len(sessions))
	for _, sessionID := range sessions {
		go func(sid string) {
			token, err := protection.GenerateToken(sid)
			require.NoError(t, err)
			tokens[sid] = token
			done <- true
		}(sessionID)
	}

	// Wait for all to complete
	for i := 0; i < len(sessions); i++ {
		<-done
	}

	// Verify all tokens are unique and valid
	for _, sessionID := range sessions {
		token := tokens[sessionID]
		assert.True(t, protection.ValidateToken(sessionID, token))
	}

	// Verify all tokens are different
	uniqueTokens := make(map[string]bool)
	for _, token := range tokens {
		uniqueTokens[token] = true
	}
	assert.Len(t, uniqueTokens, len(sessions), "All tokens should be unique")
}

func TestTokenGeneration_Uniqueness(t *testing.T) {
	protection := New()

	// Generate multiple tokens for same session (should overwrite)
	sessionID := "test-session"

	token1, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	token2, err := protection.GenerateToken(sessionID)
	require.NoError(t, err)

	// Tokens should be different each time
	assert.NotEqual(t, token1, token2)

	// Only the latest token should be valid
	assert.False(t, protection.ValidateToken(sessionID, token1), "Old token should be invalid")
	assert.True(t, protection.ValidateToken(sessionID, token2), "New token should be valid")
}
