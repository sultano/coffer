package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/choreograph/coffer/internal/config"
	"github.com/choreograph/coffer/internal/resolver"
	"github.com/choreograph/coffer/internal/secrets"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var outputFormat string

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Output resolved configuration",
	Long: `Resolve and output configuration with secrets replaced.

Merges base config with environment overlay and local overrides,
then resolves all secret references from GCP Secret Manager.`,
	RunE: runResolve,
}

func init() {
	rootCmd.AddCommand(resolveCmd)
	resolveCmd.Flags().StringVarP(&outputFormat, "format", "f", "json", "output format: json, yaml, or dotenv")
}

func runResolve(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return err
	}

	loaded, err := config.Load(projectRoot, envName)
	if err != nil {
		return err
	}

	// Flatten config to key-value pairs
	flat := config.Flatten(loaded.Values)

	// Check if there are any secret references
	refs := resolver.FindSecretRefs(flat)

	var resolved map[string]string
	if len(refs) > 0 && !dryRun {
		// Resolve secrets from GCP
		gcpProject := loaded.GetGCPProject()
		if gcpProject == "" {
			return fmt.Errorf("no GCP project configured for environment '%s'", loaded.Environment)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := secrets.New(ctx)
		if err != nil {
			return fmt.Errorf("failed to connect to GCP: %w", err)
		}
		defer client.Close()

		secretPrefix := loaded.GetSecretPrefix()
		r := resolver.New(client, gcpProject, secretPrefix)
		resolved, err = r.ResolveAll(ctx, flat)
		if err != nil {
			return err
		}
	} else {
		resolved = flat
	}

	// Apply env var mapping
	envVars := config.ToEnvVars(resolved, loaded.Project.EnvMapping)

	return outputResult(envVars, outputFormat)
}

func outputResult(envVars map[string]string, format string) error {
	switch format {
	case "json":
		return outputJSON(envVars)
	case "yaml":
		return outputYAML(envVars)
	case "dotenv", "env":
		return outputDotenv(envVars)
	default:
		return fmt.Errorf("unsupported format: %s (use json, yaml, or dotenv)", format)
	}
}

func outputJSON(envVars map[string]string) error {
	data, err := json.MarshalIndent(envVars, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputYAML(envVars map[string]string) error {
	data, err := yaml.Marshal(envVars)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func outputDotenv(envVars map[string]string) error {
	// Sort keys for consistent output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := envVars[k]
		fmt.Printf("%s=%s\n", k, quoteForDotenv(v))
	}
	return nil
}

func quoteForDotenv(value string) string {
	needsQuotes := false
	for _, r := range value {
		if r == ' ' || r == '\n' || r == '\t' || r == '"' || r == '\'' || r == '$' || r == '`' || r == '\\' || r == '#' {
			needsQuotes = true
			break
		}
	}

	if !needsQuotes {
		return value
	}

	// Escape and quote
	result := "\""
	for _, r := range value {
		switch r {
		case '"':
			result += "\\\""
		case '\\':
			result += "\\\\"
		case '\n':
			result += "\\n"
		case '\t':
			result += "\\t"
		case '$':
			result += "\\$"
		case '`':
			result += "\\`"
		default:
			result += string(r)
		}
	}
	result += "\""
	return result
}
