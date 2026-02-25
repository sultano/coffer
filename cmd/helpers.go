package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sultano/coffer/internal/config"
	"github.com/sultano/coffer/internal/resolver"
	"github.com/sultano/coffer/internal/secrets"
)

// Timeout constants for GCP operations
const (
	DefaultGCPTimeout = 30 * time.Second
	QuickGCPTimeout   = 5 * time.Second
	LongGCPTimeout    = 120 * time.Second
)

// GCPClientResult holds a GCP client and its cleanup function
type GCPClientResult struct {
	Client secrets.Client
	Cancel context.CancelFunc
}

// Close cleans up the GCP client and cancels the context
func (r *GCPClientResult) Close() {
	if r.Client != nil {
		_ = r.Client.Close()
	}
	if r.Cancel != nil {
		r.Cancel()
	}
}

// newGCPClientFunc is the function used to create GCP clients.
// Tests can override this to inject mocks.
var newGCPClientFunc = defaultNewGCPClient

// newGCPClient creates a new GCP Secret Manager client with the given timeout
func newGCPClient(timeout time.Duration) (*GCPClientResult, context.Context, error) {
	return newGCPClientFunc(timeout)
}

func defaultNewGCPClient(timeout time.Duration) (*GCPClientResult, context.Context, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	client, err := secrets.New(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	return &GCPClientResult{Client: client, Cancel: cancel}, ctx, nil
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolveNestedSecrets resolves secret references in a nested config map
func resolveNestedSecrets(loaded *config.LoadedConfig, values map[string]any) (map[string]any, error) {
	refs := resolver.FindSecretRefsNested(values)
	if len(refs) == 0 {
		return values, nil
	}

	gcpProject := loaded.GetGCPProject()
	if gcpProject == "" {
		return nil, fmt.Errorf("no GCP project configured for environment '%s'", loaded.Environment)
	}

	gcpResult, ctx, err := newGCPClient(DefaultGCPTimeout)
	if err != nil {
		return nil, err
	}
	defer gcpResult.Close()

	secretPrefix := loaded.GetSecretPrefix()
	r := resolver.New(gcpResult.Client, gcpProject, secretPrefix)
	return r.ResolveAllNested(ctx, values)
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
