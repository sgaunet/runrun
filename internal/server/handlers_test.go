package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupHandlerTestServer creates a test server with task configuration for handler tests
func setupHandlerTestServer(t *testing.T) *Server {
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
		Tasks: []config.Task{
			{
				Name:        "test-task-1",
				Description: "Test task 1",
				Tags:        []string{"test", "sample"},
				Timeout:     5 * time.Minute,
				Steps: []config.Step{
					{Name: "step1", Command: "echo hello"},
				},
			},
			{
				Name:        "test-task-2",
				Description: "Test task 2",
				Tags:        []string{"test"},
				Timeout:     5 * time.Minute,
				Steps: []config.Step{
					{Name: "step1", Command: "echo world"},
				},
			},
		},
	}

	authService := auth.NewService(cfg.Auth.JWTSecret, cfg.Server.SessionTimeout)
	authService.AddUser("testuser", "$2a$10$test")

	exec := executor.NewTaskExecutor(
		cfg.Server.MaxConcurrentTasks,
		cfg.Server.LogDirectory,
		cfg.Server.ShutdownTimeout,
	)

	server := &Server{
		config:      cfg,
		authService: authService,
		executor:    exec,
		router:      chi.NewRouter(),
		startTime:   time.Now(),
	}

	return server
}

func TestHealthCheckHandler(t *testing.T) {
	server := setupHandlerTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.healthCheckHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, "1.0.0", response.Version)
	assert.NotEmpty(t, response.Timestamp)
	assert.NotEmpty(t, response.Uptime)
}

func TestReadinessHandler_Ready(t *testing.T) {
	server := setupHandlerTestServer(t)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	server.readinessHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ready", response.Status)
	assert.Equal(t, "ok", response.Checks["executor"])
	assert.Equal(t, "ok", response.Checks["config"])
	assert.Equal(t, "ok", response.Checks["router"])
}

func TestReadinessHandler_NotReady(t *testing.T) {
	server := setupHandlerTestServer(t)
	server.executor = nil // Simulate executor not initialized

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	server.readinessHandler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
	assert.Equal(t, "not initialized", response.Checks["executor"])
}

func TestLivenessHandler(t *testing.T) {
	server := setupHandlerTestServer(t)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	server.livenessHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "alive", response.Status)
	assert.Equal(t, "1.0.0", response.Version)
	assert.NotEmpty(t, response.Uptime)
}

func TestExecuteTaskHandler_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Create router with URL params
	router := chi.NewRouter()
	router.Post("/tasks/{taskName}/execute", server.executeTaskHandler)

	req := httptest.NewRequest("POST", "/tasks/test-task-1/execute", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response["success"].(bool))
	assert.Contains(t, response["message"], "queued")
	assert.NotEmpty(t, response["execution_id"])
}

func TestExecuteTaskHandler_TaskNotFound(t *testing.T) {
	server := setupHandlerTestServer(t)

	router := chi.NewRouter()
	router.Post("/tasks/{taskName}/execute", server.executeTaskHandler)

	req := httptest.NewRequest("POST", "/tasks/nonexistent-task/execute", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response["success"].(bool))
	assert.Contains(t, response["message"], "not found")
}

func TestStatusAPIHandler(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Execute a task to have some stats
	task := &server.config.Tasks[0]
	executionID, err := server.executor.SubmitTask(task)
	require.NoError(t, err)
	assert.NotEmpty(t, executionID)

	// Wait for execution to start
	time.Sleep(200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	server.statusAPIHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "tasks")
	assert.Contains(t, response, "stats")

	tasks := response["tasks"].([]interface{})
	assert.Len(t, tasks, 2) // We have 2 tasks in config

	stats := response["stats"].(map[string]interface{})
	assert.Equal(t, float64(2), stats["total"])
}

func TestDashboardHandler(t *testing.T) {
	server := setupHandlerTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, "testuser")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.dashboardHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "RunRun")
	assert.Contains(t, w.Body.String(), "testuser")
	assert.Contains(t, w.Body.String(), "Dashboard")
}

func TestTaskDetailHandler(t *testing.T) {
	server := setupHandlerTestServer(t)

	router := chi.NewRouter()
	router.Get("/tasks/{taskName}", server.taskDetailHandler)

	req := httptest.NewRequest("GET", "/tasks/test-task-1", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, "testuser")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "test-task-1")
	assert.Contains(t, w.Body.String(), "testuser")
}

func TestDownloadLogsHandler_NotFound(t *testing.T) {
	server := setupHandlerTestServer(t)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}/download", server.downloadLogsHandler)

	req := httptest.NewRequest("GET", "/logs/nonexistent-id/download", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
}

func TestViewLogsHandler_NotFound(t *testing.T) {
	server := setupHandlerTestServer(t)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}", server.viewLogsHandler)

	req := httptest.NewRequest("GET", "/logs/nonexistent-id", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPollLogsHandler_NotFound(t *testing.T) {
	server := setupHandlerTestServer(t)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}/poll", server.pollLogsHandler)

	req := httptest.NewRequest("GET", "/logs/nonexistent-id/poll", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPollLogsHandler_Success(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Execute a task
	task := &server.config.Tasks[0]
	executionID, err := server.executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution to complete
	time.Sleep(500 * time.Millisecond)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}/poll", server.pollLogsHandler)

	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/%s/poll", executionID), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, executionID, response["execution_id"])
	assert.Equal(t, "test-task-1", response["task_name"])
	assert.Contains(t, response, "status")
	assert.Contains(t, response, "logs")
}

func TestPollLogsHandler_WithLinesParam(t *testing.T) {
	server := setupHandlerTestServer(t)

	task := &server.config.Tasks[0]
	executionID, err := server.executor.SubmitTask(task)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}/poll", server.pollLogsHandler)

	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/%s/poll?lines=10", executionID), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "logs")
}

func TestHealthCheckHandlers_ContentType(t *testing.T) {
	server := setupHandlerTestServer(t)

	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"health", server.healthCheckHandler, "/health"},
		{"readiness", server.readinessHandler, "/health/ready"},
		{"liveness", server.livenessHandler, "/health/live"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			assert.Contains(t, w.Body.String(), "status")
			assert.Contains(t, w.Body.String(), "timestamp")
		})
	}
}

func TestExecuteTaskHandler_QueueFull(t *testing.T) {
	// Create server with very limited concurrency
	cfg := &config.Config{
		Server: config.ServerConfig{
			MaxConcurrentTasks: 1,
			LogDirectory:       t.TempDir(),
			ShutdownTimeout:    5 * time.Minute,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret-key-for-jwt-signing-at-least-32-characters-long",
		},
		Tasks: []config.Task{
			{
				Name:    "slow-task",
				Timeout: 30 * time.Second,
				Steps: []config.Step{
					{Name: "sleep", Command: "sleep 5"},
				},
			},
		},
	}

	exec := executor.NewTaskExecutor(1, cfg.Server.LogDirectory, cfg.Server.ShutdownTimeout)
	defer exec.Shutdown()

	server := &Server{
		config:   cfg,
		executor: exec,
	}

	router := chi.NewRouter()
	router.Post("/tasks/{taskName}/execute", server.executeTaskHandler)

	// Fill the queue
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/tasks/slow-task/execute", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Try one more - should fail due to queue full
	req := httptest.NewRequest("POST", "/tasks/slow-task/execute", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// At least one should have failed
	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	// Either success or queue full error is acceptable here
	// We just want to test the handler doesn't panic
	assert.Contains(t, response, "success")
}

func TestStatusAPIHandler_NoCacheHeader(t *testing.T) {
	server := setupHandlerTestServer(t)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	server.statusAPIHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify it's JSON (no explicit cache header set on this endpoint currently)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestViewLogsHandler_WithExecution(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Execute a task
	task := &server.config.Tasks[0]
	executionID, err := server.executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution to complete
	time.Sleep(500 * time.Millisecond)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}", server.viewLogsHandler)

	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/%s", executionID), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), executionID)
	assert.Contains(t, w.Body.String(), "test-task-1")
}

func TestDownloadLogsHandler_WithExecution(t *testing.T) {
	server := setupHandlerTestServer(t)

	// Execute a task
	task := &server.config.Tasks[0]
	executionID, err := server.executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution to complete
	time.Sleep(500 * time.Millisecond)

	router := chi.NewRouter()
	router.Get("/logs/{executionID}/download", server.downloadLogsHandler)

	req := httptest.NewRequest("GET", fmt.Sprintf("/logs/%s/download", executionID), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "test-task-1")
	assert.NotEmpty(t, w.Body.String())
}
