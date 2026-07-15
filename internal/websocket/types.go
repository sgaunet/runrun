package websocket

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageType represents the type of WebSocket message.
type MessageType string

const (
	// MessageTypeSubscribe is sent by client to subscribe to an execution.
	MessageTypeSubscribe MessageType = "subscribe"
	// MessageTypeUnsubscribe is sent by client to unsubscribe from an execution.
	MessageTypeUnsubscribe MessageType = "unsubscribe"
	// MessageTypeLog is sent by server with log data.
	MessageTypeLog MessageType = "log"
	// MessageTypeError is sent by server when an error occurs.
	MessageTypeError MessageType = "error"
	// MessageTypePing is sent by server to check connection health.
	MessageTypePing MessageType = "ping"
	// MessageTypePong is sent by client in response to ping.
	MessageTypePong MessageType = "pong"
	// MessageTypeSubscribed is sent by server to confirm subscription.
	MessageTypeSubscribed MessageType = "subscribed"
	// MessageTypeUnsubscribed is sent by server to confirm unsubscription.
	MessageTypeUnsubscribed MessageType = "unsubscribed"
	// MessageTypeComplete is sent by server when an execution completes.
	MessageTypeComplete MessageType = "complete"
	// MessageTypeLogBatch is sent by server with multiple log lines batched together.
	MessageTypeLogBatch MessageType = "log_batch"
	// MessageTypeMetadata is sent by server with stream metadata (e.g., total line count).
	MessageTypeMetadata MessageType = "metadata"
)

// Message represents a WebSocket message.
type Message struct {
	Type        MessageType `json:"type"`
	ExecutionID string      `json:"execution_id,omitempty"`
	Data        any         `json:"data,omitempty"`
	Error       string      `json:"error,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

// LogData represents log line data in a message.
type LogData struct {
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"`
}

// StreamMetadata contains metadata about a log stream.
type StreamMetadata struct {
	TotalLines int `json:"total_lines,omitempty"`
}

// OverflowMode defines the strategy when a stream buffer is full.
type OverflowMode int

const (
	// OverflowDropOldest drops the oldest buffered lines when full.
	OverflowDropOldest OverflowMode = iota
	// OverflowBlock blocks the writer until buffer space is available.
	OverflowBlock
)

// Default values for Config, also used as fallbacks elsewhere in the package
// when a zero or negative Config field indicates "unset".
const (
	defaultReadBufferSize             = 1024
	defaultWriteBufferSize            = 1024
	defaultReadTimeout                = 60 * time.Second
	defaultWriteTimeout               = 10 * time.Second
	defaultPingInterval               = 30 * time.Second
	defaultPongTimeout                = 60 * time.Second
	defaultMaxMessageSize             = 512 * 1024 // 512 KB
	defaultSendChannelSize            = 256
	defaultMaxSubscriptionsPerClient  = 10
	defaultIdleTimeout                = 5 * time.Minute
	defaultMaxConnectionsPerExecution = 10
	defaultStreamBufferMaxLines       = 50
	defaultStreamBufferMaxBytes       = 1024 * 1024 // 1 MB
	defaultStreamBufferFlushInterval  = 100 * time.Millisecond
	defaultFileStreamBatchSize        = 100
)

// Client represents a WebSocket client connection.
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

	// LevelFilter restricts which log levels are sent to this client.
	// nil means no filtering (send all levels).
	LevelFilter map[string]bool
	FilterMu    sync.RWMutex

	// LastActivity tracks the last time we received a message from the client
	LastActivity time.Time
	ActivityMu   sync.RWMutex
}

// Hub manages all active WebSocket connections.
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

// BroadcastMessage represents a message to be broadcast to specific clients.
type BroadcastMessage struct {
	ExecutionID string
	Data        []byte
	// Level is the log level of this message (used for server-side filtering).
	// Empty string means the message bypasses level filtering (e.g., complete, metadata).
	Level string
}

// Config holds WebSocket configuration.
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

	// StreamBufferMaxLines is the maximum number of log lines to buffer before flushing
	StreamBufferMaxLines int

	// StreamBufferMaxBytes is the maximum total bytes to buffer before flushing
	StreamBufferMaxBytes int

	// StreamBufferFlushInterval is the maximum time between buffer flushes
	StreamBufferFlushInterval time.Duration

	// StreamBufferOverflowMode defines what happens when the buffer is full
	StreamBufferOverflowMode OverflowMode

	// FileStreamBatchSize is the number of lines to read from log files per batch
	FileStreamBatchSize int
}

// DefaultConfig returns the default WebSocket configuration.
func DefaultConfig() *Config {
	return &Config{
		ReadBufferSize:             defaultReadBufferSize,
		WriteBufferSize:            defaultWriteBufferSize,
		ReadTimeout:                defaultReadTimeout,
		WriteTimeout:               defaultWriteTimeout,
		PingInterval:               defaultPingInterval,
		PongTimeout:                defaultPongTimeout,
		MaxMessageSize:             defaultMaxMessageSize,
		SendChannelSize:            defaultSendChannelSize,
		MaxSubscriptionsPerClient:  defaultMaxSubscriptionsPerClient,
		IdleTimeout:                defaultIdleTimeout,
		MaxConnectionsPerExecution: defaultMaxConnectionsPerExecution,
		StreamBufferMaxLines:       defaultStreamBufferMaxLines,
		StreamBufferMaxBytes:       defaultStreamBufferMaxBytes,
		StreamBufferFlushInterval:  defaultStreamBufferFlushInterval,
		StreamBufferOverflowMode:   OverflowDropOldest,
		FileStreamBatchSize:        defaultFileStreamBatchSize,
	}
}
