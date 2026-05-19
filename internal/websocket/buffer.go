package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// StreamBuffer batches log lines for efficient WebSocket streaming.
// It accumulates lines and flushes them as a single batch message when
// the buffer reaches its line/byte limit or the flush interval elapses.
type StreamBuffer struct {
	mu           sync.Mutex
	lines        []LogData
	currentBytes int
	stopped      bool

	maxLines      int
	maxBytes      int
	flushInterval time.Duration
	overflowMode  OverflowMode

	flushFn func(executionID string, batch []LogData)
	timer   *time.Timer
	flushWg sync.WaitGroup

	executionID string
}

// NewStreamBuffer creates a new StreamBuffer with the given configuration.
func NewStreamBuffer(config *Config, executionID string, flushFn func(string, []LogData)) *StreamBuffer {
	maxLines := config.StreamBufferMaxLines
	if maxLines <= 0 {
		maxLines = 50
	}
	maxBytes := config.StreamBufferMaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	flushInterval := config.StreamBufferFlushInterval
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond
	}

	sb := &StreamBuffer{
		lines:         make([]LogData, 0, maxLines),
		maxLines:      maxLines,
		maxBytes:      maxBytes,
		flushInterval: flushInterval,
		overflowMode:  config.StreamBufferOverflowMode,
		flushFn:       flushFn,
		executionID:   executionID,
	}

	sb.timer = time.AfterFunc(flushInterval, sb.timerFlush)
	return sb
}

// Add adds a log line to the buffer. Depending on the overflow mode,
// it may drop the oldest line or block until space is available.
func (sb *StreamBuffer) Add(line LogData) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.stopped {
		return
	}

	lineSize := len(line.Line)

	// Check if adding this line would exceed limits
	if len(sb.lines) >= sb.maxLines || (sb.currentBytes+lineSize > sb.maxBytes && len(sb.lines) > 0) {
		switch sb.overflowMode {
		case OverflowDropOldest:
			sb.dropOldestLocked()
		case OverflowBlock:
			// Flush synchronously to make room
			sb.flushLocked()
		}
	}

	sb.lines = append(sb.lines, line)
	sb.currentBytes += lineSize

	// Flush immediately if we've hit the limit
	if len(sb.lines) >= sb.maxLines || sb.currentBytes >= sb.maxBytes {
		sb.flushLocked()
	}
}

// Flush forces a flush of all buffered lines.
func (sb *StreamBuffer) Flush() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.flushLocked()
}

// Stop stops the buffer, flushes remaining lines, and stops the timer.
// Stop is synchronous: when it returns, every line ever added to this buffer
// (including any in-flight async flushes from the timer or size limits) has
// been delivered through flushFn. Callers can therefore safely send a follow-up
// message — e.g. the execution-complete signal — knowing it will arrive after
// every log line.
func (sb *StreamBuffer) Stop() {
	sb.mu.Lock()
	if sb.stopped {
		sb.mu.Unlock()
		return
	}
	sb.stopped = true
	sb.timer.Stop()

	var batch []LogData
	if len(sb.lines) > 0 {
		batch = make([]LogData, len(sb.lines))
		copy(batch, sb.lines)
		sb.lines = sb.lines[:0]
		sb.currentBytes = 0
	}
	sb.mu.Unlock()

	// Wait for any previously async-dispatched batches (timer- or size-triggered)
	// to finish delivering before we flush the final batch synchronously.
	sb.flushWg.Wait()

	if batch != nil {
		sb.flushFn(sb.executionID, batch)
	}
}

// Len returns the current number of buffered lines.
func (sb *StreamBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return len(sb.lines)
}

// timerFlush is called by the flush timer.
func (sb *StreamBuffer) timerFlush() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.stopped {
		return
	}

	sb.flushLocked()
	sb.timer.Reset(sb.flushInterval)
}

// flushLocked flushes all buffered lines. Must be called with mu held.
func (sb *StreamBuffer) flushLocked() {
	if len(sb.lines) == 0 {
		return
	}

	// Copy the batch and reset
	batch := make([]LogData, len(sb.lines))
	copy(batch, sb.lines)
	sb.lines = sb.lines[:0]
	sb.currentBytes = 0

	// Send batch outside the lock via goroutine to avoid blocking.
	// flushWg lets Stop() wait for these in-flight deliveries to complete.
	sb.flushWg.Add(1)
	go func(b []LogData) {
		defer sb.flushWg.Done()
		sb.flushFn(sb.executionID, b)
	}(batch)
}

// dropOldestLocked drops the oldest line to make room. Must be called with mu held.
func (sb *StreamBuffer) dropOldestLocked() {
	if len(sb.lines) == 0 {
		return
	}
	dropped := sb.lines[0]
	sb.currentBytes -= len(dropped.Line)
	sb.lines = sb.lines[1:]
}

// BroadcastBatch marshals a batch of log lines into a single WebSocket message
// and sends it via the hub's broadcast channel.
func BroadcastBatch(hub *Hub, executionID string, batch []LogData) {
	if len(batch) == 0 {
		return
	}

	// For single-line batches, send as regular log message for backward compatibility
	if len(batch) == 1 {
		msg := Message{
			Type:        MessageTypeLog,
			ExecutionID: executionID,
			Data:        batch[0],
			Timestamp:   time.Now(),
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Error marshaling log message: %v", err)
			return
		}
		level := batch[0].Level
		if level == "" {
			level = "info"
		}
		hub.Broadcast <- &BroadcastMessage{
			ExecutionID: executionID,
			Data:        data,
			Level:       level,
		}
		return
	}

	msg := Message{
		Type:        MessageTypeLogBatch,
		ExecutionID: executionID,
		Data:        batch,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling log batch message: %v", err)
		return
	}

	hub.Broadcast <- &BroadcastMessage{
		ExecutionID: executionID,
		Data:        data,
	}
}
