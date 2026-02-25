package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
	"gopkg.in/yaml.v3"
)

var (
	configFile string
	includeAll bool
)

var runCmd = &cobra.Command{
	Use:   "run -- <command> [args...]",
	Short: "Run a command with config and secrets injected as environment variables",
	Long: `Run a command with configuration and secrets injected as environment variables.

Coffer merges your config files, resolves secret references from GCP Secret Manager,
and runs your command with all values available as environment variables.

By default, only values containing secret references are injected as env vars.
Use --all to inject all config values. Use --config-file to write the full
resolved config to a file for the app to read.`,
	DisableFlagParsing: true,
	RunE:               runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	cmdArgs, err := parseRunArgs(args)
	if err != nil {
		return err
	}

	if len(cmdArgs) == 0 {
		return fmt.Errorf("no command specified - usage: coffer run -- <command> [args...]")
	}

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

	// Resolve secret references
	refs := resolver.FindSecretRefs(flat)

	var resolved map[string]string
	if len(refs) > 0 {
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

	// BEHAVIOR: By default only keys with secret references become env vars
	// Use --all to inject all config values as env vars
	envResolved := resolved
	if !includeAll {
		secretKeys := resolver.KeysWithSecretRefs(flat)
		envResolved = filterByKeys(resolved, secretKeys)
	}

	envVars := config.ToEnvVars(envResolved, loaded.Project.EnvMapping)

	// Write config file if requested
	if configFile != "" {
		if err := writeConfigFile(loaded, configFile); err != nil {
			return err
		}
	}

	if dryRun {
		if configFile != "" {
			format, _ := formatFromPath(configFile)
			fmt.Printf("Config file: %s (format: %s)\n", configFile, format)
			fmt.Println()
		}
		return printDryRun(envVars, cmdArgs)
	}

	return executeCommand(cmdArgs, envVars)
}

func writeConfigFile(loaded *config.LoadedConfig, path string) error {
	format, err := formatFromPath(path)
	if err != nil {
		return err
	}

	values := loaded.Values

	// Resolve secrets in nested config
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

	if dryRun {
		return nil
	}

	data, err := marshalConfig(values, format)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func formatFromPath(path string) (string, error) {
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		return "yaml", nil
	case ".json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported config file extension %q (use .yaml, .yml, or .json)", ext)
	}
}

func marshalConfig(values map[string]any, format string) ([]byte, error) {
	switch format {
	case "yaml":
		return yaml.Marshal(values)
	case "json":
		return json.MarshalIndent(values, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// filterByKeys returns a new map containing only the keys present in the keys set
func filterByKeys(m map[string]string, keys map[string]bool) map[string]string {
	result := make(map[string]string, len(keys))
	for k, v := range m {
		if keys[k] {
			result[k] = v
		}
	}
	return result
}

func parseRunArgs(args []string) ([]string, error) {
	// Find the -- separator
	for i, arg := range args {
		if arg == "--" {
			// Process flags before --
			for j := 0; j < i; j++ {
				switch args[j] {
				case "-e", "--env":
					if j+1 < i {
						envName = args[j+1]
						j++
					}
				case "-p", "--project":
					if j+1 < i {
						projectPath = args[j+1]
						j++
					}
				case "-c", "--config-file":
					if j+1 < i {
						configFile = args[j+1]
						j++
					}
				case "--all":
					includeAll = true
				case "--dry-run":
					dryRun = true
				case "--no-color":
					noColor = true
					color.NoColor = true
				}
			}
			return args[i+1:], nil
		}
	}

	// No -- found, treat all args as the command
	return args, nil
}

func printDryRun(envVars map[string]string, cmdArgs []string) error {
	fmt.Println("Dry run - would execute with environment:")
	fmt.Println()

	// Sort keys for consistent output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := envVars[k]
		// Mask potential secrets
		display := v
		if len(display) > 20 {
			display = display[:20] + "..."
		}
		fmt.Printf("  %s=%s\n", k, display)
	}

	fmt.Println()
	fmt.Printf("Command: %v\n", cmdArgs)

	return nil
}

func executeCommand(cmdArgs []string, envVars map[string]string) error {
	// Build environment from current env + our vars
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}

	// Create the command
	command := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	command.Env = env
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	// Set up signal forwarding
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the command
	if err := command.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Forward signals to child process
	go func() {
		for sig := range sigChan {
			if command.Process != nil {
				_ = command.Process.Signal(sig)
			}
		}
	}()

	// Wait for command to complete
	err := command.Wait()
	signal.Stop(sigChan)
	close(sigChan)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Propagate the exit code
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}
