package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Sentinel errors returned while loading a configuration file.
var (
	// ErrConfigFileNotFound indicates the configuration file path does not exist.
	ErrConfigFileNotFound = errors.New("configuration file not found")
	// ErrConfigFileEmpty indicates the configuration file exists but has no content.
	ErrConfigFileEmpty = errors.New("configuration file is empty")
)

// LoadConfig loads and validates a configuration file.
func LoadConfig(path string) (*Config, error) {
	// Parse YAML file
	config, err := parseYAML(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables
	expandEnvVars(config)

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// parseYAML reads and unmarshals a YAML configuration file.
func parseYAML(path string) (*Config, error) {
	// The config path is an operator-provided CLI flag (trusted input), not
	// end-user data. Clean it to normalize traversal segments before use.
	cleanPath := filepath.Clean(path)

	// Check if file exists
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrConfigFileNotFound, cleanPath)
	}

	// Read file content
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Check if file is empty
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrConfigFileEmpty, cleanPath)
	}

	// Unmarshal YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}
