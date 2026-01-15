package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	ProjectFileName = ".coffer.yaml"
	DefaultBaseName = "base.yaml"
	LocalFileName   = "local.yaml"
)

// LoadProject loads the .coffer.yaml project configuration
func LoadProject(projectRoot string) (*ProjectConfig, error) {
	configPath := filepath.Join(projectRoot, ProjectFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s found in %s - run 'coffer init' to create one", ProjectFileName, projectRoot)
		}
		return nil, fmt.Errorf("failed to read %s: %w", ProjectFileName, err)
	}

	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", ProjectFileName, err)
	}

	// Apply defaults
	if cfg.Config.Path == "" {
		cfg.Config.Path = "./config"
	}
	if cfg.Config.Base == "" {
		cfg.Config.Base = DefaultBaseName
	}

	return &cfg, nil
}

// Load loads and merges configuration for the specified environment
// BEHAVIOR: Config files are merged in strict order: base.yaml -> {env}.yaml -> local.yaml
// BEHAVIOR: Later files override earlier files (local.yaml has highest priority)
// BEHAVIOR: Environment file is optional if env is defined in .coffer.yaml environments
// BEHAVIOR: local.yaml is always optional and intended for developer overrides
func Load(projectRoot, environment string) (*LoadedConfig, error) {
	project, err := LoadProject(projectRoot)
	if err != nil {
		return nil, err
	}

	// Resolve environment from flag, project defaults, or error
	env := resolveEnvironment(environment, project)
	if env == "" {
		return nil, fmt.Errorf("no environment specified - use --env flag or set defaults.env in %s", ProjectFileName)
	}

	configDir := filepath.Join(projectRoot, project.Config.Path)

	// Load base config
	basePath := filepath.Join(configDir, project.Config.Base)
	baseValues, err := loadYAMLFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load base config: %w", err)
	}

	merged := baseValues

	// Load environment overlay if it exists
	envPath := filepath.Join(configDir, env+".yaml")
	if fileExists(envPath) {
		envValues, err := loadYAMLFile(envPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load environment config %s: %w", env+".yaml", err)
		}
		merged = DeepMerge(merged, envValues)
	} else if !isKnownEnvironment(env, project) {
		return nil, fmt.Errorf("environment '%s' not found - no %s.yaml file exists", env, env)
	}

	// Load local overrides (optional, highest priority)
	localPath := filepath.Join(configDir, LocalFileName)
	if fileExists(localPath) {
		localValues, err := loadYAMLFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load local config: %w", err)
		}
		merged = DeepMerge(merged, localValues)
	}

	return &LoadedConfig{
		Project:     project,
		Environment: env,
		Values:      merged,
		ProjectRoot: projectRoot,
	}, nil
}

func resolveEnvironment(flagEnv string, project *ProjectConfig) string {
	if flagEnv != "" {
		return flagEnv
	}
	return project.Defaults.Env
}

func isKnownEnvironment(env string, project *ProjectConfig) bool {
	if project.Environments == nil {
		return false
	}
	_, exists := project.Environments[env]
	return exists
}

func loadYAMLFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, err
	}

	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	if values == nil {
		values = make(map[string]any)
	}

	return values, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetGCPProject returns the GCP project for the given environment
// BEHAVIOR: Environment-specific GCP project takes precedence over default
// BEHAVIOR: Falls back to gcp.project if no environment-specific project defined
func (lc *LoadedConfig) GetGCPProject() string {
	// Check environment-specific GCP project first
	if envCfg, ok := lc.Project.Environments[lc.Environment]; ok {
		if envCfg.GCP.Project != "" {
			return envCfg.GCP.Project
		}
	}
	// Fall back to default GCP project
	return lc.Project.GCP.Project
}

// GetSecretPrefix returns the secret prefix for the project
func (lc *LoadedConfig) GetSecretPrefix() string {
	return lc.Project.GCP.SecretPrefix
}

// ListEnvironments returns all defined environments
func (pc *ProjectConfig) ListEnvironments() []string {
	envs := make([]string, 0, len(pc.Environments))
	for env := range pc.Environments {
		envs = append(envs, env)
	}
	return envs
}
