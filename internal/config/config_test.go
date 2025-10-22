package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_ValidConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
  log_level: "info"
  max_concurrent_tasks: 5
  session_timeout: 24h
  log_directory: "./logs"
  shutdown_timeout: 5m

auth:
  jwt_secret: "this-is-a-very-long-secret-key-for-jwt-signing-at-least-32-chars"
  users:
    - username: "admin"
      password: "hashed-password"

tasks:
  - name: "test-task"
    description: "Test task"
    tags: ["test"]
    timeout: 1m
    steps:
      - name: "step1"
        command: "echo test"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration
	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify server config
	if config.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", config.Server.Port)
	}
	if config.Server.LogLevel != "info" {
		t.Errorf("Expected log_level 'info', got %s", config.Server.LogLevel)
	}
	if config.Server.MaxConcurrentTasks != 5 {
		t.Errorf("Expected max_concurrent_tasks 5, got %d", config.Server.MaxConcurrentTasks)
	}
	if config.Server.SessionTimeout != 24*time.Hour {
		t.Errorf("Expected session_timeout 24h, got %v", config.Server.SessionTimeout)
	}

	// Verify auth config
	if len(config.Auth.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(config.Auth.Users))
	}
	if config.Auth.Users[0].Username != "admin" {
		t.Errorf("Expected username 'admin', got %s", config.Auth.Users[0].Username)
	}

	// Verify tasks
	if len(config.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(config.Tasks))
	}
	if config.Tasks[0].Name != "test-task" {
		t.Errorf("Expected task name 'test-task', got %s", config.Tasks[0].Name)
	}
	if len(config.Tasks[0].Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(config.Tasks[0].Steps))
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml")
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 99999
  log_level: "info"
  max_concurrent_tasks: 5
  session_timeout: 24h
  log_directory: "./logs"
  shutdown_timeout: 5m

auth:
  jwt_secret: "this-is-a-very-long-secret-key-for-jwt-signing"
  users:
    - username: "admin"
      password: "hashed-password"

tasks:
  - name: "test"
    description: "Test"
    timeout: 1m
    steps:
      - name: "step1"
        command: "echo test"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Error("Expected validation error for invalid port, got nil")
	}
}

func TestLoadConfig_DuplicateTaskNames(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
  log_level: "info"
  max_concurrent_tasks: 5
  session_timeout: 24h
  log_directory: "./logs"
  shutdown_timeout: 5m

auth:
  jwt_secret: "this-is-a-very-long-secret-key-for-jwt-signing"
  users:
    - username: "admin"
      password: "hashed-password"

tasks:
  - name: "task1"
    description: "Task 1"
    timeout: 1m
    steps:
      - name: "step1"
        command: "echo 1"
  - name: "task1"
    description: "Task 1 duplicate"
    timeout: 1m
    steps:
      - name: "step2"
        command: "echo 2"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Error("Expected error for duplicate task names, got nil")
	}
}

func TestExpandEnvVars(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_VAR", "test_value")
	os.Setenv("TEST_PORT", "9000")
	defer os.Unsetenv("TEST_VAR")
	defer os.Unsetenv("TEST_PORT")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple expansion",
			input:    "$TEST_VAR",
			expected: "test_value",
		},
		{
			name:     "Braces expansion",
			input:    "${TEST_VAR}",
			expected: "test_value",
		},
		{
			name:     "Default value - var exists",
			input:    "${TEST_VAR:-default}",
			expected: "test_value",
		},
		{
			name:     "Default value - var missing",
			input:    "${MISSING_VAR:-default}",
			expected: "default",
		},
		{
			name:     "Mixed text",
			input:    "prefix_${TEST_VAR}_suffix",
			expected: "prefix_test_value_suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandString(tt.input)
			if result != tt.expected {
				t.Errorf("expandString(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
