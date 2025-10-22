package config

import "time"

// Config represents the root configuration structure
type Config struct {
	Server ServerConfig `yaml:"server" validate:"required"`
	Auth   AuthConfig   `yaml:"auth" validate:"required"`
	Tasks  []Task       `yaml:"tasks" validate:"required,min=1,dive"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port               int           `yaml:"port" validate:"required,min=1,max=65535"`
	LogLevel           string        `yaml:"log_level" validate:"required,oneof=debug info warn error"`
	MaxConcurrentTasks int           `yaml:"max_concurrent_tasks" validate:"required,min=1"`
	SessionTimeout     time.Duration `yaml:"session_timeout" validate:"required"`
	LogDirectory       string        `yaml:"log_directory" validate:"required"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout" validate:"required"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret" validate:"required,min=32"`
	Users     []User `yaml:"users" validate:"required,min=1,dive"`
}

// User represents a user account
type User struct {
	Username string `yaml:"username" validate:"required"`
	Password string `yaml:"password" validate:"required"` // BCrypt hash
}

// Task represents a task definition
type Task struct {
	Name             string            `yaml:"name" validate:"required"`
	Description      string            `yaml:"description" validate:"required"`
	Tags             []string          `yaml:"tags"`
	Timeout          time.Duration     `yaml:"timeout" validate:"required"`
	WorkingDirectory string            `yaml:"working_directory"`
	Environment      map[string]string `yaml:"environment"`
	Steps            []Step            `yaml:"steps" validate:"required,min=1,dive"`
}

// Step represents a single execution step within a task
type Step struct {
	Name    string `yaml:"name" validate:"required"`
	Command string `yaml:"command" validate:"required"`
}
