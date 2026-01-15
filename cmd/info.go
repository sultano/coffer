package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show project configuration and status",
	Long: `Display information about the coffer configuration, authentication status,
and detected secrets.`,
	RunE: runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	bold := color.New(color.Bold)

	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	// Project info
	bold.Println("Project")
	fmt.Printf("  Path: %s\n", projectRoot)

	project, err := config.LoadProject(projectRoot)
	if err != nil {
		red.Printf("  Status: not initialized\n")
		fmt.Println()
		fmt.Println("Run 'coffer init' to initialize this project")
		return nil
	}
	green.Printf("  Status: initialized\n")
	fmt.Printf("  Config path: %s\n", project.Config.Path)
	fmt.Println()

	// Authentication
	bold.Println("Authentication")
	account, err := getGCloudAccount()
	if err != nil {
		red.Printf("  GCP: not authenticated\n")
		fmt.Println("  Run: gcloud auth application-default login")
	} else {
		green.Printf("  GCP: %s\n", account)

		// Check Secret Manager access with quick timeout
		gcpResult, _, gcpErr := newGCPClient(QuickGCPTimeout)
		if gcpErr != nil {
			yellow.Printf("  Secret Manager: connection failed\n")
		} else {
			green.Printf("  Secret Manager: accessible\n")
			gcpResult.Close()
		}
	}
	fmt.Println()

	// Environments
	bold.Println("Environments")
	envs := project.ListEnvironments()
	if len(envs) == 0 {
		yellow.Println("  No environments defined")
	} else {
		sort.Strings(envs)
		for _, env := range envs {
			gcpProject := project.GCP.Project
			if envCfg, ok := project.Environments[env]; ok && envCfg.GCP.Project != "" {
				gcpProject = envCfg.GCP.Project
			}

			defaultMarker := ""
			if env == project.Defaults.Env {
				defaultMarker = " (default)"
			}

			fmt.Printf("  %s%s\n", env, defaultMarker)
			fmt.Printf("    GCP Project: %s\n", gcpProject)

			// Check if env file exists
			envFile := filepath.Join(projectRoot, project.Config.Path, env+".yaml")
			if fileExists(envFile) {
				fmt.Printf("    Config: %s.yaml\n", env)
			}
		}
	}
	fmt.Println()

	// Config files
	bold.Println("Config Files")
	configDir := filepath.Join(projectRoot, project.Config.Path)
	baseFile := filepath.Join(configDir, project.Config.Base)
	if fileExists(baseFile) {
		green.Printf("  ✓ %s\n", project.Config.Base)
	} else {
		red.Printf("  ✗ %s (missing)\n", project.Config.Base)
	}

	localFile := filepath.Join(configDir, "local.yaml")
	if fileExists(localFile) {
		green.Printf("  ✓ local.yaml (local overrides)\n")
	}
	fmt.Println()

	// Secret references (if env specified or default exists)
	env := envName
	if env == "" {
		env = project.Defaults.Env
	}

	if env != "" {
		bold.Printf("Secrets (%s)\n", env)

		loaded, err := config.Load(projectRoot, env)
		if err != nil {
			yellow.Printf("  Could not load config: %v\n", err)
		} else {
			flat := config.Flatten(loaded.Values)
			refs := resolver.FindSecretRefs(flat)

			if len(refs) == 0 {
				yellow.Println("  No secret references found")
			} else {
				for _, ref := range refs {
					version := ref.Version
					if version == "" {
						version = "latest"
					}
					fmt.Printf("  ${secret:%s} (version: %s)\n", ref.Name, version)
				}
			}
		}
		fmt.Println()
	}

	// Env var mappings
	if len(project.EnvMapping) > 0 {
		bold.Println("Env Var Mappings")
		keys := make([]string, 0, len(project.EnvMapping))
		for k := range project.EnvMapping {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Printf("  %s -> %s\n", k, project.EnvMapping[k])
		}
		fmt.Println()
	}

	return nil
}
