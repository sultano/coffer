package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
)

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a single configuration value",
	Long: `Get a single configuration value using dot notation.

Examples:
  coffer get database.host
  coffer get app.log_level --env prod`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	loaded, err := config.Load(projectRoot, envName)
	if err != nil {
		return err
	}

	// Flatten config
	flat := config.Flatten(loaded.Values)

	// Look for the key
	value, ok := flat[key]
	if !ok {
		// Try to find partial matches for better error message
		var suggestions []string
		for k := range flat {
			if strings.HasPrefix(k, key+".") || strings.HasPrefix(k, key) {
				suggestions = append(suggestions, k)
			}
		}

		if len(suggestions) > 0 {
			return fmt.Errorf("key '%s' not found. Did you mean one of: %v", key, suggestions)
		}
		return fmt.Errorf("key '%s' not found", key)
	}

	// Check if value contains a secret reference
	if resolver.ContainsSecretRef(value) {
		gcpProject := loaded.GetGCPProject()
		if gcpProject == "" {
			return fmt.Errorf("no GCP project configured for environment '%s'", loaded.Environment)
		}

		gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
		if err != nil {
			return err
		}
		defer gcpResult.Close()

		secretPrefix := loaded.GetSecretPrefix()
		r := resolver.New(gcpResult.Client, gcpProject, secretPrefix)
		value, err = r.ResolveValue(ctx, value)
		if err != nil {
			return err
		}
	}

	fmt.Println(value)
	return nil
}
