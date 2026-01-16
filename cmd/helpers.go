package cmd

import (
	"context"
	"os"
	"time"

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
	Client *secrets.GCPClient
	Cancel context.CancelFunc
}

// Close cleans up the GCP client and cancels the context
func (r *GCPClientResult) Close() {
	if r.Client != nil {
		r.Client.Close()
	}
	if r.Cancel != nil {
		r.Cancel()
	}
}

// newGCPClient creates a new GCP Secret Manager client with the given timeout
func newGCPClient(timeout time.Duration) (*GCPClientResult, context.Context, error) {
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
