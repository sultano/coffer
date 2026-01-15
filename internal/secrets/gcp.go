package secrets

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/choreograph/coffer/internal/resolver"
)

// GCPClient implements resolver.SecretProvider using GCP Secret Manager
type GCPClient struct {
	client *secretmanager.Client
}

// New creates a new GCP Secret Manager client
func New(ctx context.Context) (*GCPClient, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret manager client: %w", err)
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

	result, err := c.client.AccessSecretVersion(ctx, req)
	if err != nil {
		if isNotFoundError(err) {
			return "", fmt.Errorf("secret '%s' not found in project '%s'", ref.Name, gcpProject)
		}
		if isPermissionError(err) {
			return "", fmt.Errorf("permission denied accessing secret '%s' - check your GCP credentials and IAM permissions", ref.Name)
		}
		return "", fmt.Errorf("failed to access secret '%s': %w", ref.Name, err)
	}

	return string(result.Payload.Data), nil
}

// buildSecretPath constructs the full GCP Secret Manager path
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
	return len(path) > 0 && path[len(path)-1] != '/'
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
		if err != nil {
			break
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

	_, err := c.client.CreateSecret(ctx, req)
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

	_, err := c.client.AddSecretVersion(ctx, req)
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

	if err := c.client.DeleteSecret(ctx, req); err != nil {
		if isNotFoundError(err) {
			return fmt.Errorf("secret '%s' not found", secretName)
		}
		if isPermissionError(err) {
			return fmt.Errorf("permission denied deleting secret '%s' - check your GCP IAM permissions", secretName)
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

	_, err := c.client.GetSecret(ctx, req)
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
	return err != nil && (contains(err.Error(), "NotFound") || contains(err.Error(), "not found"))
}

func isPermissionError(err error) bool {
	return err != nil && (contains(err.Error(), "PermissionDenied") || contains(err.Error(), "permission denied") || contains(err.Error(), "403"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
