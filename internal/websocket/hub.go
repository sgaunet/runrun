package websocket

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// clientIDCounter is used to generate unique client IDs
var clientIDCounter atomic.Uint64

func generateClientID() string {
	return fmt.Sprintf("client-%d", clientIDCounter.Add(1))
}

// NewHub creates a new WebSocket hub
func NewHub(config *Config) *Hub {
	if config == nil {
		config = DefaultConfig()
	}
	return &Hub{
		Clients:             make(map[*Client]bool),
		Subscriptions:       make(map[string]map[*Client]bool),
		Register:            make(chan *Client),
		Unregister:          make(chan *Client),
		Broadcast:           make(chan *BroadcastMessage, 256),
		stop:                make(chan struct{}),
		config:              config,
		executionConnCounts: make(map[string]int),
	}
}

// Run starts the hub's main event loop
func (h *Hub) Run() {
	var idleTicker *time.Ticker
	if h.config.IdleTimeout > 0 {
		idleTicker = time.NewTicker(h.config.IdleTimeout / 2)
		defer idleTicker.Stop()
	} else {
		// Create a stopped ticker so select doesn't panic
		idleTicker = time.NewTicker(time.Hour)
		idleTicker.Stop()
	}

	for {
		select {
		case client := <-h.Register:
			h.registerClient(client)

		case client := <-h.Unregister:
			h.unregisterClient(client)

		case message := <-h.Broadcast:
			h.broadcastMessage(message)

		case <-idleTicker.C:
			h.evictIdleClients()

		case <-h.stop:
			return
		}
	}
}

// registerClient registers a new client
func (h *Hub) registerClient(client *Client) {
	h.ClientsMu.Lock()
	h.Clients[client] = true
	h.ClientsMu.Unlock()

	log.Printf("WebSocket client registered: %s", client.ID)
}

// unregisterClient unregisters a client and cleans up resources
func (h *Hub) unregisterClient(client *Client) {
	h.ClientsMu.Lock()
	if _, ok := h.Clients[client]; ok {
		delete(h.Clients, client)
		close(client.Send)
	}
	h.ClientsMu.Unlock()

	// Remove from all subscriptions and update connection counts
	h.SubscriptionsMu.Lock()
	for executionID := range client.Subscriptions {
		if clients, ok := h.Subscriptions[executionID]; ok {
			delete(clients, client)
			if len(clients) == 0 {
				delete(h.Subscriptions, executionID)
			}
		}
		// Decrement connection count
		h.connCountsMu.Lock()
		if h.executionConnCounts[executionID] > 0 {
			h.executionConnCounts[executionID]--
			if h.executionConnCounts[executionID] == 0 {
				delete(h.executionConnCounts, executionID)
			}
		}
		h.connCountsMu.Unlock()
	}
	h.SubscriptionsMu.Unlock()

	log.Printf("WebSocket client unregistered: %s", client.ID)
}

// broadcastMessage sends a message to all clients subscribed to an execution
func (h *Hub) broadcastMessage(message *BroadcastMessage) {
	h.SubscriptionsMu.RLock()
	clients, ok := h.Subscriptions[message.ExecutionID]
	h.SubscriptionsMu.RUnlock()

	if !ok || len(clients) == 0 {
		return
	}

	// Send to all subscribed clients
	var wg sync.WaitGroup
	for client := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			select {
			case c.Send <- message.Data:
				// Message sent successfully
			default:
				// Client's send channel is full, unregister the client
				log.Printf("Client %s send buffer full, unregistering", c.ID)
				h.Unregister <- c
			}
		}(client)
	}
	wg.Wait()
}

// evictIdleClients removes clients that have been idle beyond the timeout.
// Must be called from within the Run loop (processes unregistration inline).
func (h *Hub) evictIdleClients() {
	if h.config.IdleTimeout <= 0 {
		return
	}

	cutoff := time.Now().Add(-h.config.IdleTimeout)
	var toEvict []*Client

	h.ClientsMu.RLock()
	for client := range h.Clients {
		if client.GetLastActivity().Before(cutoff) {
			toEvict = append(toEvict, client)
		}
	}
	h.ClientsMu.RUnlock()

	for _, client := range toEvict {
		log.Printf("Evicting idle WebSocket client: %s (last activity: %s)", client.ID, client.GetLastActivity().Format(time.RFC3339))
		h.unregisterClient(client)
	}
}

// RegisterClient creates a new client for the given connection and registers it with the Hub
func (h *Hub) RegisterClient(conn *websocket.Conn) *Client {
	client := NewClient(h, conn, generateClientID(), h.config)
	h.Register <- client
	return client
}

// UnregisterClient unregisters a client from the Hub
func (h *Hub) UnregisterClient(client *Client) {
	h.Unregister <- client
}

// Subscribe adds a client to an execution's subscription list
func (h *Hub) Subscribe(client *Client, executionID string) {
	// Add to client's subscription list
	client.SubscribeMu.Lock()
	client.Subscriptions[executionID] = true
	client.SubscribeMu.Unlock()

	// Add to hub's subscription map
	h.SubscriptionsMu.Lock()
	if h.Subscriptions[executionID] == nil {
		h.Subscriptions[executionID] = make(map[*Client]bool)
	}
	h.Subscriptions[executionID][client] = true
	h.SubscriptionsMu.Unlock()

	// Increment connection count
	h.connCountsMu.Lock()
	h.executionConnCounts[executionID]++
	h.connCountsMu.Unlock()

	log.Printf("Client %s subscribed to execution %s", client.ID, executionID)
}

// Unsubscribe removes a client from an execution's subscription list
func (h *Hub) Unsubscribe(client *Client, executionID string) {
	// Remove from client's subscription list
	client.SubscribeMu.Lock()
	delete(client.Subscriptions, executionID)
	client.SubscribeMu.Unlock()

	// Remove from hub's subscription map
	h.SubscriptionsMu.Lock()
	if clients, ok := h.Subscriptions[executionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.Subscriptions, executionID)
		}
	}
	h.SubscriptionsMu.Unlock()

	// Decrement connection count
	h.connCountsMu.Lock()
	if h.executionConnCounts[executionID] > 0 {
		h.executionConnCounts[executionID]--
		if h.executionConnCounts[executionID] == 0 {
			delete(h.executionConnCounts, executionID)
		}
	}
	h.connCountsMu.Unlock()

	log.Printf("Client %s unsubscribed from execution %s", client.ID, executionID)
}

// GetSubscriberCount returns the number of clients subscribed to an execution
func (h *Hub) GetSubscriberCount(executionID string) int {
	h.SubscriptionsMu.RLock()
	defer h.SubscriptionsMu.RUnlock()

	if clients, ok := h.Subscriptions[executionID]; ok {
		return len(clients)
	}
	return 0
}

// ConnectionLimitReached returns true if the execution has reached its max connections
func (h *Hub) ConnectionLimitReached(executionID string) bool {
	if h.config.MaxConnectionsPerExecution <= 0 {
		return false
	}
	h.connCountsMu.RLock()
	defer h.connCountsMu.RUnlock()
	return h.executionConnCounts[executionID] >= h.config.MaxConnectionsPerExecution
}

// GetConnectionCount returns the current connection count for an execution
func (h *Hub) GetConnectionCount(executionID string) int {
	h.connCountsMu.RLock()
	defer h.connCountsMu.RUnlock()
	return h.executionConnCounts[executionID]
}

// Shutdown gracefully shuts down the hub
func (h *Hub) Shutdown() {
	h.ClientsMu.Lock()
	defer h.ClientsMu.Unlock()

	log.Println("Shutting down WebSocket hub...")
	for client := range h.Clients {
		h.Unregister <- client
	}
}

// Stop signals the hub's Run loop to exit
func (h *Hub) Stop() {
	close(h.stop)
}
