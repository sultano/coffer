package secrets

import (
	"errors"
	"strings"
	"testing"

	"github.com/sultano/coffer/internal/resolver"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBuildSecretPath(t *testing.T) {
	tests := []struct {
		name       string
		ref        resolver.SecretRef
		gcpProject string
		expected   string
	}{
		{
			name:       "simple ref with default version",
			ref:        resolver.SecretRef{Name: "db-password", Version: "latest"},
			gcpProject: "my-project",
			expected:   "projects/my-project/secrets/db-password/versions/latest",
		},
		{
			name:       "simple ref with explicit version",
			ref:        resolver.SecretRef{Name: "db-password", Version: "3"},
			gcpProject: "my-project",
			expected:   "projects/my-project/secrets/db-password/versions/3",
		},
		{
			name:       "simple ref with empty version defaults to latest",
			ref:        resolver.SecretRef{Name: "api-key"},
			gcpProject: "my-project",
			expected:   "projects/my-project/secrets/api-key/versions/latest",
		},
		{
			name: "full path without version",
			ref: resolver.SecretRef{
				Name:       "other-secret",
				IsFullPath: true,
				FullPath:   "projects/other-project/secrets/other-secret",
			},
			gcpProject: "my-project",
			expected:   "projects/other-project/secrets/other-secret/versions/latest",
		},
		{
			name: "full path with version",
			ref: resolver.SecretRef{
				Name:       "other-secret",
				Version:    "5",
				IsFullPath: true,
				FullPath:   "projects/other-project/secrets/other-secret",
			},
			gcpProject: "my-project",
			expected:   "projects/other-project/secrets/other-secret/versions/5",
		},
		{
			name: "full path already containing version",
			ref: resolver.SecretRef{
				Name:       "other-secret",
				IsFullPath: true,
				FullPath:   "projects/other-project/secrets/other-secret/versions/2",
			},
			gcpProject: "my-project",
			expected:   "projects/other-project/secrets/other-secret/versions/2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSecretPath(tt.ref, tt.gcpProject)
			if got != tt.expected {
				t.Errorf("buildSecretPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestContainsVersion(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"projects/p/secrets/s/versions/1", true},
		{"projects/p/secrets/s/versions/latest", true},
		{"projects/p/secrets/s", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := containsVersion(tt.path); got != tt.expected {
				t.Errorf("containsVersion(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"gRPC NotFound", status.Error(codes.NotFound, "secret not found"), true},
		{"string not found", errors.New("resource not found"), true},
		{"string NotFound", errors.New("NotFound: no such secret"), true},
		{"other error", errors.New("connection refused"), false},
		{"permission denied", status.Error(codes.PermissionDenied, "denied"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFoundError(tt.err); got != tt.expected {
				t.Errorf("isNotFoundError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormatAuthError_Nil(t *testing.T) {
	err := formatAuthError(nil)
	if err != nil {
		t.Errorf("expected nil for nil input, got %v", err)
	}
}

func TestFormatAuthError_ProjectNotFound(t *testing.T) {
	err := formatAuthError(errors.New("project xyz not found"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "GCP project not found") {
		t.Errorf("expected project not found message, got: %v", err)
	}
}

func TestFormatAuthError_GenericAuth(t *testing.T) {
	err := formatAuthError(errors.New("authentication failed"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "coffer auth status") {
		t.Errorf("expected auth status suggestion, got: %v", err)
	}
}

func TestIsPermissionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"gRPC PermissionDenied", status.Error(codes.PermissionDenied, "denied"), true},
		{"string PermissionDenied", errors.New("PermissionDenied: access denied"), true},
		{"string permission denied lowercase", errors.New("permission denied for resource"), true},
		{"string 403", errors.New("HTTP 403 forbidden"), true},
		{"other error", errors.New("connection refused"), false},
		{"not found", status.Error(codes.NotFound, "not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermissionError(tt.err); got != tt.expected {
				t.Errorf("isPermissionError() = %v, want %v", got, tt.expected)
			}
		})
	}
}
