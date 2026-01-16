package secrets

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sultano/coffer/internal/resolver"
)

const (
	maxRetries     = 3
	initialBackoff = 100 * time.Millisecond
	maxBackoff     = 5 * time.Second
)

// GCPClient implements resolver.SecretProvider using GCP Secret Manager
type GCPClient struct {
	client *secretmanager.Client
}

// New creates a new GCP Secret Manager client
func New(ctx context.Context) (*GCPClient, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, formatAuthError(err)
	}
	return &GCPClient{client: client}, nil
}

// Close closes the client connection
func (c *GCPClient) Close() error {
	return c.client.Close()
}

// GetSecret fetches a secret value from GCP Secret Manager
func (c *GCPClient) GetSecret(ctx context.Context, ref resolver.SecretRef, gcpProject string) (string, error) {
	name := buildSecretPath(ref, gcpProject)

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	result, err := retry(ctx, func() (*secretmanagerpb.AccessSecretVersionResponse, error) {
		return c.client.AccessSecretVersion(ctx, req)
	})
	if err != nil {
		if isNotFoundError(err) {
			return "", fmt.Errorf("secret '%s' not found in project '%s'", ref.Name, gcpProject)
		}
		if isPermissionError(err) {
			return "", FormatPermissionError(ref.Name, gcpProject, "accessing")
		}
		return "", fmt.Errorf("failed to access secret '%s': %w", ref.Name, err)
	}

	return string(result.Payload.Data), nil
}

// buildSecretPath constructs the full GCP Secret Manager path
// BEHAVIOR: Full paths (IsFullPath=true) are used as-is, with version appended if missing
// BEHAVIOR: Simple refs are expanded to projects/{project}/secrets/{name}/versions/{version}
// BEHAVIOR: Default version is "latest" when not specified
func buildSecretPath(ref resolver.SecretRef, gcpProject string) string {
	if ref.IsFullPath {
		path := ref.FullPath
		// Append version if not already included in path
		if ref.Version != "" && !containsVersion(path) {
			path += "/versions/" + ref.Version
		} else if !containsVersion(path) {
			path += "/versions/latest"
		}
		return path
	}

	version := ref.Version
	if version == "" {
		version = "latest"
	}

	return fmt.Sprintf("projects/%s/secrets/%s/versions/%s", gcpProject, ref.Name, version)
}

func containsVersion(path string) bool {
	return strings.Contains(path, "/versions/")
}

// ListSecrets lists all secrets in a GCP project
func (c *GCPClient) ListSecrets(ctx context.Context, gcpProject string) ([]string, error) {
	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", gcpProject),
	}

	iter := c.client.ListSecrets(ctx, req)
	var secrets []string

	for {
		secret, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}
		secrets = append(secrets, secret.Name)
	}

	return secrets, nil
}

// CreateSecret creates a new secret in GCP Secret Manager
func (c *GCPClient) CreateSecret(ctx context.Context, gcpProject, secretName string) error {
	req := &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", gcpProject),
		SecretId: secretName,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}

	err := retryNoResult(ctx, func() error {
		_, err := c.client.CreateSecret(ctx, req)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create secret '%s': %w", secretName, err)
	}

	return nil
}

// AddSecretVersion adds a new version to an existing secret
func (c *GCPClient) AddSecretVersion(ctx context.Context, gcpProject, secretName, value string) error {
	req := &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%s/secrets/%s", gcpProject, secretName),
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	}

	err := retryNoResult(ctx, func() error {
		_, err := c.client.AddSecretVersion(ctx, req)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to add version to secret '%s': %w", secretName, err)
	}

	return nil
}

// SetSecret creates or updates a secret with a new value
func (c *GCPClient) SetSecret(ctx context.Context, gcpProject, secretName, value string) error {
	// Try to add version first (secret may already exist)
	err := c.AddSecretVersion(ctx, gcpProject, secretName, value)
	if err == nil {
		return nil
	}

	// If failed, try to create the secret first
	if createErr := c.CreateSecret(ctx, gcpProject, secretName); createErr != nil {
		return fmt.Errorf("secret does not exist and failed to create: %w", createErr)
	}

	// Now add the version
	return c.AddSecretVersion(ctx, gcpProject, secretName, value)
}

// DeleteSecret deletes a secret from GCP Secret Manager
func (c *GCPClient) DeleteSecret(ctx context.Context, gcpProject, secretName string) error {
	req := &secretmanagerpb.DeleteSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s", gcpProject, secretName),
	}

	err := retryNoResult(ctx, func() error {
		return c.client.DeleteSecret(ctx, req)
	})
	if err != nil {
		if isNotFoundError(err) {
			return fmt.Errorf("secret '%s' not found", secretName)
		}
		if isPermissionError(err) {
			return FormatPermissionError(secretName, gcpProject, "deleting")
		}
		return fmt.Errorf("failed to delete secret '%s': %w", secretName, err)
	}

	return nil
}

// SecretExists checks if a secret exists in GCP Secret Manager
func (c *GCPClient) SecretExists(ctx context.Context, gcpProject, secretName string) (bool, error) {
	req := &secretmanagerpb.GetSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s", gcpProject, secretName),
	}

	_, err := retry(ctx, func() (*secretmanagerpb.Secret, error) {
		return c.client.GetSecret(ctx, req)
	})
	if err != nil {
		// Check if it's a not found error
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.NotFound
	}
	msg := err.Error()
	return strings.Contains(msg, "NotFound") || strings.Contains(msg, "not found")
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.PermissionDenied
	}
	msg := err.Error()
	return strings.Contains(msg, "PermissionDenied") || strings.Contains(msg, "permission denied") || strings.Contains(msg, "403")
}

// formatAuthError provides actionable error messages for authentication failures
func formatAuthError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	// No credentials found
	if strings.Contains(msg, "could not find default credentials") ||
		strings.Contains(msg, "google: could not find") {
		return fmt.Errorf(`GCP credentials not found

Run one of:
  gcloud auth application-default login    (for local development)
  export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json    (for service account)

For more info: https://cloud.google.com/docs/authentication`)
	}

	// Credentials expired or invalid
	if strings.Contains(msg, "token expired") ||
		strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "Token has been expired") {
		return fmt.Errorf(`GCP credentials expired

Run: gcloud auth application-default login`)
	}

	// Project not set or invalid
	if strings.Contains(msg, "project") && strings.Contains(msg, "not found") {
		return fmt.Errorf(`GCP project not found or not accessible

Check:
  1. Project ID is correct in your config
  2. You have access to the project: gcloud projects describe PROJECT_ID`)
	}

	// Generic auth failure
	if strings.Contains(msg, "authentication") || strings.Contains(msg, "credentials") {
		return fmt.Errorf(`GCP authentication failed: %w

Run: coffer auth status    (to check auth status)
Run: coffer auth login     (to authenticate)`, err)
	}

	return fmt.Errorf("failed to create GCP client: %w", err)
}

// FormatPermissionError provides actionable error messages for permission denied errors
func FormatPermissionError(secretName, gcpProject, operation string) error {
	return fmt.Errorf(`permission denied %s secret '%s'

Check:
  1. You are authenticated: coffer auth status
  2. You have the required IAM role on project '%s':
     - Secret read: roles/secretmanager.secretAccessor
     - Secret write: roles/secretmanager.admin

Grant access:
  gcloud projects add-iam-policy-binding %s \
    --member="user:YOUR_EMAIL" \
    --role="roles/secretmanager.secretAccessor"`, operation, secretName, gcpProject, gcpProject)
}

// isRetriableError returns true for transient errors that should be retried
func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	// Don't retry permanent errors
	if isNotFoundError(err) || isPermissionError(err) {
		return false
	}
	// Check gRPC status codes
	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.DeadlineExceeded:
			return true
		}
	}
	// Retry on common transient error messages
	msg := err.Error()
	return strings.Contains(msg, "connection") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "quota")
}

// retry executes fn with exponential backoff
func retry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var result T
	var err error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if !isRetriableError(err) {
			return result, err
		}
		if attempt == maxRetries {
			break
		}
		// Exponential backoff with jitter
		backoff := initialBackoff * time.Duration(1<<attempt)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		backoff = backoff + jitter

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return result, fmt.Errorf("failed after %d retries: %w", maxRetries, err)
}

// retryNoResult executes fn with exponential backoff for functions that return only error
func retryNoResult(ctx context.Context, fn func() error) error {
	_, err := retry(ctx, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
