package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sgaunet/runrun/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTaskExecutor(t *testing.T) {
	logDir := t.TempDir()

	executor := NewTaskExecutor(3, logDir, 5*time.Second)
	require.NotNil(t, executor)

	assert.Equal(t, 3, executor.maxWorkers)
	assert.Equal(t, logDir, executor.logDirectory)
	assert.Equal(t, 5*time.Second, executor.shutdownTimeout)
	assert.NotNil(t, executor.taskQueue)
	assert.NotNil(t, executor.executions)
	assert.Equal(t, 6, cap(executor.taskQueue), "Queue capacity should be maxWorkers*2")

	// Cleanup
	err := executor.Shutdown()
	assert.NoError(t, err)
}

func TestSubmitTask_Success(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:        "test-task",
		Description: "Test task",
		Timeout:     10 * time.Second,
		Steps: []config.Step{
			{
				Name:    "echo-step",
				Command: "echo 'hello world'",
			},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)
	assert.NotEmpty(t, executionID)

	// Verify execution was stored
	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.Equal(t, task.Name, execution.TaskName)
	// Status may be queued or running at this point due to concurrent processing
	assert.NotEqual(t, StatusSuccess, execution.Status, "Should not be completed yet")

	// Wait for execution to complete
	time.Sleep(500 * time.Millisecond)

	execution, err = executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, execution.Status)
}

func TestSubmitTask_MultipleSteps(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "multi-step-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "step1", Command: "echo 'step 1'"},
			{Name: "step2", Command: "echo 'step 2'"},
			{Name: "step3", Command: "echo 'step 3'"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, execution.Status)
	assert.Len(t, execution.Steps, 3)

	// Verify all steps completed
	for i, step := range execution.Steps {
		assert.Equal(t, StatusSuccess, step.Status, "Step %d should succeed", i)
		assert.NotNil(t, step.FinishedAt)
		assert.Greater(t, step.Duration, time.Duration(0))
	}
}

func TestSubmitTask_FailedStep(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "failing-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "step1", Command: "echo 'step 1'"},
			{Name: "failing-step", Command: "exit 1"},
			{Name: "step3", Command: "echo 'should not run'"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, execution.Status)
	assert.NotNil(t, execution.Error)

	// Should have 2 steps: successful first step and failed second step
	assert.Len(t, execution.Steps, 2)
	assert.Equal(t, StatusSuccess, execution.Steps[0].Status)
	assert.Equal(t, StatusFailed, execution.Steps[1].Status)
}

func TestSubmitTask_WithWorkingDirectory(t *testing.T) {
	logDir := t.TempDir()
	workDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Create a test file in working directory
	testFile := filepath.Join(workDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	task := &config.Task{
		Name:             "workdir-task",
		Timeout:          10 * time.Second,
		WorkingDirectory: workDir,
		Steps: []config.Step{
			{Name: "check-file", Command: "cat test.txt"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, execution.Status)
	assert.Contains(t, string(execution.Steps[0].Output), "test content")
}

func TestSubmitTask_WithEnvironment(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "env-task",
		Timeout: 10 * time.Second,
		Environment: map[string]string{
			"TEST_VAR": "test_value",
			"FOO":      "bar",
		},
		Steps: []config.Step{
			{Name: "check-env", Command: "echo $TEST_VAR $FOO"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, execution.Status)

	output := string(execution.Steps[0].Output)
	assert.Contains(t, output, "test_value")
	assert.Contains(t, output, "bar")
}

func TestSubmitTask_QueueFull(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(1, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Create tasks that will take time to execute
	longTask := &config.Task{
		Name:    "long-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "sleep", Command: "sleep 3"},
		},
	}

	// Fill the queue (capacity is maxWorkers*2 = 2)
	// Submit tasks rapidly to fill the queue before worker can process them
	var errs []error
	for i := 0; i < 4; i++ {
		_, err := executor.SubmitTask(longTask)
		errs = append(errs, err)
	}

	// At least one submission should fail due to queue being full
	errorCount := 0
	for _, err := range errs {
		if err != nil {
			errorCount++
			assert.Contains(t, err.Error(), "queue is full")
		}
	}
	assert.Greater(t, errorCount, 0, "At least one task submission should fail when queue is full")
}

func TestSubmitTask_AfterShutdown(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)

	// Shutdown executor
	err := executor.Shutdown()
	require.NoError(t, err)

	// Try to submit task after shutdown
	task := &config.Task{
		Name:    "test-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'test'"},
		},
	}

	_, err = executor.SubmitTask(task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestGetQueueDepthAndCapacity(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(3, logDir, 5*time.Second)
	defer executor.Shutdown()

	assert.Equal(t, 6, executor.GetQueueCapacity())
	assert.Equal(t, 0, executor.GetQueueDepth())

	// Submit a long-running task
	task := &config.Task{
		Name:    "long-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "sleep", Command: "sleep 2"},
		},
	}

	_, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Queue depth may be 0 or 1 depending on worker processing speed
	depth := executor.GetQueueDepth()
	assert.True(t, depth >= 0 && depth <= 1)
}

func TestShutdown_Graceful(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)

	// Submit a quick task
	task := &config.Task{
		Name:    "quick-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'test'"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Shutdown should wait for task to complete
	err = executor.Shutdown()
	assert.NoError(t, err)

	// Task should have completed
	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)
	assert.NotEqual(t, StatusQueued, execution.Status)
}

func TestShutdown_Timeout(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(1, logDir, 100*time.Millisecond)

	// Submit a long-running task that exceeds shutdown timeout
	task := &config.Task{
		Name:    "very-long-task",
		Timeout: 30 * time.Second,
		Steps: []config.Step{
			{Name: "sleep", Command: "sleep 5"},
		},
	}

	_, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Give task time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown should timeout
	err = executor.Shutdown()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestGetExecution_NotFound(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	_, err := executor.GetExecution("non-existent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListExecutions(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Submit multiple tasks with same name
	task := &config.Task{
		Name:    "list-test-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'test'"},
		},
	}

	_, err := executor.SubmitTask(task)
	require.NoError(t, err)

	_, err = executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for executions to start
	time.Sleep(200 * time.Millisecond)

	executions, err := executor.ListExecutions("list-test-task")
	require.NoError(t, err)
	assert.Len(t, executions, 2)
}

func TestConcurrentTaskExecution(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(3, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "concurrent-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "sleep", Command: "sleep 0.1"},
		},
	}

	// Submit 5 tasks concurrently
	executionIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		id, err := executor.SubmitTask(task)
		require.NoError(t, err)
		executionIDs[i] = id
	}

	// Wait for all executions
	time.Sleep(1 * time.Second)

	// Verify all completed successfully
	for _, id := range executionIDs {
		execution, err := executor.GetExecution(id)
		require.NoError(t, err)
		assert.Equal(t, StatusSuccess, execution.Status)
	}
}

func TestLogFileCreation(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "log-test-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'test log output'"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution and log writing
	time.Sleep(500 * time.Millisecond)

	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)

	// Verify log file was created
	assert.NotEmpty(t, execution.LogFilePath)
	assert.FileExists(t, execution.LogFilePath)

	// Verify log file contains expected content
	content, err := os.ReadFile(execution.LogFilePath)
	require.NoError(t, err)

	logContent := string(content)
	assert.Contains(t, logContent, "log-test-task")
	assert.Contains(t, logContent, "test log output")
	assert.Contains(t, logContent, execution.ID)
}

func TestTaskExecutionTiming(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "timing-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "sleep", Command: "sleep 0.1"},
		},
	}

	executionID, err := executor.SubmitTask(task)
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	execution, err := executor.GetExecution(executionID)
	require.NoError(t, err)

	// Verify timing information
	assert.False(t, execution.StartedAt.IsZero())
	assert.NotNil(t, execution.FinishedAt)
	assert.Greater(t, execution.Duration, time.Duration(0))
	assert.Greater(t, execution.Duration, 100*time.Millisecond)
}
func TestReadLogFile_Success(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "test.log")

	content := "test log content\nline 2\nline 3"
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	result, err := ReadLogFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(result))
}

func TestReadLogFile_EmptyPath(t *testing.T) {
	_, err := ReadLogFile("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log file path is empty")
}

func TestReadLogFile_NonExistent(t *testing.T) {
	_, err := ReadLogFile("/nonexistent/path/to/log.log")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read log file")
}

func TestListTaskLogs_NoDirectory(t *testing.T) {
	logDir := t.TempDir()

	logs, err := ListTaskLogs(logDir, "nonexistent-task")
	require.NoError(t, err)
	assert.Empty(t, logs)
}

func TestListTaskLogs_EmptyDirectory(t *testing.T) {
	logDir := t.TempDir()
	taskDir := filepath.Join(logDir, "test-task")
	err := os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	logs, err := ListTaskLogs(logDir, "test-task")
	require.NoError(t, err)
	assert.Empty(t, logs)
}

func TestListTaskLogs_WithLogs(t *testing.T) {
	logDir := t.TempDir()
	taskDir := filepath.Join(logDir, "test-task")
	err := os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	// Create multiple log files
	log1 := filepath.Join(taskDir, "exec1.log")
	log2 := filepath.Join(taskDir, "exec2.log")
	log3 := filepath.Join(taskDir, "exec3.log")

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(log1, []byte("log1"), 0644)
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(log2, []byte("log2"), 0644)
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(log3, []byte("log3"), 0644)

	// Create a non-log file (should be ignored)
	os.WriteFile(filepath.Join(taskDir, "other.txt"), []byte("other"), 0644)

	logs, err := ListTaskLogs(logDir, "test-task")
	require.NoError(t, err)
	assert.Len(t, logs, 3)

	// Should be sorted newest first
	assert.Contains(t, logs[0], "exec3.log")
}

func TestGetLogFilePath_Success(t *testing.T) {
	logDir := t.TempDir()
	taskDir := filepath.Join(logDir, "test-task")
	err := os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	executionID := "abc12345-6789-0123-4567-890123456789"
	logFile := filepath.Join(taskDir, "test-task_abc12345.log")
	os.WriteFile(logFile, []byte("log"), 0644)

	path, err := GetLogFilePath(logDir, "test-task", executionID)
	require.NoError(t, err)
	assert.Equal(t, logFile, path)
}

func TestGetLogFilePath_NoDirectory(t *testing.T) {
	logDir := t.TempDir()

	_, err := GetLogFilePath(logDir, "nonexistent-task", "exec-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no logs found for task")
}

func TestGetLogFilePath_NotFound(t *testing.T) {
	logDir := t.TempDir()
	taskDir := filepath.Join(logDir, "test-task")
	err := os.MkdirAll(taskDir, 0755)
	require.NoError(t, err)

	_, err = GetLogFilePath(logDir, "test-task", "nonexistent-exec")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log file not found for execution")
}

func TestTailLogFile_Success(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "test.log")

	content := "line 1\nline 2\nline 3\nline 4\nline 5"
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	lines, err := TailLogFile(logFile, 3)
	require.NoError(t, err)
	assert.Len(t, lines, 3)
	assert.Equal(t, "line 3", lines[0])
	assert.Equal(t, "line 4", lines[1])
	assert.Equal(t, "line 5", lines[2])
}

func TestTailLogFile_LessThanRequested(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "test.log")

	content := "line 1\nline 2"
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	lines, err := TailLogFile(logFile, 10)
	require.NoError(t, err)
	assert.Len(t, lines, 2)
	assert.Equal(t, "line 1", lines[0])
	assert.Equal(t, "line 2", lines[1])
}

func TestTailLogFile_InvalidPath(t *testing.T) {
	_, err := TailLogFile("/nonexistent/file.log", 10)
	assert.Error(t, err)
}

func TestGetLatestExecution(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Add test executions
	now := time.Now()
	exec1 := &Execution{
		ID:        "exec1",
		TaskName:  "test-task",
		Status:    StatusSuccess,
		StartedAt: now.Add(-2 * time.Hour),
	}
	exec2 := &Execution{
		ID:        "exec2",
		TaskName:  "test-task",
		Status:    StatusSuccess,
		StartedAt: now.Add(-1 * time.Hour),
	}
	exec3 := &Execution{
		ID:        "exec3",
		TaskName:  "other-task",
		Status:    StatusSuccess,
		StartedAt: now,
	}

	executor.AddTestExecution("exec1", exec1)
	executor.AddTestExecution("exec2", exec2)
	executor.AddTestExecution("exec3", exec3)

	// Get latest for test-task
	latest, err := executor.GetLatestExecution("test-task")
	require.NoError(t, err)
	assert.Equal(t, "exec2", latest.ID)
}

func TestGetLatestExecution_NotFound(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	_, err := executor.GetLatestExecution("nonexistent-task")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executions found")
}

func TestGetStats(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Add test executions with different statuses
	executor.AddTestExecution("exec1", &Execution{Status: StatusRunning})
	executor.AddTestExecution("exec2", &Execution{Status: StatusSuccess})
	executor.AddTestExecution("exec3", &Execution{Status: StatusSuccess})
	executor.AddTestExecution("exec4", &Execution{Status: StatusFailed})
	executor.AddTestExecution("exec5", &Execution{Status: StatusQueued})
	executor.AddTestExecution("exec6", &Execution{Status: StatusQueued})

	stats := executor.GetStats()
	assert.Equal(t, 6, stats.Total)
	assert.Equal(t, 1, stats.Running)
	assert.Equal(t, 2, stats.Success)
	assert.Equal(t, 1, stats.Failed)
	assert.Equal(t, 2, stats.Queued)
}

func TestGetStats_Empty(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	stats := executor.GetStats()
	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 0, stats.Running)
	assert.Equal(t, 0, stats.Success)
	assert.Equal(t, 0, stats.Failed)
	assert.Equal(t, 0, stats.Queued)
}

func TestGetTaskStatus_Success(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	now := time.Now()
	executor.AddTestExecution("exec1", &Execution{
		TaskName:  "test-task",
		Status:    StatusSuccess,
		StartedAt: now,
	})

	status := executor.GetTaskStatus("test-task")
	assert.Equal(t, "success", status)
}

func TestGetTaskStatus_Idle(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	status := executor.GetTaskStatus("nonexistent-task")
	assert.Equal(t, "idle", status)
}

func TestAddTestExecution(t *testing.T) {
	logDir := t.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	exec := &Execution{
		ID:       "test-exec",
		TaskName: "test-task",
		Status:   StatusSuccess,
	}

	executor.AddTestExecution("test-exec", exec)

	// Verify it was added
	retrieved, err := executor.GetExecution("test-exec")
	require.NoError(t, err)
	assert.Equal(t, exec.ID, retrieved.ID)
	assert.Equal(t, exec.TaskName, retrieved.TaskName)
	assert.Equal(t, exec.Status, retrieved.Status)
}

// Benchmark tests for critical paths

func BenchmarkTaskSubmission(b *testing.B) {
	logDir := b.TempDir()
	executor := NewTaskExecutor(10, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "bench-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'benchmark'"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.SubmitTask(task)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTaskExecution(b *testing.B) {
	logDir := b.TempDir()
	executor := NewTaskExecutor(10, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "bench-exec-task",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'test'"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executionID, err := executor.SubmitTask(task)
		if err != nil {
			b.Fatal(err)
		}

		// Wait for completion
		for {
			exec, err := executor.GetExecution(executionID)
			if err != nil {
				b.Fatal(err)
			}
			if exec.Status != StatusRunning && exec.Status != StatusQueued {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func BenchmarkConcurrentTaskExecution(b *testing.B) {
	logDir := b.TempDir()
	executor := NewTaskExecutor(10, logDir, 5*time.Second)
	defer executor.Shutdown()

	task := &config.Task{
		Name:    "bench-concurrent",
		Timeout: 10 * time.Second,
		Steps: []config.Step{
			{Name: "echo", Command: "echo 'concurrent test'"},
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			executionID, err := executor.SubmitTask(task)
			if err != nil {
				b.Error(err)
				continue
			}

			// Wait for completion
			for {
				exec, err := executor.GetExecution(executionID)
				if err != nil {
					b.Error(err)
					break
				}
				if exec.Status != StatusRunning && exec.Status != StatusQueued {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	})
}

func BenchmarkGetExecution(b *testing.B) {
	logDir := b.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Create test execution
	exec := &Execution{
		ID:       "bench-exec",
		TaskName: "bench-task",
		Status:   StatusSuccess,
	}
	executor.AddTestExecution("bench-exec", exec)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.GetExecution("bench-exec")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListExecutions(b *testing.B) {
	logDir := b.TempDir()
	executor := NewTaskExecutor(2, logDir, 5*time.Second)
	defer executor.Shutdown()

	// Create multiple test executions
	for i := 0; i < 100; i++ {
		exec := &Execution{
			ID:       "exec-" + string(rune(i)),
			TaskName: "bench-task",
			Status:   StatusSuccess,
		}
		executor.AddTestExecution(exec.ID, exec)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.ListExecutions("bench-task")
		if err != nil {
			b.Fatal(err)
		}
	}
}
