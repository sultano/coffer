package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sultano/coffer/internal/config"
)

// execTestCmd runs a command and captures stdout/stderr
func execTestCmd(args ...string) (string, error) {
	// Capture stdout
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	// Reset global state
	envName = ""
	projectPath = ""
	dryRun = false
	noColor = true

	rootCmd.SetArgs(args)
	err := rootCmd.Execute()

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	r.Close()

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

	t.Run("outputs json format", func(t *testing.T) {
		output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "json")
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !strings.Contains(output, `"DB_HOST"`) {
			t.Errorf("output should contain DB_HOST mapping, got: %s", output)
		}
		if !strings.Contains(output, "dev-db") {
			t.Errorf("output should contain dev-db (from dev.yaml), got: %s", output)
		}
	})

	t.Run("outputs yaml format", func(t *testing.T) {
		output, err := execTestCmd("resolve", "--path", dir, "--env", "dev", "--format", "yaml")
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !strings.Contains(output, "DB_HOST:") {
			t.Errorf("output should contain DB_HOST:, got: %s", output)
		}
	})

	t.Run("outputs dotenv format", func(t *testing.T) {
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
