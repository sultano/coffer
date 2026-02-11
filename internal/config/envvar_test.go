package config

import (
	"strings"
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

func TestQuoteForDotenv(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "\"with space\""},
		{"with\nnewline", "\"with\\nnewline\""},
		{"with$dollar", "\"with\\$dollar\""},
		{"with\"quote", "\"with\\\"quote\""},
		{"with#hash", "\"with#hash\""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := QuoteForDotenv(tt.input)
			if result != tt.expected {
				t.Errorf("QuoteForDotenv(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCleanEnvVarName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid name", "DB_HOST", "DB_HOST"},
		{"leading digit stripped", "1FOO", "FOO"},
		{"hyphens to underscores", "API-KEY", "API_KEY"},
		{"special chars stripped", "FOO@BAR!BAZ", "FOOBARBAZ"},
		{"empty string", "", ""},
		{"digits after first char", "VAR2", "VAR2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanEnvVarName(tt.input)
			if got != tt.expected {
				t.Errorf("cleanEnvVarName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatAsDotenv(t *testing.T) {
	t.Run("basic formatting", func(t *testing.T) {
		envVars := map[string]string{
			"DB_HOST": "localhost",
		}

		result := FormatAsDotenv(envVars)

		if !strings.Contains(result, "DB_HOST=localhost") {
			t.Errorf("expected DB_HOST=localhost in output, got: %s", result)
		}
		if !strings.HasSuffix(result, "\n") {
			t.Error("expected output to end with newline")
		}
	})

	t.Run("values needing quotes", func(t *testing.T) {
		envVars := map[string]string{
			"MSG": "hello world",
		}

		result := FormatAsDotenv(envVars)

		if !strings.Contains(result, `MSG="hello world"`) {
			t.Errorf("expected quoted value, got: %s", result)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		result := FormatAsDotenv(map[string]string{})
		if result != "" {
			t.Errorf("expected empty string for empty map, got: %q", result)
		}
	})
}
