package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	projectPath string
	envName     string
	dryRun      bool
	noColor     bool

	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "coffer",
	Short: "Configuration and secrets management using GCP Secret Manager",
	Long: `Coffer is a language-agnostic CLI for managing configuration and secrets.

Config is versioned in git with environment-specific overlays.
Secrets are stored in GCP Secret Manager and referenced in configs.
At runtime, coffer merges configs and resolves secret references.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if noColor || os.Getenv("NO_COLOR") != "" {
			color.NoColor = true
		}
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&projectPath, "path", "p", "", "path to project directory (default: current directory)")
	rootCmd.PersistentFlags().StringVarP(&envName, "env", "e", "", "environment name (e.g., dev, staging, prod)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview what would be done without making changes")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	rootCmd.Version = version
	rootCmd.SetVersionTemplate(versionString() + "\n")
	rootCmd.AddCommand(versionCmd)
}

func versionString() string {
	return fmt.Sprintf("coffer %s (commit: %s, built: %s)", version, commit, date)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(versionString())
	},
}

func getProjectRoot() (string, error) {
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return "", fmt.Errorf("invalid project path: %w", err)
		}
		return abs, nil
	}
	return os.Getwd()
}
