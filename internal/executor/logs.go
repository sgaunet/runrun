package executor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time
	Message   string
}

// ReadLogFile reads a log file and returns its contents.
func ReadLogFile(logFilePath string) ([]byte, error) {
	if logFilePath == "" {
		return nil, ErrEmptyLogPath
	}

	// logFilePath is always execution.LogFilePath, built internally by
	// WriteLogFile from the configured log directory and an internally
	// generated execution ID — not from external input.
	content, err := os.ReadFile(logFilePath) //nolint:gosec // G304: path built from internal execution id within configured log dir
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	return content, nil
}

// ListTaskLogs lists all log files for a task.
func ListTaskLogs(logDirectory, taskName string) ([]string, error) {
	taskLogDir := filepath.Join(logDirectory, taskName)

	// Check if directory exists
	if _, err := os.Stat(taskLogDir); os.IsNotExist(err) {
		return []string{}, nil // No logs yet
	}

	// Read directory
	entries, err := os.ReadDir(taskLogDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}

	// Collect log files
	var logFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".log") {
			logFiles = append(logFiles, filepath.Join(taskLogDir, entry.Name()))
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(logFiles, func(i, j int) bool {
		info1, _ := os.Stat(logFiles[i])
		info2, _ := os.Stat(logFiles[j])
		return info1.ModTime().After(info2.ModTime())
	})

	return logFiles, nil
}

// GetLogFilePath returns the log file path for an execution.
func GetLogFilePath(logDirectory, taskName, executionID string) (string, error) {
	taskLogDir := filepath.Join(logDirectory, taskName)

	// Check if directory exists
	if _, err := os.Stat(taskLogDir); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s", ErrNoLogsForTask, taskName)
	}

	// Find log file with execution ID
	entries, err := os.ReadDir(taskLogDir)
	if err != nil {
		return "", fmt.Errorf("failed to read log directory: %w", err)
	}

	executionIDShort := executionID
	if len(executionIDShort) > executionIDShortLen {
		executionIDShort = executionIDShort[:executionIDShortLen]
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.Contains(entry.Name(), executionIDShort) && strings.HasSuffix(entry.Name(), ".log") {
			return filepath.Join(taskLogDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrLogFileNotFound, executionID)
}

// CountFileLines counts the number of lines in a file efficiently.
func CountFileLines(filePath string) (int, error) {
	// filePath is always execution.LogFilePath, built internally by
	// WriteLogFile from the configured log directory and an internally
	// generated execution ID — not from external input.
	f, err := os.Open(filePath) //nolint:gosec // G304: path built from internal execution id within configured log dir
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = f.Close() // read-only file descriptor; nothing to flush, safe to ignore
	}()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, scannerInitialBufferSize), scannerMaxLineSize)
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("error reading file: %w", err)
	}

	return count, nil
}

// ReadLogSegment reads a range of lines from a log file in a single pass.
// startLine is 0-indexed. Returns the lines, total line count, and any error.
func ReadLogSegment(logFilePath string, startLine, count int) ([]string, int, error) {
	if startLine < 0 {
		startLine = 0
	}

	// logFilePath is always execution.LogFilePath, built internally by
	// WriteLogFile from the configured log directory and an internally
	// generated execution ID — not from external input.
	f, err := os.Open(logFilePath) //nolint:gosec // G304: path built from internal execution id within configured log dir
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() {
		_ = f.Close() // read-only file descriptor; nothing to flush, safe to ignore
	}()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, scannerInitialBufferSize), scannerMaxLineSize)

	lines := make([]string, 0, count)
	lineNum := 0

	for scanner.Scan() {
		if lineNum >= startLine && len(lines) < count {
			lines = append(lines, scanner.Text())
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return lines, lineNum, fmt.Errorf("error reading log file: %w", err)
	}

	return lines, lineNum, nil
}

// TailLogFile returns the last N lines from a log file.
func TailLogFile(logFilePath string, lines int) ([]string, error) {
	content, err := ReadLogFile(logFilePath)
	if err != nil {
		return nil, err
	}

	// Split into lines
	allLines := strings.Split(string(content), "\n")

	// Remove empty last line if present
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}

	// Return last N lines
	if len(allLines) <= lines {
		return allLines, nil
	}

	return allLines[len(allLines)-lines:], nil
}
