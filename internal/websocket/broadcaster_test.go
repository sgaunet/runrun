package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBroadcaster(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	require.NotNil(t, broadcaster)
	assert.Equal(t, hub, broadcaster.Hub)
}

func TestBroadcastLog(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	// Start hub
	go hub.Run()
	defer hub.Shutdown()

	// Create client and subscribe
	client := &Client{
		ID:            "test-client",
		Hub:           hub,
		Send:          make(chan []byte, 10),
		Subscriptions: make(map[string]bool),
	}

	executionID := "exec-123"
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, executionID)

	// Broadcast a log
	logLine := "Test log line"
	broadcaster.BroadcastLog(executionID, logLine)

	// Wait for broadcast
	time.Sleep(100 * time.Millisecond)

	// Client should receive the message
	select {
	case msg := <-client.Send:
		var message Message
		err := json.Unmarshal(msg, &message)
		require.NoError(t, err)

		assert.Equal(t, MessageTypeLog, message.Type)
		assert.Equal(t, executionID, message.ExecutionID)

		// Check LogData
		logData, ok := message.Data.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, logLine, logData["line"])
	case <-time.After(1 * time.Second):
		t.Fatal("Did not receive broadcast message")
	}
}

func TestBroadcastLogWithLevel(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	// Start hub
	go hub.Run()
	defer hub.Shutdown()

	// Create client and subscribe
	client := &Client{
		ID:            "test-client-2",
		Hub:           hub,
		Send:          make(chan []byte, 10),
		Subscriptions: make(map[string]bool),
	}

	executionID := "exec-456"
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, executionID)

	// Broadcast a log with level
	logLine := "Error log line"
	level := "error"
	broadcaster.BroadcastLogWithLevel(executionID, logLine, level)

	// Wait for broadcast
	time.Sleep(100 * time.Millisecond)

	// Client should receive the message
	select {
	case msg := <-client.Send:
		var message Message
		err := json.Unmarshal(msg, &message)
		require.NoError(t, err)

		assert.Equal(t, MessageTypeLog, message.Type)
		assert.Equal(t, executionID, message.ExecutionID)

		// Check LogData
		logData, ok := message.Data.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, logLine, logData["line"])
		assert.Equal(t, level, logData["level"])
	case <-time.After(1 * time.Second):
		t.Fatal("Did not receive broadcast message")
	}
}

func TestBroadcastLog_NoSubscribers(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	// Start hub
	go hub.Run()
	defer hub.Shutdown()

	// Broadcast to non-existent execution (should not panic)
	broadcaster.BroadcastLog("nonexistent", "test message")
	time.Sleep(50 * time.Millisecond)

	// Test passes if no panic
}

func TestHasSubscribers_True(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	client := &Client{
		ID:            "test-client-3",
		Hub:           hub,
		Send:          make(chan []byte, 10),
		Subscriptions: make(map[string]bool),
	}

	executionID := "exec-789"
	hub.Subscribe(client, executionID)

	hasSubscribers := broadcaster.HasSubscribers(executionID)
	assert.True(t, hasSubscribers)
}

func TestHasSubscribers_False(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	hasSubscribers := broadcaster.HasSubscribers("nonexistent")
	assert.False(t, hasSubscribers)
}

func TestBroadcastLogWithLevel_DifferentLevels(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	// Start hub
	go hub.Run()
	defer hub.Shutdown()

	client := &Client{
		ID:            "test-client-4",
		Hub:           hub,
		Send:          make(chan []byte, 100),
		Subscriptions: make(map[string]bool),
	}

	executionID := "exec-levels"
	hub.Register <- client
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, executionID)

	levels := []string{"info", "warn", "error", "debug"}

	for _, level := range levels {
		broadcaster.BroadcastLogWithLevel(executionID, "Test message", level)
	}

	time.Sleep(200 * time.Millisecond)

	// Should have received all messages
	receivedCount := len(client.Send)
	assert.Equal(t, len(levels), receivedCount, "Should receive message for each level")
}

func TestBroadcastLog_MultipleClients(t *testing.T) {
	hub := NewHub(nil)
	broadcaster := NewBroadcaster(hub)

	// Start hub
	go hub.Run()

	executionID := "exec-multi"

	// Create multiple clients
	client1 := &Client{
		ID:            "client-1",
		Hub:           hub,
		Send:          make(chan []byte, 10),
		Subscriptions: make(map[string]bool),
	}
	client2 := &Client{
		ID:            "client-2",
		Hub:           hub,
		Send:          make(chan []byte, 10),
		Subscriptions: make(map[string]bool),
	}

	// Register and subscribe both clients
	hub.Register <- client1
	hub.Register <- client2
	time.Sleep(50 * time.Millisecond)

	hub.Subscribe(client1, executionID)
	hub.Subscribe(client2, executionID)

	// Broadcast a message
	broadcaster.BroadcastLog(executionID, "Multi-client test")
	time.Sleep(100 * time.Millisecond)

	// Both clients should receive the message
	assert.Len(t, client1.Send, 1, "Client 1 should receive message")
	assert.Len(t, client2.Send, 1, "Client 2 should receive message")

	// Unregister clients before shutdown
	hub.Unregister <- client1
	hub.Unregister <- client2
	time.Sleep(50 * time.Millisecond)
}
