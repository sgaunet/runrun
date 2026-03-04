package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// Broadcaster provides methods to broadcast log messages to WebSocket clients.
// It supports both unbuffered (immediate) and buffered (batched) broadcasting.
type Broadcaster struct {
	Hub *Hub

	// buffers holds per-execution stream buffers for batched mode
	buffers   map[string]*StreamBuffer
	buffersMu sync.Mutex
}

// NewBroadcaster creates a new log broadcaster
func NewBroadcaster(hub *Hub) *Broadcaster {
	return &Broadcaster{
		Hub:     hub,
		buffers: make(map[string]*StreamBuffer),
	}
}

// EnableBuffering creates a StreamBuffer for the given execution ID.
// Subsequent BroadcastLog/BroadcastLogWithLevel calls for this execution
// will be batched through the buffer.
func (b *Broadcaster) EnableBuffering(executionID string) {
	b.buffersMu.Lock()
	defer b.buffersMu.Unlock()

	if _, exists := b.buffers[executionID]; exists {
		return
	}

	buf := NewStreamBuffer(b.Hub.config, executionID, func(eid string, batch []LogData) {
		BroadcastBatch(b.Hub, eid, batch)
	})
	b.buffers[executionID] = buf
}

// FlushBuffer flushes and removes the buffer for an execution.
// Call this when an execution completes to ensure all lines are sent.
func (b *Broadcaster) FlushBuffer(executionID string) {
	b.buffersMu.Lock()
	buf, exists := b.buffers[executionID]
	if exists {
		delete(b.buffers, executionID)
	}
	b.buffersMu.Unlock()

	if exists {
		buf.Stop()
	}
}

// BroadcastLog broadcasts a log line to all clients subscribed to an execution.
// If buffering is enabled for this execution, the line is added to the buffer.
func (b *Broadcaster) BroadcastLog(executionID, logLine string) {
	b.buffersMu.Lock()
	buf, buffered := b.buffers[executionID]
	b.buffersMu.Unlock()

	if buffered {
		buf.Add(LogData{
			Line:      logLine,
			Timestamp: time.Now(),
		})
		return
	}

	// Unbuffered: send immediately (backward compatible)
	b.broadcastLogImmediate(executionID, logLine, "")
}

// BroadcastLogWithLevel broadcasts a log line with a specific level.
// If buffering is enabled for this execution, the line is added to the buffer.
func (b *Broadcaster) BroadcastLogWithLevel(executionID, logLine, level string) {
	b.buffersMu.Lock()
	buf, buffered := b.buffers[executionID]
	b.buffersMu.Unlock()

	if buffered {
		buf.Add(LogData{
			Line:      logLine,
			Timestamp: time.Now(),
			Level:     level,
		})
		return
	}

	b.broadcastLogImmediate(executionID, logLine, level)
}

// BroadcastComplete broadcasts a completion message to all clients subscribed to an execution.
// It also flushes any active buffer for this execution.
func (b *Broadcaster) BroadcastComplete(executionID, status string) {
	// Flush buffer first so all log lines arrive before the complete message
	b.FlushBuffer(executionID)

	msg := Message{
		Type:        MessageTypeComplete,
		ExecutionID: executionID,
		Data: map[string]string{
			"status": status,
		},
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling complete message: %v", err)
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

// broadcastLogImmediate sends a single log line immediately without buffering.
func (b *Broadcaster) broadcastLogImmediate(executionID, logLine, level string) {
	logData := LogData{
		Line:      logLine,
		Timestamp: time.Now(),
	}
	if level != "" {
		logData.Level = level
	}

	msg := Message{
		Type:        MessageTypeLog,
		ExecutionID: executionID,
		Data:        logData,
		Timestamp:   time.Now(),
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
