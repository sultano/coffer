package resolver

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// SecretRef represents a parsed secret reference
type SecretRef struct {
	Name       string // Secret name (e.g., "db-password")
	Version    string // Optional version (e.g., "2" or "latest")
	FullPath   string // Full GCP path if specified (e.g., "projects/x/secrets/y")
	IsFullPath bool   // True if FullPath was explicitly specified
}

// BEHAVIOR: Pattern must match ${secret:...} syntax exactly
// Supports: ${secret:name}, ${secret:name@version}, ${secret:projects/x/secrets/name}
// The capture group extracts everything between "secret:" and "}"
var secretRefPattern = regexp.MustCompile(`\$\{secret:([^}]+)\}`)

// SecretProvider fetches secret values by name
type SecretProvider interface {
	GetSecret(ctx context.Context, ref SecretRef, gcpProject string) (string, error)
}

// Resolver resolves secret references in configuration values
type Resolver struct {
	provider     SecretProvider
	gcpProject   string
	secretPrefix string
}

// New creates a new Resolver with the given secret provider
func New(provider SecretProvider, gcpProject, secretPrefix string) *Resolver {
	return &Resolver{
		provider:     provider,
		gcpProject:   gcpProject,
		secretPrefix: secretPrefix,
	}
}

// ResolveAll resolves all secret references in the config map
func (r *Resolver) ResolveAll(ctx context.Context, config map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(config))

	for key, value := range config {
		resolved, err := r.ResolveValue(ctx, value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %s: %w", key, err)
		}
		result[key] = resolved
	}

	return result, nil
}

// ResolveValue resolves secret references in a single value
func (r *Resolver) ResolveValue(ctx context.Context, value string) (string, error) {
	if !ContainsSecretRef(value) {
		return value, nil
	}

	result := value
	matches := secretRefPattern.FindAllStringSubmatch(value, -1)

	for _, match := range matches {
		fullMatch := match[0]
		refStr := match[1]

		ref := ParseSecretRef(refStr)

		// Apply prefix to non-full-path references
		if !ref.IsFullPath && r.secretPrefix != "" {
			ref.Name = r.secretPrefix + ref.Name
		}

		secretValue, err := r.provider.GetSecret(ctx, ref, r.gcpProject)
		if err != nil {
			return "", err
		}

		result = strings.Replace(result, fullMatch, secretValue, 1)
	}

	return result, nil
}

// ContainsSecretRef checks if a value contains a secret reference
func ContainsSecretRef(value string) bool {
	return strings.Contains(value, "${secret:")
}

// ParseSecretRef parses a secret reference string
// BEHAVIOR: Must handle three formats:
//  1. Simple: "secret-name" -> Name="secret-name", Version="latest"
//  2. Versioned: "secret-name@2" -> Name="secret-name", Version="2"
//  3. Full path: "projects/x/secrets/y" -> IsFullPath=true, FullPath set
//
// BEHAVIOR: Full paths take precedence - if starts with "projects/", treat as full path
// BEHAVIOR: Default version is "latest" when not specified
func ParseSecretRef(refStr string) SecretRef {
	ref := SecretRef{}

	// Check for full path (projects/x/secrets/y)
	if strings.HasPrefix(refStr, "projects/") {
		ref.FullPath = refStr
		ref.IsFullPath = true

		// Extract name from path
		parts := strings.Split(refStr, "/")
		if len(parts) >= 4 {
			ref.Name = parts[3]
			// Check for version suffix
			if idx := strings.Index(ref.Name, "@"); idx != -1 {
				ref.Version = ref.Name[idx+1:]
				ref.Name = ref.Name[:idx]
			}
		}
		return ref
	}

	// Simple reference: name or name@version
	if idx := strings.Index(refStr, "@"); idx != -1 {
		ref.Name = refStr[:idx]
		ref.Version = refStr[idx+1:]
	} else {
		ref.Name = refStr
		ref.Version = "latest"
	}

	return ref
}

// FindSecretRefs finds all secret references in a config map
func FindSecretRefs(config map[string]string) []SecretRef {
	refs := make([]SecretRef, 0)
	seen := make(map[string]bool)

	for _, value := range config {
		matches := secretRefPattern.FindAllStringSubmatch(value, -1)
		for _, match := range matches {
			refStr := match[1]
			if !seen[refStr] {
				seen[refStr] = true
				refs = append(refs, ParseSecretRef(refStr))
			}
		}
	}

	return refs
}
