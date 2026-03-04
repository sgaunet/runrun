package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads and validates a configuration file
func LoadConfig(filepath string) (*Config, error) {
	// Parse YAML file
	config, err := parseYAML(filepath)
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

// parseYAML reads and unmarshals a YAML configuration file
func parseYAML(filepath string) (*Config, error) {
	// Check if file exists
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", filepath)
	}

	// Read file content
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Check if file is empty
	if len(data) == 0 {
		return nil, fmt.Errorf("configuration file is empty: %s", filepath)
	}

	// Unmarshal YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}
