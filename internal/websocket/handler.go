package websocket

import (
	"log"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// CheckOrigin enforces a same-origin policy for the WebSocket handshake,
	// which prevents Cross-Site WebSocket Hijacking (CSWSH). Non-browser
	// clients (no Origin header) are still allowed through; they must
	// authenticate the same way browser clients do.
	CheckOrigin: checkOrigin,
}

// checkOrigin reports whether a WebSocket upgrade request's Origin header
// (when present) matches the request's own host. It mirrors the origin
// check used by the server's primary WebSocket handler; see
// docs/websocket-authentication.md for the full security rationale.
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients (curl, scripts, CLI tools) don't send Origin.
		return true
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		log.Printf("Invalid origin URL: %s - %s", sanitizeLogValue(origin), sanitizeLogValue(err))
		return false
	}

	if originURL.Host == r.Host {
		return true
	}

	log.Printf("WebSocket origin rejected: %s (expected: %s)", sanitizeLogValue(originURL.Host), sanitizeLogValue(r.Host))
	return false
}

// Handler handles WebSocket connection upgrades.
type Handler struct {
	Hub    *Hub
	Config *Config
}

// NewHandler creates a new WebSocket handler.
func NewHandler(hub *Hub, config *Config) *Handler {
	return &Handler{
		Hub:    hub,
		Config: config,
	}
}

// ServeWS handles WebSocket requests from clients.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	executionID := chi.URLParam(r, "executionID")
	if executionID == "" {
		http.Error(w, "Execution ID is required", http.StatusBadRequest)
		return
	}

	// Check connection limit before upgrading
	if h.Hub.ConnectionLimitReached(executionID) {
		http.Error(w, "Too many connections for this execution", http.StatusTooManyRequests)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Create client
	clientID := uuid.New().String()
	client := NewClient(h.Hub, conn, clientID, h.Config)

	// Register client with hub
	h.Hub.Register <- client

	// Auto-subscribe to the execution ID from URL
	h.Hub.Subscribe(client, executionID)
	client.sendSubscribed(executionID)

	// Start client goroutines
	go client.WritePump(h.Config)
	go client.ReadPump(h.Config)

	log.Printf("WebSocket connection established for execution %s (client: %s)",
		sanitizeLogValue(executionID), sanitizeLogValue(clientID))
}
