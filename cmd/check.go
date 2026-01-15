package cmd

import (
	"fmt"
	"sort"

	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var checkAll bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate configuration and secret references",
	Long: `Validate that configuration files are valid and all referenced secrets exist.

Use --all to check secrets across all defined environments.`,
	RunE: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().BoolVar(&checkAll, "all", false, "check all environments")
}

func runCheck(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	project, err := config.LoadProject(projectRoot)
	if err != nil {
		return err
	}

	if checkAll {
		return checkAllEnvironments(projectRoot, project)
	}

	return checkEnvironment(projectRoot, project, envName)
}

func checkEnvironment(projectRoot string, project *config.ProjectConfig, env string) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)

	// Load config for this environment
	loaded, err := config.Load(projectRoot, env)
	if err != nil {
		return err
	}

	fmt.Printf("Checking environment: %s\n\n", loaded.Environment)

	// Flatten and find secret references
	flat := config.Flatten(loaded.Values)
	refs := resolver.FindSecretRefs(flat)

	if len(refs) == 0 {
		yellow.Println("No secret references found in configuration")
		return nil
	}

	fmt.Printf("Found %d secret references\n\n", len(refs))

	// Get GCP project
	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured for environment '%s'", loaded.Environment)
	}

	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	// Check each secret
	secretPrefix := loaded.GetSecretPrefix()
	var missing []string
	for _, ref := range refs {
		// Apply prefix to non-full-path references
		fullName := ref.Name
		if !ref.IsFullPath && secretPrefix != "" {
			fullName = secretPrefix + ref.Name
		}

		exists, err := gcpResult.Client.SecretExists(ctx, gcpProject, fullName)
		if err != nil {
			red.Printf("✗ %s - error: %v\n", ref.Name, err)
			missing = append(missing, ref.Name)
			continue
		}

		if exists {
			version := ref.Version
			if version == "" {
				version = "latest"
			}
			if secretPrefix != "" {
				green.Printf("✓ %s (stored as: %s, version: %s)\n", ref.Name, fullName, version)
			} else {
				green.Printf("✓ %s (version: %s)\n", ref.Name, version)
			}
		} else {
			red.Printf("✗ %s - NOT FOUND in GCP Secret Manager\n", ref.Name)
			if secretPrefix != "" {
				fmt.Printf("  (looking for: %s)\n", fullName)
			}
			missing = append(missing, ref.Name)
		}
	}

	fmt.Println()

	if len(missing) > 0 {
		red.Printf("Error: %d secret(s) not found\n", len(missing))
		fmt.Println()
		fmt.Println("To create missing secrets:")
		for _, name := range missing {
			fmt.Printf("  coffer secret set %s\n", name)
		}
		return fmt.Errorf("%d secret(s) missing", len(missing))
	}

	green.Println("All secrets validated successfully")
	return nil
}

func checkAllEnvironments(projectRoot string, project *config.ProjectConfig) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	envs := project.ListEnvironments()
	if len(envs) == 0 {
		return fmt.Errorf("no environments defined in %s", config.ProjectFileName)
	}

	sort.Strings(envs)

	fmt.Printf("Checking environments: %v\n\n", envs)

	// Collect all secrets across environments
	type secretStatus struct {
		env    string
		exists bool
		err    error
	}
	secretsByName := make(map[string][]secretStatus)

	// Use longer timeout for checking all environments
	gcpResult, ctx, err := newGCPClient(LongGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	for _, env := range envs {
		loaded, err := config.Load(projectRoot, env)
		if err != nil {
			red.Printf("Error loading %s: %v\n", env, err)
			continue
		}

		flat := config.Flatten(loaded.Values)
		refs := resolver.FindSecretRefs(flat)
		gcpProject := loaded.GetGCPProject()
		secretPrefix := loaded.GetSecretPrefix()

		for _, ref := range refs {
			// Apply prefix to non-full-path references
			fullName := ref.Name
			if !ref.IsFullPath && secretPrefix != "" {
				fullName = secretPrefix + ref.Name
			}

			exists, checkErr := gcpResult.Client.SecretExists(ctx, gcpProject, fullName)
			secretsByName[ref.Name] = append(secretsByName[ref.Name], secretStatus{
				env:    env,
				exists: exists,
				err:    checkErr,
			})
		}
	}

	// Print results
	fmt.Println("Secrets:")
	var issues []string

	// Sort secret names
	names := make([]string, 0, len(secretsByName))
	for name := range secretsByName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		statuses := secretsByName[name]
		fmt.Printf("  %-20s", name)

		for _, status := range statuses {
			if status.err != nil {
				red.Printf(" ✗ %s", status.env)
				issues = append(issues, fmt.Sprintf("%s error in %s: %v", name, status.env, status.err))
			} else if status.exists {
				green.Printf(" ✓ %s", status.env)
			} else {
				red.Printf(" ✗ %s", status.env)
				issues = append(issues, fmt.Sprintf("%s missing in %s", name, status.env))
			}
		}
		fmt.Println()
	}

	fmt.Println()

	if len(issues) > 0 {
		red.Printf("Issues found: %d\n", len(issues))
		fmt.Println()
		fmt.Println("To fix missing secrets:")
		fmt.Println("  coffer secret set <name> --env <environment>")
		return fmt.Errorf("%d issue(s) found", len(issues))
	}

	green.Println("All environments validated successfully")
	return nil
}
