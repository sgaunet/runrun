package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const (
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "session"
)

// LoginRequest represents the login request payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// LoginHandler handles user login requests
func (s *Service) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request based on Content-Type
	var req LoginRequest
	contentType := r.Header.Get("Content-Type")

	if contentType == "application/json" {
		// Parse as JSON
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON request", http.StatusBadRequest)
			return
		}
	} else {
		// Parse as form data (default for browser forms)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form request", http.StatusBadRequest)
			return
		}
		req.Username = r.FormValue("username")
		req.Password = r.FormValue("password")
	}

	// Validate credentials
	token, err := s.Authenticate(req.Username, req.Password)
	if err != nil {
		log.Printf("Login failed for user %s: %v", req.Username, err)

		// Check if this is a browser form submission or API call
		acceptHeader := r.Header.Get("Accept")

		// If JSON was sent or JSON is explicitly requested, return JSON error
		if contentType == "application/json" || acceptHeader == "application/json" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(LoginResponse{
				Success: false,
				Message: "Invalid username or password",
			})
			return
		}

		// Otherwise, redirect back to login page with error (browser form submission)
		http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(s.sessionTimeout),
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("User %s logged in successfully", req.Username)

	// Check if this is a browser form submission or API call
	acceptHeader := r.Header.Get("Accept")

	// If JSON was sent or JSON is explicitly requested, return JSON
	if contentType == "application/json" || acceptHeader == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Message: "Login successful",
		})
		return
	}

	// Otherwise, redirect to dashboard (browser form submission)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// LogoutHandler handles user logout requests
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
		log.Printf("Session revoked for token: %s", cookie.Value[:10]+"...")
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})

	// Check if this is a browser form submission or API call
	acceptHeader := r.Header.Get("Accept")
	contentType := r.Header.Get("Content-Type")

	// If JSON was sent or JSON is explicitly requested, return JSON
	if contentType == "application/json" || acceptHeader == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Message: "Logout successful",
		})
		return
	}

	// Otherwise, redirect to login page (browser form submission)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// LoginPageHandler serves the login page HTML
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
	w.Write([]byte(`
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
	`))
}
