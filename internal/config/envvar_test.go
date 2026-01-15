package config

import (
	"testing"
)

func TestAutoConvertToEnvVar(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"database.host", "DATABASE_HOST"},
		{"app.log_level", "APP_LOG_LEVEL"},
		{"api.key-name", "API_KEY_NAME"},
		{"simple", "SIMPLE"},
		{"deeply.nested.key.value", "DEEPLY_NESTED_KEY_VALUE"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := autoConvertToEnvVar(tt.input)
			if result != tt.expected {
				t.Errorf("autoConvertToEnvVar(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToEnvVars(t *testing.T) {
	flat := map[string]string{
		"database.host":     "localhost",
		"database.password": "secret",
		"app.name":          "myapp",
	}

	mapping := map[string]string{
		"database.password": "DB_PASS",
	}

	result := ToEnvVars(flat, mapping)

	// Check explicit mapping
	if result["DB_PASS"] != "secret" {
		t.Errorf("expected DB_PASS=secret, got %q", result["DB_PASS"])
	}

	// Check auto-conversion
	if result["DATABASE_HOST"] != "localhost" {
		t.Errorf("expected DATABASE_HOST=localhost, got %q", result["DATABASE_HOST"])
	}

	if result["APP_NAME"] != "myapp" {
		t.Errorf("expected APP_NAME=myapp, got %q", result["APP_NAME"])
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "\"with space\""},
		{"with\nnewline", "\"with\\nnewline\""},
		{"with$dollar", "\"with\\$dollar\""},
		{"with\"quote", "\"with\\\"quote\""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := quoteIfNeeded(tt.input)
			if result != tt.expected {
				t.Errorf("quoteIfNeeded(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
