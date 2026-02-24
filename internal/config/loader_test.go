package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProject(t *testing.T) {
	t.Run("loads valid project config", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".coffer.yaml", `
version: 1
config:
  path: ./config
  base: base.yaml
gcp:
  project: test-project
environments:
  dev:
    gcp:
      project: test-project-dev
  prod:
    gcp:
      project: test-project-prod
defaults:
  env: dev
`)

		cfg, err := LoadProject(dir)
		if err != nil {
			t.Fatalf("LoadProject failed: %v", err)
		}

		if cfg.Version != 1 {
			t.Errorf("Version = %d, want 1", cfg.Version)
		}
		if cfg.Config.Path != "./config" {
			t.Errorf("Config.Path = %q, want ./config", cfg.Config.Path)
		}
		if cfg.GCP.Project != "test-project" {
			t.Errorf("GCP.Project = %q, want test-project", cfg.GCP.Project)
		}
		if cfg.Defaults.Env != "dev" {
			t.Errorf("Defaults.Env = %q, want dev", cfg.Defaults.Env)
		}
		if len(cfg.Environments) != 2 {
			t.Errorf("len(Environments) = %d, want 2", len(cfg.Environments))
		}
	})

	t.Run("applies defaults for missing fields", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".coffer.yaml", `
version: 1
gcp:
  project: test-project
`)

		cfg, err := LoadProject(dir)
		if err != nil {
			t.Fatalf("LoadProject failed: %v", err)
		}

		if cfg.Config.Path != "./config" {
			t.Errorf("Config.Path = %q, want ./config (default)", cfg.Config.Path)
		}
		if cfg.Config.Base != "base.yaml" {
			t.Errorf("Config.Base = %q, want base.yaml (default)", cfg.Config.Base)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		dir := t.TempDir()

		_, err := LoadProject(dir)
		if err == nil {
			t.Fatal("expected error for missing .coffer.yaml")
		}
	})

	t.Run("returns error for invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".coffer.yaml", `invalid: yaml: content: [`)

		_, err := LoadProject(dir)
		if err == nil {
			t.Fatal("expected error for invalid YAML")
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("merges base and env configs", func(t *testing.T) {
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
  log_level: info
database:
  host: localhost
  port: 5432
`,
			"config/dev.yaml": `
app:
  log_level: debug
database:
  host: dev-db
`,
		})

		loaded, err := Load(dir, "dev")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		// Check merge: dev overrides base
		app := loaded.Values["app"].(map[string]any)
		if app["log_level"] != "debug" {
			t.Errorf("app.log_level = %q, want debug (from dev.yaml)", app["log_level"])
		}
		if app["name"] != "myapp" {
			t.Errorf("app.name = %q, want myapp (from base.yaml)", app["name"])
		}

		db := loaded.Values["database"].(map[string]any)
		if db["host"] != "dev-db" {
			t.Errorf("database.host = %q, want dev-db (from dev.yaml)", db["host"])
		}
		if db["port"] != 5432 {
			t.Errorf("database.port = %v, want 5432 (from base.yaml)", db["port"])
		}
	})

	t.Run("local.yaml overrides everything", func(t *testing.T) {
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
database:
  password: ${secret:db-password}
`,
			"config/dev.yaml": `
database:
  host: dev-db
`,
			"config/local.yaml": `
database:
  password: local-override
  host: localhost
`,
		})

		loaded, err := Load(dir, "dev")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		db := loaded.Values["database"].(map[string]any)
		if db["password"] != "local-override" {
			t.Errorf("database.password = %q, want local-override", db["password"])
		}
		if db["host"] != "localhost" {
			t.Errorf("database.host = %q, want localhost (local overrides dev)", db["host"])
		}
	})

	t.Run("uses default env when not specified", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  staging:
    gcp:
      project: test-staging
defaults:
  env: staging
`,
			"config/base.yaml": `
app:
  name: myapp
`,
			"config/staging.yaml": `
app:
  env: staging
`,
		})

		loaded, err := Load(dir, "") // empty env
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if loaded.Environment != "staging" {
			t.Errorf("Environment = %q, want staging", loaded.Environment)
		}
	})

	t.Run("COFFER_ENV used when flag is empty", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  staging:
    gcp:
      project: test-staging
`,
			"config/base.yaml": `
app:
  name: myapp
`,
			"config/staging.yaml": `
app:
  env: staging
`,
		})

		t.Setenv("COFFER_ENV", "staging")

		loaded, err := Load(dir, "")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if loaded.Environment != "staging" {
			t.Errorf("Environment = %q, want staging", loaded.Environment)
		}
	})

	t.Run("flag overrides COFFER_ENV", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: test-project
environments:
  staging:
    gcp:
      project: test-staging
  prod:
    gcp:
      project: test-prod
`,
			"config/base.yaml": `
app:
  name: myapp
`,
			"config/prod.yaml": `
app:
  env: prod
`,
		})

		t.Setenv("COFFER_ENV", "staging")

		loaded, err := Load(dir, "prod")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if loaded.Environment != "prod" {
			t.Errorf("Environment = %q, want prod", loaded.Environment)
		}
	})

	t.Run("COFFER_ENV overrides defaults.env", func(t *testing.T) {
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
			"config/prod.yaml": `
app:
  env: prod
`,
		})

		t.Setenv("COFFER_ENV", "prod")

		loaded, err := Load(dir, "")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if loaded.Environment != "prod" {
			t.Errorf("Environment = %q, want prod", loaded.Environment)
		}
	})

	t.Run("returns error for missing base config", func(t *testing.T) {
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
			// No base.yaml
		})

		_, err := Load(dir, "dev")
		if err == nil {
			t.Fatal("expected error for missing base.yaml")
		}
	})

	t.Run("returns error for unknown environment", func(t *testing.T) {
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
		})

		_, err := Load(dir, "nonexistent")
		if err == nil {
			t.Fatal("expected error for unknown environment")
		}
	})
}

func TestGetGCPProject(t *testing.T) {
	t.Run("returns env-specific project", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: default-project
environments:
  prod:
    gcp:
      project: prod-project
`,
			"config/base.yaml": `
app:
  name: test
`,
			"config/prod.yaml": `
app:
  env: prod
`,
		})

		loaded, err := Load(dir, "prod")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		gcpProject := loaded.GetGCPProject()
		if gcpProject != "prod-project" {
			t.Errorf("GetGCPProject() = %q, want prod-project", gcpProject)
		}
	})

	t.Run("falls back to default project", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: default-project
environments:
  dev:
    gcp: {}
`,
			"config/base.yaml": `
app:
  name: test
`,
			"config/dev.yaml": `
app:
  env: dev
`,
		})

		loaded, err := Load(dir, "dev")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		gcpProject := loaded.GetGCPProject()
		if gcpProject != "default-project" {
			t.Errorf("GetGCPProject() = %q, want default-project", gcpProject)
		}
	})
}

func TestGetSecretPrefix(t *testing.T) {
	t.Run("returns configured prefix", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: default-project
  secret_prefix: service-a-
environments:
  dev:
    gcp:
      project: dev-project
`,
			"config/base.yaml": `
app:
  name: test
`,
			"config/dev.yaml": `
app:
  env: dev
`,
		})

		loaded, err := Load(dir, "dev")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		prefix := loaded.GetSecretPrefix()
		if prefix != "service-a-" {
			t.Errorf("GetSecretPrefix() = %q, want service-a-", prefix)
		}
	})

	t.Run("returns empty when no prefix configured", func(t *testing.T) {
		dir := setupTestProject(t, map[string]string{
			".coffer.yaml": `
version: 1
config:
  path: ./config
gcp:
  project: default-project
environments:
  dev:
    gcp:
      project: dev-project
`,
			"config/base.yaml": `
app:
  name: test
`,
			"config/dev.yaml": `
app:
  env: dev
`,
		})

		loaded, err := Load(dir, "dev")
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		prefix := loaded.GetSecretPrefix()
		if prefix != "" {
			t.Errorf("GetSecretPrefix() = %q, want empty string", prefix)
		}
	})
}

func TestListEnvironments(t *testing.T) {
	cfg := &ProjectConfig{
		Environments: map[string]EnvConfig{
			"dev":     {},
			"staging": {},
			"prod":    {},
		},
	}

	envs := cfg.ListEnvironments()
	if len(envs) != 3 {
		t.Errorf("ListEnvironments() returned %d envs, want 3", len(envs))
	}

	// Check all envs are present
	envMap := make(map[string]bool)
	for _, env := range envs {
		envMap[env] = true
	}

	for _, expected := range []string{"dev", "staging", "prod"} {
		if !envMap[expected] {
			t.Errorf("ListEnvironments() missing %q", expected)
		}
	}
}

// Helper functions

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

func setupTestProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, dir, name, content)
	}
	return dir
}
