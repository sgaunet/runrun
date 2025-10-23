package templates

import "time"

// BaseData contains data available to all templates
type BaseData struct {
	Title       string
	CurrentUser string
	CSRFToken   string
}

// LoginPageData contains data for the login page
type LoginPageData struct {
	BaseData
	Error string
}

// TaskCard represents a task card for display
type TaskCard struct {
	Name        string
	Description string
	Tags        []string
	Status      string
	LastRun     *time.Time
	Duration    string
}

// DashboardStats represents dashboard statistics
type DashboardStats struct {
	TotalTasks      int
	RunningTasks    int
	SuccessTasks    int
	FailedTasks     int
	IdleTasks       int
	QueuedTasks     int
	TotalExecutions int
}

// DashboardPageData contains data for the dashboard page
type DashboardPageData struct {
	BaseData
	Tasks []TaskCard
	Stats DashboardStats
}

// ExecutionInfo represents an execution for display
type ExecutionInfo struct {
	ID         string
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
	Duration   string
}

// TaskDetailPageData contains data for the task detail page
type TaskDetailPageData struct {
	BaseData
	TaskName    string
	Description string
	Tags        []string
	Status      string
	Executions  []ExecutionInfo
}
