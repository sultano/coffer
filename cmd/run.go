package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
)

var runCmd = &cobra.Command{
	Use:   "run -- <command> [args...]",
	Short: "Run a command with config and secrets injected as environment variables",
	Long: `Run a command with configuration and secrets injected as environment variables.

Coffer merges your config files, resolves secret references from GCP Secret Manager,
and runs your command with all values available as environment variables.`,
	DisableFlagParsing: true,
	RunE:               runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	// Parse flags manually since we disabled flag parsing
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

	// Apply env var mapping
	envVars := config.ToEnvVars(resolved, loaded.Project.EnvMapping)

	if dryRun {
		return printDryRun(envVars, cmdArgs)
	}

	return executeCommand(cmdArgs, envVars)
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
				command.Process.Signal(sig)
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
