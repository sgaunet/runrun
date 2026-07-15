package websocket

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// clientIDCounter is used to generate unique client IDs.
var clientIDCounter atomic.Uint64

const (
	// defaultBroadcastChannelSize is the buffer size of the hub's Broadcast channel.
	defaultBroadcastChannelSize = 256

	// idleCheckDivisor controls how often the hub checks for idle clients,
	// relative to the configured idle timeout: every IdleTimeout/idleCheckDivisor.
	idleCheckDivisor = 2
)

func generateClientID() string {
	return fmt.Sprintf("client-%d", clientIDCounter.Add(1))
}

// NewHub creates a new WebSocket hub.
func NewHub(config *Config) *Hub {
	if config == nil {
		config = DefaultConfig()
	}
	return &Hub{
		Clients:             make(map[*Client]bool),
		Subscriptions:       make(map[string]map[*Client]bool),
		Register:            make(chan *Client),
		Unregister:          make(chan *Client),
		Broadcast:           make(chan *BroadcastMessage, defaultBroadcastChannelSize),
		stop:                make(chan struct{}),
		config:              config,
		executionConnCounts: make(map[string]int),
	}
}

// Run starts the hub's main event loop.
func (h *Hub) Run() {
	var idleTicker *time.Ticker
	if h.config.IdleTimeout > 0 {
		idleTicker = time.NewTicker(h.config.IdleTimeout / idleCheckDivisor)
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

// registerClient registers a new client.
func (h *Hub) registerClient(client *Client) {
	h.ClientsMu.Lock()
	h.Clients[client] = true
	h.ClientsMu.Unlock()

	log.Printf("WebSocket client registered: %s", client.ID)
}

// unregisterClient unregisters a client and cleans up resources.
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

// broadcastMessage sends a message to all clients subscribed to an execution.
func (h *Hub) broadcastMessage(message *BroadcastMessage) {
	// Snapshot the subscriber set under the read lock. Iterating the map
	// directly while Subscribe/Unsubscribe may mutate it from other goroutines
	// would be a data race, and holding the lock across the sends below would
	// serialize broadcasts against every subscription change.
	h.SubscriptionsMu.RLock()
	subs, ok := h.Subscriptions[message.ExecutionID]
	if !ok || len(subs) == 0 {
		h.SubscriptionsMu.RUnlock()
		return
	}
	clients := make([]*Client, 0, len(subs))
	for c := range subs {
		clients = append(clients, c)
	}
	h.SubscriptionsMu.RUnlock()

	// Send to all subscribed clients.
	var (
		wg      sync.WaitGroup
		evictMu sync.Mutex
		toEvict []*Client
	)
	for _, client := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()

			// Apply server-side level filter if the message has a level
			if message.Level != "" {
				c.FilterMu.RLock()
				filter := c.LevelFilter
				c.FilterMu.RUnlock()
				if !MatchesFilter(message.Level, filter) {
					return
				}
			}

			select {
			case c.Send <- message.Data:
				// Message sent successfully
			default:
				// Client's send channel is full. Record it for eviction after
				// all sends finish; do NOT send on h.Unregister here. This
				// method runs on the Run goroutine's stack (Run blocks in
				// wg.Wait below), and h.Unregister is drained only by that same
				// loop, so blocking on it would deadlock the hub.
				log.Printf("Client %s send buffer full, unregistering", c.ID)
				evictMu.Lock()
				toEvict = append(toEvict, c)
				evictMu.Unlock()
			}
		}(client)
	}
	wg.Wait()

	// Every sender goroutine for this broadcast has returned, so nothing is
	// still sending on these clients' Send channels; unregistering now (which
	// closes Send) is safe. unregisterClient runs inline on the Run goroutine
	// and does its own locking.
	for _, c := range toEvict {
		h.unregisterClient(c)
	}
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

// RegisterClient creates a new client for the given connection and registers it with the Hub.
func (h *Hub) RegisterClient(conn *websocket.Conn) *Client {
	client := NewClient(h, conn, generateClientID(), h.config)
	h.Register <- client
	return client
}

// UnregisterClient unregisters a client from the Hub.
func (h *Hub) UnregisterClient(client *Client) {
	h.Unregister <- client
}

// Subscribe adds a client to an execution's subscription list.
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

	log.Printf("Client %s subscribed to execution %s", sanitizeLogValue(client.ID), sanitizeLogValue(executionID))
}

// Unsubscribe removes a client from an execution's subscription list.
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

	log.Printf("Client %s unsubscribed from execution %s", sanitizeLogValue(client.ID), sanitizeLogValue(executionID))
}

// GetSubscriberCount returns the number of clients subscribed to an execution.
func (h *Hub) GetSubscriberCount(executionID string) int {
	h.SubscriptionsMu.RLock()
	defer h.SubscriptionsMu.RUnlock()

	if clients, ok := h.Subscriptions[executionID]; ok {
		return len(clients)
	}
	return 0
}

// ConnectionLimitReached returns true if the execution has reached its max connections.
func (h *Hub) ConnectionLimitReached(executionID string) bool {
	if h.config.MaxConnectionsPerExecution <= 0 {
		return false
	}
	h.connCountsMu.RLock()
	defer h.connCountsMu.RUnlock()
	return h.executionConnCounts[executionID] >= h.config.MaxConnectionsPerExecution
}

// GetConnectionCount returns the current connection count for an execution.
func (h *Hub) GetConnectionCount(executionID string) int {
	h.connCountsMu.RLock()
	defer h.connCountsMu.RUnlock()
	return h.executionConnCounts[executionID]
}

// GetConfig returns the hub's configuration.
func (h *Hub) GetConfig() *Config {
	return h.config
}

// Shutdown gracefully shuts down the hub by unregistering every client.
//
// Unregistration is routed through h.Unregister so that unregisterClient runs
// on the Run goroutine, serialized with broadcastMessage. Closing a client's
// Send channel there (rather than directly from the caller) is what keeps it
// from racing an in-flight broadcast sender, which would send on a closed
// channel and panic. The client set is snapshotted first so ClientsMu is not
// held across the channel sends: the original code held it and deadlocked,
// because the Run loop's unregisterClient handler needs the same lock.
func (h *Hub) Shutdown() {
	log.Println("Shutting down WebSocket hub...")

	h.ClientsMu.RLock()
	clients := make([]*Client, 0, len(h.Clients))
	for client := range h.Clients {
		clients = append(clients, client)
	}
	h.ClientsMu.RUnlock()

	for _, client := range clients {
		select {
		case h.Unregister <- client:
		case <-h.stop:
			// Run loop has already exited; nothing will drain h.Unregister.
			return
		}
	}
}

// Stop signals the hub's Run loop to exit.
func (h *Hub) Stop() {
	close(h.stop)
}
