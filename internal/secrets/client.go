package secrets

import (
	"context"

	"github.com/sultano/coffer/internal/resolver"
)

// Client defines the interface for secret management operations.
// GCPClient implements this interface.
type Client interface {
	GetSecret(ctx context.Context, ref resolver.SecretRef, gcpProject string) (string, error)
	ListSecrets(ctx context.Context, gcpProject string) ([]string, error)
	CreateSecret(ctx context.Context, gcpProject, secretName string) error
	AddSecretVersion(ctx context.Context, gcpProject, secretName, value string) error
	SetSecret(ctx context.Context, gcpProject, secretName, value string) error
	DeleteSecret(ctx context.Context, gcpProject, secretName string) error
	SecretExists(ctx context.Context, gcpProject, secretName string) (bool, error)
	Close() error
}
