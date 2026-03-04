package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sgaunet/runrun/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteStep_Success(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "echo-test",
		Command: "echo 'hello world'",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "echo-test", result.Name)
	assert.Equal(t, step.Command, result.Command)
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Output), "hello world")
	assert.False(t, result.StartedAt.IsZero())
	assert.NotNil(t, result.FinishedAt)
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestExecuteStep_Failure(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "failing-step",
		Command: "exit 42",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	assert.Error(t, err)
	require.NotNil(t, result)

	assert.Equal(t, StatusFailed, result.Status)
	assert.Equal(t, 42, result.ExitCode)
	assert.NotNil(t, result.Error)
	assert.Contains(t, err.Error(), "failed")
}

func TestExecuteStep_WithWorkingDirectory(t *testing.T) {
	stepExec := &DefaultStepExecutor{}
	workDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(workDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	step := &config.Step{
		Name:    "read-file",
		Command: "cat test.txt",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, workDir, nil)
	require.NoError(t, err)
	assert.Contains(t, string(result.Output), "test content")
}

func TestExecuteStep_WithEnvironment(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	env := map[string]string{
		"TEST_VAR": "test_value",
		"FOO":      "bar",
	}

	step := &config.Step{
		Name:    "env-test",
		Command: "echo $TEST_VAR $FOO",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", env)
	require.NoError(t, err)

	output := string(result.Output)
	assert.Contains(t, output, "test_value")
	assert.Contains(t, output, "bar")
}

func TestExecuteStep_WithTimeout(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	step := &config.Step{
		Name:    "timeout-step",
		Command: "sleep 5",
	}

	result, err := stepExec.ExecuteStep(ctx, step, "", nil)
	assert.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusFailed, result.Status)
}

func TestExecuteStep_OutputCapture(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "output-test",
		Command: "echo stdout; echo stderr >&2",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	require.NoError(t, err)

	output := string(result.Output)
	// Both stdout and stderr should be captured
	assert.Contains(t, output, "stdout")
	assert.Contains(t, output, "stderr")
}

func TestExecuteStep_MultilineOutput(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "multiline-test",
		Command: "echo 'line 1'; echo 'line 2'; echo 'line 3'",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	require.NoError(t, err)

	output := string(result.Output)
	assert.Contains(t, output, "line 1")
	assert.Contains(t, output, "line 2")
	assert.Contains(t, output, "line 3")
}

func TestExecuteStep_InvalidCommand(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "invalid-command",
		Command: "nonexistentcommand12345",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	assert.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusFailed, result.Status)
	assert.NotEqual(t, 0, result.ExitCode)
}

func TestWriteLogFile_Success(t *testing.T) {
	logDir := t.TempDir()

	now := time.Now()
	finishTime := now.Add(2 * time.Second)

	execution := &Execution{
		ID:         "test-exec-id-12345678",
		TaskName:   "test-task",
		Status:     StatusSuccess,
		StartedAt:  now,
		FinishedAt: &finishTime,
		Duration:   2 * time.Second,
		Task: &config.Task{
			Name:             "test-task",
			WorkingDirectory: "/tmp/test",
			Environment: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
		},
		Steps: []*StepExecution{
			{
				Name:       "step1",
				Command:    "echo 'test'",
				Status:     StatusSuccess,
				StartedAt:  now,
				FinishedAt: &finishTime,
				Duration:   1 * time.Second,
				ExitCode:   0,
				Output:     []byte("test output\n"),
			},
		},
	}

	err := WriteLogFile(execution, logDir)
	require.NoError(t, err)

	// Verify log file was created
	assert.NotEmpty(t, execution.LogFilePath)
	assert.FileExists(t, execution.LogFilePath)

	// Verify log file is in correct directory
	expectedDir := filepath.Join(logDir, "test-task")
	assert.True(t, strings.HasPrefix(execution.LogFilePath, expectedDir))

	// Read and verify log content
	content, err := os.ReadFile(execution.LogFilePath)
	require.NoError(t, err)

	logContent := string(content)
	assert.Contains(t, logContent, "test-task")
	assert.Contains(t, logContent, "test-exec-id")
	assert.Contains(t, logContent, "Working directory: /tmp/test")
	assert.Contains(t, logContent, "VAR1=value1")
	assert.Contains(t, logContent, "VAR2=value2")
	assert.Contains(t, logContent, "Step 1/1: step1")
	assert.Contains(t, logContent, "echo 'test'")
	assert.Contains(t, logContent, "test output")
	assert.Contains(t, logContent, "exit code: 0")
	assert.Contains(t, logContent, "Execution completed: success")
}

func TestWriteLogFile_WithError(t *testing.T) {
	logDir := t.TempDir()

	now := time.Now()
	finishTime := now.Add(1 * time.Second)

	execution := &Execution{
		ID:         "error-exec-id",
		TaskName:   "error-task",
		Status:     StatusFailed,
		StartedAt:  now,
		FinishedAt: &finishTime,
		Duration:   1 * time.Second,
		Error:      assert.AnError,
		Task: &config.Task{
			Name: "error-task",
		},
		Steps: []*StepExecution{
			{
				Name:       "failing-step",
				Command:    "exit 1",
				Status:     StatusFailed,
				StartedAt:  now,
				FinishedAt: &finishTime,
				Duration:   500 * time.Millisecond,
				ExitCode:   1,
				Output:     []byte("error output\n"),
				Error:      assert.AnError,
			},
		},
	}

	err := WriteLogFile(execution, logDir)
	require.NoError(t, err)

	content, err := os.ReadFile(execution.LogFilePath)
	require.NoError(t, err)

	logContent := string(content)
	assert.Contains(t, logContent, "error-task")
	assert.Contains(t, logContent, "exit code: 1")
	assert.Contains(t, logContent, "Error:")
}

func TestWriteLogFile_MultipleSteps(t *testing.T) {
	logDir := t.TempDir()

	now := time.Now()
	finishTime := now.Add(3 * time.Second)

	execution := &Execution{
		ID:         "multi-step-id",
		TaskName:   "multi-step-task",
		Status:     StatusSuccess,
		StartedAt:  now,
		FinishedAt: &finishTime,
		Duration:   3 * time.Second,
		Task: &config.Task{
			Name: "multi-step-task",
		},
		Steps: []*StepExecution{
			{
				Name:       "step1",
				Command:    "echo 'first'",
				Status:     StatusSuccess,
				StartedAt:  now,
				FinishedAt: &finishTime,
				Duration:   1 * time.Second,
				ExitCode:   0,
				Output:     []byte("first\n"),
			},
			{
				Name:       "step2",
				Command:    "echo 'second'",
				Status:     StatusSuccess,
				StartedAt:  now.Add(1 * time.Second),
				FinishedAt: &finishTime,
				Duration:   1 * time.Second,
				ExitCode:   0,
				Output:     []byte("second\n"),
			},
			{
				Name:       "step3",
				Command:    "echo 'third'",
				Status:     StatusSuccess,
				StartedAt:  now.Add(2 * time.Second),
				FinishedAt: &finishTime,
				Duration:   1 * time.Second,
				ExitCode:   0,
				Output:     []byte("third\n"),
			},
		},
	}

	err := WriteLogFile(execution, logDir)
	require.NoError(t, err)

	content, err := os.ReadFile(execution.LogFilePath)
	require.NoError(t, err)

	logContent := string(content)
	assert.Contains(t, logContent, "Step 1/3: step1")
	assert.Contains(t, logContent, "Step 2/3: step2")
	assert.Contains(t, logContent, "Step 3/3: step3")
	assert.Contains(t, logContent, "first")
	assert.Contains(t, logContent, "second")
	assert.Contains(t, logContent, "third")
}

func TestWriteLogFile_NilExecution(t *testing.T) {
	logDir := t.TempDir()

	err := WriteLogFile(nil, logDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestWriteLogFile_InvalidDirectory(t *testing.T) {
	// Use a file as the log directory (should fail)
	tempFile := filepath.Join(t.TempDir(), "file.txt")
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	require.NoError(t, err)

	now := time.Now()
	execution := &Execution{
		ID:        "test-id",
		TaskName:  "test-task",
		StartedAt: now,
		Task: &config.Task{
			Name: "test-task",
		},
		Steps: []*StepExecution{},
	}

	// Should fail because tempFile is a file, not a directory
	err = WriteLogFile(execution, tempFile)
	assert.Error(t, err)
}

func TestExecuteStep_ShellExpansion(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "shell-expansion",
		Command: "echo $HOME | wc -c",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	require.NoError(t, err)

	// Should have output (HOME should be expanded)
	assert.NotEmpty(t, result.Output)
	assert.Equal(t, StatusSuccess, result.Status)
}

func TestExecuteStep_Pipes(t *testing.T) {
	stepExec := &DefaultStepExecutor{}

	step := &config.Step{
		Name:    "pipe-test",
		Command: "echo 'test' | grep 'test'",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, "", nil)
	require.NoError(t, err)
	assert.Contains(t, string(result.Output), "test")
}

func TestExecuteStep_ComplexCommand(t *testing.T) {
	stepExec := &DefaultStepExecutor{}
	workDir := t.TempDir()

	step := &config.Step{
		Name:    "complex-command",
		Command: "for i in 1 2 3; do echo $i; done",
	}

	result, err := stepExec.ExecuteStep(context.Background(), step, workDir, nil)
	require.NoError(t, err)

	output := string(result.Output)
	assert.Contains(t, output, "1")
	assert.Contains(t, output, "2")
	assert.Contains(t, output, "3")
}
