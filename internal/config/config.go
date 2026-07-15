// Package config defines RunRun's YAML configuration schema and provides
// loading, environment variable expansion, and validation for it.
package config

import "time"

// Config represents the root configuration structure.
type Config struct {
	Server ServerConfig `validate:"required"            yaml:"server"`
	Auth   AuthConfig   `validate:"required"            yaml:"auth"`
	Tasks  []Task       `validate:"required,min=1,dive" yaml:"tasks"`
}

// ServerConfig holds server-related configuration.
type ServerConfig struct {
	Port               int           `validate:"required,min=1,max=65535"             yaml:"port"`
	LogLevel           string        `validate:"required,oneof=debug info warn error" yaml:"log_level"`
	MaxConcurrentTasks int           `validate:"required,min=1"                       yaml:"max_concurrent_tasks"`
	SessionTimeout     time.Duration `validate:"required"                             yaml:"session_timeout"`
	LogDirectory       string        `validate:"required"                             yaml:"log_directory"`
	ShutdownTimeout    time.Duration `validate:"required"                             yaml:"shutdown_timeout"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	JWTSecret string `validate:"required,min=32"     yaml:"jwt_secret"`
	Users     []User `validate:"required,min=1,dive" yaml:"users"`
}

// User represents a user account.
type User struct {
	Username string `validate:"required" yaml:"username"`
	Password string `validate:"required" yaml:"password"` // BCrypt hash
}

// Task represents a task definition.
type Task struct {
	Name             string            `validate:"required"            yaml:"name"`
	Description      string            `validate:"required"            yaml:"description"`
	Tags             []string          `yaml:"tags"`
	Timeout          time.Duration     `validate:"required"            yaml:"timeout"`
	WorkingDirectory string            `yaml:"working_directory"`
	Environment      map[string]string `yaml:"environment"`
	Steps            []Step            `validate:"required,min=1,dive" yaml:"steps"`
}

// Step represents a single execution step within a task.
type Step struct {
	Name    string `validate:"required" yaml:"name"`
	Command string `validate:"required" yaml:"command"`
}
