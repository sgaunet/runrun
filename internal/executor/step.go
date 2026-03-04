package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sgaunet/runrun/internal/config"
)

// DefaultStepExecutor implements step execution using os/exec
type DefaultStepExecutor struct {
	logDirectory string
}

// ExecuteStep executes a single step
func (s *DefaultStepExecutor) ExecuteStep(ctx context.Context, step *config.Step, workingDir string, env map[string]string) (*StepExecution, error) {
	stepExec := &StepExecution{
		Name:      step.Name,
		Command:   step.Command,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}

	// Create command - run through shell for proper variable expansion
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", step.Command)

	// Set working directory
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Set environment variables
	cmd.Env = os.Environ() // Start with system environment
	for key, value := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Record finish time
	finishTime := time.Now()
	stepExec.FinishedAt = &finishTime
	stepExec.Duration = finishTime.Sub(stepExec.StartedAt)

	// Combine stdout and stderr
	output := append(stdout.Bytes(), stderr.Bytes()...)
	stepExec.Output = output

	// Get exit code
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			stepExec.ExitCode = exitErr.ExitCode()
		} else {
			stepExec.ExitCode = -1
		}
		stepExec.Status = StatusFailed
		stepExec.Error = err
		return stepExec, fmt.Errorf("step '%s' failed: %w", step.Name, err)
	}

	stepExec.ExitCode = 0
	stepExec.Status = StatusSuccess
	return stepExec, nil
}

// WriteLogFile writes execution logs to a file
func WriteLogFile(execution *Execution, logDirectory string) error {
	if execution == nil {
		return fmt.Errorf("execution is nil")
	}

	// Create log directory structure: logs/{taskName}/
	taskLogDir := filepath.Join(logDirectory, execution.TaskName)
	if err := os.MkdirAll(taskLogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Log filename: {taskName}/{YYYY-MM-DD}_{HH-MM-SS}_{executionID}.log
	timestamp := execution.StartedAt.Format("2006-01-02_15-04-05")
	executionIDShort := execution.ID
	if len(executionIDShort) > 8 {
		executionIDShort = executionIDShort[:8]
	}
	logFileName := fmt.Sprintf("%s_%s.log", timestamp, executionIDShort)
	logFilePath := filepath.Join(taskLogDir, logFileName)

	// Create log file
	file, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer file.Close()

	// Write execution header
	fmt.Fprintf(file, "[%s] Execution started: %s (ID: %s)\n",
		execution.StartedAt.Format("2006-01-02 15:04:05"),
		execution.TaskName,
		execution.ID)

	if execution.Task.WorkingDirectory != "" {
		fmt.Fprintf(file, "[%s] Working directory: %s\n",
			execution.StartedAt.Format("2006-01-02 15:04:05"),
			execution.Task.WorkingDirectory)
	}

	if len(execution.Task.Environment) > 0 {
		fmt.Fprintf(file, "[%s] Environment variables:\n",
			execution.StartedAt.Format("2006-01-02 15:04:05"))
		for key, value := range execution.Task.Environment {
			fmt.Fprintf(file, "  %s=%s\n", key, value)
		}
	}

	fmt.Fprintf(file, "[%s] %s\n",
		execution.StartedAt.Format("2006-01-02 15:04:05"),
		"─────────────────────────────────────────────")

	// Write step logs
	for i, step := range execution.Steps {
		fmt.Fprintf(file, "[%s] Step %d/%d: %s\n",
			step.StartedAt.Format("2006-01-02 15:04:05"),
			i+1,
			len(execution.Steps),
			step.Name)
		fmt.Fprintf(file, "[%s] Command: %s\n",
			step.StartedAt.Format("2006-01-02 15:04:05"),
			step.Command)

		// Write step output
		if len(step.Output) > 0 {
			if _, err := file.Write(step.Output); err != nil {
				return fmt.Errorf("failed to write step output: %w", err)
			}
			if step.Output[len(step.Output)-1] != '\n' {
				if _, err := file.WriteString("\n"); err != nil {
					return fmt.Errorf("failed to write newline: %w", err)
				}
			}
		}

		if step.FinishedAt != nil {
			fmt.Fprintf(file, "[%s] Step completed (exit code: %d, duration: %s)\n",
				step.FinishedAt.Format("2006-01-02 15:04:05"),
				step.ExitCode,
				step.Duration)
		}

		fmt.Fprintf(file, "[%s] %s\n",
			step.StartedAt.Format("2006-01-02 15:04:05"),
			"─────────────────────────────────────────────")
	}

	// Write execution footer
	if execution.FinishedAt != nil {
		fmt.Fprintf(file, "[%s] Execution completed: %s\n",
			execution.FinishedAt.Format("2006-01-02 15:04:05"),
			execution.Status)
		fmt.Fprintf(file, "[%s] Total duration: %s\n",
			execution.FinishedAt.Format("2006-01-02 15:04:05"),
			execution.Duration)
	}

	if execution.Error != nil {
		fmt.Fprintf(file, "[%s] Error: %v\n",
			time.Now().Format("2006-01-02 15:04:05"),
			execution.Error)
	}

	execution.LogFilePath = logFilePath
	return nil
}
