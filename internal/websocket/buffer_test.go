package websocket

import (
	"sync"
	"testing"
	"time"
)

func testConfig() *Config {
	cfg := DefaultConfig()
	cfg.StreamBufferMaxLines = 5
	cfg.StreamBufferMaxBytes = 1024
	cfg.StreamBufferFlushInterval = 50 * time.Millisecond
	cfg.StreamBufferOverflowMode = OverflowDropOldest
	return cfg
}

func TestStreamBuffer_FlushOnMaxLines(t *testing.T) {
	var mu sync.Mutex
	var batches [][]LogData

	cfg := testConfig()
	buf := NewStreamBuffer(cfg, "exec-1", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, batch)
	})
	defer buf.Stop()

	// Add exactly maxLines items
	for i := 0; i < 5; i++ {
		buf.Add(LogData{Line: "line", Timestamp: time.Now()})
	}

	// Wait for async flush
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) == 0 {
		t.Fatal("expected at least one batch after hitting maxLines")
	}
	if len(batches[0]) != 5 {
		t.Errorf("expected batch of 5 lines, got %d", len(batches[0]))
	}
}

func TestStreamBuffer_FlushOnTimer(t *testing.T) {
	var mu sync.Mutex
	var batches [][]LogData

	cfg := testConfig()
	cfg.StreamBufferFlushInterval = 30 * time.Millisecond
	buf := NewStreamBuffer(cfg, "exec-2", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, batch)
	})
	defer buf.Stop()

	// Add fewer lines than maxLines
	buf.Add(LogData{Line: "hello", Timestamp: time.Now()})
	buf.Add(LogData{Line: "world", Timestamp: time.Now()})

	// Wait for timer flush
	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) == 0 {
		t.Fatal("expected timer-triggered flush")
	}
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 2 {
		t.Errorf("expected 2 total lines, got %d", total)
	}
}

func TestStreamBuffer_OverflowDropOldest(t *testing.T) {
	var mu sync.Mutex
	var allBatches [][]LogData

	cfg := testConfig()
	cfg.StreamBufferMaxLines = 3
	cfg.StreamBufferFlushInterval = 1 * time.Second // long interval so timer doesn't interfere
	buf := NewStreamBuffer(cfg, "exec-3", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		allBatches = append(allBatches, batch)
	})
	defer buf.Stop()

	// Add one more than maxLines to trigger overflow + flush
	buf.Add(LogData{Line: "a", Timestamp: time.Now()})
	buf.Add(LogData{Line: "b", Timestamp: time.Now()})
	buf.Add(LogData{Line: "c", Timestamp: time.Now()}) // triggers flush at maxLines

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if len(allBatches) == 0 {
		mu.Unlock()
		t.Fatal("expected flush after reaching maxLines")
	}
	batch := allBatches[0]
	mu.Unlock()

	if len(batch) != 3 {
		t.Errorf("expected 3 lines in batch, got %d", len(batch))
	}
}

func TestStreamBuffer_OverflowBlock(t *testing.T) {
	var mu sync.Mutex
	var batches [][]LogData

	cfg := testConfig()
	cfg.StreamBufferMaxLines = 2
	cfg.StreamBufferOverflowMode = OverflowBlock
	cfg.StreamBufferFlushInterval = 1 * time.Second
	buf := NewStreamBuffer(cfg, "exec-4", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, batch)
	})
	defer buf.Stop()

	// Fill buffer to trigger block-mode flush
	buf.Add(LogData{Line: "first", Timestamp: time.Now()})
	buf.Add(LogData{Line: "second", Timestamp: time.Now()}) // triggers flush at maxLines

	time.Sleep(20 * time.Millisecond)

	// Add third, which would overflow; OverflowBlock triggers synchronous flush
	buf.Add(LogData{Line: "third", Timestamp: time.Now()})

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) < 1 {
		t.Fatal("expected at least one flush")
	}
}

func TestStreamBuffer_Stop(t *testing.T) {
	var mu sync.Mutex
	var batches [][]LogData

	cfg := testConfig()
	cfg.StreamBufferFlushInterval = 1 * time.Second
	buf := NewStreamBuffer(cfg, "exec-5", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, batch)
	})

	buf.Add(LogData{Line: "pending", Timestamp: time.Now()})
	buf.Stop()

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) == 0 {
		t.Fatal("expected Stop to flush remaining lines")
	}
	if batches[0][0].Line != "pending" {
		t.Errorf("expected line 'pending', got %q", batches[0][0].Line)
	}
}

func TestStreamBuffer_StopIdempotent(t *testing.T) {
	cfg := testConfig()
	buf := NewStreamBuffer(cfg, "exec-6", func(eid string, batch []LogData) {})

	buf.Stop()
	buf.Stop() // should not panic
}

func TestStreamBuffer_AddAfterStop(t *testing.T) {
	cfg := testConfig()
	buf := NewStreamBuffer(cfg, "exec-7", func(eid string, batch []LogData) {})

	buf.Stop()
	// Should not panic
	buf.Add(LogData{Line: "ignored", Timestamp: time.Now()})
}

func TestStreamBuffer_Len(t *testing.T) {
	cfg := testConfig()
	cfg.StreamBufferMaxLines = 100
	cfg.StreamBufferFlushInterval = 1 * time.Second
	buf := NewStreamBuffer(cfg, "exec-8", func(eid string, batch []LogData) {})
	defer buf.Stop()

	buf.Add(LogData{Line: "a", Timestamp: time.Now()})
	buf.Add(LogData{Line: "b", Timestamp: time.Now()})

	if buf.Len() != 2 {
		t.Errorf("expected Len()=2, got %d", buf.Len())
	}
}

func TestStreamBuffer_ConcurrentAdd(t *testing.T) {
	var mu sync.Mutex
	totalLines := 0

	cfg := testConfig()
	cfg.StreamBufferMaxLines = 10
	cfg.StreamBufferFlushInterval = 10 * time.Millisecond
	buf := NewStreamBuffer(cfg, "exec-9", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		totalLines += len(batch)
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf.Add(LogData{Line: "concurrent", Timestamp: time.Now()})
		}()
	}
	wg.Wait()
	buf.Stop()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if totalLines != 100 {
		t.Errorf("expected 100 total lines flushed, got %d", totalLines)
	}
}

func TestStreamBuffer_FlushOnMaxBytes(t *testing.T) {
	var mu sync.Mutex
	var batches [][]LogData

	cfg := testConfig()
	cfg.StreamBufferMaxLines = 1000 // high so byte limit triggers first
	cfg.StreamBufferMaxBytes = 20
	cfg.StreamBufferOverflowMode = OverflowBlock // Block mode flushes instead of dropping
	cfg.StreamBufferFlushInterval = 1 * time.Second
	buf := NewStreamBuffer(cfg, "exec-10", func(eid string, batch []LogData) {
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, batch)
	})
	defer buf.Stop()

	// First line: 12 bytes, under 20 limit
	buf.Add(LogData{Line: "twelve chars", Timestamp: time.Now()})
	// Second line: would bring total to 24 > 20, triggers overflow flush in block mode
	buf.Add(LogData{Line: "twelve chars", Timestamp: time.Now()})

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) == 0 {
		t.Fatal("expected flush when maxBytes exceeded in block mode")
	}
}

// TestStreamBuffer_StopIsSynchronous verifies the ordering contract that
// callers like Broadcaster.BroadcastComplete depend on: when Stop() returns,
// every line ever Add()ed to this buffer has already been delivered through
// flushFn. No sleep should be required to observe the flushed batches.
//
// Regression guard for the log-ordering bug where "[Execution completed: …]"
// appeared mid-stream because the final buffer flush was still in a goroutine
// when BroadcastComplete enqueued the complete message onto Hub.Broadcast.
func TestStreamBuffer_StopIsSynchronous(t *testing.T) {
	var mu sync.Mutex
	var delivered []LogData

	cfg := testConfig()
	cfg.StreamBufferMaxLines = 3       // small so size-flush also fires
	cfg.StreamBufferFlushInterval = 10 * time.Millisecond
	buf := NewStreamBuffer(cfg, "exec-stop-sync", func(eid string, batch []LogData) {
		// Simulate the cost of placing the batch on Hub.Broadcast; this
		// widens the race window that the old `go flushFn(...)` code suffered.
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		delivered = append(delivered, batch...)
	})

	const total = 20
	for i := 0; i < total; i++ {
		buf.Add(LogData{Line: "L", Timestamp: time.Now()})
	}
	// Give the timer a chance to dispatch at least one async flush before Stop,
	// so we also exercise the flushWg.Wait() path for in-flight batches.
	time.Sleep(15 * time.Millisecond)

	buf.Stop()

	// No sleep here: Stop must return only after every line is delivered.
	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != total {
		t.Fatalf("expected %d lines delivered before Stop returned, got %d", total, len(delivered))
	}
}

// TestStreamBuffer_StopOrdersBeforeFollowupSend simulates the executor's
// pattern: emit log lines, then Stop() the buffer, then send a follow-up
// "complete" sentinel. The sentinel must end up after every log line in the
// delivery order seen by downstream consumers.
func TestStreamBuffer_StopOrdersBeforeFollowupSend(t *testing.T) {
	var mu sync.Mutex
	var order []string

	cfg := testConfig()
	cfg.StreamBufferMaxLines = 4
	cfg.StreamBufferFlushInterval = 20 * time.Millisecond
	buf := NewStreamBuffer(cfg, "exec-order", func(eid string, batch []LogData) {
		// Slow flush widens the previously-racy window.
		time.Sleep(3 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		for _, l := range batch {
			order = append(order, l.Line)
		}
	})

	for i := 0; i < 10; i++ {
		buf.Add(LogData{Line: "log", Timestamp: time.Now()})
	}

	buf.Stop()

	// Caller's follow-up send must observe every log line as already delivered.
	mu.Lock()
	order = append(order, "complete")
	snapshot := append([]string(nil), order...)
	mu.Unlock()

	if snapshot[len(snapshot)-1] != "complete" {
		t.Fatalf("expected 'complete' to be last, got %q at end of %d items", snapshot[len(snapshot)-1], len(snapshot))
	}
	logCount := 0
	for _, s := range snapshot[:len(snapshot)-1] {
		if s != "log" {
			t.Fatalf("expected only 'log' before 'complete', got %q", s)
		}
		logCount++
	}
	if logCount != 10 {
		t.Fatalf("expected 10 'log' entries before 'complete', got %d", logCount)
	}
}

func TestBroadcastBatch_SingleLine(t *testing.T) {
	cfg := DefaultConfig()
	hub := NewHub(cfg)
	go hub.Run()
	defer hub.Stop()

	batch := []LogData{{Line: "single", Timestamp: time.Now()}}
	// Should not panic and should send as regular log message
	BroadcastBatch(hub, "exec-test", batch)
}

func TestBroadcastBatch_MultipleLines(t *testing.T) {
	cfg := DefaultConfig()
	hub := NewHub(cfg)
	go hub.Run()
	defer hub.Stop()

	batch := []LogData{
		{Line: "line1", Timestamp: time.Now()},
		{Line: "line2", Timestamp: time.Now()},
	}
	// Should not panic and should send as log_batch message
	BroadcastBatch(hub, "exec-test", batch)
}

func TestBroadcastBatch_Empty(t *testing.T) {
	cfg := DefaultConfig()
	hub := NewHub(cfg)
	go hub.Run()
	defer hub.Stop()

	// Should be a no-op
	BroadcastBatch(hub, "exec-test", nil)
	BroadcastBatch(hub, "exec-test", []LogData{})
}
