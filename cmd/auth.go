package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage GCP authentication",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check that Application Default Credentials are present",
	RunE:  runAuthStatus,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate to GCP (wraps gcloud auth)",
	RunE:  runAuthLogin,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLoginCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	fmt.Println("Checking GCP authentication...")
	fmt.Println()

	if err := findADC(); err != nil {
		_, _ = red.Printf("✗ No Application Default Credentials found\n")
		fmt.Println("  Run: coffer auth login")
		return nil
	}
	_, _ = green.Printf("✓ Application Default Credentials found\n")

	// The gcloud CLI login is separate from ADC and may be a different identity.
	if account, err := getGCloudAccount(); err == nil {
		fmt.Printf("  gcloud CLI account: %s\n", account)
	}

	fmt.Println()
	fmt.Println("Secret Manager access is not verified here. Run: coffer check --env <env>")

	return nil
}

// findADC reports whether Application Default Credentials can be found,
// without using them to call any API.
func findADC() error {
	ctx, cancel := context.WithTimeout(context.Background(), QuickGCPTimeout)
	defer cancel()
	_, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	return err
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	fmt.Println("Starting GCP authentication...")
	fmt.Println()

	// Run gcloud auth application-default login
	gcloudCmd := exec.Command("gcloud", "auth", "application-default", "login")
	gcloudCmd.Stdin = os.Stdin
	gcloudCmd.Stdout = os.Stdout
	gcloudCmd.Stderr = os.Stderr

	if err := gcloudCmd.Run(); err != nil {
		return fmt.Errorf("gcloud auth failed: %w", err)
	}

	fmt.Println()
	color.Green("✓ Authentication complete")
	fmt.Println()
	fmt.Println("Run 'coffer check --env <env>' to verify Secret Manager access")

	return nil
}

func getGCloudAccount() (string, error) {
	cmd := exec.Command("gcloud", "config", "get-value", "account")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	account := trimNewline(string(output))
	if account == "" || account == "(unset)" {
		return "", fmt.Errorf("no account configured")
	}
	return account, nil
}

func getGCloudProject() (string, error) {
	cmd := exec.Command("gcloud", "config", "get-value", "project")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	project := trimNewline(string(output))
	if project == "(unset)" {
		return "", nil
	}
	return project, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
