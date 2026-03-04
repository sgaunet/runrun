package websocket

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	// MessageTypeSubscribe is sent by client to subscribe to an execution
	MessageTypeSubscribe MessageType = "subscribe"
	// MessageTypeUnsubscribe is sent by client to unsubscribe from an execution
	MessageTypeUnsubscribe MessageType = "unsubscribe"
	// MessageTypeLog is sent by server with log data
	MessageTypeLog MessageType = "log"
	// MessageTypeError is sent by server when an error occurs
	MessageTypeError MessageType = "error"
	// MessageTypePing is sent by server to check connection health
	MessageTypePing MessageType = "ping"
	// MessageTypePong is sent by client in response to ping
	MessageTypePong MessageType = "pong"
	// MessageTypeSubscribed is sent by server to confirm subscription
	MessageTypeSubscribed MessageType = "subscribed"
	// MessageTypeUnsubscribed is sent by server to confirm unsubscription
	MessageTypeUnsubscribed MessageType = "unsubscribed"
)

// Message represents a WebSocket message
type Message struct {
	Type        MessageType `json:"type"`
	ExecutionID string      `json:"execution_id,omitempty"`
	Data        interface{} `json:"data,omitempty"`
	Error       string      `json:"error,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

// LogData represents log line data in a message
type LogData struct {
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"`
}

// Client represents a WebSocket client connection
type Client struct {
	// ID is the unique identifier for this client
	ID string

	// Hub is the WebSocket hub managing this client
	Hub *Hub

	// Conn is the WebSocket connection
	Conn *websocket.Conn

	// Send is the channel for outbound messages
	Send chan []byte

	// Subscriptions tracks which execution IDs this client is subscribed to
	Subscriptions map[string]bool
	SubscribeMu   sync.RWMutex

	// LastActivity tracks the last time we received a message from the client
	LastActivity time.Time
	ActivityMu   sync.RWMutex
}

// Hub manages all active WebSocket connections
type Hub struct {
	// Clients is the set of registered clients
	Clients   map[*Client]bool
	ClientsMu sync.RWMutex

	// Subscriptions maps execution IDs to subscribed clients
	Subscriptions   map[string]map[*Client]bool
	SubscriptionsMu sync.RWMutex

	// Register is the channel for client registration
	Register chan *Client

	// Unregister is the channel for client unregistration
	Unregister chan *Client

	// Broadcast is the channel for broadcasting messages to clients
	Broadcast chan *BroadcastMessage

	// stop signals the Run loop to exit
	stop chan struct{}

	// config holds WebSocket configuration
	config *Config

	// executionConnCounts tracks connection count per execution ID
	executionConnCounts map[string]int
	connCountsMu        sync.RWMutex
}

// BroadcastMessage represents a message to be broadcast to specific clients
type BroadcastMessage struct {
	ExecutionID string
	Data        []byte
}

// Config holds WebSocket configuration
type Config struct {
	// ReadBufferSize is the buffer size for reading messages
	ReadBufferSize int

	// WriteBufferSize is the buffer size for writing messages
	WriteBufferSize int

	// ReadTimeout is the maximum time to wait for a read operation
	ReadTimeout time.Duration

	// WriteTimeout is the maximum time to wait for a write operation
	WriteTimeout time.Duration

	// PingInterval is the interval for sending ping messages
	PingInterval time.Duration

	// PongTimeout is the maximum time to wait for a pong response
	PongTimeout time.Duration

	// MaxMessageSize is the maximum size of a message in bytes
	MaxMessageSize int64

	// SendChannelSize is the size of the send channel buffer
	SendChannelSize int

	// MaxSubscriptionsPerClient is the maximum number of subscriptions per client
	MaxSubscriptionsPerClient int

	// IdleTimeout is the duration after which idle connections are closed
	IdleTimeout time.Duration

	// MaxConnectionsPerExecution limits concurrent connections per execution ID
	MaxConnectionsPerExecution int
}

// DefaultConfig returns the default WebSocket configuration
func DefaultConfig() *Config {
	return &Config{
		ReadBufferSize:             1024,
		WriteBufferSize:            1024,
		ReadTimeout:                60 * time.Second,
		WriteTimeout:               10 * time.Second,
		PingInterval:               30 * time.Second,
		PongTimeout:                60 * time.Second,
		MaxMessageSize:             512 * 1024, // 512 KB
		SendChannelSize:            256,
		MaxSubscriptionsPerClient:  10,
		IdleTimeout:                5 * time.Minute,
		MaxConnectionsPerExecution: 10,
	}
}
