package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"cloud.google.com/go/secretmanager/apiv1"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage GCP authentication",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check GCP authentication status",
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

	// Check gcloud account
	account, err := getGCloudAccount()
	if err != nil {
		_, _ = red.Printf("✗ Not authenticated to gcloud\n")
		fmt.Println("  Run: gcloud auth application-default login")
		return nil
	}
	_, _ = green.Printf("✓ Authenticated as: %s\n", account)

	// Check project
	project, err := getGCloudProject()
	if err == nil && project != "" {
		_, _ = green.Printf("✓ GCP Project: %s\n", project)
	}

	// Check Secret Manager access
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		_, _ = red.Printf("✗ Secret Manager access: failed to create client\n")
		fmt.Printf("  Error: %v\n", err)
		return nil
	}
	defer func() { _ = client.Close() }()

	_, _ = green.Printf("✓ Secret Manager access: granted\n")

	return nil
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
	fmt.Println("Run 'coffer auth status' to verify access")

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
