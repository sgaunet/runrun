package executor

import "errors"

// Sentinel errors returned by the executor package. Callers can match against
// these with errors.Is after unwrapping.
var (
	// ErrExecutorShuttingDown is returned by SubmitTask when the executor is
	// no longer accepting new tasks.
	ErrExecutorShuttingDown = errors.New("executor is shutting down")

	// ErrQueueFull is returned by SubmitTask when the task queue is at capacity.
	ErrQueueFull = errors.New("task queue is full, cannot accept new tasks")

	// ErrShutdownTimeout is returned by Shutdown when workers do not stop
	// within the configured shutdown timeout.
	ErrShutdownTimeout = errors.New("shutdown timeout exceeded")

	// ErrEmptyLogPath is returned by ReadLogFile when given an empty path.
	ErrEmptyLogPath = errors.New("log file path is empty")

	// ErrNoLogsForTask is returned when no log directory exists for a task.
	ErrNoLogsForTask = errors.New("no logs found for task")

	// ErrLogFileNotFound is returned when no log file matches the requested
	// execution ID.
	ErrLogFileNotFound = errors.New("log file not found for execution")

	// ErrExecutionNotFound is returned when an execution ID is unknown.
	ErrExecutionNotFound = errors.New("execution not found")

	// ErrNoExecutionsForTask is returned when a task has no recorded executions.
	ErrNoExecutionsForTask = errors.New("no executions found for task")

	// ErrExecutionNil is returned by WriteLogFile when passed a nil execution.
	ErrExecutionNil = errors.New("execution is nil")
)
