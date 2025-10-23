package executor

import (
	"fmt"
	"time"
)

// GetExecution retrieves an execution by ID
func (e *TaskExecutor) GetExecution(executionID string) (*Execution, error) {
	e.executionsMutex.RLock()
	defer e.executionsMutex.RUnlock()

	execution, exists := e.executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	return execution, nil
}

// ListExecutions returns all executions for a task
func (e *TaskExecutor) ListExecutions(taskName string) ([]*Execution, error) {
	e.executionsMutex.RLock()
	defer e.executionsMutex.RUnlock()

	var executions []*Execution
	for _, exec := range e.executions {
		if taskName == "" || exec.TaskName == taskName {
			executions = append(executions, exec)
		}
	}

	return executions, nil
}

// updateExecutionStatus updates the status of an execution
func (e *TaskExecutor) updateExecutionStatus(executionID string, status ExecutionStatus) {
	e.executionsMutex.Lock()
	defer e.executionsMutex.Unlock()

	if execution, exists := e.executions[executionID]; exists {
		execution.Status = status
	}
}

// setExecutionStartTime sets the start time of an execution
func (e *TaskExecutor) setExecutionStartTime(executionID string, startTime time.Time) {
	e.executionsMutex.Lock()
	defer e.executionsMutex.Unlock()

	if execution, exists := e.executions[executionID]; exists {
		execution.StartedAt = startTime
	}
}

// setExecutionFinishTime sets the finish time and calculates duration
func (e *TaskExecutor) setExecutionFinishTime(executionID string, finishTime time.Time) {
	e.executionsMutex.Lock()
	defer e.executionsMutex.Unlock()

	if execution, exists := e.executions[executionID]; exists {
		execution.FinishedAt = &finishTime
		execution.Duration = finishTime.Sub(execution.StartedAt)
	}
}

// setExecutionError sets the error for an execution
func (e *TaskExecutor) setExecutionError(executionID string, err error) {
	e.executionsMutex.Lock()
	defer e.executionsMutex.Unlock()

	if execution, exists := e.executions[executionID]; exists {
		execution.Error = err
	}
}

// addStepExecution adds a step execution to an execution
func (e *TaskExecutor) addStepExecution(executionID string, step *StepExecution) {
	e.executionsMutex.Lock()
	defer e.executionsMutex.Unlock()

	if execution, exists := e.executions[executionID]; exists {
		execution.Steps = append(execution.Steps, step)
	}
}

// getExecutionStatus gets the current status of an execution
func (e *TaskExecutor) getExecutionStatus(executionID string) ExecutionStatus {
	e.executionsMutex.RLock()
	defer e.executionsMutex.RUnlock()

	if execution, exists := e.executions[executionID]; exists {
		return execution.Status
	}

	return StatusPending
}

// setLogFilePath sets the log file path for an execution
func (e *TaskExecutor) setLogFilePath(executionID, logPath string) {
	e.executionsMutex.Lock()
	defer e.executionsMutex.Unlock()

	if execution, exists := e.executions[executionID]; exists {
		execution.LogFilePath = logPath
	}
}
