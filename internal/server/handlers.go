// Package server implements RunRun's HTTP server: request routing, page
// and API handlers, and the WebSocket-based real-time log streaming used
// to monitor task executions.
package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/sgaunet/runrun/internal/auth"
	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/executor"
	mw "github.com/sgaunet/runrun/internal/middleware"
	"github.com/sgaunet/runrun/internal/templates"
	"github.com/sgaunet/runrun/internal/templates/layouts"
	"github.com/sgaunet/runrun/internal/templates/pages"
	ws "github.com/sgaunet/runrun/internal/websocket"
)

// JSON field names shared across the JSON and WebSocket message payloads
// built in this file. Centralized as constants to avoid repeating the same
// string literals throughout the handlers below.
const (
	fieldType        = "type"
	fieldData        = "data"
	fieldExecutionID = "execution_id"
	fieldTimestamp   = "timestamp"
	fieldStatus      = "status"
	fieldSuccess     = "success"
	fieldMessage     = "message"
)

// WebSocket message "type" field values.
const (
	wsMsgTypeLog      = "log"
	wsMsgTypeLogBatch = "log_batch"
	wsMsgTypeError    = "error"
	wsMsgTypeMetadata = "metadata"
	wsMsgTypeComplete = "complete"
)

// appVersion is the version reported by the health-check endpoints.
const appVersion = "1.0.0"

// statusIdle marks a task that has never been executed.
const statusIdle = "idle"

// Tuning and size constants used by the handlers below.
const (
	// shortExecutionIDLen is the number of leading characters of an
	// execution ID used to keep downloaded log filenames short.
	shortExecutionIDLen = 8
	// defaultSegmentCount is the number of log lines returned by the
	// segment endpoint when the caller does not specify a count.
	defaultSegmentCount = 500
	// maxSegmentCount is the largest number of log lines the segment
	// endpoint will return in a single request.
	maxSegmentCount = 5000
	// wsBufferSize is the read/write buffer size (in bytes) used for
	// WebSocket connections.
	wsBufferSize = 1024
	// wsReadLimitBytes caps the size of messages accepted from the client
	// on a real-time log-streaming WebSocket connection.
	wsReadLimitBytes = 512
	// defaultStreamBatchSize is the fallback number of log lines batched
	// into a single WebSocket message when the hub configuration does not
	// specify one.
	defaultStreamBatchSize = 100
	// completeMessageWait is how long wsLogsFromFile waits for the client
	// to acknowledge (or disconnect after) the final "complete" message
	// before the handler returns and the connection is closed.
	completeMessageWait = 5 * time.Second
)

// ErrLogPathOutsideLogDir indicates that an execution's log file path does
// not resolve to a location inside the server's configured log directory.
// Execution log paths are always generated internally by the executor, but
// this is checked defensively before opening the file.
var ErrLogPathOutsideLogDir = errors.New("log file path is outside the configured log directory")

// withRenderNonce returns ctx with the per-request CSP nonce attached
// via templ.WithNonce, so every <script> tag emitted by templ (including
// templ-generated `script` blocks) is rendered with the corresponding
// nonce attribute.
func withRenderNonce(ctx context.Context) context.Context {
	return templ.WithNonce(ctx, mw.NonceFromContext(ctx))
}

// baseData populates the fields common to every page render (title +
// current user + CSRF token + per-request CSP nonce + asset version).
func (s *Server) baseData(r *http.Request, title, currentUser, csrfToken string) layouts.BaseData {
	return layouts.BaseData{
		Title:        title,
		CurrentUser:  currentUser,
		CSRFToken:    csrfToken,
		CSPNonce:     mw.NonceFromContext(r.Context()),
		AssetVersion: s.assetVersion,
	}
}

// writeJSON encodes data as JSON to the response writer, logging any errors.
func writeJSON(w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// writeWSJSON writes a JSON message to a WebSocket connection, logging any errors.
func writeWSJSON(conn *websocket.Conn, v any) {
	if err := conn.WriteJSON(v); err != nil {
		log.Printf("Failed to write WebSocket message: %v", err)
	}
}

// sendLogBatch sends a batch of log lines as a single WebSocket message.
// For single-line batches, sends as a regular "log" message for backward compatibility.
func sendLogBatch(conn *websocket.Conn, executionID string, batch []map[string]any) error {
	if len(batch) == 0 {
		return nil
	}

	if len(batch) == 1 {
		msg := map[string]any{
			fieldType: wsMsgTypeLog,
			fieldData: batch[0],
		}
		if err := conn.WriteJSON(msg); err != nil {
			return fmt.Errorf("write websocket log message: %w", err)
		}
		return nil
	}

	msg := map[string]any{
		fieldType:        wsMsgTypeLogBatch,
		fieldExecutionID: executionID,
		fieldData:        batch,
		fieldTimestamp:   time.Now().Format(time.RFC3339),
	}
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write websocket log batch: %w", err)
	}
	return nil
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Timestamp string            `json:"timestamp"`
	Uptime    string            `json:"uptime,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// healthCheckHandler handles basic health check requests.
func (s *Server) healthCheckHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "healthy",
		Version:   appVersion,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(s.startTime).String(),
	}

	w.WriteHeader(http.StatusOK)
	writeJSON(w, response)
}

// readinessHandler handles readiness probe requests
// Returns 200 if the server is ready to accept traffic, 503 otherwise.
func (s *Server) readinessHandler(w http.ResponseWriter, _ *http.Request) {
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
		Version:   appVersion,
		Checks:    checks,
	}

	if isReady {
		response.Status = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		response.Status = "not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	writeJSON(w, response)
}

// livenessHandler handles liveness probe requests
// Returns 200 if the server is alive and functioning.
func (s *Server) livenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "alive",
		Version:   appVersion,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(s.startTime).String(),
	}

	w.WriteHeader(http.StatusOK)
	writeJSON(w, response)
}

// executeTaskHandler handles task execution requests.
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
		writeJSON(w, map[string]any{
			fieldSuccess: false,
			fieldMessage: fmt.Sprintf("Task '%s' not found", taskName),
		})
		return
	}

	// Submit task for execution
	executionID, err := s.executor.SubmitTask(task)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]any{
			fieldSuccess: false,
			fieldMessage: fmt.Sprintf("Failed to queue task: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		fieldSuccess:     true,
		fieldMessage:     fmt.Sprintf("Task '%s' execution queued", taskName),
		fieldExecutionID: executionID,
	})
}

// statusAPIHandler returns the status of all tasks.
func (s *Server) statusAPIHandler(w http.ResponseWriter, _ *http.Request) {
	// Get statistics from executor
	stats := s.executor.GetStats()

	// Build task status from config with real status
	tasks := make([]map[string]any, 0, len(s.config.Tasks))
	for _, task := range s.config.Tasks {
		// Get latest execution for this task
		latest, err := s.executor.GetLatestExecution(task.Name)

		status := statusIdle
		var lastRun any
		var duration any

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

		tasks = append(tasks, map[string]any{
			"name":        task.Name,
			"description": task.Description,
			"tags":        task.Tags,
			fieldStatus:   status,
			"last_run":    lastRun,
			"duration":    duration,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"tasks": tasks,
		"stats": map[string]any{
			"total":      len(s.config.Tasks),
			"running":    stats.Running,
			fieldSuccess: stats.Success,
			"failed":     stats.Failed,
			"queued":     stats.Queued,
			"executions": stats.Total,
		},
	})
}

// shortExecutionID truncates an execution ID to shortExecutionIDLen
// characters, used to keep downloaded log filenames short and readable.
func shortExecutionID(executionID string) string {
	if len(executionID) > shortExecutionIDLen {
		return executionID[:shortExecutionIDLen]
	}
	return executionID
}

// downloadLogsHandler handles log file downloads.
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

	// Serve file for download. http.ServeContent (rather than a direct
	// w.Write) is used so the response is built from a named file with an
	// explicit, already-set Content-Type instead of writing raw bytes to
	// the ResponseWriter directly; it also gives us Range support for free.
	filename := fmt.Sprintf("%s_%s.log", execution.TaskName, shortExecutionID(executionID))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(content))
}

// pollLogsHandler provides HTTP polling fallback for clients without WebSocket.
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
		_, _ = fmt.Sscanf(linesParam, "%d", &lines)
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
	response := map[string]any{
		fieldExecutionID: execution.ID,
		"task_name":      execution.TaskName,
		fieldStatus:      execution.Status,
		"started_at":     execution.StartedAt,
		"finished_at":    execution.FinishedAt,
		"logs":           logLines,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	writeJSON(w, response)
}

// parseSegmentParams extracts and normalizes the start/count/mode query
// parameters accepted by segmentLogsHandler.
func parseSegmentParams(r *http.Request) (int, int, string) {
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	mode := r.URL.Query().Get("mode")

	if count <= 0 {
		count = defaultSegmentCount
	}
	if count > maxSegmentCount {
		count = maxSegmentCount
	}
	if start < 0 {
		start = 0
	}
	if mode == "" {
		mode = "head"
	}

	return start, count, mode
}

// segmentLineEntry is a single log line returned by segmentLogsHandler,
// annotated with its detected level and absolute line number.
type segmentLineEntry struct {
	Line   string `json:"line"`
	Level  string `json:"level"`
	Number int    `json:"number"`
}

// buildSegmentLineEntries annotates each log line with its detected level
// and its absolute line number (start-relative).
func buildSegmentLineEntries(lines []string, start int) []segmentLineEntry {
	entries := make([]segmentLineEntry, len(lines))
	for i, line := range lines {
		entries[i] = segmentLineEntry{
			Line:   line,
			Level:  ws.ParseLogLevel(line),
			Number: start + i,
		}
	}
	return entries
}

// segmentLogsHandler returns a paginated segment of log lines for a completed execution.
// Query params: start (0-indexed line, default 0), count (lines to return, default 500, max 5000),
// mode ("head" from start, "tail" from end).
func (s *Server) segmentLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution not found: %v", err), http.StatusNotFound)
		return
	}

	if execution.LogFilePath == "" {
		http.Error(w, "Log file not available", http.StatusNotFound)
		return
	}

	start, count, mode := parseSegmentParams(r)

	// For tail mode, calculate start from end
	if mode == "tail" {
		totalLines, err := executor.CountFileLines(execution.LogFilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to count lines: %v", err), http.StatusInternalServerError)
			return
		}
		start = max(totalLines-count, 0)
	}

	lines, totalLines, err := executor.ReadLogSegment(execution.LogFilePath, start, count)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read log segment: %v", err), http.StatusInternalServerError)
		return
	}

	responseLines := buildSegmentLineEntries(lines, start)
	hasMore := start+len(lines) < totalLines

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	writeJSON(w, map[string]any{
		fieldExecutionID: executionID,
		"task_name":      execution.TaskName,
		fieldStatus:      execution.Status,
		"lines":          responseLines,
		"total_lines":    totalLines,
		"start":          start,
		"count":          len(lines),
		"has_more":       hasMore,
	})
}

// upgrader configures the WebSocket protocol upgrade from HTTP
//
// SECURITY: Origin validation (CheckOrigin) prevents Cross-Site WebSocket Hijacking (CSWSH) attacks
// where malicious websites could establish WebSocket connections to this server using
// the victim's authenticated session cookies.
//
// Security Policy:
// - Browser connections: MUST come from same origin (same host)
// - Non-browser clients (CLI, scripts): Allowed (no Origin header)
// - Different scheme (http vs https) on same host: Allowed
//
// See docs/websocket-authentication.md for complete security documentation.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  wsBufferSize,
	WriteBufferSize: wsBufferSize,

	// CheckOrigin validates the Origin header to enforce same-origin policy
	// This is a critical security control for browser-based WebSocket connections
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		// Allow requests without Origin header
		// - Non-browser clients (curl, scripts, CLI tools) don't send Origin
		// - These clients still require authentication (JWT token)
		if origin == "" {
			return true
		}

		// Parse and validate Origin header format
		originURL, err := url.Parse(origin)
		if err != nil {
			log.Printf("Invalid origin URL: %s - %v", origin, err)
			return false
		}

		// Enforce same-origin policy: origin host must match request host
		// Note: Scheme (http vs https) is ignored - only host is checked
		// This allows development with http and production with https
		expectedHost := r.Host
		if originURL.Host == expectedHost {
			return true
		}

		// Reject and log cross-origin attempts for security monitoring
		log.Printf("WebSocket origin rejected: %s (expected: %s)", originURL.Host, expectedHost)
		return false
	},
}

// wsAuthToken extracts the session token from a WebSocket upgrade request:
// it prefers the session cookie (browser clients) and falls back to the
// "Authorization: Bearer <token>" header (CLI/programmatic clients).
func wsAuthToken(r *http.Request) string {
	if cookie, err := r.Cookie("session"); err == nil {
		return cookie.Value
	}

	const bearerPrefix = "Bearer "
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > len(bearerPrefix) && strings.HasPrefix(authHeader, bearerPrefix) {
		return strings.TrimPrefix(authHeader, bearerPrefix)
	}

	return ""
}

// authenticateWebSocket validates the caller's session token for a
// WebSocket log-streaming request. On failure it writes the appropriate
// HTTP error response itself and returns ok=false; the caller must not
// proceed with the upgrade in that case.
//
// This manual authentication step is required because wsLogsHandler sits
// outside the normal auth middleware chain (see the handler's doc comment).
func (s *Server) authenticateWebSocket(w http.ResponseWriter, r *http.Request, executionID string) bool {
	token := wsAuthToken(r)
	if token == "" {
		log.Printf("WebSocket auth failed for %s: no session token", strconv.Quote(executionID))
		http.Error(w, "Unauthorized: no session token", http.StatusUnauthorized)
		return false
	}

	username, err := s.authService.ValidateSession(token)
	if err != nil {
		log.Printf("WebSocket auth failed for %s: invalid session - %v", strconv.Quote(executionID), err)
		http.Error(w, "Unauthorized: invalid session", http.StatusUnauthorized)
		return false
	}

	log.Printf("WebSocket connection authorized for user %s viewing execution %s",
		strconv.Quote(username), strconv.Quote(executionID))
	return true
}

// wsLogsHandler handles WebSocket connections for real-time log streaming
// wsLogsHandler streams execution logs via WebSocket connection
//
// SECURITY NOTE: This handler is intentionally placed OUTSIDE the middleware chain
// because WebSocket upgrades require the http.Hijacker interface, which can be broken
// by middleware that wraps the ResponseWriter (e.g., compression middleware).
// Therefore, authentication MUST be performed manually within this handler.
//
// Authentication Flow:
// 1. Extract JWT token from session cookie (preferred) or Authorization header (fallback)
// 2. Validate token is present (return 401 if missing)
// 3. Validate JWT signature and check session exists (return 401 if invalid)
// 4. Verify execution exists (return 404 if not found)
// 5. Upgrade connection and stream logs
//
// Supported authentication methods (checked in order):
//   - Session cookie: Cookie: session=<jwt-token>
//   - Authorization header: Authorization: Bearer <jwt-token>
//
// See docs/websocket-authentication.md for complete documentation.
func (s *Server) wsLogsHandler(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")

	if ok := s.authenticateWebSocket(w, r, executionID); !ok {
		return
	}

	// === AUTHORIZATION PHASE ===
	// Verify the requested execution exists
	execution, err := s.executor.GetExecution(executionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution not found: %v", err), http.StatusNotFound)
		return
	}

	// === WEBSOCKET UPGRADE ===
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Printf("Failed to close WebSocket connection: %v", cerr)
		}
	}()

	// Parse optional level filter from query parameters (?level=error,warn)
	levelFilter := ws.ParseLevelFilter(r.URL.Query().Get("level"))

	// Determine mode: running executions get real-time streaming via Hub,
	// completed executions get log file streaming
	isRunning := execution.FinishedAt == nil

	if isRunning {
		// Real-time mode: register client with Hub and subscribe to execution
		s.wsLogsRealtime(conn, r, executionID, levelFilter)
	} else {
		// Completed mode: stream from log file
		s.wsLogsFromFile(conn, r, execution, executionID, levelFilter)
	}
}

// wsLogsRealtime handles WebSocket connections for running executions.
// It registers the client with the Hub so it receives real-time broadcasts from the executor.
func (s *Server) wsLogsRealtime(conn *websocket.Conn, r *http.Request, executionID string, levelFilter map[string]bool) {
	// Create a Hub client for this connection
	client := s.wsHub.RegisterClient(conn)

	// Apply server-side level filter if specified
	if levelFilter != nil {
		client.SetLevelFilter(levelFilter)
	}

	// Subscribe to this execution's log stream
	s.wsHub.Subscribe(client, executionID)

	// Ensure cleanup on exit
	defer func() {
		s.wsHub.Unsubscribe(client, executionID)
		s.wsHub.UnregisterClient(client)
	}()

	// Read pump: listen for client messages (close, pong, etc.)
	// This blocks until the client disconnects or an error occurs
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(wsReadLimitBytes)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Write pump: forward messages from Hub's send channel to the WebSocket
	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				// Hub closed the channel
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// openExecutionLogFile opens an execution's log file for reading after
// confirming the resolved path stays within the server's configured log
// directory. Execution log paths are always generated internally by the
// executor from a UUID and the configured task name (never taken directly
// from client input), but this check guards against path traversal
// defensively should that ever change.
func (s *Server) openExecutionLogFile(path string) (*os.File, error) {
	logDir, err := filepath.Abs(s.config.Server.LogDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve log directory: %w", err)
	}

	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("resolve log file path: %w", err)
	}

	rel, err := filepath.Rel(logDir, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%w: %s", ErrLogPathOutsideLogDir, path)
	}

	//nolint:gosec // G304: absPath was cleaned, resolved to absolute, and just confirmed above to reside inside s.config.Server.LogDirectory.
	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return file, nil
}

// sendLogFileComplete sends the "complete" message that signals to the
// client that no further log data will be sent for this execution.
func sendLogFileComplete(conn *websocket.Conn, executionID string, execution *executor.Execution) {
	writeWSJSON(conn, map[string]any{
		fieldType:        wsMsgTypeComplete,
		fieldExecutionID: executionID,
		fieldData:        map[string]string{fieldStatus: string(execution.Status)},
		fieldTimestamp:   time.Now().Format(time.RFC3339),
	})
}

// sendMissingLogFileMessage informs the client that a completed execution
// has no log file available, then sends the closing "complete" message.
func sendMissingLogFileMessage(conn *websocket.Conn, executionID string, execution *executor.Execution) {
	writeWSJSON(conn, map[string]any{
		fieldType: wsMsgTypeLog,
		fieldData: map[string]any{
			"line": fmt.Sprintf("[Execution completed with status: %s]\n[Log file not available]",
				execution.Status),
			fieldTimestamp: time.Now().Format(time.RFC3339),
			"level":        "info",
		},
	})

	sendLogFileComplete(conn, executionID, execution)
}

// sendLogFileMetadata sends the total line count for the log file about to
// be streamed, if it was successfully determined.
func sendLogFileMetadata(conn *websocket.Conn, executionID string, totalLines int) {
	writeWSJSON(conn, map[string]any{
		fieldType:        wsMsgTypeMetadata,
		fieldExecutionID: executionID,
		fieldData: map[string]any{
			"total_lines": totalLines,
		},
		fieldTimestamp: time.Now().Format(time.RFC3339),
	})
}

// buildLogFileLineMessage builds the WebSocket payload for a single
// streamed log line. ok is false if the line should be skipped, either
// because it precedes startLine or because it fails the level filter.
func buildLogFileLineMessage(line string, lineNum, startLine int, levelFilter map[string]bool) (map[string]any, bool) {
	if lineNum <= startLine {
		return nil, false
	}

	level := ws.ParseLogLevel(line)
	if !ws.MatchesFilter(level, levelFilter) {
		return nil, false
	}

	return map[string]any{
		"line":         line,
		fieldTimestamp: time.Now().Format(time.RFC3339),
		"level":        level,
	}, true
}

// streamLogFileLines reads execution log lines from file in batches and
// forwards them to the WebSocket connection, applying levelFilter and
// skipping lines up to startLine. It returns false if a write to the
// connection failed, signalling the caller to stop without further writes.
func streamLogFileLines(
	conn *websocket.Conn, file *os.File, executionID string, startLine, batchSize int, levelFilter map[string]bool,
) bool {
	reader := bufio.NewReader(file)
	lineNum := 0
	batch := make([]map[string]any, 0, batchSize)

	// flush sends any pending batched lines, clearing the batch. It
	// reports false only if the send itself failed.
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		err := sendLogBatch(conn, executionID, batch)
		batch = batch[:0]
		return err == nil
	}

	for {
		line, readErr := reader.ReadString('\n')

		if line != "" {
			lineNum++
			if msg, ok := buildLogFileLineMessage(line, lineNum, startLine, levelFilter); ok {
				batch = append(batch, msg)
				if len(batch) >= batchSize && !flush() {
					return false
				}
			}
		}

		if readErr != nil {
			break
		}
	}

	return flush()
}

// watchForDisconnect returns a channel that is closed once the client
// either disconnects or sends any message on the connection (clients
// streaming logs from a completed execution aren't expected to send any).
func watchForDisconnect(conn *websocket.Conn) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return done
}

// wsLogsFromFile streams log content from a completed execution's log file.
func (s *Server) wsLogsFromFile(
	conn *websocket.Conn, r *http.Request, execution *executor.Execution, executionID string, levelFilter map[string]bool,
) {
	if execution.LogFilePath == "" {
		sendMissingLogFileMessage(conn, executionID, execution)
		return
	}

	// Count total lines first for metadata
	totalLines, countErr := executor.CountFileLines(execution.LogFilePath)

	// Open and stream the log file
	file, err := s.openExecutionLogFile(execution.LogFilePath)
	if err != nil {
		writeWSJSON(conn, map[string]any{
			fieldType: wsMsgTypeError,
			fieldData: map[string]any{
				fieldMessage: fmt.Sprintf("Failed to open log file: %v", err),
			},
		})
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Printf("Failed to close log file: %v", cerr)
		}
	}()

	// Send metadata with total line count if available
	if countErr == nil {
		sendLogFileMetadata(conn, executionID, totalLines)
	}

	// Channel to detect client disconnect
	done := watchForDisconnect(conn)

	// Parse optional start_line query param to skip N lines
	startLine, _ := strconv.Atoi(r.URL.Query().Get("start_line"))

	// Stream lines from the log file in batches for efficiency
	batchSize := s.wsHub.GetConfig().FileStreamBatchSize
	if batchSize <= 0 {
		batchSize = defaultStreamBatchSize
	}

	if !streamLogFileLines(conn, file, executionID, startLine, batchSize, levelFilter) {
		return
	}

	sendLogFileComplete(conn, executionID, execution)

	// Wait briefly for client to receive the complete message before closing
	select {
	case <-done:
	case <-time.After(completeMessageWait):
	case <-r.Context().Done():
	}
}

// Templ-based handlers

// buildTaskCards constructs the dashboard's task cards from configuration
// and each task's most recent execution, along with a count of tasks that
// have no execution history yet.
func buildTaskCards(cfg *config.Config, exec *executor.TaskExecutor) ([]templates.TaskCard, int) {
	cards := make([]templates.TaskCard, 0, len(cfg.Tasks))
	idleCount := 0

	for _, task := range cfg.Tasks {
		latest, err := exec.GetLatestExecution(task.Name)

		status := statusIdle
		var lastRun *time.Time
		duration := ""

		if err == nil {
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

		cards = append(cards, templates.TaskCard{
			Name:        task.Name,
			Description: task.Description,
			Tags:        task.Tags,
			Status:      status,
			LastRun:     lastRun,
			Duration:    duration,
		})
	}

	return cards, idleCount
}

// dashboardHandlerTempl serves the dashboard using templ templates.
func (s *Server) dashboardHandlerTempl(w http.ResponseWriter, r *http.Request) {
	username := auth.GetUsernameFromContext(r)

	// Get statistics from executor
	stats := s.executor.GetStats()

	taskCards, idleCount := buildTaskCards(s.config, s.executor)

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
		BaseData: s.baseData(r, "Dashboard", username, csrfToken),
		Tasks:    taskCards,
		Stats:    dashboardStats,
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Dashboard(data).Render(withRenderNonce(r.Context()), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// taskDetailHandlerTempl serves the task detail page using templ templates.
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
		BaseData:    s.baseData(r, task.Name, username, csrfToken),
		TaskName:    task.Name,
		Description: task.Description,
		Tags:        task.Tags,
		Status:      statusIdle,
		Executions:  taskExecutions,
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TaskDetail(data).Render(withRenderNonce(r.Context()), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// loginPageHandlerTempl serves the login page using templ templates.
func (s *Server) loginPageHandlerTempl(w http.ResponseWriter, r *http.Request) {
	// Get error from query parameter if present
	errorMsg := r.URL.Query().Get("error")

	data := pages.LoginPageData{
		BaseData: s.baseData(r, "Login", "", ""),
		Error:    errorMsg,
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Login(data).Render(withRenderNonce(r.Context()), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// viewLogsHandlerTempl serves the logs page using templ templates.
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
		BaseData:    s.baseData(r, "Logs - "+execution.TaskName, username, ""),
		ExecutionID: executionID,
		TaskName:    execution.TaskName,
		Status:      string(execution.Status),
	}

	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Logs(data).Render(withRenderNonce(r.Context()), w); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// getCSRFToken retrieves or generates a CSRF token for the current session.
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
