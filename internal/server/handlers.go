package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/executor"
	"github.com/sgaunet/runrun/internal/templates"
	"github.com/sgaunet/runrun/internal/templates/layouts"
	"github.com/sgaunet/runrun/internal/templates/pages"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Timestamp string            `json:"timestamp"`
	Uptime    string            `json:"uptime,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// healthCheckHandler handles basic health check requests
func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(s.startTime).String(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// readinessHandler handles readiness probe requests
// Returns 200 if the server is ready to accept traffic, 503 otherwise
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	checks := make(map[string]string)
	isReady := true

	// Check if executor is running
	if s.executor != nil {
		checks["executor"] = "ok"
	} else {
		checks["executor"] = "not initialized"
		isReady = false
	}

	// Check if configuration is loaded
	if s.config != nil {
		checks["config"] = "ok"
	} else {
		checks["config"] = "not loaded"
		isReady = false
	}

	// Check if router is set up
	if s.router != nil {
		checks["router"] = "ok"
	} else {
		checks["router"] = "not initialized"
		isReady = false
	}

	response := HealthResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "1.0.0",
		Checks:    checks,
	}

	if isReady {
		response.Status = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		response.Status = "not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// livenessHandler handles liveness probe requests
// Returns 200 if the server is alive and functioning
func (s *Server) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "alive",
		Version:   "1.0.0",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(s.startTime).String(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// dashboardHandler serves the main dashboard page
func (s *Server) dashboardHandler(w http.ResponseWriter, r *http.Request) {
	username := auth.GetUsernameFromContext(r)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>RunRun - Dashboard</title>
    <link rel="stylesheet" href="/static/css/styles.css">
</head>
<body>
    <nav>
        <h1>RunRun</h1>
        <div class="user-info">
            <span>Logged in as %s</span>
            <form action="/logout" method="POST" style="display: inline;">
                <button type="submit" class="btn btn-secondary">Logout</button>
            </form>
        </div>
    </nav>
    <div class="container">
        <h2>Tasks</h2>
        <div class="task-grid" id="taskGrid">
            <!-- Tasks will be loaded here -->
        </div>
    </div>
    <script>
        // Load tasks via API
        fetch('/api/status')
            .then(res => res.json())
            .then(data => {
                const grid = document.getElementById('taskGrid');
                if (data.tasks && data.tasks.length > 0) {
                    grid.innerHTML = data.tasks.map(task => generateTaskCard(task)).join('');
                } else {
                    grid.innerHTML = '<p>No tasks configured</p>';
                }
            })
            .catch(err => console.error('Failed to load tasks:', err));

        function generateTaskCard(task) {
            return '<div class="task-card">' +
                '<div class="task-header">' +
                '<h3 class="task-title">' + task.name + '</h3>' +
                '<span class="status-badge status-' + (task.status || 'idle') + '">' + (task.status || 'idle') + '</span>' +
                '</div>' +
                '<p class="task-description">' + task.description + '</p>' +
                '<div class="task-tags">' +
                (task.tags || []).map(tag => '<span class="tag">' + tag + '</span>').join('') +
                '</div>' +
                '<div class="task-actions">' +
                '<a href="/tasks/' + task.name + '" class="btn btn-secondary">View Details</a>' +
                '<button onclick="runTask(\'' + task.name + '\')" class="btn btn-primary">Run Now</button>' +
                '</div>' +
                '</div>';
        }

        function runTask(taskName) {
            if (!confirm('Run task "' + taskName + '"?')) return;

            fetch('/tasks/' + taskName + '/execute', { method: 'POST' })
                .then(res => res.json())
                .then(data => {
                    alert(data.message || 'Task started');
                    window.location.reload();
                })
                .catch(err => alert('Failed to start task: ' + err));
        }
    </script>
</body>
</html>
	`, username)
}

// taskDetailHandler serves the task detail page
func (s *Server) taskDetailHandler(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "taskName")
	username := auth.GetUsernameFromContext(r)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>RunRun - %s</title>
    <link rel="stylesheet" href="/static/css/styles.css">
</head>
<body>
    <nav>
        <h1>RunRun</h1>
        <div class="user-info">
            <span>Logged in as %s</span>
            <form action="/logout" method="POST" style="display: inline;">
                <button type="submit" class="btn btn-secondary">Logout</button>
            </form>
        </div>
    </nav>
    <div class="container">
        <a href="/">&larr; Back to Dashboard</a>
        <h2>%s</h2>
        <button onclick="runTask()" class="btn btn-primary mt-2">Run Task</button>

        <h3 class="mt-3">Execution History</h3>
        <div id="history">
            <p>No executions yet</p>
        </div>

        <h3 class="mt-3">Live Logs</h3>
        <div class="log-container" id="logs">
            <div class="log-line">Waiting for execution...</div>
        </div>
    </div>
    <script>
        function runTask() {
            if (!confirm('Run task "%s"?')) return;

            fetch('/tasks/%s/execute', { method: 'POST' })
                .then(res => res.json())
                .then(data => {
                    alert(data.message || 'Task started');
                    location.reload();
                })
                .catch(err => alert('Failed to start task: ' + err));
        }
    </script>
</body>
</html>
	`, taskName, username, taskName, taskName, taskName)
}

// executeTaskHandler handles task execution requests
func (s *Server) executeTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "taskName")

	// Find task in config
	var task *config.Task
	for i := range s.config.Tasks {
		if s.config.Tasks[i].Name == taskName {
			task = &s.config.Tasks[i]
			break
		}
	}

	if task == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Task '%s' not found", taskName),
		})
		return
	}

	// Submit task for execution
	executionID, err := s.executor.SubmitTask(task)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to queue task: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("Task '%s' execution queued", taskName),
		"execution_id": executionID,
	})
}

// statusAPIHandler returns the status of all tasks
func (s *Server) statusAPIHandler(w http.ResponseWriter, r *http.Request) {
	// Get statistics from executor
	stats := s.executor.GetStats()

	// Build task status from config with real status
	tasks := make([]map[string]interface{}, 0, len(s.config.Tasks))
	for _, task := range s.config.Tasks {
		// Get latest execution for this task
		latest, err := s.executor.GetLatestExecution(task.Name)

		status := "idle"
		var lastRun interface{}
		var duration interface{}

		if err == nil {
			// Task has been executed at least once
			status = string(latest.Status)
			lastRun = latest.StartedAt

			if latest.FinishedAt != nil {
				duration = latest.Duration.Seconds()
			} else if latest.Status == executor.StatusRunning {
				duration = time.Since(latest.StartedAt).Seconds()
			}
		}

		tasks = append(tasks, map[string]interface{}{
			"name":        task.Name,
			"description": task.Description,
			"tags":        task.Tags,
			"status":      status,
			"last_run":    lastRun,
			"duration":    duration,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": tasks,
		"stats": map[string]interface{}{
			"total":       len(s.config.Tasks),
			"running":     stats.Running,
			"success":     stats.Success,
			"failed":      stats.Failed,
			"queued":      stats.Queued,
			"executions":  stats.Total,
		},
	})
}

// viewLogsHandler serves log viewing page
func (s *Server) viewLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	// Get execution from executor
	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution not found: %v", err), http.StatusNotFound)
		return
	}

	// Read log file if it exists
	var logContent string
	if execution.LogFilePath != "" {
		content, err := executor.ReadLogFile(execution.LogFilePath)
		if err != nil {
			logContent = fmt.Sprintf("Error reading log file: %v", err)
		} else {
			// Escape HTML but preserve formatting
			logContent = html.EscapeString(string(content))
		}
	} else {
		logContent = "Log file not yet created (execution may still be running)"
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>RunRun - Logs %s</title>
    <link rel="stylesheet" href="/static/css/styles.css">
</head>
<body>
    <nav>
        <h1>RunRun</h1>
        <div class="user-info">
            <a href="/" class="btn btn-secondary">Back to Dashboard</a>
        </div>
    </nav>
    <div class="container">
        <h2>Execution Logs</h2>
        <p>Execution ID: %s</p>
        <p>Task: %s</p>
        <p>Status: %s</p>

        <div style="margin-top: 1rem;">
            <button onclick="copyLogs()" class="btn btn-secondary">Copy</button>
            <a href="/logs/%s/download" class="btn btn-secondary">Download</a>
        </div>

        <div class="log-container mt-2" id="logs">%s</div>
    </div>
    <script>
        function copyLogs() {
            const logs = document.getElementById('logs').textContent;
            navigator.clipboard.writeText(logs);
            alert('Logs copied to clipboard');
        }
    </script>
</body>
</html>
	`, executionID, executionID, execution.TaskName, execution.Status, executionID, logContent)
}

// downloadLogsHandler handles log file downloads
func (s *Server) downloadLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	// Get execution from executor
	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution not found: %v", err), http.StatusNotFound)
		return
	}

	// Read log file
	if execution.LogFilePath == "" {
		http.Error(w, "Log file not yet created (execution may still be running)", http.StatusNotFound)
		return
	}

	content, err := executor.ReadLogFile(execution.LogFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read log file: %v", err), http.StatusInternalServerError)
		return
	}

	// Serve file for download
	executionIDShort := executionID
	if len(executionIDShort) > 8 {
		executionIDShort = executionIDShort[:8]
	}
	filename := fmt.Sprintf("%s_%s.log", execution.TaskName, executionIDShort)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Write(content)
}

// pollLogsHandler provides HTTP polling fallback for clients without WebSocket
func (s *Server) pollLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	// Get execution from executor
	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution not found: %v", err), http.StatusNotFound)
		return
	}

	// Determine how many lines to return (default: all, or tail N lines)
	lines := 100 // Default tail lines
	if linesParam := r.URL.Query().Get("lines"); linesParam != "" {
		fmt.Sscanf(linesParam, "%d", &lines)
	}

	var logLines []string
	if execution.LogFilePath != "" {
		// Read log file tail
		tailLines, err := executor.TailLogFile(execution.LogFilePath, lines)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read log file: %v", err), http.StatusInternalServerError)
			return
		}
		logLines = tailLines
	}

	// Return JSON response
	response := map[string]interface{}{
		"execution_id": execution.ID,
		"task_name":    execution.TaskName,
		"status":       execution.Status,
		"started_at":   execution.StartedAt,
		"finished_at":  execution.FinishedAt,
		"logs":         logLines,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	json.NewEncoder(w).Encode(response)
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from same origin
		return true
	},
}

// wsLogsHandler handles WebSocket connections for real-time log streaming
func (s *Server) wsLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	// Manually validate authentication before upgrading to WebSocket
	// The auth middleware's redirect doesn't work for WebSocket upgrades
	token := ""
	cookie, err := r.Cookie("session")
	if err == nil {
		token = cookie.Value
	}

	// If no cookie, try Authorization header
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := []string{}
			for _, part := range []string{authHeader} {
				parts = append(parts, part)
			}
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			}
		}
	}

	// Validate session
	if token == "" {
		http.Error(w, "Unauthorized: no session token", http.StatusUnauthorized)
		return
	}

	_, err = s.authService.ValidateSession(token)
	if err != nil {
		http.Error(w, "Unauthorized: invalid session", http.StatusUnauthorized)
		return
	}

	// Get execution from executor
	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution not found: %v", err), http.StatusNotFound)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// If no log file, send a message and close
	if execution.LogFilePath == "" {
		msg := map[string]interface{}{
			"type": "log",
			"data": map[string]interface{}{
				"line":      "Waiting for execution to start...",
				"timestamp": time.Now().Format(time.RFC3339),
				"level":     "info",
			},
		}
		conn.WriteJSON(msg)
		time.Sleep(2 * time.Second) // Give client time to receive message
		return
	}

	// Open log file
	file, err := os.Open(execution.LogFilePath)
	if err != nil {
		msg := map[string]interface{}{
			"type": "error",
			"data": map[string]interface{}{
				"message": fmt.Sprintf("Failed to open log file: %v", err),
			},
		}
		conn.WriteJSON(msg)
		return
	}
	defer file.Close()

	// Seek to beginning of file to stream all logs
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Printf("Failed to seek log file: %v", err)
		return
	}

	// Create buffered reader for efficient line reading
	reader := bufio.NewReader(file)

	// Channel to signal when to stop
	done := make(chan bool)

	// Start goroutine to listen for client close messages
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(done)
				return
			}
		}
	}()

	// Stream log lines
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Read all available lines
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						// Check if execution is finished
						currentExec, _ := s.executor.GetExecution(executionID)
						if currentExec != nil && currentExec.FinishedAt != nil {
							// Execution finished, send final message and close
							msg := map[string]interface{}{
								"type": "log",
								"data": map[string]interface{}{
									"line":      fmt.Sprintf("\n[Execution finished with status: %s]", currentExec.Status),
									"timestamp": currentExec.FinishedAt.Format(time.RFC3339),
									"level":     "info",
								},
							}
							conn.WriteJSON(msg)
							time.Sleep(500 * time.Millisecond)
							return
						}
						// No more lines available yet, break and wait
						break
					}
					// Other error, close connection
					log.Printf("Error reading log file: %v", err)
					return
				}

				// Send line to client
				msg := map[string]interface{}{
					"type": "log",
					"data": map[string]interface{}{
						"line":      line,
						"timestamp": time.Now().Format(time.RFC3339),
						"level":     "info",
					},
				}

				if err := conn.WriteJSON(msg); err != nil {
					log.Printf("Error writing to WebSocket: %v", err)
					return
				}
			}
		}
	}
}


// Templ-based handlers

// dashboardHandlerTempl serves the dashboard using templ templates
func (s *Server) dashboardHandlerTempl(w http.ResponseWriter, r *http.Request) {
	username := auth.GetUsernameFromContext(r)

	// Get statistics from executor
	stats := s.executor.GetStats()

	// Build task cards from config with real status
	taskCards := make([]templates.TaskCard, 0, len(s.config.Tasks))
	idleCount := 0

	for _, task := range s.config.Tasks {
		// Get latest execution for this task
		latest, err := s.executor.GetLatestExecution(task.Name)

		status := "idle"
		var lastRun *time.Time
		duration := ""

		if err == nil {
			// Task has been executed at least once
			status = string(latest.Status)
			lastRun = &latest.StartedAt

			if latest.FinishedAt != nil {
				duration = latest.Duration.Round(time.Second).String()
			} else if latest.Status == executor.StatusRunning {
				duration = time.Since(latest.StartedAt).Round(time.Second).String()
			}
		} else {
			idleCount++
		}

		card := templates.TaskCard{
			Name:        task.Name,
			Description: task.Description,
			Tags:        task.Tags,
			Status:      status,
			LastRun:     lastRun,
			Duration:    duration,
		}
		taskCards = append(taskCards, card)
	}

	// Build dashboard statistics
	dashboardStats := templates.DashboardStats{
		TotalTasks:      len(s.config.Tasks),
		RunningTasks:    stats.Running,
		SuccessTasks:    stats.Success,
		FailedTasks:     stats.Failed,
		IdleTasks:       idleCount,
		QueuedTasks:     stats.Queued,
		TotalExecutions: stats.Total,
	}

	// Get or generate CSRF token for this session
	csrfToken := s.getCSRFToken(r)

	// Prepare page data
	data := pages.DashboardPageData{
		BaseData: layouts.BaseData{
			Title:       "Dashboard",
			CurrentUser: username,
			CSRFToken:   csrfToken,
		},
		Tasks: taskCards,
		Stats: dashboardStats,
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Dashboard(data).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// taskDetailHandlerTempl serves the task detail page using templ templates
func (s *Server) taskDetailHandlerTempl(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "taskName")
	username := auth.GetUsernameFromContext(r)

	// Find task in config
	var task *config.Task
	for i := range s.config.Tasks {
		if s.config.Tasks[i].Name == taskName {
			task = &s.config.Tasks[i]
			break
		}
	}

	if task == nil {
		http.NotFound(w, r)
		return
	}

	// Get execution history for this task
	executions, err := s.executor.ListExecutions(taskName)
	taskExecutions := make([]templates.ExecutionInfo, 0)
	if err == nil {
		for _, exec := range executions {
			duration := "N/A"
			if exec.FinishedAt != nil {
				duration = exec.Duration.String()
			} else if exec.Status == executor.StatusRunning {
				duration = time.Since(exec.StartedAt).Round(time.Second).String()
			}

			taskExecutions = append(taskExecutions, templates.ExecutionInfo{
				ID:         exec.ID,
				Status:     string(exec.Status),
				StartedAt:  exec.StartedAt,
				FinishedAt: exec.FinishedAt,
				Duration:   duration,
			})
		}
	}

	// Get or generate CSRF token for this session
	csrfToken := s.getCSRFToken(r)

	// Prepare page data
	data := pages.TaskDetailPageData{
		BaseData: layouts.BaseData{
			Title:       task.Name,
			CurrentUser: username,
			CSRFToken:   csrfToken,
		},
		TaskName:    task.Name,
		Description: task.Description,
		Tags:        task.Tags,
		Status:      "idle",
		Executions:  taskExecutions,
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TaskDetail(data).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// loginPageHandlerTempl serves the login page using templ templates
func (s *Server) loginPageHandlerTempl(w http.ResponseWriter, r *http.Request) {
	// Get error from query parameter if present
	errorMsg := r.URL.Query().Get("error")

	data := pages.LoginPageData{
		BaseData: layouts.BaseData{
			Title:       "Login",
			CurrentUser: "",
			CSRFToken:   "",
		},
		Error: errorMsg,
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Login(data).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// viewLogsHandlerTempl serves the logs page using templ templates
func (s *Server) viewLogsHandlerTempl(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")
	username := auth.GetUsernameFromContext(r)

	// Get execution from executor
	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Prepare page data
	data := pages.LogsPageData{
		BaseData: layouts.BaseData{
			Title:       "Logs - " + execution.TaskName,
			CurrentUser: username,
			CSRFToken:   "",
		},
		ExecutionID: executionID,
		TaskName:    execution.TaskName,
		Status:      string(execution.Status),
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Logs(data).Render(r.Context(), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// getCSRFToken retrieves or generates a CSRF token for the current session
func (s *Server) getCSRFToken(r *http.Request) string {
	// Get session cookie
	sessionCookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		// No session cookie, return empty string (shouldn't happen on authenticated routes)
		return ""
	}

	sessionID := sessionCookie.Value

	// Check if token already exists for this session
	existingToken := s.csrf.GetToken(sessionID)
	if existingToken != "" {
		return existingToken
	}

	// Generate new token for this session
	token, err := s.csrf.GenerateToken(sessionID)
	if err != nil {
		log.Printf("Failed to generate CSRF token: %v", err)
		return ""
	}

	return token
}
