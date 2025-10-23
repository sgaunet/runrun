package executor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/sgaunet/runrun/internal/config"
)

// NewTaskExecutor creates a new task executor with worker pool
func NewTaskExecutor(maxWorkers int, logDirectory string, shutdownTimeout time.Duration) *TaskExecutor {
	ctx, cancel := context.WithCancel(context.Background())

	executor := &TaskExecutor{
		maxWorkers:      maxWorkers,
		logDirectory:    logDirectory,
		shutdownTimeout: shutdownTimeout,
		taskQueue:       make(chan *TaskRequest, maxWorkers*2), // Buffer for smooth operation
		ctx:             ctx,
		cancel:          cancel,
		executions:      make(map[string]*Execution),
	}

	// Start worker pool
	executor.startWorkers()

	return executor
}

// startWorkers initializes the worker pool
func (e *TaskExecutor) startWorkers() {
	for i := 0; i < e.maxWorkers; i++ {
		e.workerWg.Add(1)
		go e.worker(i)
	}
	log.Printf("Started %d task executor workers", e.maxWorkers)
}

// worker processes tasks from the queue
func (e *TaskExecutor) worker(id int) {
	defer e.workerWg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Worker %d recovered from panic: %v", id, r)
			// Restart worker
			e.workerWg.Add(1)
			go e.worker(id)
		}
	}()

	log.Printf("Worker %d started", id)

	for {
		select {
		case <-e.ctx.Done():
			log.Printf("Worker %d shutting down", id)
			return
		case req := <-e.taskQueue:
			if req == nil {
				continue
			}
			log.Printf("Worker %d executing task: %s (execution ID: %s)", id, req.Task.Name, req.ExecutionID)
			e.executeTask(req)
		}
	}
}

// SubmitTask submits a task for execution
func (e *TaskExecutor) SubmitTask(task *config.Task) (string, error) {
	select {
	case <-e.ctx.Done():
		return "", fmt.Errorf("executor is shutting down")
	default:
	}

	// Generate execution ID
	executionID := uuid.New().String()

	// Create execution record
	execution := &Execution{
		ID:       executionID,
		TaskName: task.Name,
		Task:     task,
		Status:   StatusQueued,
		Steps:    make([]*StepExecution, 0, len(task.Steps)),
	}

	// Store execution
	e.executionsMutex.Lock()
	e.executions[executionID] = execution
	e.executionsMutex.Unlock()

	// Create task request
	req := &TaskRequest{
		ExecutionID: executionID,
		Task:        task,
		Context:     context.Background(),
	}

	// Submit to queue (non-blocking)
	select {
	case e.taskQueue <- req:
		log.Printf("Task '%s' queued for execution (ID: %s)", task.Name, executionID)
		return executionID, nil
	default:
		// Queue is full
		e.updateExecutionStatus(executionID, StatusFailed)
		return "", fmt.Errorf("task queue is full, cannot accept new tasks")
	}
}

// executeTask executes a task with all its steps
func (e *TaskExecutor) executeTask(req *TaskRequest) {
	executionID := req.ExecutionID
	task := req.Task

	// Update status to running
	e.updateExecutionStatus(executionID, StatusRunning)
	e.setExecutionStartTime(executionID, time.Now())

	// Create step executor
	stepExec := &DefaultStepExecutor{
		logDirectory: e.logDirectory,
	}

	// Execute steps sequentially
	var lastError error
	for i, step := range task.Steps {
		log.Printf("Executing step %d/%d for task '%s': %s", i+1, len(task.Steps), task.Name, step.Name)

		// Create context with timeout
		stepCtx, cancel := context.WithTimeout(req.Context, task.Timeout)

		// Execute step
		stepExec, err := stepExec.ExecuteStep(stepCtx, &step, task.WorkingDirectory, task.Environment)
		cancel()

		// Store step execution
		e.addStepExecution(executionID, stepExec)

		if err != nil {
			log.Printf("Step '%s' failed for task '%s': %v", step.Name, task.Name, err)
			lastError = err
			break
		}

		log.Printf("Step '%s' completed successfully for task '%s'", step.Name, task.Name)
	}

	// Update final status
	finishTime := time.Now()
	if lastError != nil {
		e.updateExecutionStatus(executionID, StatusFailed)
		e.setExecutionError(executionID, lastError)
	} else {
		e.updateExecutionStatus(executionID, StatusSuccess)
	}
	e.setExecutionFinishTime(executionID, finishTime)

	// Write log file
	execution, err := e.GetExecution(executionID)
	if err == nil {
		if err := WriteLogFile(execution, e.logDirectory); err != nil {
			log.Printf("Failed to write log file for execution %s: %v", executionID, err)
		} else {
			e.setLogFilePath(executionID, execution.LogFilePath)
			log.Printf("Log file written: %s", execution.LogFilePath)
		}
	}

	log.Printf("Task '%s' execution completed with status: %s (ID: %s)",
		task.Name, e.getExecutionStatus(executionID), executionID)
}

// Shutdown gracefully shuts down the executor
func (e *TaskExecutor) Shutdown() error {
	log.Println("Shutting down task executor...")

	// Stop accepting new tasks
	e.cancel()

	// Close task queue
	close(e.taskQueue)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		e.workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All workers stopped gracefully")
		return nil
	case <-time.After(e.shutdownTimeout):
		log.Println("Worker shutdown timeout exceeded")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// GetQueueDepth returns the current queue depth
func (e *TaskExecutor) GetQueueDepth() int {
	return len(e.taskQueue)
}

// GetQueueCapacity returns the queue capacity
func (e *TaskExecutor) GetQueueCapacity() int {
	return cap(e.taskQueue)
}
