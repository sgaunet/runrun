package websocket

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// NewClient creates a new WebSocket client
func NewClient(hub *Hub, conn *websocket.Conn, id string, config *Config) *Client {
	return &Client{
		ID:            id,
		Hub:           hub,
		Conn:          conn,
		Send:          make(chan []byte, config.SendChannelSize),
		Subscriptions: make(map[string]bool),
		LastActivity:  time.Now(),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump(config *Config) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic recovered in ReadPump for client %s: %v", c.ID, r)
		}
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	_ = c.Conn.SetReadDeadline(time.Now().Add(config.PongTimeout))
	c.Conn.SetPongHandler(func(string) error {
		c.UpdateActivity()
		return c.Conn.SetReadDeadline(time.Now().Add(config.PongTimeout))
	})

	c.Conn.SetReadLimit(config.MaxMessageSize)

	for {
		_, messageData, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", c.ID, err)
			}
			break
		}

		c.UpdateActivity()
		c.handleMessage(messageData)
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump(config *Config) {
	ticker := time.NewTicker(config.PingInterval)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic recovered in WritePump for client %s: %v", c.ID, r)
		}
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(config.WriteTimeout))
			if !ok {
				// Hub closed the channel
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Write error for client %s: %v", c.ID, err)
				return
			}

		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(config.WriteTimeout))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Error unmarshaling message from client %s: %v", c.ID, err)
		c.sendError("Invalid message format")
		return
	}

	switch msg.Type {
	case MessageTypeSubscribe:
		if msg.ExecutionID == "" {
			c.sendError("Execution ID is required for subscribe")
			return
		}
		if c.Hub.ConnectionLimitReached(msg.ExecutionID) {
			c.sendError("Too many connections for this execution")
			return
		}
		c.Hub.Subscribe(c, msg.ExecutionID)
		c.sendSubscribed(msg.ExecutionID)

	case MessageTypeUnsubscribe:
		if msg.ExecutionID == "" {
			c.sendError("Execution ID is required for unsubscribe")
			return
		}
		c.Hub.Unsubscribe(c, msg.ExecutionID)
		c.sendUnsubscribed(msg.ExecutionID)

	case MessageTypePong:
		// Pong received, activity already updated

	default:
		c.sendError("Unknown message type")
	}
}

// sendError sends an error message to the client
func (c *Client) sendError(errMsg string) {
	msg := Message{
		Type:      MessageTypeError,
		Error:     errMsg,
		Timestamp: time.Now(),
	}
	c.sendMessage(msg)
}

// sendSubscribed sends a subscription confirmation to the client
func (c *Client) sendSubscribed(executionID string) {
	msg := Message{
		Type:        MessageTypeSubscribed,
		ExecutionID: executionID,
		Timestamp:   time.Now(),
	}
	c.sendMessage(msg)
}

// sendUnsubscribed sends an unsubscription confirmation to the client
func (c *Client) sendUnsubscribed(executionID string) {
	msg := Message{
		Type:        MessageTypeUnsubscribed,
		ExecutionID: executionID,
		Timestamp:   time.Now(),
	}
	c.sendMessage(msg)
}

// sendMessage sends a message to the client
func (c *Client) sendMessage(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message for client %s: %v", c.ID, err)
		return
	}

	select {
	case c.Send <- data:
	default:
		log.Printf("Client %s send buffer full", c.ID)
	}
}

// UpdateActivity updates the client's last activity timestamp
func (c *Client) UpdateActivity() {
	c.ActivityMu.Lock()
	c.LastActivity = time.Now()
	c.ActivityMu.Unlock()
}

// GetLastActivity returns the client's last activity time
func (c *Client) GetLastActivity() time.Time {
	c.ActivityMu.RLock()
	defer c.ActivityMu.RUnlock()
	return c.LastActivity
}
