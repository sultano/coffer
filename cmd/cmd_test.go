package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
)

// execTestCmd runs a command and captures stdout/stderr
func execTestCmd(args ...string) (string, error) {
	// Capture stdout
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldColorOutput := color.Output
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	color.Output = w

	// Reset global state
	envName = ""
	projectPath = ""
	dryRun = false
	noColor = true
	yesFlag = false

	rootCmd.SetArgs(args)
	err := rootCmd.Execute()

	// Restore stdout and read captured output
	_ = w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	color.Output = oldColorOutput

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()

	return buf.String(), err
}

// setupTestProject creates a temp project for testing
func setupTestProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
	return dir
}

func TestVersionCommand(t *testing.T) {
	output, err := execTestCmd("version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	if !strings.Contains(output, "coffer") {
		t.Errorf("version output should contain 'coffer', got: %s", output)
	}
}

func TestHelpCommand(t *testing.T) {
	// Help output goes through Cobra's internal mechanism
	// Just verify the command doesn't error
	_, err := execTestCmd("--help")
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}
}

func TestResolveCommand(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
env_mapping:
  database.host: DB_HOST
  app.name: APP_NAME
defaults:
  env: dev
`,
		"config/base.yaml": `
app:
  name: testapp
  port: 8080
database:
  host: localhost
  port: 5432
`,
		"config/dev.yaml": `
app:
  debug: true
database:
  host: dev-db
`,
	})

	// Reset global state for each test
	envName = ""
	projectPath = dir

	t.Run("outputs nested json format", func(t *testing.T) {
		output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "json")
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		// JSON should contain nested structure, not flat env vars
		if !strings.Contains(output, `"database"`) {
			t.Errorf("output should contain nested 'database' key, got: %s", output)
		}
		if !strings.Contains(output, `"host"`) {
			t.Errorf("output should contain nested 'host' key, got: %s", output)
		}
		if !strings.Contains(output, "dev-db") {
			t.Errorf("output should contain dev-db (from dev.yaml), got: %s", output)
		}
		if strings.Contains(output, "DB_HOST") {
			t.Errorf("nested JSON should not contain env var names like DB_HOST, got: %s", output)
		}
	})

	t.Run("outputs nested yaml format", func(t *testing.T) {
		output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "yaml")
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !strings.Contains(output, "database:") {
			t.Errorf("output should contain nested 'database:', got: %s", output)
		}
		if !strings.Contains(output, "host: dev-db") {
			t.Errorf("output should contain 'host: dev-db', got: %s", output)
		}
	})

	t.Run("outputs dotenv format with env var mapping", func(t *testing.T) {
		output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "dotenv")
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !strings.Contains(output, "DB_HOST=dev-db") {
			t.Errorf("output should contain DB_HOST=dev-db, got: %s", output)
		}
		if !strings.Contains(output, "APP_NAME=testapp") {
			t.Errorf("output should contain APP_NAME=testapp, got: %s", output)
		}
	})
}

func TestGetCommand(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
		"config/base.yaml": `
app:
  name: myapp
  port: 3000
database:
  host: localhost
`,
		"config/dev.yaml": `
database:
  host: dev-host
`,
	})

	t.Run("gets nested value", func(t *testing.T) {
		output, err := execTestCmd("get", "app.name", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}

		if strings.TrimSpace(output) != "myapp" {
			t.Errorf("get app.name = %q, want myapp", strings.TrimSpace(output))
		}
	})

	t.Run("gets overridden value", func(t *testing.T) {
		output, err := execTestCmd("get", "database.host", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}

		if strings.TrimSpace(output) != "dev-host" {
			t.Errorf("get database.host = %q, want dev-host", strings.TrimSpace(output))
		}
	})

	t.Run("returns error for missing key", func(t *testing.T) {
		_, err := execTestCmd("get", "nonexistent.key", "--path", dir, "--env", "dev")
		if err == nil {
			t.Fatal("expected error for missing key")
		}
	})
}

func TestInfoCommand(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
  prod:
    gcp:
      project: test-prod
defaults:
  env: dev
`,
		"config/base.yaml": `
app:
  name: myapp
`,
		"config/dev.yaml": `
app:
  env: dev
`,
	})

	output, err := execTestCmd("info", "--path", dir)
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}

	// Info command outputs to stdout with colors - just verify some content appears
	// The output contains info about the project path, environments, and config files
	if len(output) == 0 {
		t.Error("info command should produce output")
	}

	// Check for key structural elements (these appear regardless of color codes)
	if !strings.Contains(output, dir) && !strings.Contains(output, "Path") {
		t.Errorf("info output should contain project path or 'Path', got: %s", output)
	}
}

func TestInitCommand(t *testing.T) {
	dir := t.TempDir()

	// Run init with dry-run
	output, err := execTestCmd("init", "--path", dir, "--dry-run", "--gcp-project", "my-project")
	if err != nil {
		t.Fatalf("init --dry-run failed: %v", err)
	}

	if !strings.Contains(output, "Dry run") {
		t.Errorf("dry-run output should mention 'Dry run', got: %s", output)
	}

	// Verify no files were created
	if _, err := os.Stat(filepath.Join(dir, ".coffer.yaml")); err == nil {
		t.Error(".coffer.yaml should not exist in dry-run mode")
	}
}

func TestLocalOverrides(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
		"config/base.yaml": `
database:
  password: ${secret:db-password}
  host: base-host
`,
		"config/dev.yaml": `
database:
  host: dev-host
`,
		"config/local.yaml": `
database:
  password: local-password
  host: local-host
`,
	})

	// Local should override both base and dev
	output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "dotenv")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if !strings.Contains(output, "DATABASE_PASSWORD=local-password") {
		t.Errorf("local.yaml should override secret reference, got: %s", output)
	}
	if !strings.Contains(output, "DATABASE_HOST=local-host") {
		t.Errorf("local.yaml should override dev.yaml, got: %s", output)
	}
}

func TestEnvMapping(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
env_mapping:
  database.host: DB_HOST
  database.port: DB_PORT
  app.secret_key: APP_SECRET
defaults:
  env: dev
`,
		"config/base.yaml": `
database:
  host: localhost
  port: 5432
  name: mydb
app:
  secret_key: abc123
`,
		"config/dev.yaml": `
app:
  debug: true
`,
	})

	output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "dotenv")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Explicit mappings
	if !strings.Contains(output, "DB_HOST=localhost") {
		t.Errorf("should use explicit mapping DB_HOST, got: %s", output)
	}
	if !strings.Contains(output, "DB_PORT=5432") {
		t.Errorf("should use explicit mapping DB_PORT, got: %s", output)
	}
	if !strings.Contains(output, "APP_SECRET=abc123") {
		t.Errorf("should use explicit mapping APP_SECRET, got: %s", output)
	}

	// Auto-converted (not in explicit mapping)
	if !strings.Contains(output, "DATABASE_NAME=mydb") {
		t.Errorf("unmapped keys should auto-convert to DATABASE_NAME, got: %s", output)
	}
}

func TestFindUnusedSecrets(t *testing.T) {
	t.Run("finds unused secrets", func(t *testing.T) {
		gcpSecrets := []string{
			"projects/p/secrets/db-password",
			"projects/p/secrets/api-key",
			"projects/p/secrets/unused-secret",
		}
		referenced := map[string]bool{
			"db-password": true,
			"api-key":     true,
		}

		unused := findUnusedSecrets(gcpSecrets, referenced, "")

		if len(unused) != 1 {
			t.Fatalf("expected 1 unused secret, got %d", len(unused))
		}
		if unused[0] != "unused-secret" {
			t.Errorf("expected unused-secret, got %s", unused[0])
		}
	})

	t.Run("filters by prefix", func(t *testing.T) {
		gcpSecrets := []string{
			"projects/p/secrets/svc-a-db-password",
			"projects/p/secrets/svc-a-api-key",
			"projects/p/secrets/svc-b-other-secret",
		}
		referenced := map[string]bool{
			"svc-a-db-password": true,
		}

		unused := findUnusedSecrets(gcpSecrets, referenced, "svc-a-")

		if len(unused) != 1 {
			t.Fatalf("expected 1 unused secret, got %d", len(unused))
		}
		if unused[0] != "svc-a-api-key" {
			t.Errorf("expected svc-a-api-key, got %s", unused[0])
		}
	})

	t.Run("returns empty when all used", func(t *testing.T) {
		gcpSecrets := []string{
			"projects/p/secrets/db-password",
		}
		referenced := map[string]bool{
			"db-password": true,
		}

		unused := findUnusedSecrets(gcpSecrets, referenced, "")

		if len(unused) != 0 {
			t.Errorf("expected 0 unused secrets, got %d", len(unused))
		}
	})

	t.Run("returns empty when no secrets in GCP", func(t *testing.T) {
		gcpSecrets := []string{}
		referenced := map[string]bool{
			"db-password": true,
		}

		unused := findUnusedSecrets(gcpSecrets, referenced, "")

		if len(unused) != 0 {
			t.Errorf("expected 0 unused secrets, got %d", len(unused))
		}
	})
}

func TestValidateCommand(t *testing.T) {
	t.Run("passes with valid config", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
defaults:
  env: dev
`,
			"config/base.yaml": `
app:
  name: myapp
`,
			"config/dev.yaml": `
app:
  debug: true
`,
		})

		_, err := execTestCmd("validate", "--path", dir)
		if err != nil {
			t.Fatalf("validate should pass with valid config: %v", err)
		}
	})

	t.Run("fails with invalid YAML", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
`,
			"config/base.yaml": `invalid: yaml: [`,
			"config/dev.yaml": `
app:
  debug: true
`,
		})

		_, err := execTestCmd("validate", "--path", dir)
		if err == nil {
			t.Fatal("expected error for invalid YAML")
		}
	})

	t.Run("warns about missing env config file", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
  prod:
    gcp:
      project: test-prod
`,
			"config/base.yaml": `
app:
  name: myapp
`,
			"config/dev.yaml": `
app:
  debug: true
`,
			// Note: no prod.yaml
		})

		output, err := execTestCmd("validate", "--path", dir)
		if err != nil {
			t.Fatalf("validate failed: %v", err)
		}

		if !strings.Contains(output, "prod.yaml not found") {
			t.Errorf("expected warning about missing prod.yaml, got: %s", output)
		}
	})

	t.Run("warns about local.yaml not in gitignore", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
`,
			"config/base.yaml": `
app:
  name: myapp
`,
			"config/dev.yaml": `
app:
  debug: true
`,
			"config/local.yaml": `
app:
  local: true
`,
		})

		output, err := execTestCmd("validate", "--path", dir)
		if err != nil {
			t.Fatalf("validate failed: %v", err)
		}

		if !strings.Contains(output, "local.yaml") && !strings.Contains(output, ".gitignore") {
			t.Errorf("expected warning about local.yaml gitignore, got: %s", output)
		}
	})
}

func TestParseEnvFile(t *testing.T) {
	t.Run("parses simple key-value pairs", func(t *testing.T) {
		dir := t.TempDir()
		envFile := filepath.Join(dir, "test.env")
		content := `DB_PASSWORD=secret123
API_KEY=myapikey`
		if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entries, err := parseEnvFile(envFile)
		if err != nil {
			t.Fatalf("parseEnvFile failed: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}

		if entries[0].key != "DB_PASSWORD" || entries[0].value != "secret123" {
			t.Errorf("unexpected first entry: %+v", entries[0])
		}
	})

	t.Run("handles quoted values", func(t *testing.T) {
		dir := t.TempDir()
		envFile := filepath.Join(dir, "test.env")
		content := `DOUBLE="double quoted"
SINGLE='single quoted'`
		if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entries, err := parseEnvFile(envFile)
		if err != nil {
			t.Fatalf("parseEnvFile failed: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}

		if entries[0].value != "double quoted" {
			t.Errorf("expected 'double quoted', got %q", entries[0].value)
		}
		if entries[1].value != "single quoted" {
			t.Errorf("expected 'single quoted', got %q", entries[1].value)
		}
	})

	t.Run("skips comments and empty lines", func(t *testing.T) {
		dir := t.TempDir()
		envFile := filepath.Join(dir, "test.env")
		content := `# This is a comment
DB_PASSWORD=secret

# Another comment
API_KEY=key`
		if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entries, err := parseEnvFile(envFile)
		if err != nil {
			t.Fatalf("parseEnvFile failed: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("skips empty values", func(t *testing.T) {
		dir := t.TempDir()
		envFile := filepath.Join(dir, "test.env")
		content := `EMPTY=
HAS_VALUE=something`
		if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entries, err := parseEnvFile(envFile)
		if err != nil {
			t.Fatalf("parseEnvFile failed: %v", err)
		}

		if len(entries) != 1 {
			t.Fatalf("expected 1 entry (empty skipped), got %d", len(entries))
		}
	})
}

func TestEnvKeyToSecretName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"DB_PASSWORD", "db-password"},
		{"API_KEY", "api-key"},
		{"SIMPLE", "simple"},
		{"MULTI_WORD_NAME", "multi-word-name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := envKeyToSecretName(tt.input)
			if result != tt.expected {
				t.Errorf("envKeyToSecretName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ab", "****"},
		{"abcd", "****"},
		{"abcde", "ab****de"},
		{"supersecret", "su****et"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := maskValue(tt.input)
			if result != tt.expected {
				t.Errorf("maskValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCollectReferencedSecrets(t *testing.T) {
	t.Run("collects secrets from all environments", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
  prod:
    gcp:
      project: test-prod
`,
			"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": `
app:
  api_key: ${secret:dev-api-key}
`,
			"config/prod.yaml": `
app:
  api_key: ${secret:prod-api-key}
`,
		})

		project, err := config.LoadProject(dir)
		if err != nil {
			t.Fatalf("failed to load project: %v", err)
		}

		referenced := collectReferencedSecrets(dir, project, "")

		// Should find secrets from base + both envs
		if !referenced["db-password"] {
			t.Error("expected db-password to be referenced")
		}
		if !referenced["dev-api-key"] {
			t.Error("expected dev-api-key to be referenced")
		}
		if !referenced["prod-api-key"] {
			t.Error("expected prod-api-key to be referenced")
		}
	})

	t.Run("applies prefix to collected secrets", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
  secret_prefix: svc-
environments:
  dev:
    gcp:
      project: test-dev
`,
			"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": `
app:
  debug: true
`,
		})

		project, err := config.LoadProject(dir)
		if err != nil {
			t.Fatalf("failed to load project: %v", err)
		}

		referenced := collectReferencedSecrets(dir, project, "svc-")

		// Should have prefixed name
		if !referenced["svc-db-password"] {
			t.Error("expected svc-db-password to be referenced (with prefix)")
		}
		if referenced["db-password"] {
			t.Error("db-password should not be referenced without prefix")
		}
	})
}

// --- Pure function tests ---

func TestParseRunArgs(t *testing.T) {
	// Save and restore global state
	saveEnv := envName
	savePath := projectPath
	saveDry := dryRun
	t.Cleanup(func() {
		envName = saveEnv
		projectPath = savePath
		dryRun = saveDry
	})

	t.Run("flags before separator", func(t *testing.T) {
		envName = ""
		projectPath = ""
		dryRun = false

		args, err := parseRunArgs([]string{"--env", "prod", "--", "echo", "hello"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(args) != 2 || args[0] != "echo" || args[1] != "hello" {
			t.Errorf("expected [echo hello], got %v", args)
		}
		if envName != "prod" {
			t.Errorf("expected envName=prod, got %q", envName)
		}
	})

	t.Run("no separator", func(t *testing.T) {
		envName = ""
		args, err := parseRunArgs([]string{"echo", "hello"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(args) != 2 || args[0] != "echo" {
			t.Errorf("expected [echo hello], got %v", args)
		}
	})

	t.Run("dry-run flag", func(t *testing.T) {
		dryRun = false
		args, err := parseRunArgs([]string{"--dry-run", "--", "ls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !dryRun {
			t.Error("expected dryRun=true")
		}
		if len(args) != 1 || args[0] != "ls" {
			t.Errorf("expected [ls], got %v", args)
		}
	})

	t.Run("project flag", func(t *testing.T) {
		projectPath = ""
		_, err := parseRunArgs([]string{"-p", "/tmp/proj", "--", "ls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if projectPath != "/tmp/proj" {
			t.Errorf("expected projectPath=/tmp/proj, got %q", projectPath)
		}
	})

	t.Run("env short flag", func(t *testing.T) {
		envName = ""
		_, err := parseRunArgs([]string{"-e", "staging", "--", "ls"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if envName != "staging" {
			t.Errorf("expected envName=staging, got %q", envName)
		}
	})
}

func TestTrimNewline(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\n", "hello"},
		{"hello\r\n", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"\n\n", ""},
		{"line\r\n\n", "line"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got := trimNewline(tt.input)
			if got != tt.expected {
				t.Errorf("trimNewline(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUnquoteValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"double quoted"`, "double quoted"},
		{`'single quoted'`, "single quoted"},
		{"unquoted", "unquoted"},
		{"x", "x"},
		{"", ""},
		{`""`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := unquoteValue(tt.input)
			if got != tt.expected {
				t.Errorf("unquoteValue(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestOutputNestedJSON(t *testing.T) {
	values := map[string]any{
		"app": map[string]any{
			"name": "testapp",
			"port": 8080,
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputNestedJSON(values)

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("failed to read output: %v", copyErr)
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, `"app"`) || !strings.Contains(output, `"name"`) || !strings.Contains(output, `"testapp"`) {
		t.Errorf("expected nested JSON output, got: %s", output)
	}
}

func TestOutputNestedYAML(t *testing.T) {
	values := map[string]any{
		"app": map[string]any{
			"name": "testapp",
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputNestedYAML(values)

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("failed to read output: %v", copyErr)
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "app:") || !strings.Contains(output, "name: testapp") {
		t.Errorf("expected nested YAML output, got: %s", output)
	}
}

func TestOutputDotenv(t *testing.T) {
	envVars := map[string]string{
		"KEY": "value",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputDotenv(envVars)

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("failed to read output: %v", copyErr)
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "KEY=value") {
		t.Errorf("expected dotenv KEY=value, got: %s", output)
	}
}

func TestPrintDryRun(t *testing.T) {
	envVars := map[string]string{
		"DB_HOST":     "localhost",
		"DB_PASSWORD": "secret",
	}
	cmdArgs := []string{"echo", "hello"}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printDryRun(envVars, cmdArgs)

	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Dry run") {
		t.Error("expected 'Dry run' in output")
	}
	if !strings.Contains(output, "DB_HOST=localhost") {
		t.Error("expected DB_HOST in output")
	}
	if !strings.Contains(output, "Command:") {
		t.Error("expected 'Command:' in output")
	}
}

// --- Mock GCP client for integration tests ---

type mockSecretClient struct {
	secrets    map[string]string // name -> value
	secretList []string          // full GCP paths
}

func newMockSecretClient(secretData map[string]string) *mockSecretClient {
	m := &mockSecretClient{
		secrets: secretData,
	}
	for name := range secretData {
		m.secretList = append(m.secretList, fmt.Sprintf("projects/test-project/secrets/%s", name))
	}
	return m
}

func (m *mockSecretClient) GetSecret(_ context.Context, ref resolver.SecretRef, _ string) (string, error) {
	name := ref.Name
	if v, ok := m.secrets[name]; ok {
		return v, nil
	}
	return "", fmt.Errorf("secret '%s' not found", name)
}

func (m *mockSecretClient) ListSecrets(_ context.Context, _ string) ([]string, error) {
	return m.secretList, nil
}

func (m *mockSecretClient) CreateSecret(_ context.Context, _, secretName string) error {
	return nil
}

func (m *mockSecretClient) AddSecretVersion(_ context.Context, _, secretName, value string) error {
	m.secrets[secretName] = value
	return nil
}

func (m *mockSecretClient) SetSecret(_ context.Context, _, secretName, value string) error {
	m.secrets[secretName] = value
	return nil
}

func (m *mockSecretClient) DeleteSecret(_ context.Context, _, secretName string) error {
	if _, ok := m.secrets[secretName]; !ok {
		return fmt.Errorf("secret '%s' not found", secretName)
	}
	delete(m.secrets, secretName)
	return nil
}

func (m *mockSecretClient) SecretExists(_ context.Context, _, secretName string) (bool, error) {
	_, ok := m.secrets[secretName]
	return ok, nil
}

func (m *mockSecretClient) Close() error {
	return nil
}

// withMockGCPClient sets up the mock and returns a cleanup function
func withMockGCPClient(t *testing.T, mock *mockSecretClient) {
	t.Helper()
	original := newGCPClientFunc
	newGCPClientFunc = func(_ time.Duration) (*GCPClientResult, context.Context, error) {
		ctx := context.Background()
		return &GCPClientResult{Client: mock, Cancel: func() {}}, ctx, nil
	}
	t.Cleanup(func() { newGCPClientFunc = original })
}

// --- Integration tests with mocked GCP ---

func TestRunCheck_WithMock(t *testing.T) {
	t.Run("all secrets found", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password": "secret123",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": `
app:
  debug: true
`,
		})

		output, err := execTestCmd("check", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("check failed: %v", err)
		}
		if !strings.Contains(output, "db-password") {
			t.Errorf("expected db-password in output, got: %s", output)
		}
		if !strings.Contains(output, "All secrets validated") {
			t.Errorf("expected success message, got: %s", output)
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": `
app:
  debug: true
`,
		})

		output, err := execTestCmd("check", "--path", dir, "--env", "dev")
		if err == nil {
			t.Fatal("expected error for missing secret")
		}
		if !strings.Contains(output, "NOT FOUND") {
			t.Errorf("expected NOT FOUND in output, got: %s", output)
		}
	})
}

func TestRunSecretList_WithMock(t *testing.T) {
	t.Run("lists secrets", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password": "secret",
			"api-key":     "key123",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "list", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret list failed: %v", err)
		}
		if !strings.Contains(output, "db-password") {
			t.Errorf("expected db-password in output, got: %s", output)
		}
		if !strings.Contains(output, "api-key") {
			t.Errorf("expected api-key in output, got: %s", output)
		}
	})

	t.Run("filters by prefix", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"svc-db-password": "secret",
			"other-key":       "key",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
  secret_prefix: "svc-"
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "list", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret list failed: %v", err)
		}
		if !strings.Contains(output, "db-password") {
			t.Errorf("expected db-password (stripped prefix) in output, got: %s", output)
		}
		if strings.Contains(output, "other-key") {
			t.Errorf("should not show other-key (wrong prefix), got: %s", output)
		}
	})
}

func TestRunSecretGet_WithMock(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password": "supersecret",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "get", "db-password", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret get failed: %v", err)
		}
		if !strings.Contains(output, "supersecret") {
			t.Errorf("expected secret value in output, got: %s", output)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		_, err := execTestCmd("secret", "get", "missing-secret", "--path", dir, "--env", "dev")
		if err == nil {
			t.Fatal("expected error for missing secret")
		}
	})
}

func TestRunSecretSet_WithMock(t *testing.T) {
	t.Run("set with value arg", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "set", "new-secret", "myvalue", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret set failed: %v", err)
		}
		if !strings.Contains(output, "updated successfully") {
			t.Errorf("expected success message, got: %s", output)
		}
		if mock.secrets["new-secret"] != "myvalue" {
			t.Errorf("expected secret to be set, got: %q", mock.secrets["new-secret"])
		}
	})

	t.Run("dry-run", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "set", "new-secret", "myvalue", "--path", dir, "--env", "dev", "--dry-run")
		if err != nil {
			t.Fatalf("secret set --dry-run failed: %v", err)
		}
		if !strings.Contains(output, "Would set secret") {
			t.Errorf("expected dry-run message, got: %s", output)
		}
		if _, exists := mock.secrets["new-secret"]; exists {
			t.Error("secret should not be set in dry-run mode")
		}
	})
}

func TestRunSecretDelete_WithMock(t *testing.T) {
	t.Run("without --yes flag shows preview", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password": "secret",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "delete", "db-password", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret delete preview failed: %v", err)
		}
		if !strings.Contains(output, "Would delete secret") {
			t.Errorf("expected preview message, got: %s", output)
		}
		// Secret should still exist
		if _, ok := mock.secrets["db-password"]; !ok {
			t.Error("secret should still exist without --yes")
		}
	})

	t.Run("with --yes deletes", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password": "secret",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "delete", "db-password", "--yes", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret delete failed: %v", err)
		}
		if !strings.Contains(output, "deleted") {
			t.Errorf("expected deletion message, got: %s", output)
		}
		if _, ok := mock.secrets["db-password"]; ok {
			t.Error("secret should have been deleted")
		}
	})
}

func TestRunSecretUnused_WithMock(t *testing.T) {
	t.Run("finds unused secrets", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password":   "secret",
			"unused-secret": "unused",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "unused", "--path", dir)
		if err != nil {
			t.Fatalf("secret unused failed: %v", err)
		}
		if !strings.Contains(output, "unused-secret") {
			t.Errorf("expected unused-secret in output, got: %s", output)
		}
	})

	t.Run("all secrets used", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{
			"db-password": "secret",
		})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": "app:\n  debug: true\n",
		})

		output, err := execTestCmd("secret", "unused", "--path", dir)
		if err != nil {
			t.Fatalf("secret unused failed: %v", err)
		}
		if !strings.Contains(output, "No unused secrets") {
			t.Errorf("expected 'No unused secrets', got: %s", output)
		}
	})
}

func TestRunSecretImport_WithMock(t *testing.T) {
	t.Run("dry run without --yes", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		envFile := filepath.Join(dir, "secrets.env")
		if err := os.WriteFile(envFile, []byte("DB_PASSWORD=secret123\nAPI_KEY=mykey\n"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}

		output, err := execTestCmd("secret", "import", envFile, "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret import preview failed: %v", err)
		}
		if !strings.Contains(output, "2 secret(s) to import") {
			t.Errorf("expected import preview, got: %s", output)
		}
		if !strings.Contains(output, "run with --yes") {
			t.Errorf("expected --yes prompt, got: %s", output)
		}
		// Secrets should not be imported
		if len(mock.secrets) != 0 {
			t.Error("secrets should not be imported without --yes")
		}
	})

	t.Run("with --yes imports", func(t *testing.T) {
		mock := newMockSecretClient(map[string]string{})
		withMockGCPClient(t, mock)

		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
defaults:
  env: dev
`,
			"config/base.yaml": "app:\n  name: test\n",
			"config/dev.yaml":  "app:\n  debug: true\n",
		})

		envFile := filepath.Join(dir, "secrets.env")
		if err := os.WriteFile(envFile, []byte("DB_PASSWORD=secret123\nAPI_KEY=mykey\n"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}

		output, err := execTestCmd("secret", "import", envFile, "--yes", "--path", dir, "--env", "dev")
		if err != nil {
			t.Fatalf("secret import failed: %v", err)
		}
		if !strings.Contains(output, "Imported") {
			t.Errorf("expected import success messages, got: %s", output)
		}
		if mock.secrets["db-password"] != "secret123" {
			t.Errorf("expected db-password=secret123, got: %q", mock.secrets["db-password"])
		}
		if mock.secrets["api-key"] != "mykey" {
			t.Errorf("expected api-key=mykey, got: %q", mock.secrets["api-key"])
		}
	})
}

func TestRunCheckAll_WithMock(t *testing.T) {
	mock := newMockSecretClient(map[string]string{
		"db-password": "secret",
		"api-key":     "key",
	})
	withMockGCPClient(t, mock)

	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
  staging:
    gcp:
      project: test-project
defaults:
  env: dev
`,
		"config/base.yaml": `
database:
  password: ${secret:db-password}
`,
		"config/dev.yaml": `
app:
  key: ${secret:api-key}
`,
		"config/staging.yaml": `
app:
  key: ${secret:api-key}
`,
	})

	output, err := execTestCmd("check", "--all", "--path", dir)
	if err != nil {
		t.Fatalf("check --all failed: %v", err)
	}
	if !strings.Contains(output, "All environments validated") {
		t.Errorf("expected success message, got: %s", output)
	}
}

func TestRunResolve_WithSecrets_Mock(t *testing.T) {
	mock := newMockSecretClient(map[string]string{
		"db-password": "resolved-secret",
	})
	withMockGCPClient(t, mock)

	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
defaults:
  env: dev
`,
		"config/base.yaml": `
database:
  host: localhost
  password: ${secret:db-password}
`,
		"config/dev.yaml": "app:\n  debug: true\n",
	})

	output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "dotenv")
	if err != nil {
		t.Fatalf("resolve with secrets failed: %v", err)
	}
	if !strings.Contains(output, "DATABASE_PASSWORD=resolved-secret") {
		t.Errorf("expected resolved secret value, got: %s", output)
	}
}

func TestValidateEnvMapping(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-dev
env_mapping:
  database.host: DB_HOST
  nonexistent.key: MISSING_KEY
defaults:
  env: dev
`,
		"config/base.yaml": `
database:
  host: localhost
`,
		"config/dev.yaml": `
app:
  debug: true
`,
	})

	output, err := execTestCmd("validate", "--path", dir)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if !strings.Contains(output, "nonexistent.key") {
		t.Errorf("expected warning about unmapped key, got: %s", output)
	}
}

func TestRunRun_DryRun_WithMock(t *testing.T) {
	mock := newMockSecretClient(map[string]string{
		"db-password": "secret123",
	})
	withMockGCPClient(t, mock)

	dir := setupTestProject(t, map[string]string{
		".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project
defaults:
  env: dev
`,
		"config/base.yaml": `
database:
  host: localhost
  password: ${secret:db-password}
`,
		"config/dev.yaml": "app:\n  debug: true\n",
	})

	output, err := execTestCmd("run", "--dry-run", "-p", dir, "-e", "dev", "--", "echo", "hello")
	if err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}
	if !strings.Contains(output, "Dry run") {
		t.Errorf("expected 'Dry run' in output, got: %s", output)
	}
	if !strings.Contains(output, "DATABASE_HOST=localhost") {
		t.Errorf("expected DATABASE_HOST=localhost, got: %s", output)
	}
	if !strings.Contains(output, "Command:") {
		t.Errorf("expected command display, got: %s", output)
	}
}
