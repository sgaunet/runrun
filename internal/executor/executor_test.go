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
	var ids []string
	var errs []error
	for i := 0; i < 4; i++ {
		id, err := executor.SubmitTask(longTask)
		ids = append(ids, id)
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
