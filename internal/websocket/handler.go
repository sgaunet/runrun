package websocket

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking for production
		return true
	},
}

// Handler handles WebSocket connection upgrades
type Handler struct {
	Hub    *Hub
	Config *Config
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub, config *Config) *Handler {
	return &Handler{
		Hub:    hub,
		Config: config,
	}
}

// ServeWS handles WebSocket requests from clients
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

	log.Printf("WebSocket connection established for execution %s (client: %s)", executionID, clientID)
}
