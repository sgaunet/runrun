package executor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/sgaunet/runrun/internal/config"
)

// shellPath and shellFlag define how step commands are invoked: through a
// shell so operators can use pipes, variable expansion, and other shell
// features in their configured commands.
const (
	shellPath = "/bin/sh"
	shellFlag = "-c"
)

// DefaultStepExecutor implements step execution using os/exec.
type DefaultStepExecutor struct {
	logDirectory string
	broadcaster  LogBroadcaster
	executionID  string
}

// ExecuteStep executes a single step.
func (s *DefaultStepExecutor) ExecuteStep(
	ctx context.Context, step *config.Step, workingDir string, env map[string]string,
) (*StepExecution, error) {
	stepExec := &StepExecution{
		Name:      step.Name,
		Command:   step.Command,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}

	cmd := buildStepCommand(ctx, step, workingDir, env)

	var stdout, stderr bytes.Buffer
	var err error
	if s.broadcaster != nil && s.executionID != "" {
		err = s.runWithBroadcast(cmd, &stdout, &stderr)
	} else {
		err = runCommand(cmd, &stdout, &stderr)
	}

	return finalizeStepExecution(stepExec, step.Name, stdout.Bytes(), stderr.Bytes(), err)
}

// buildStepCommand constructs the exec.Cmd for a step, applying the
// configured working directory and environment variables.
func buildStepCommand(ctx context.Context, step *config.Step, workingDir string, env map[string]string) *exec.Cmd {
	// Run through a shell for proper variable expansion, pipes, etc. The
	// command text comes from the operator's trusted YAML task config, not
	// from end-user input.
	cmd := exec.CommandContext(ctx, shellPath, shellFlag, step.Command) //nolint:gosec // G204: trusted operator task config

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	cmd.Env = os.Environ() // Start with system environment
	for key, value := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	return cmd
}

// runCommand executes cmd capturing stdout/stderr, without real-time
// broadcasting.
func runCommand(cmd *exec.Cmd, stdout, stderr *bytes.Buffer) error {
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command run failed: %w", err)
	}

	return nil
}

// runWithBroadcast executes cmd while streaming stdout/stderr lines to the
// broadcaster in real time, in addition to capturing them into stdout/stderr.
func (s *DefaultStepExecutor) runWithBroadcast(cmd *exec.Cmd, stdout, stderr *bytes.Buffer) error {
	stdoutPR, stdoutPW := io.Pipe()
	stderrPR, stderrPW := io.Pipe()

	cmd.Stdout = io.MultiWriter(stdout, stdoutPW)
	cmd.Stderr = io.MultiWriter(stderr, stderrPW)

	var wg sync.WaitGroup
	wg.Add(broadcastReaderCount)

	// Goroutine to read stdout lines and broadcast
	go func() {
		defer wg.Done()
		s.streamLines(stdoutPR, "")
	}()

	// Goroutine to read stderr lines and broadcast
	go func() {
		defer wg.Done()
		s.streamLines(stderrPR, "error")
	}()

	// Execute command
	err := cmd.Run()

	// Close pipe writers so scanner goroutines finish. io.PipeWriter.Close
	// never returns a non-nil error, so there is nothing to check here.
	_ = stdoutPW.Close()
	_ = stderrPW.Close()

	// Wait for all lines to be broadcast before continuing
	wg.Wait()

	if err != nil {
		return fmt.Errorf("command run failed: %w", err)
	}

	return nil
}

// streamLines scans r line by line and broadcasts each line to the step's
// executionID. An empty level broadcasts at the broadcaster's default level;
// any other value is passed through as the log level (e.g. "error").
func (s *DefaultStepExecutor) streamLines(r io.Reader, level string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, scannerInitialBufferSize), scannerMaxLineSize)
	for scanner.Scan() {
		if level == "" {
			s.broadcaster.BroadcastLog(s.executionID, scanner.Text())
		} else {
			s.broadcaster.BroadcastLogWithLevel(s.executionID, scanner.Text(), level)
		}
	}
}

// finalizeStepExecution records timing, output, and exit code on stepExec
// based on the command's outcome, returning the (possibly wrapped) step error.
func finalizeStepExecution(
	stepExec *StepExecution, stepName string, stdout, stderr []byte, err error,
) (*StepExecution, error) {
	finishTime := time.Now()
	stepExec.FinishedAt = &finishTime
	stepExec.Duration = finishTime.Sub(stepExec.StartedAt)

	output := make([]byte, 0, len(stdout)+len(stderr))
	output = append(output, stdout...)
	output = append(output, stderr...)
	stepExec.Output = output

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stepExec.ExitCode = exitErr.ExitCode()
		} else {
			stepExec.ExitCode = -1
		}
		stepExec.Status = StatusFailed
		stepExec.Error = err
		return stepExec, fmt.Errorf("step '%s' failed: %w", stepName, err)
	}

	stepExec.ExitCode = 0
	stepExec.Status = StatusSuccess
	return stepExec, nil
}

// WriteLogFile writes execution logs to a file.
func WriteLogFile(execution *Execution, logDirectory string) error {
	if execution == nil {
		return ErrExecutionNil
	}

	logFilePath, err := buildLogFilePath(execution, logDirectory)
	if err != nil {
		return err
	}

	// logFilePath is built from the configured log directory plus the
	// execution's task name and internally generated ID/timestamp — not from
	// external input.
	file, err := os.Create(logFilePath) //nolint:gosec // G304: path built from internal execution id within configured log dir
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("failed to close log file %s: %v", logFilePath, closeErr)
		}
	}()

	if err := writeLogHeader(file, execution); err != nil {
		return err
	}

	for i, step := range execution.Steps {
		if err := writeStepLog(file, i, len(execution.Steps), step); err != nil {
			return err
		}
	}

	if err := writeLogFooter(file, execution); err != nil {
		return err
	}

	execution.LogFilePath = logFilePath
	return nil
}

// buildLogFilePath creates the task's log directory if needed and returns
// the path of the log file for execution.
func buildLogFilePath(execution *Execution, logDirectory string) (string, error) {
	// Create log directory structure: logs/{taskName}/
	taskLogDir := filepath.Join(logDirectory, execution.TaskName)
	if err := os.MkdirAll(taskLogDir, logDirPermissions); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}

	// Log filename: {taskName}/{YYYY-MM-DD}_{HH-MM-SS}_{executionID}.log
	timestamp := execution.StartedAt.Format(logFileNameTimeFormat)
	executionIDShort := execution.ID
	if len(executionIDShort) > executionIDShortLen {
		executionIDShort = executionIDShort[:executionIDShortLen]
	}
	logFileName := fmt.Sprintf("%s_%s.log", timestamp, executionIDShort)

	return filepath.Join(taskLogDir, logFileName), nil
}

// writeLogHeader writes the execution header (start time, working directory,
// environment variables) to file.
func writeLogHeader(file *os.File, execution *Execution) error {
	startedAt := execution.StartedAt.Format(logTimestampFormat)

	if _, err := fmt.Fprintf(file, "[%s] Execution started: %s (ID: %s)\n",
		startedAt, execution.TaskName, execution.ID); err != nil {
		return fmt.Errorf("failed to write execution header: %w", err)
	}

	if execution.Task.WorkingDirectory != "" {
		if _, err := fmt.Fprintf(file, "[%s] Working directory: %s\n",
			startedAt, execution.Task.WorkingDirectory); err != nil {
			return fmt.Errorf("failed to write working directory: %w", err)
		}
	}

	if len(execution.Task.Environment) > 0 {
		if _, err := fmt.Fprintf(file, "[%s] Environment variables:\n", startedAt); err != nil {
			return fmt.Errorf("failed to write environment header: %w", err)
		}
		for key, value := range execution.Task.Environment {
			if _, err := fmt.Fprintf(file, "  %s=%s\n", key, value); err != nil {
				return fmt.Errorf("failed to write environment variable: %w", err)
			}
		}
	}

	if _, err := fmt.Fprintf(file, "[%s] %s\n", startedAt, logSeparator); err != nil {
		return fmt.Errorf("failed to write header separator: %w", err)
	}

	return nil
}

// writeStepLog writes a single step's command, output, and completion line
// to file. index is 0-based; total is the number of steps in the execution.
func writeStepLog(file *os.File, index, total int, step *StepExecution) error {
	startedAt := step.StartedAt.Format(logTimestampFormat)

	if _, err := fmt.Fprintf(file, "[%s] Step %d/%d: %s\n", startedAt, index+1, total, step.Name); err != nil {
		return fmt.Errorf("failed to write step header: %w", err)
	}
	if _, err := fmt.Fprintf(file, "[%s] Command: %s\n", startedAt, step.Command); err != nil {
		return fmt.Errorf("failed to write step command: %w", err)
	}

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
		if _, err := fmt.Fprintf(file, "[%s] Step completed (exit code: %d, duration: %s)\n",
			step.FinishedAt.Format(logTimestampFormat), step.ExitCode, step.Duration); err != nil {
			return fmt.Errorf("failed to write step completion: %w", err)
		}
	}

	if _, err := fmt.Fprintf(file, "[%s] %s\n", startedAt, logSeparator); err != nil {
		return fmt.Errorf("failed to write step separator: %w", err)
	}

	return nil
}

// writeLogFooter writes the execution footer (completion time, duration,
// and error, if any) to file.
func writeLogFooter(file *os.File, execution *Execution) error {
	if execution.FinishedAt != nil {
		finishedAt := execution.FinishedAt.Format(logTimestampFormat)
		if _, err := fmt.Fprintf(file, "[%s] Execution completed: %s\n", finishedAt, execution.Status); err != nil {
			return fmt.Errorf("failed to write execution completion: %w", err)
		}
		if _, err := fmt.Fprintf(file, "[%s] Total duration: %s\n", finishedAt, execution.Duration); err != nil {
			return fmt.Errorf("failed to write execution duration: %w", err)
		}
	}

	if execution.Error != nil {
		if _, err := fmt.Fprintf(file, "[%s] Error: %v\n",
			time.Now().Format(logTimestampFormat), execution.Error); err != nil {
			return fmt.Errorf("failed to write execution error: %w", err)
		}
	}

	return nil
}
