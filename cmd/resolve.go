package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
	"gopkg.in/yaml.v3"
)

var outputFormat string

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Output configuration with secrets resolved",
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

	switch outputFormat {
	case "dotenv", "env":
		return resolveFlatOutput(loaded)
	case "json", "yaml":
		return resolveNestedOutput(loaded, outputFormat)
	default:
		return fmt.Errorf("unsupported format: %s (use json, yaml, or dotenv)", outputFormat)
	}
}

// resolveNestedOutput outputs the nested config structure with secrets resolved
func resolveNestedOutput(loaded *config.LoadedConfig, format string) error {
	values := loaded.Values

	refs := resolver.FindSecretRefsNested(values)
	if len(refs) > 0 && !dryRun {
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
		resolved, err := r.ResolveAllNested(ctx, values)
		if err != nil {
			return err
		}
		values = resolved
	}

	switch format {
	case "json":
		return outputNestedJSON(values)
	case "yaml":
		return outputNestedYAML(values)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// resolveFlatOutput outputs flat env var key-value pairs (for dotenv format)
func resolveFlatOutput(loaded *config.LoadedConfig) error {
	flat := config.Flatten(loaded.Values)

	refs := resolver.FindSecretRefs(flat)

	var resolved map[string]string
	if len(refs) > 0 && !dryRun {
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
		resolved, err = r.ResolveAll(ctx, flat)
		if err != nil {
			return err
		}
	} else {
		resolved = flat
	}

	envVars := config.ToEnvVars(resolved, loaded.Project.EnvMapping)
	return outputDotenv(envVars)
}

func outputNestedJSON(values map[string]any) error {
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputNestedYAML(values map[string]any) error {
	data, err := yaml.Marshal(values)
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
		fmt.Printf("%s=%s\n", k, config.QuoteForDotenv(v))
	}
	return nil
}
