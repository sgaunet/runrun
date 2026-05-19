package websocket

import "testing"

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		// Bracketed formats
		{"[ERROR] something went wrong", "error"},
		{"[WARN] disk space low", "warn"},
		{"[INFO] server started", "info"},
		{"[DEBUG] variable x = 42", "debug"},
		{"[error] lowercase bracket", "error"},
		{"[WARNING] extended warning", "warn"},
		{"[ERR] short error", "error"},
		{"[FATAL] critical failure", "error"},
		{"[TRACE] trace message", "debug"},

		// Structured log formats
		{"time=2024-01-01 level=error msg=failed", "error"},
		{"time=2024-01-01 level=warn msg=slow", "warn"},
		{"time=2024-01-01 level=info msg=ok", "info"},
		{"time=2024-01-01 level=debug msg=trace", "debug"},
		{"level=fatal crash occurred", "error"},
		{"level=warning potential issue", "warn"},

		// Space-separated
		{"2024-01-01 ERROR failed to connect", "error"},
		{"2024-01-01 WARN connection slow", "warn"},

		// Default (no level detected)
		{"just a regular log line", "info"},
		{"", "info"},
		{"123 some numbers", "info"},
	}

	for _, tt := range tests {
		got := ParseLogLevel(tt.line)
		if got != tt.expected {
			t.Errorf("ParseLogLevel(%q) = %q, want %q", tt.line, got, tt.expected)
		}
	}
}

func TestParseLevelFilter(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string]bool
	}{
		{"", nil},
		{"all", nil},
		{"error", map[string]bool{"error": true}},
		{"error,warn", map[string]bool{"error": true, "warn": true}},
		{"ERROR,WARN", map[string]bool{"error": true, "warn": true}},
		{" error , warn ", map[string]bool{"error": true, "warn": true}},
		{"invalid", nil},
		{"error,invalid,warn", map[string]bool{"error": true, "warn": true}},
		{"debug,info,warn,error", map[string]bool{"debug": true, "info": true, "warn": true, "error": true}},
	}

	for _, tt := range tests {
		got := ParseLevelFilter(tt.input)
		if tt.expected == nil {
			if got != nil {
				t.Errorf("ParseLevelFilter(%q) = %v, want nil", tt.input, got)
			}
			continue
		}
		if len(got) != len(tt.expected) {
			t.Errorf("ParseLevelFilter(%q) = %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for k := range tt.expected {
			if !got[k] {
				t.Errorf("ParseLevelFilter(%q) missing key %q", tt.input, k)
			}
		}
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		level    string
		filter   map[string]bool
		expected bool
	}{
		// nil filter matches everything
		{"error", nil, true},
		{"info", nil, true},
		{"", nil, true},

		// Specific filter
		{"error", map[string]bool{"error": true}, true},
		{"warn", map[string]bool{"error": true}, false},
		{"info", map[string]bool{"error": true, "warn": true}, false},
		{"warn", map[string]bool{"error": true, "warn": true}, true},

		// Empty level defaults to info
		{"", map[string]bool{"info": true}, true},
		{"", map[string]bool{"error": true}, false},
	}

	for _, tt := range tests {
		got := MatchesFilter(tt.level, tt.filter)
		if got != tt.expected {
			t.Errorf("MatchesFilter(%q, %v) = %v, want %v", tt.level, tt.filter, got, tt.expected)
		}
	}
}
