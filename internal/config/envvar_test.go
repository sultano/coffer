package config

import (
	"os"
	"os/exec"
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
		{"3rd.party", "_3RD_PARTY"},
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

	result, err := ToEnvVars(flat, mapping)
	if err != nil {
		t.Fatalf("ToEnvVars failed: %v", err)
	}

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

func TestToEnvVars_CollisionExplicitMapping(t *testing.T) {
	flat := map[string]string{
		"database.host": "localhost",
		"app.host":      "myapp.example.com",
	}

	mapping := map[string]string{
		"database.host": "HOST",
		"app.host":      "HOST",
	}

	_, err := ToEnvVars(flat, mapping)
	if err == nil {
		t.Fatal("expected error for colliding env var name, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "app.host") || !strings.Contains(msg, "database.host") || !strings.Contains(msg, "HOST") {
		t.Errorf("expected error to name both keys and the env var, got: %s", msg)
	}
}

func TestToEnvVars_CollisionAutoConversion(t *testing.T) {
	flat := map[string]string{
		"api.key": "key-one",
		"api-key": "key-two",
	}

	_, err := ToEnvVars(flat, nil)
	if err == nil {
		t.Fatal("expected error for colliding env var name, got nil")
	}

	// Keys are reported sorted, so the message is deterministic regardless of map iteration order
	msg := err.Error()
	if !strings.Contains(msg, `"api-key"`) || !strings.Contains(msg, `"api.key"`) || !strings.Contains(msg, "API_KEY") {
		t.Errorf("expected error to name both keys and the env var, got: %s", msg)
	}
}

func TestToEnvVars_InvalidName(t *testing.T) {
	tests := []struct {
		name    string
		flat    map[string]string
		mapping map[string]string
		key     string
	}{
		{"mapping target with equals", map[string]string{"db.pass": "x"}, map[string]string{"db.pass": "A=B"}, "db.pass"},
		{"mapping target with shell chars", map[string]string{"db.pass": "x"}, map[string]string{"db.pass": "X;rm"}, "db.pass"},
		{"mapping target leading digit", map[string]string{"db.pass": "x"}, map[string]string{"db.pass": "1X"}, "db.pass"},
		{"mapping target empty", map[string]string{"db.pass": "x"}, map[string]string{"db.pass": ""}, "db.pass"},
		{"derived name empty", map[string]string{"!!!": "x"}, nil, "!!!"},
		{"derived name non-ascii letter", map[string]string{"café": "x"}, nil, "café"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ToEnvVars(tt.flat, tt.mapping)
			if err == nil {
				t.Fatal("expected error for invalid env var name, got nil")
			}
			if !strings.Contains(err.Error(), `"`+tt.key+`"`) {
				t.Errorf("expected error to name config key %q, got: %s", tt.key, err)
			}
			if !strings.Contains(err.Error(), "env_mapping") {
				t.Errorf("expected error to point at env_mapping, got: %s", err)
			}
		})
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
		{"", ""},
		{"allowlisted_./:@%+,=-chars", "allowlisted_./:@%+,=-chars"},
		{"postgres://u:p@h:5432/db", "postgres://u:p@h:5432/db"},
		{"a;b", "\"a;b\""},
		{"a&b", "\"a&b\""},
		{"a|b", "\"a|b\""},
		{"a<b>c", "\"a<b>c\""},
		{"(a)", "\"(a)\""},
		{"a*", "\"a*\""},
		{"~/x", "\"~/x\""},
		{"{a,b}", "\"{a,b}\""},
		{"a!b", "\"a!b\""},
		{"it's", "\"it's\""},
		{"a`b`", "\"a\\`b\\`\""},
		{"tab\there", "\"tab\\there\""},
		{"back\\slash", "\"back\\\\slash\""},
		{"café", "\"café\""},
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

// TestQuoteForDotenv_ShellEval feeds output through the eval-and-export
// pattern people use with `coffer resolve -f dotenv` and checks that no value
// can run a command, and that the exported value survives unchanged.
func TestQuoteForDotenv_ShellEval(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	const script = `eval $(printf "%s\n" "$1" | sed "s/^/export /"); printf "%s" "$COFFER_T"`

	tests := []struct {
		name  string
		value string
		// roundTrip is false where the shell pattern itself is lossy: the
		// dotenv \n and \t escapes are not shell escapes, and the unquoted
		// $(...) collapses runs of whitespace.
		roundTrip bool
	}{
		{"semicolon redirect", "a;>pwned", true},
		{"and redirect", "a&&>pwned", true},
		{"pipe", "a|tee pwned", true},
		{"subshell", "(touch pwned)", true},
		{"command substitution", "$(touch pwned)", true},
		{"backticks", "`touch pwned`", true},
		{"double quote breakout", `a";touch pwned;"`, true},
		{"single quotes", `it's 'quoted'`, true},
		{"backslash", `a\";touch pwned;\`, true},
		{"bang", "a!b!!", true},
		{"unicode", "pässwörd-日本", true},
		{"tilde", "~/x", true},
		{"glob", "*", true},
		{"newline", "a\ntouch pwned", false},
		{"tab", "a\t;touch pwned", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			line := "COFFER_T=" + QuoteForDotenv(tt.value)

			cmd := exec.Command("sh", "-c", script, "sh", line)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sh failed for %q: %v\n%s", line, err, out)
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("eval of %q had side effects: created %s", line, entries[0].Name())
			}

			if tt.roundTrip && string(out) != tt.value {
				t.Errorf("eval of %q exported %q, want %q", line, out, tt.value)
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
		{"leading digit prefixed with underscore", "1FOO", "_1FOO"},
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
