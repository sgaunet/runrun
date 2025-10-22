package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Regex patterns for environment variable expansion
var (
	// Matches ${VAR_NAME} or ${VAR_NAME:-default_value}
	envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]*))?\}`)
	// Matches $VAR_NAME
	simpleEnvVarPattern = regexp.MustCompile(`\$([A-Za-z0-9_]+)`)
)

// expandEnvVars expands environment variables in the configuration
func expandEnvVars(config *Config) error {
	// Expand server config
	config.Server.LogDirectory = expandString(config.Server.LogDirectory)

	// Expand auth config
	config.Auth.JWTSecret = expandString(config.Auth.JWTSecret)

	// Expand tasks
	for i := range config.Tasks {
		task := &config.Tasks[i]
		task.Name = expandString(task.Name)
		task.Description = expandString(task.Description)
		task.WorkingDirectory = expandString(task.WorkingDirectory)

		// Expand environment variables map
		if task.Environment != nil {
			expandedEnv := make(map[string]string)
			for key, value := range task.Environment {
				expandedEnv[key] = expandString(value)
			}
			task.Environment = expandedEnv
		}

		// Expand steps
		for j := range task.Steps {
			step := &task.Steps[j]
			step.Name = expandString(step.Name)
			step.Command = expandString(step.Command)
		}
	}

	return nil
}

// expandString expands environment variables in a string
func expandString(s string) string {
	// First, expand ${VAR_NAME:-default} pattern
	s = envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		submatches := envVarPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		varName := submatches[1]
		defaultValue := ""
		if len(submatches) > 2 {
			defaultValue = submatches[2]
		}

		value := os.Getenv(varName)
		if value == "" {
			return defaultValue
		}
		return value
	})

	// Then, expand simple $VAR_NAME pattern
	s = simpleEnvVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		submatches := simpleEnvVarPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		varName := submatches[1]
		value := os.Getenv(varName)
		if value == "" {
			return match // Keep original if not found
		}
		return value
	})

	return s
}

// ValidateEnvVars checks if all required environment variables are set
func ValidateEnvVars(config *Config) error {
	missing := []string{}

	// Check all string fields for unresolved environment variables
	checkString := func(s, context string) {
		if strings.Contains(s, "${") && !envVarPattern.MatchString(s) {
			missing = append(missing, fmt.Sprintf("%s contains unresolved variable", context))
		}
	}

	checkString(config.Server.LogDirectory, "server.log_directory")
	checkString(config.Auth.JWTSecret, "auth.jwt_secret")

	for i, task := range config.Tasks {
		checkString(task.WorkingDirectory, fmt.Sprintf("tasks[%d].working_directory", i))
		for key, value := range task.Environment {
			checkString(value, fmt.Sprintf("tasks[%d].environment.%s", i, key))
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("unresolved environment variables:\n  - %s", strings.Join(missing, "\n  - "))
	}

	return nil
}
