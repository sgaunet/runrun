package websocket

import (
	"log"
	"sync"
)

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		Clients:       make(map[*Client]bool),
		Subscriptions: make(map[string]map[*Client]bool),
		Register:      make(chan *Client),
		Unregister:    make(chan *Client),
		Broadcast:     make(chan *BroadcastMessage, 256),
	}
}

// Run starts the hub's main event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.registerClient(client)

		case client := <-h.Unregister:
			h.unregisterClient(client)

		case message := <-h.Broadcast:
			h.broadcastMessage(message)
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

	// Remove from all subscriptions
	h.SubscriptionsMu.Lock()
	for executionID := range client.Subscriptions {
		if clients, ok := h.Subscriptions[executionID]; ok {
			delete(clients, client)
			if len(clients) == 0 {
				delete(h.Subscriptions, executionID)
			}
		}
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

// Shutdown gracefully shuts down the hub
func (h *Hub) Shutdown() {
	h.ClientsMu.Lock()
	defer h.ClientsMu.Unlock()

	log.Println("Shutting down WebSocket hub...")
	for client := range h.Clients {
		h.Unregister <- client
	}
}
