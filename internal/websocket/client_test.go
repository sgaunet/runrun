package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_HandleMessage_Subscribe(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-subscribe", config)

	executionID := "exec-test-123"
	msg := Message{
		Type:        MessageTypeSubscribe,
		ExecutionID: executionID,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	client.handleMessage(data)

	// Verify subscription
	hub.SubscriptionsMu.RLock()
	clients, ok := hub.Subscriptions[executionID]
	hub.SubscriptionsMu.RUnlock()
	assert.True(t, ok)
	assert.True(t, clients[client])

	// Check that confirmation message was sent
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeSubscribed, respMsg.Type)
		assert.Equal(t, executionID, respMsg.ExecutionID)
	case <-time.After(1 * time.Second):
		t.Fatal("No confirmation message sent")
	}
}

func TestClient_HandleMessage_SubscribeWithoutExecutionID(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-subscribe-no-id", config)

	msg := Message{
		Type:        MessageTypeSubscribe,
		ExecutionID: "", // Missing execution ID
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	client.handleMessage(data)

	// Should send error message
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeError, respMsg.Type)
		assert.Contains(t, respMsg.Error, "Execution ID is required")
	case <-time.After(1 * time.Second):
		t.Fatal("No error message sent")
	}
}

func TestClient_HandleMessage_Unsubscribe(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-unsubscribe", config)

	executionID := "exec-unsub-456"

	// Subscribe first
	hub.Subscribe(client, executionID)

	// Then unsubscribe
	msg := Message{
		Type:        MessageTypeUnsubscribe,
		ExecutionID: executionID,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	client.handleMessage(data)

	// Verify unsubscription
	hub.SubscriptionsMu.RLock()
	clients, ok := hub.Subscriptions[executionID]
	hub.SubscriptionsMu.RUnlock()
	assert.False(t, ok || len(clients) > 0)

	// Check confirmation
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeUnsubscribed, respMsg.Type)
		assert.Equal(t, executionID, respMsg.ExecutionID)
	case <-time.After(1 * time.Second):
		t.Fatal("No confirmation message sent")
	}
}

func TestClient_HandleMessage_UnsubscribeWithoutExecutionID(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-unsub-no-id", config)

	msg := Message{
		Type:        MessageTypeUnsubscribe,
		ExecutionID: "", // Missing execution ID
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	client.handleMessage(data)

	// Should send error
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeError, respMsg.Type)
		assert.Contains(t, respMsg.Error, "Execution ID is required")
	case <-time.After(1 * time.Second):
		t.Fatal("No error message sent")
	}
}

func TestClient_HandleMessage_Pong(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-pong", config)

	initialActivity := client.GetLastActivity()
	time.Sleep(10 * time.Millisecond)

	msg := Message{
		Type: MessageTypePong,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	client.handleMessage(data)

	// Activity should be updated (happens in ReadPump normally)
	// Since handleMessage doesn't update activity, this just tests it doesn't error
	newActivity := client.GetLastActivity()
	assert.False(t, newActivity.Before(initialActivity))
}

func TestClient_HandleMessage_UnknownType(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-unknown", config)

	msg := Message{
		Type: MessageType("unknown"),
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	client.handleMessage(data)

	// Should send error
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeError, respMsg.Type)
		assert.Contains(t, respMsg.Error, "Unknown message type")
	case <-time.After(1 * time.Second):
		t.Fatal("No error message sent")
	}
}

func TestClient_HandleMessage_InvalidJSON(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-invalid-json", config)

	// Send invalid JSON
	invalidData := []byte("{invalid json}")

	client.handleMessage(invalidData)

	// Should send error
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeError, respMsg.Type)
		assert.Contains(t, respMsg.Error, "Invalid message format")
	case <-time.After(1 * time.Second):
		t.Fatal("No error message sent")
	}
}

func TestClient_SendError(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-send-error", config)

	errorMsg := "test error message"
	client.sendError(errorMsg)

	// Verify error message was sent
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeError, respMsg.Type)
		assert.Equal(t, errorMsg, respMsg.Error)
		assert.False(t, respMsg.Timestamp.IsZero())
	case <-time.After(1 * time.Second):
		t.Fatal("No error message sent")
	}
}

func TestClient_SendSubscribed(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-send-subscribed", config)

	executionID := "exec-789"
	client.sendSubscribed(executionID)

	// Verify subscribed message
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeSubscribed, respMsg.Type)
		assert.Equal(t, executionID, respMsg.ExecutionID)
		assert.False(t, respMsg.Timestamp.IsZero())
	case <-time.After(1 * time.Second):
		t.Fatal("No subscribed message sent")
	}
}

func TestClient_SendUnsubscribed(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-send-unsubscribed", config)

	executionID := "exec-101"
	client.sendUnsubscribed(executionID)

	// Verify unsubscribed message
	select {
	case response := <-client.Send:
		var respMsg Message
		err := json.Unmarshal(response, &respMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypeUnsubscribed, respMsg.Type)
		assert.Equal(t, executionID, respMsg.ExecutionID)
		assert.False(t, respMsg.Timestamp.IsZero())
	case <-time.After(1 * time.Second):
		t.Fatal("No unsubscribed message sent")
	}
}

func TestClient_SendMessage_FullBuffer(t *testing.T) {
	hub := NewHub()

	// Create client with very small buffer
	client := &Client{
		ID:            "test-full-buffer",
		Hub:           hub,
		Send:          make(chan []byte, 1),
		Subscriptions: make(map[string]bool),
		LastActivity:  time.Now(),
	}

	// Fill the buffer
	client.Send <- []byte("msg1")

	// Try to send another message (should not block)
	msg := Message{
		Type:      MessageTypeError,
		Error:     "test",
		Timestamp: time.Now(),
	}

	client.sendMessage(msg)
	// Should not block or panic
}

func TestMessage_Serialization(t *testing.T) {
	msg := Message{
		Type:        MessageTypeLog,
		ExecutionID: "exec-serialize",
		Data: LogData{
			Line:      "test log line",
			Timestamp: time.Now(),
			Level:     "info",
		},
		Timestamp: time.Now(),
	}

	// Serialize
	data, err := json.Marshal(msg)
	require.NoError(t, err)

	// Deserialize
	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.Type, decoded.Type)
	assert.Equal(t, msg.ExecutionID, decoded.ExecutionID)
	assert.NotNil(t, decoded.Data)
}

func TestLogData_Serialization(t *testing.T) {
	logData := LogData{
		Line:      "error occurred",
		Timestamp: time.Now(),
		Level:     "error",
	}

	// Serialize
	data, err := json.Marshal(logData)
	require.NoError(t, err)

	// Deserialize
	var decoded LogData
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, logData.Line, decoded.Line)
	assert.Equal(t, logData.Level, decoded.Level)
}

func TestBroadcastMessage_Creation(t *testing.T) {
	execID := "test-exec"
	data := []byte("broadcast data")

	msg := &BroadcastMessage{
		ExecutionID: execID,
		Data:        data,
	}

	assert.Equal(t, execID, msg.ExecutionID)
	assert.Equal(t, data, msg.Data)
}

func TestClient_MultipleSubscribeOperations(t *testing.T) {
	hub := NewHub()
	config := DefaultConfig()

	client := NewClient(hub, nil, "test-multi-sub", config)

	// Subscribe to multiple executions
	executions := []string{"exec1", "exec2", "exec3"}

	for _, execID := range executions {
		msg := Message{
			Type:        MessageTypeSubscribe,
			ExecutionID: execID,
		}
		data, _ := json.Marshal(msg)
		client.handleMessage(data)

		// Drain confirmation message
		<-client.Send
	}

	// Verify all subscriptions
	for _, execID := range executions {
		hub.SubscriptionsMu.RLock()
		clients, ok := hub.Subscriptions[execID]
		hub.SubscriptionsMu.RUnlock()
		assert.True(t, ok)
		assert.True(t, clients[client])
	}

	// Verify client has all subscriptions
	client.SubscribeMu.RLock()
	assert.Len(t, client.Subscriptions, 3)
	client.SubscribeMu.RUnlock()
}
