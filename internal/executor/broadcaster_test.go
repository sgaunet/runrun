package executor

import (
	"sync"
	"testing"
	"time"

	"github.com/sgaunet/runrun/internal/config"
)

// mockBroadcaster records all broadcast calls for testing
type mockBroadcaster struct {
	mu       sync.Mutex
	logs     []broadcastCall
	complete []completeCall
}

type broadcastCall struct {
	executionID string
	line        string
	level       string
}

type completeCall struct {
	executionID string
	status      string
}

func (m *mockBroadcaster) BroadcastLog(executionID, logLine string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, broadcastCall{executionID: executionID, line: logLine})
}

func (m *mockBroadcaster) BroadcastLogWithLevel(executionID, logLine, level string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, broadcastCall{executionID: executionID, line: logLine, level: level})
}

func (m *mockBroadcaster) BroadcastComplete(executionID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.complete = append(m.complete, completeCall{executionID: executionID, status: status})
}

func (m *mockBroadcaster) EnableBuffering(_ string) {}
func (m *mockBroadcaster) FlushBuffer(_ string)     {}

func (m *mockBroadcaster) getLogs() []broadcastCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]broadcastCall, len(m.logs))
	copy(result, m.logs)
	return result
}

func (m *mockBroadcaster) getComplete() []completeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]completeCall, len(m.complete))
	copy(result, m.complete)
	return result
}

func TestExecuteTaskBroadcastsLogs(t *testing.T) {
	mock := &mockBroadcaster{}
	logDir := t.TempDir()

	executor := NewTaskExecutor(1, logDir, 30*time.Second)
	executor.SetBroadcaster(mock)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "test-broadcast",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo-step", Command: "echo hello"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Wait for execution to complete
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for execution to complete")
		default:
			exec, err := executor.GetExecution(executionID)
			if err == nil && exec.FinishedAt != nil {
				goto done
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
done:

	// Verify BroadcastLog was called with "hello" output
	logs := mock.getLogs()
	foundOutput := false
	for _, l := range logs {
		if l.line == "hello" && l.executionID == executionID {
			foundOutput = true
			break
		}
	}
	if !foundOutput {
		t.Errorf("expected BroadcastLog to be called with 'hello', got %d log calls: %+v", len(logs), logs)
	}

	// Verify BroadcastComplete was called
	completes := mock.getComplete()
	if len(completes) == 0 {
		t.Fatal("expected BroadcastComplete to be called")
	}
	if completes[0].executionID != executionID {
		t.Errorf("expected execution ID %s, got %s", executionID, completes[0].executionID)
	}
	if completes[0].status != "success" {
		t.Errorf("expected status 'success', got %s", completes[0].status)
	}
}

func TestExecuteTaskWithoutBroadcaster(t *testing.T) {
	logDir := t.TempDir()

	executor := NewTaskExecutor(1, logDir, 30*time.Second)
	// No broadcaster set - should work fine (backward compatible)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "test-no-broadcast",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo-step", Command: "echo works"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Wait for execution to complete
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for execution to complete")
		default:
			exec, err := executor.GetExecution(executionID)
			if err == nil && exec.FinishedAt != nil {
				if exec.Status != StatusSuccess {
					t.Fatalf("expected success, got %s", exec.Status)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
