package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage secrets in GCP Secret Manager",
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets",
	RunE:  runSecretList,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a secret value",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretGet,
}

var secretSetCmd = &cobra.Command{
	Use:   "set <name> [value]",
	Short: "Create or update a secret",
	Long: `Create or update a secret in GCP Secret Manager.

If value is not provided, it will be read from stdin.
Use --from-file to read the value from a file.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSecretSet,
}

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret",
	Long: `Delete a secret from GCP Secret Manager.

Requires --yes flag to confirm deletion.`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretDelete,
}

var secretUnusedCmd = &cobra.Command{
	Use:   "unused",
	Short: "List secrets not referenced in config",
	Long: `Find secrets in GCP that are not referenced in your config files.

Note: This only checks the current project's config. Secrets may be
used by other services or applications outside of coffer.`,
	RunE: runSecretUnused,
}

var secretImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import secrets from a .env file",
	Long: `Import secrets from a .env file into GCP Secret Manager.

Each KEY=VALUE pair in the file will be converted to a secret.
The key name is converted to lowercase with underscores replaced by hyphens.

Example:
  DB_PASSWORD=secret123  ->  secret: db-password`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretImport,
}

var fromFile string
var yesFlag bool

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretDeleteCmd)
	secretCmd.AddCommand(secretUnusedCmd)
	secretCmd.AddCommand(secretImportCmd)

	secretSetCmd.Flags().StringVar(&fromFile, "from-file", "", "read secret value from file")
	secretDeleteCmd.Flags().BoolVar(&yesFlag, "yes", false, "confirm deletion")
	secretImportCmd.Flags().BoolVar(&yesFlag, "yes", false, "confirm import without prompting")
}

func runSecretList(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	// Load full config to get environment-specific settings
	loaded, err := loadConfigWithFallback(projectRoot)
	if err != nil {
		return err
	}

	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured in %s", config.ProjectFileName)
	}

	secretPrefix := loaded.GetSecretPrefix()

	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	secretsList, err := gcpResult.Client.ListSecrets(ctx, gcpProject)
	if err != nil {
		return err
	}

	// Filter by prefix if set
	var filtered []string
	for _, s := range secretsList {
		parts := strings.Split(s, "/")
		name := parts[len(parts)-1]
		if secretPrefix == "" || strings.HasPrefix(name, secretPrefix) {
			filtered = append(filtered, name)
		}
	}

	if len(filtered) == 0 {
		if secretPrefix != "" {
			fmt.Printf("No secrets found with prefix '%s' in project: %s\n", secretPrefix, gcpProject)
		} else {
			fmt.Println("No secrets found in project:", gcpProject)
		}
		return nil
	}

	if secretPrefix != "" {
		fmt.Printf("Secrets in %s (prefix: %s):\n\n", gcpProject, secretPrefix)
	} else {
		fmt.Printf("Secrets in %s:\n\n", gcpProject)
	}

	for _, name := range filtered {
		// Strip prefix for display if set
		displayName := name
		if secretPrefix != "" {
			displayName = strings.TrimPrefix(name, secretPrefix)
		}
		fmt.Printf("  %s\n", displayName)
	}

	return nil
}

// loadConfigWithFallback loads config with env flag or default env
func loadConfigWithFallback(projectRoot string) (*config.LoadedConfig, error) {
	// If env is specified, use it
	if envName != "" {
		return config.Load(projectRoot, envName)
	}

	// Try to load project config to get default env
	project, err := config.LoadProject(projectRoot)
	if err != nil {
		return nil, err
	}

	// Use default env if set, otherwise use empty (will use root-level settings)
	env := project.Defaults.Env
	if env == "" {
		// Create a minimal LoadedConfig with just project settings
		return &config.LoadedConfig{
			Project:     project,
			Environment: "",
			Values:      make(map[string]any),
			ProjectRoot: projectRoot,
		}, nil
	}

	return config.Load(projectRoot, env)
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	secretName := args[0]

	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	loaded, err := loadConfigWithFallback(projectRoot)
	if err != nil {
		return err
	}

	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured in %s", config.ProjectFileName)
	}

	secretPrefix := loaded.GetSecretPrefix()

	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	ref := resolver.ParseSecretRef(secretName)
	// Apply prefix to non-full-path references
	if !ref.IsFullPath && secretPrefix != "" {
		ref.Name = secretPrefix + ref.Name
	}

	value, err := gcpResult.Client.GetSecret(ctx, ref, gcpProject)
	if err != nil {
		return err
	}

	fmt.Print(value)
	// Add newline if value doesn't end with one
	if len(value) > 0 && value[len(value)-1] != '\n' {
		fmt.Println()
	}

	return nil
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	secretName := args[0]
	var value string

	if fromFile != "" {
		// Read from file
		data, err := os.ReadFile(fromFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		value = string(data)
	} else if len(args) > 1 {
		// Value provided as argument
		value = args[1]
	} else {
		// Read from stdin
		fmt.Fprint(os.Stderr, "Enter secret value: ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read value: %w", err)
		}
		value = strings.TrimSuffix(line, "\n")
	}

	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	loaded, err := loadConfigWithFallback(projectRoot)
	if err != nil {
		return err
	}

	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured in %s", config.ProjectFileName)
	}

	secretPrefix := loaded.GetSecretPrefix()
	fullSecretName := secretName
	if secretPrefix != "" {
		fullSecretName = secretPrefix + secretName
	}

	if dryRun {
		fmt.Printf("Would set secret: %s (in GCP: %s)\n", secretName, fullSecretName)
		return nil
	}

	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	if err := gcpResult.Client.SetSecret(ctx, gcpProject, fullSecretName, value); err != nil {
		return err
	}

	color.Green("✓ Secret '%s' updated successfully", secretName)
	if secretPrefix != "" {
		fmt.Printf("  (stored as: %s)\n", fullSecretName)
	}
	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	secretName := args[0]

	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	loaded, err := loadConfigWithFallback(projectRoot)
	if err != nil {
		return err
	}

	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured in %s", config.ProjectFileName)
	}

	secretPrefix := loaded.GetSecretPrefix()
	fullSecretName := secretName
	if secretPrefix != "" {
		fullSecretName = secretPrefix + secretName
	}

	if !yesFlag {
		fmt.Printf("Would delete secret: %s\n", secretName)
		if secretPrefix != "" {
			fmt.Printf("  (in GCP: %s)\n", fullSecretName)
		}
		fmt.Println()
		fmt.Println("To confirm, run with --yes flag")
		return nil
	}

	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	if err := gcpResult.Client.DeleteSecret(ctx, gcpProject, fullSecretName); err != nil {
		return err
	}

	color.Green("✓ Secret '%s' deleted", secretName)
	if secretPrefix != "" {
		fmt.Printf("  (deleted: %s)\n", fullSecretName)
	}
	return nil
}

func runSecretUnused(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	project, err := config.LoadProject(projectRoot)
	if err != nil {
		return err
	}

	gcpProject := project.GCP.Project
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured in %s", config.ProjectFileName)
	}

	secretPrefix := project.GCP.SecretPrefix

	// Collect all secret references across all environments
	referencedSecrets := collectReferencedSecrets(projectRoot, project, secretPrefix)

	// List secrets from GCP
	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	secretsList, err := gcpResult.Client.ListSecrets(ctx, gcpProject)
	if err != nil {
		return err
	}

	// Find unused secrets
	unused := findUnusedSecrets(secretsList, referencedSecrets, secretPrefix)

	if len(unused) == 0 {
		color.Green("No unused secrets found")
		return nil
	}

	yellow := color.New(color.FgYellow)
	_, _ = yellow.Printf("Found %d potentially unused secret(s):\n\n", len(unused))

	fmt.Println("Warning: These secrets may be used by other services outside this config.")
	fmt.Println()

	for _, name := range unused {
		displayName := name
		if secretPrefix != "" {
			displayName = strings.TrimPrefix(name, secretPrefix)
		}
		fmt.Printf("  %s\n", displayName)
	}

	fmt.Println()
	fmt.Println("To delete a secret:")
	fmt.Println("  coffer secret delete <name> --yes")

	return nil
}

// collectReferencedSecrets scans all environments and returns a set of referenced secret names
func collectReferencedSecrets(projectRoot string, project *config.ProjectConfig, secretPrefix string) map[string]bool {
	referencedSecrets := make(map[string]bool)

	envs := project.ListEnvironments()
	for _, env := range envs {
		loaded, loadErr := config.Load(projectRoot, env)
		if loadErr != nil {
			continue // Skip environments that fail to load
		}

		flat := config.Flatten(loaded.Values)
		refs := resolver.FindSecretRefs(flat)

		for _, ref := range refs {
			// Store the full name (with prefix applied)
			name := ref.Name
			if !ref.IsFullPath && secretPrefix != "" {
				name = secretPrefix + ref.Name
			}
			referencedSecrets[name] = true
		}
	}

	return referencedSecrets
}

// findUnusedSecrets compares GCP secrets against referenced secrets and returns unused ones
func findUnusedSecrets(gcpSecrets []string, referenced map[string]bool, secretPrefix string) []string {
	var unused []string
	for _, s := range gcpSecrets {
		parts := strings.Split(s, "/")
		name := parts[len(parts)-1]

		// Filter by prefix if set
		if secretPrefix != "" && !strings.HasPrefix(name, secretPrefix) {
			continue
		}

		if !referenced[name] {
			unused = append(unused, name)
		}
	}
	return unused
}

func runSecretImport(cmd *cobra.Command, args []string) error {
	envFile := args[0]

	// Parse .env file
	entries, err := parseEnvFile(envFile)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No secrets found in", envFile)
		return nil
	}

	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	loaded, err := loadConfigWithFallback(projectRoot)
	if err != nil {
		return err
	}

	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return fmt.Errorf("no GCP project configured in %s", config.ProjectFileName)
	}

	secretPrefix := loaded.GetSecretPrefix()

	// Show what will be imported
	fmt.Printf("Found %d secret(s) to import:\n\n", len(entries))

	for _, entry := range entries {
		secretName := envKeyToSecretName(entry.key)
		fullName := secretName
		if secretPrefix != "" {
			fullName = secretPrefix + secretName
		}

		maskedValue := maskValue(entry.value)
		fmt.Printf("  %s = %s\n", entry.key, maskedValue)
		fmt.Printf("    -> %s\n", fullName)
	}

	fmt.Println()

	if !yesFlag {
		fmt.Println("To import these secrets, run with --yes flag")
		return nil
	}

	// Actually import - use longer timeout for batch operations
	gcpResult, ctx, err := newGCPClient(LongGCPTimeout)
	if err != nil {
		return err
	}
	defer gcpResult.Close()

	var imported, failed int
	for _, entry := range entries {
		secretName := envKeyToSecretName(entry.key)
		fullName := secretName
		if secretPrefix != "" {
			fullName = secretPrefix + secretName
		}

		if err := gcpResult.Client.SetSecret(ctx, gcpProject, fullName, entry.value); err != nil {
			color.Red("✗ Failed to import %s: %v", entry.key, err)
			failed++
		} else {
			color.Green("✓ Imported %s as %s", entry.key, fullName)
			imported++
		}
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("imported %d secret(s), %d failed", imported, failed)
	}

	color.Green("Successfully imported %d secret(s)", imported)
	return nil
}

type envEntry struct {
	key   string
	value string
}

func parseEnvFile(path string) ([]envEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var entries []envEntry
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Remove surrounding quotes from value
		value = unquoteValue(value)

		if key != "" && value != "" {
			entries = append(entries, envEntry{key: key, value: value})
		}
	}

	return entries, nil
}

func unquoteValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func envKeyToSecretName(key string) string {
	// Convert ENV_VAR_NAME to env-var-name
	name := strings.ToLower(key)
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func maskValue(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}
