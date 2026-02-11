package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sultano/coffer/internal/config"
	"gopkg.in/yaml.v3"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file syntax",
	Long: `Validate configuration files for syntax errors and structural issues.

Checks:
  - YAML syntax in all config files
  - Secret references are well-formed
  - env_mapping keys exist in config
  - Environment config files exist`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

type validationResult struct {
	errors   []string
	warnings []string
}

func (v *validationResult) addError(format string, args ...any) {
	v.errors = append(v.errors, fmt.Sprintf(format, args...))
}

func (v *validationResult) addWarning(format string, args ...any) {
	v.warnings = append(v.warnings, fmt.Sprintf(format, args...))
}

func runValidate(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	result := &validationResult{}
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)

	fmt.Println("Validating configuration...")
	fmt.Println()

	// 1. Validate project config
	project, err := validateProjectConfig(projectRoot, result)
	if err != nil {
		return err
	}

	// 2. Validate config files exist and have valid YAML
	validateConfigFiles(projectRoot, project, result)

	// 3. Validate secret references
	validateSecretRefs(projectRoot, project, result)

	// 4. Validate env_mapping
	validateEnvMapping(projectRoot, project, result)

	// 5. Check local.yaml gitignore
	checkLocalGitignore(projectRoot, result)

	// Print results
	if len(result.warnings) > 0 {
		_, _ = yellow.Println("Warnings:")
		for _, w := range result.warnings {
			fmt.Printf("  - %s\n", w)
		}
		fmt.Println()
	}

	if len(result.errors) > 0 {
		_, _ = red.Println("Errors:")
		for _, e := range result.errors {
			fmt.Printf("  - %s\n", e)
		}
		fmt.Println()
		return fmt.Errorf("validation failed with %d error(s)", len(result.errors))
	}

	_, _ = green.Println("Validation passed")
	return nil
}

func validateProjectConfig(projectRoot string, result *validationResult) (*config.ProjectConfig, error) {
	project, err := config.LoadProject(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", config.ProjectFileName, err)
	}

	// Check required fields
	if project.GCP.Project == "" {
		result.addError("gcp.project is required in %s", config.ProjectFileName)
	}

	fmt.Printf("  %s %s\n", color.GreenString("✓"), config.ProjectFileName)
	return project, nil
}

func validateConfigFiles(projectRoot string, project *config.ProjectConfig, result *validationResult) {
	configDir := filepath.Join(projectRoot, project.Config.Path)

	// Check base config
	basePath := filepath.Join(configDir, project.Config.Base)
	if err := validateYAMLFile(basePath); err != nil {
		result.addError("%s: %v", project.Config.Base, err)
	} else {
		fmt.Printf("  %s %s\n", color.GreenString("✓"), project.Config.Base)
	}

	// Check environment configs
	for env := range project.Environments {
		envFile := env + ".yaml"
		envPath := filepath.Join(configDir, envFile)

		if _, err := os.Stat(envPath); os.IsNotExist(err) {
			result.addWarning("environment '%s' defined but %s not found", env, envFile)
		} else if err := validateYAMLFile(envPath); err != nil {
			result.addError("%s: %v", envFile, err)
		} else {
			fmt.Printf("  %s %s\n", color.GreenString("✓"), envFile)
		}
	}

	// Check local.yaml if exists
	localPath := filepath.Join(configDir, config.LocalFileName)
	if _, err := os.Stat(localPath); err == nil {
		if err := validateYAMLFile(localPath); err != nil {
			result.addError("%s: %v", config.LocalFileName, err)
		} else {
			fmt.Printf("  %s %s\n", color.GreenString("✓"), config.LocalFileName)
		}
	}
}

func validateYAMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}

	var content map[string]any
	if err := yaml.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("invalid YAML syntax: %w", err)
	}

	return nil
}

func validateSecretRefs(projectRoot string, project *config.ProjectConfig, result *validationResult) {
	// Pattern for valid secret references
	validRefPattern := regexp.MustCompile(`^\$\{secret:([a-zA-Z0-9_\-/@]+)\}$`)
	// Pattern to find potential malformed refs
	partialRefPattern := regexp.MustCompile(`\$\{secret:[^}]*`)

	for _, env := range project.ListEnvironments() {
		loaded, err := config.Load(projectRoot, env)
		if err != nil {
			continue
		}

		flat := config.Flatten(loaded.Values)

		for key, value := range flat {
			// Check for malformed secret references
			if strings.Contains(value, "${secret:") {
				// Find all potential refs in the value
				matches := partialRefPattern.FindAllString(value, -1)
				for _, match := range matches {
					fullRef := match + "}"
					if !validRefPattern.MatchString(fullRef) && !strings.HasSuffix(match, "}") {
						result.addError("malformed secret reference in %s.%s: %s", env, key, match)
					}
				}
			}
		}
	}
}

func validateEnvMapping(projectRoot string, project *config.ProjectConfig, result *validationResult) {
	if len(project.EnvMapping) == 0 {
		return
	}

	// Load default env to check mappings
	env := project.Defaults.Env
	if env == "" && len(project.Environments) > 0 {
		for e := range project.Environments {
			env = e
			break
		}
	}

	if env == "" {
		return
	}

	loaded, err := config.Load(projectRoot, env)
	if err != nil {
		return
	}

	flat := config.Flatten(loaded.Values)

	for configKey := range project.EnvMapping {
		if _, exists := flat[configKey]; !exists {
			result.addWarning("env_mapping key '%s' not found in config", configKey)
		}
	}
}

func checkLocalGitignore(projectRoot string, result *validationResult) {
	// Check if local.yaml exists
	configDir := filepath.Join(projectRoot, "config")
	localPath := filepath.Join(configDir, config.LocalFileName)

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return // No local.yaml, nothing to check
	}

	// Check if .gitignore exists and contains local.yaml
	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		result.addWarning("local.yaml exists but no .gitignore found - consider adding local.yaml to .gitignore")
		return
	}

	content := string(data)
	patterns := []string{"local.yaml", "**/local.yaml", "config/local.yaml"}

	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return // Found in gitignore
		}
	}

	result.addWarning("local.yaml exists but not in .gitignore - local overrides may be committed accidentally")
}
