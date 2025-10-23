package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sgaunet/runrun/internal/auth"
)

// healthCheckHandler handles health check requests
func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"version": "1.0.0",
	})
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
    <link rel="stylesheet" href="/static/css/style.css">
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
    <link rel="stylesheet" href="/static/css/style.css">
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

	// TODO: Implement actual task execution
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Task '%s' execution queued", taskName),
		"execution_id": "placeholder-id",
	})
}

// statusAPIHandler returns the status of all tasks
func (s *Server) statusAPIHandler(w http.ResponseWriter, r *http.Request) {
	// Build task status from config
	tasks := make([]map[string]interface{}, 0, len(s.config.Tasks))
	for _, task := range s.config.Tasks {
		tasks = append(tasks, map[string]interface{}{
			"name":        task.Name,
			"description": task.Description,
			"tags":        task.Tags,
			"status":      "idle", // TODO: Get real status from executor
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": tasks,
	})
}

// viewLogsHandler serves log viewing page
func (s *Server) viewLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>RunRun - Logs %s</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <nav>
        <h1>RunRun</h1>
    </nav>
    <div class="container">
        <h2>Execution Logs: %s</h2>
        <button onclick="copyLogs()" class="btn btn-secondary">Copy</button>
        <a href="/logs/%s/download" class="btn btn-secondary">Download</a>

        <div class="log-container mt-2" id="logs">
            <div class="log-line">Loading logs...</div>
        </div>
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
	`, executionID, executionID, executionID)
}

// downloadLogsHandler handles log file downloads
func (s *Server) downloadLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	// TODO: Implement actual log file serving
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.log\"", executionID))
	fmt.Fprintf(w, "Log file for execution: %s\n", executionID)
	fmt.Fprintf(w, "TODO: Implement actual log retrieval\n")
}

// logWebSocketHandler handles WebSocket connections for live log streaming
func (s *Server) logWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	// TODO: Implement WebSocket connection for live logs
	http.Error(w, fmt.Sprintf("WebSocket not yet implemented for execution: %s", executionID), http.StatusNotImplemented)
}
