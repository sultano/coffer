package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize coffer in a project",
	Long: `Initialize coffer configuration in the current directory.

Creates:
  - .coffer.yaml with basic configuration
  - config/ directory with base.yaml
  - Updates .gitignore to exclude local.yaml`,
	RunE: runInit,
}

var gcpProjectFlag string

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&gcpProjectFlag, "gcp-project", "", "GCP project ID for Secret Manager")
}

func runInit(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	// Check if already initialized
	cofferPath := filepath.Join(projectRoot, ".coffer.yaml")
	if fileExists(cofferPath) {
		_, _ = yellow.Println("Project already initialized (.coffer.yaml exists)")
		return nil
	}

	// Get GCP project if not provided
	gcpProject := gcpProjectFlag
	if gcpProject == "" {
		// Try to get from gcloud
		project, _ := getGCloudProject()
		if project != "" {
			fmt.Printf("Detected GCP project: %s\n", project)
			fmt.Print("Use this project? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer == "" || answer == "y" || answer == "yes" {
				gcpProject = project
			}
		}

		if gcpProject == "" {
			fmt.Print("Enter GCP project ID: ")
			reader := bufio.NewReader(os.Stdin)
			gcpProject, _ = reader.ReadString('\n')
			gcpProject = strings.TrimSpace(gcpProject)
		}
	}

	if dryRun {
		fmt.Println("Dry run - would create:")
		fmt.Println("  .coffer.yaml")
		fmt.Println("  config/base.yaml")
		fmt.Println("  Update .gitignore")
		return nil
	}

	// Create config directory
	configDir := filepath.Join(projectRoot, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create .coffer.yaml
	cofferContent := fmt.Sprintf(`version: 1

config:
  path: ./config
  base: base.yaml

gcp:
  project: %s

environments:
  dev:
    gcp:
      project: %s
  staging:
    gcp:
      project: %s
  prod:
    gcp:
      project: %s

env_mapping:
  # database.host: DB_HOST
  # database.password: DB_PASS

defaults:
  env: dev
`, gcpProject, gcpProject, gcpProject, gcpProject)

	if err := os.WriteFile(cofferPath, []byte(cofferContent), 0644); err != nil {
		return fmt.Errorf("failed to create .coffer.yaml: %w", err)
	}
	_, _ = green.Println("✓ Created .coffer.yaml")

	// Create base.yaml
	basePath := filepath.Join(configDir, "base.yaml")
	if !fileExists(basePath) {
		baseContent := `# Base configuration - shared across all environments
# Override in environment-specific files (dev.yaml, prod.yaml, etc.)

app:
  name: myapp
  log_level: info

# Example secret reference:
# database:
#   password: ${secret:db-password}
`
		if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
			return fmt.Errorf("failed to create base.yaml: %w", err)
		}
		_, _ = green.Println("✓ Created config/base.yaml")
	} else {
		_, _ = yellow.Println("  config/base.yaml already exists, skipping")
	}

	// Create dev.yaml
	devPath := filepath.Join(configDir, "dev.yaml")
	if !fileExists(devPath) {
		devContent := `# Development environment overrides
app:
  log_level: debug
`
		if err := os.WriteFile(devPath, []byte(devContent), 0644); err != nil {
			return fmt.Errorf("failed to create dev.yaml: %w", err)
		}
		_, _ = green.Println("✓ Created config/dev.yaml")
	}

	// Update .gitignore
	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	gitignoreEntries := []string{
		"config/local.yaml",
		".env.local",
	}

	if err := updateGitignore(gitignorePath, gitignoreEntries); err != nil {
		_, _ = yellow.Printf("  Warning: could not update .gitignore: %v\n", err)
	} else {
		_, _ = green.Println("✓ Updated .gitignore")
	}

	fmt.Println()
	_, _ = green.Println("Coffer initialized successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .coffer.yaml to configure your GCP project(s)")
	fmt.Println("  2. Add configuration to config/base.yaml")
	fmt.Println("  3. Create environment overlays (config/dev.yaml, config/prod.yaml)")
	fmt.Println("  4. Run: coffer auth status")
	fmt.Println("  5. Run: coffer check --env dev")

	return nil
}

func updateGitignore(path string, entries []string) error {
	existing := make(map[string]bool)

	// Read existing .gitignore
	if fileExists(path) {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			existing[strings.TrimSpace(line)] = true
		}
	}

	// Add missing entries
	var toAdd []string
	for _, entry := range entries {
		if !existing[entry] {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	// Append to file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Add newline if file doesn't end with one
	if fileExists(path) {
		data, _ := os.ReadFile(path)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}

	if _, err := f.WriteString("\n# Coffer local files\n"); err != nil {
		return err
	}
	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	return nil
}
