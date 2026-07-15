package websocket

import (
	"strings"
)

// LogLevel represents a log severity level.
type LogLevel string

// Recognized log severity levels, from least to most severe.
const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// validLevels is the set of recognized log levels.
var validLevels = map[string]LogLevel{
	string(LogLevelDebug): LogLevelDebug,
	string(LogLevelInfo):  LogLevelInfo,
	string(LogLevelWarn):  LogLevelWarn,
	string(LogLevelError): LogLevelError,
}

// ParseLogLevel detects the log level from a log line.
// It checks for common patterns like [INFO], [WARN], [ERROR], [DEBUG],
// as well as lowercase variants and structured log formats.
// Returns the detected level, or "info" as default.
func ParseLogLevel(line string) string {
	lower := strings.ToLower(line)

	// Check for bracketed patterns: [ERROR], [WARN], [INFO], [DEBUG]
	// Also handles lowercase and mixed case
	for _, pattern := range []struct {
		substr string
		level  string
	}{
		{"[error]", string(LogLevelError)},
		{"[err]", string(LogLevelError)},
		{"[fatal]", string(LogLevelError)},
		{"[warn]", string(LogLevelWarn)},
		{"[warning]", string(LogLevelWarn)},
		{"[info]", string(LogLevelInfo)},
		{"[debug]", string(LogLevelDebug)},
		{"[trace]", string(LogLevelDebug)},
		// Common structured log formats: level=error, level=ERROR
		{"level=error", string(LogLevelError)},
		{"level=fatal", string(LogLevelError)},
		{"level=warn", string(LogLevelWarn)},
		{"level=warning", string(LogLevelWarn)},
		{"level=info", string(LogLevelInfo)},
		{"level=debug", string(LogLevelDebug)},
		// Space-separated: ERROR , WARN , etc.
		{" error ", string(LogLevelError)},
		{" fatal ", string(LogLevelError)},
		{" warn ", string(LogLevelWarn)},
		{" warning ", string(LogLevelWarn)},
	} {
		if strings.Contains(lower, pattern.substr) {
			return pattern.level
		}
	}

	return string(LogLevelInfo)
}

// ParseLevelFilter parses a comma-separated level filter string into a set of levels.
// Returns nil if the filter is empty or "all", meaning no filtering.
func ParseLevelFilter(filter string) map[string]bool {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" || filter == "all" {
		return nil
	}

	levels := make(map[string]bool)
	for part := range strings.SplitSeq(filter, ",") {
		part = strings.TrimSpace(part)
		if _, ok := validLevels[part]; ok {
			levels[part] = true
		}
	}

	if len(levels) == 0 {
		return nil
	}
	return levels
}

// MatchesFilter returns true if the given level passes the filter.
// A nil filter matches everything.
func MatchesFilter(level string, filter map[string]bool) bool {
	if filter == nil {
		return true
	}
	if level == "" {
		level = string(LogLevelInfo)
	}
	return filter[level]
}
