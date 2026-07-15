package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "session"

	// ContentTypeJSON is the MIME type for JSON content.
	ContentTypeJSON = "application/json"

	// tokenLogPrefixLen is how many leading characters of a session token
	// are safe to include in diagnostic logs. The full token is a bearer
	// credential and must never be logged.
	tokenLogPrefixLen = 10
)

// LoginRequest represents the login request payload.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response.
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// loginPageHTML is the static fallback login page served by LoginPageHandler.
const loginPageHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>RunRun - Login</title>
    <style>
        body { font-family: Arial, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
        .login-box { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); width: 300px; }
        h1 { margin-top: 0; text-align: center; }
        input { width: 100%; padding: 0.5rem; margin: 0.5rem 0; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        button { width: 100%; padding: 0.75rem; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 1rem; }
        button:hover { background: #0056b3; }
        .error { color: red; font-size: 0.9rem; margin-top: 0.5rem; display: none; }
    </style>
</head>
<body>
    <div class="login-box">
        <h1>RunRun</h1>
        <form id="loginForm">
            <input type="text" id="username" name="username" placeholder="Username" required>
            <input type="password" id="password" name="password" placeholder="Password" required>
            <button type="submit">Login</button>
            <div class="error" id="error">Invalid username or password</div>
        </form>
    </div>
    <script>
        document.getElementById('loginForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            const errorDiv = document.getElementById('error');

            try {
                const response = await fetch('/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username, password })
                });

                const data = await response.json();

                if (data.success) {
                    window.location.href = '/';
                } else {
                    errorDiv.style.display = 'block';
                }
            } catch (err) {
                errorDiv.style.display = 'block';
            }
        });
    </script>
</body>
</html>
	`

// LoginHandler handles user login requests.
func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, contentType, errMsg := parseLoginRequest(r)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	token, err := s.Authenticate(req.Username, req.Password)
	if err != nil {
		s.respondLoginFailure(w, r, req.Username, contentType, err)
		return
	}

	s.respondLoginSuccess(w, r, req.Username, contentType, token)
}

// LogoutHandler handles user logout requests.
func (s *Service) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get token from cookie
	cookie, err := r.Cookie(SessionCookieName)
	if err == nil && cookie.Value != "" {
		// Revoke session
		s.RevokeSession(cookie.Value)
		prefix := tokenLogPrefix(cookie.Value)
		log.Printf("Session revoked for token: %s", strconv.Quote(prefix))
	}

	// Clear cookie
	//nolint:gosec // G124: Secure is computed at runtime via isSecureRequest(r); see its doc comment for rationale.
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})

	// Check if this is a browser form submission or API call
	acceptHeader := r.Header.Get("Accept")
	contentType := r.Header.Get("Content-Type")

	// If JSON was sent or JSON is explicitly requested, return JSON
	if contentType == ContentTypeJSON || acceptHeader == ContentTypeJSON {
		w.Header().Set("Content-Type", ContentTypeJSON)
		if err := json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Message: "Logout successful",
		}); err != nil {
			log.Printf("Failed to encode logout response: %v", err)
		}
		return
	}

	// Otherwise, redirect to login page (browser form submission)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// LoginPageHandler serves the login page HTML.
func (s *Service) LoginPageHandler(w http.ResponseWriter, r *http.Request) {
	// Check if already logged in
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if _, err := s.ValidateSession(cookie.Value); err == nil {
			// Already logged in, redirect to dashboard
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	// Serve login page (will be implemented with templ later)
	w.Header().Set("Content-Type", "text/html")
	if _, err := w.Write([]byte(loginPageHTML)); err != nil {
		log.Printf("Failed to write login page response: %v", err)
	}
}

// parseLoginRequest extracts login credentials from the request body,
// decoding JSON when the client sent Content-Type: application/json and
// falling back to an HTML form submission otherwise. errMsg is non-empty
// (and suitable for http.Error) when the body could not be parsed.
func parseLoginRequest(r *http.Request) (LoginRequest, string, string) {
	var req LoginRequest
	contentType := r.Header.Get("Content-Type")

	if contentType == ContentTypeJSON {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, contentType, "Invalid JSON request"
		}
		return req, contentType, ""
	}

	if err := r.ParseForm(); err != nil {
		return req, contentType, "Invalid form request"
	}
	req.Username = r.FormValue("username")
	req.Password = r.FormValue("password")
	return req, contentType, ""
}

// respondLoginFailure logs a failed login attempt and writes the
// appropriate JSON or redirect response depending on how the client asked
// to be answered.
func (s *Service) respondLoginFailure(w http.ResponseWriter, r *http.Request, username, contentType string, authErr error) {
	log.Printf("Login failed for user %s: %v", strconv.Quote(username), authErr)

	// Check if this is a browser form submission or API call
	acceptHeader := r.Header.Get("Accept")

	// If JSON was sent or JSON is explicitly requested, return JSON error
	if contentType == ContentTypeJSON || acceptHeader == ContentTypeJSON {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		}); err != nil {
			log.Printf("Failed to encode login response: %v", err)
		}
		return
	}

	// Otherwise, redirect back to login page with error (browser form submission)
	http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
}

// respondLoginSuccess sets the session cookie, logs the successful login,
// and writes the appropriate JSON or redirect response depending on how
// the client asked to be answered.
func (s *Service) respondLoginSuccess(w http.ResponseWriter, r *http.Request, username, contentType, token string) {
	// Set session cookie
	//nolint:gosec // G124: Secure is computed at runtime via isSecureRequest(r); see its doc comment for rationale.
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(s.sessionTimeout),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("User %s logged in successfully", strconv.Quote(username))

	// Check if this is a browser form submission or API call
	acceptHeader := r.Header.Get("Accept")

	// If JSON was sent or JSON is explicitly requested, return JSON
	if contentType == ContentTypeJSON || acceptHeader == ContentTypeJSON {
		w.Header().Set("Content-Type", ContentTypeJSON)
		if err := json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Message: "Login successful",
		}); err != nil {
			log.Printf("Failed to encode login response: %v", err)
		}
		return
	}

	// Otherwise, redirect to dashboard (browser form submission)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// tokenLogPrefix returns a short, bounded prefix of a session token for
// diagnostic logging. The full token is a bearer credential and must never
// be logged. Session tokens are attacker-influenced input (an arbitrary
// cookie value), so this also guards against slicing past the end of a
// short string.
func tokenLogPrefix(token string) string {
	if len(token) <= tokenLogPrefixLen {
		return token + "..."
	}
	return token[:tokenLogPrefixLen] + "..."
}
