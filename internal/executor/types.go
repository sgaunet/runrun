package executor

import (
	"context"
	"sync"
	"time"

	"github.com/sgaunet/runrun/internal/config"
)

// ExecutionStatus represents the status of a task execution
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusQueued    ExecutionStatus = "queued"
	StatusRunning   ExecutionStatus = "running"
	StatusSuccess   ExecutionStatus = "success"
	StatusFailed    ExecutionStatus = "failed"
	StatusCancelled ExecutionStatus = "cancelled"
	StatusTimeout   ExecutionStatus = "timeout"
)

// Execution represents a task execution instance
type Execution struct {
	ID          string
	TaskName    string
	Task        *config.Task
	Status      ExecutionStatus
	StartedAt   time.Time
	FinishedAt  *time.Time
	Duration    time.Duration
	Steps       []*StepExecution
	Error       error
	LogFilePath string
}

// StepExecution represents a single step execution
type StepExecution struct {
	Name       string
	Command    string
	Status     ExecutionStatus
	StartedAt  time.Time
	FinishedAt *time.Time
	Duration   time.Duration
	ExitCode   int
	Output     []byte
	Error      error
}

// TaskRequest represents a request to execute a task
type TaskRequest struct {
	ExecutionID string
	Task        *config.Task
	Context     context.Context
}

// TaskExecutor manages task execution with worker pool
type TaskExecutor struct {
	// Configuration
	maxWorkers     int
	logDirectory   string
	shutdownTimeout time.Duration

	// Task queue
	taskQueue chan *TaskRequest

	// Worker management
	workerWg sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc

	// State management
	executions      map[string]*Execution
	executionsMutex sync.RWMutex
}

// StateManager interface for execution state tracking
type StateManager interface {
	GetExecution(executionID string) (*Execution, error)
	UpdateExecution(executionID string, execution *Execution) error
	ListExecutions(taskName string) ([]*Execution, error)
}

// StepExecutor interface for executing individual steps
type StepExecutor interface {
	ExecuteStep(ctx context.Context, step *config.Step, workingDir string, env map[string]string) (*StepExecution, error)
}
