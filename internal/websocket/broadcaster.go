package websocket

import (
	"encoding/json"
	"log"
	"time"
)

// Broadcaster provides methods to broadcast log messages to WebSocket clients
type Broadcaster struct {
	Hub *Hub
}

// NewBroadcaster creates a new log broadcaster
func NewBroadcaster(hub *Hub) *Broadcaster {
	return &Broadcaster{
		Hub: hub,
	}
}

// BroadcastLog broadcasts a log line to all clients subscribed to an execution
func (b *Broadcaster) BroadcastLog(executionID, logLine string) {
	msg := Message{
		Type:        MessageTypeLog,
		ExecutionID: executionID,
		Data: LogData{
			Line:      logLine,
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling log message: %v", err)
		return
	}

	b.Hub.Broadcast <- &BroadcastMessage{
		ExecutionID: executionID,
		Data:        data,
	}
}

// BroadcastLogWithLevel broadcasts a log line with a specific level
func (b *Broadcaster) BroadcastLogWithLevel(executionID, logLine, level string) {
	msg := Message{
		Type:        MessageTypeLog,
		ExecutionID: executionID,
		Data: LogData{
			Line:      logLine,
			Timestamp: time.Now(),
			Level:     level,
		},
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling log message: %v", err)
		return
	}

	b.Hub.Broadcast <- &BroadcastMessage{
		ExecutionID: executionID,
		Data:        data,
	}
}

// HasSubscribers returns true if there are any subscribers for an execution
func (b *Broadcaster) HasSubscribers(executionID string) bool {
	return b.Hub.GetSubscriberCount(executionID) > 0
}
