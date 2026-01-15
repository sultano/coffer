package resolver

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestContainsSecretRef(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"plain text", false},
		{"${secret:db-password}", true},
		{"prefix ${secret:key} suffix", true},
		{"${other:ref}", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ContainsSecretRef(tt.input)
			if result != tt.expected {
				t.Errorf("ContainsSecretRef(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseSecretRef(t *testing.T) {
	tests := []struct {
		input    string
		expected SecretRef
	}{
		{
			input: "db-password",
			expected: SecretRef{
				Name:    "db-password",
				Version: "latest",
			},
		},
		{
			input: "db-password@2",
			expected: SecretRef{
				Name:    "db-password",
				Version: "2",
			},
		},
		{
			input: "projects/other-project/secrets/my-secret",
			expected: SecretRef{
				Name:       "my-secret",
				FullPath:   "projects/other-project/secrets/my-secret",
				IsFullPath: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseSecretRef(tt.input)
			if result.Name != tt.expected.Name {
				t.Errorf("ParseSecretRef(%q).Name = %q, want %q", tt.input, result.Name, tt.expected.Name)
			}
			if result.Version != tt.expected.Version {
				t.Errorf("ParseSecretRef(%q).Version = %q, want %q", tt.input, result.Version, tt.expected.Version)
			}
			if result.IsFullPath != tt.expected.IsFullPath {
				t.Errorf("ParseSecretRef(%q).IsFullPath = %v, want %v", tt.input, result.IsFullPath, tt.expected.IsFullPath)
			}
		})
	}
}

func TestFindSecretRefs(t *testing.T) {
	config := map[string]string{
		"database.host":     "localhost",
		"database.password": "${secret:db-password}",
		"api.key":           "${secret:api-key@2}",
		"plain":             "value",
	}

	refs := FindSecretRefs(config)

	if len(refs) != 2 {
		t.Errorf("FindSecretRefs() returned %d refs, want 2", len(refs))
	}

	names := make(map[string]bool)
	for _, ref := range refs {
		names[ref.Name] = true
	}

	if !names["db-password"] {
		t.Error("expected to find db-password ref")
	}
	if !names["api-key"] {
		t.Error("expected to find api-key ref")
	}
}

func TestFindSecretRefs_NoDuplicates(t *testing.T) {
	config := map[string]string{
		"key1": "${secret:same-secret}",
		"key2": "${secret:same-secret}",
	}

	refs := FindSecretRefs(config)

	if len(refs) != 1 {
		t.Errorf("FindSecretRefs() returned %d refs, want 1 (no duplicates)", len(refs))
	}
}

func TestSecretRefPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected [][]string
	}{
		{
			input:    "${secret:db-password}",
			expected: [][]string{{"${secret:db-password}", "db-password"}},
		},
		{
			input:    "prefix ${secret:key} suffix",
			expected: [][]string{{"${secret:key}", "key"}},
		},
		{
			input: "${secret:key1} and ${secret:key2}",
			expected: [][]string{
				{"${secret:key1}", "key1"},
				{"${secret:key2}", "key2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := secretRefPattern.FindAllStringSubmatch(tt.input, -1)
			if !reflect.DeepEqual(matches, tt.expected) {
				t.Errorf("pattern.FindAllStringSubmatch(%q) = %v, want %v", tt.input, matches, tt.expected)
			}
		})
	}
}

// MockSecretProvider implements SecretProvider for testing
type MockSecretProvider struct {
	secrets map[string]string
	calls   []SecretRef
}

func NewMockProvider(secrets map[string]string) *MockSecretProvider {
	return &MockSecretProvider{secrets: secrets}
}

func (m *MockSecretProvider) GetSecret(ctx context.Context, ref SecretRef, gcpProject string) (string, error) {
	m.calls = append(m.calls, ref)
	if val, ok := m.secrets[ref.Name]; ok {
		return val, nil
	}
	return "", fmt.Errorf("secret not found: %s", ref.Name)
}

func TestResolver_ResolveValue(t *testing.T) {
	provider := NewMockProvider(map[string]string{
		"db-password": "super-secret-123",
		"api-key":     "key-abc-xyz",
	})
	resolver := New(provider, "test-project", "")
	ctx := context.Background()

	t.Run("resolves single secret reference", func(t *testing.T) {
		result, err := resolver.ResolveValue(ctx, "${secret:db-password}")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}
		if result != "super-secret-123" {
			t.Errorf("ResolveValue() = %q, want super-secret-123", result)
		}
	})

	t.Run("resolves embedded secret reference", func(t *testing.T) {
		result, err := resolver.ResolveValue(ctx, "password=${secret:db-password}")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}
		if result != "password=super-secret-123" {
			t.Errorf("ResolveValue() = %q, want password=super-secret-123", result)
		}
	})

	t.Run("resolves multiple secret references", func(t *testing.T) {
		result, err := resolver.ResolveValue(ctx, "${secret:db-password}:${secret:api-key}")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}
		if result != "super-secret-123:key-abc-xyz" {
			t.Errorf("ResolveValue() = %q, want super-secret-123:key-abc-xyz", result)
		}
	})

	t.Run("returns plain text unchanged", func(t *testing.T) {
		result, err := resolver.ResolveValue(ctx, "plain text value")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}
		if result != "plain text value" {
			t.Errorf("ResolveValue() = %q, want plain text value", result)
		}
	})

	t.Run("returns error for missing secret", func(t *testing.T) {
		_, err := resolver.ResolveValue(ctx, "${secret:nonexistent}")
		if err == nil {
			t.Fatal("expected error for missing secret")
		}
	})
}

func TestResolver_ResolveAll(t *testing.T) {
	provider := NewMockProvider(map[string]string{
		"db-password": "secret-pass",
		"api-key":     "api-123",
	})
	resolver := New(provider, "test-project", "")
	ctx := context.Background()

	t.Run("resolves all secrets in config", func(t *testing.T) {
		config := map[string]string{
			"database.host":     "localhost",
			"database.password": "${secret:db-password}",
			"api.key":           "${secret:api-key}",
		}

		result, err := resolver.ResolveAll(ctx, config)
		if err != nil {
			t.Fatalf("ResolveAll failed: %v", err)
		}

		if result["database.host"] != "localhost" {
			t.Errorf("database.host = %q, want localhost", result["database.host"])
		}
		if result["database.password"] != "secret-pass" {
			t.Errorf("database.password = %q, want secret-pass", result["database.password"])
		}
		if result["api.key"] != "api-123" {
			t.Errorf("api.key = %q, want api-123", result["api.key"])
		}
	})

	t.Run("returns error if any secret fails", func(t *testing.T) {
		config := map[string]string{
			"good":    "${secret:db-password}",
			"missing": "${secret:nonexistent}",
		}

		_, err := resolver.ResolveAll(ctx, config)
		if err == nil {
			t.Fatal("expected error for missing secret")
		}
	})
}

func TestResolver_TracksSecretCalls(t *testing.T) {
	provider := NewMockProvider(map[string]string{
		"secret1": "value1",
		"secret2": "value2",
	})
	resolver := New(provider, "my-gcp-project", "")
	ctx := context.Background()

	config := map[string]string{
		"key1": "${secret:secret1}",
		"key2": "${secret:secret2@3}",
	}

	_, err := resolver.ResolveAll(ctx, config)
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	// Verify the provider was called correctly
	if len(provider.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(provider.calls))
	}

	// Check that version was parsed correctly
	callsByName := make(map[string]SecretRef)
	for _, call := range provider.calls {
		callsByName[call.Name] = call
	}

	if callsByName["secret1"].Version != "latest" {
		t.Errorf("secret1 version = %q, want latest", callsByName["secret1"].Version)
	}
	if callsByName["secret2"].Version != "3" {
		t.Errorf("secret2 version = %q, want 3", callsByName["secret2"].Version)
	}
}

func TestResolver_SecretPrefix(t *testing.T) {
	provider := NewMockProvider(map[string]string{
		"service-a-db-password": "prefixed-secret-value",
		"db-password":           "unprefixed-value",
	})
	ctx := context.Background()

	t.Run("applies prefix to secret name", func(t *testing.T) {
		resolver := New(provider, "test-project", "service-a-")
		result, err := resolver.ResolveValue(ctx, "${secret:db-password}")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}
		if result != "prefixed-secret-value" {
			t.Errorf("ResolveValue() = %q, want prefixed-secret-value", result)
		}

		// Verify the provider was called with prefixed name
		if len(provider.calls) == 0 {
			t.Fatal("expected provider to be called")
		}
		if provider.calls[0].Name != "service-a-db-password" {
			t.Errorf("provider called with name %q, want service-a-db-password", provider.calls[0].Name)
		}
	})

	t.Run("empty prefix does not modify name", func(t *testing.T) {
		provider := NewMockProvider(map[string]string{
			"db-password": "unprefixed-value",
		})
		resolver := New(provider, "test-project", "")
		result, err := resolver.ResolveValue(ctx, "${secret:db-password}")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}
		if result != "unprefixed-value" {
			t.Errorf("ResolveValue() = %q, want unprefixed-value", result)
		}
	})

	t.Run("does not apply prefix to full path references", func(t *testing.T) {
		provider := NewMockProvider(map[string]string{
			"cross-project-secret": "cross-project-value",
		})
		resolver := New(provider, "test-project", "service-a-")
		_, err := resolver.ResolveValue(ctx, "${secret:projects/other/secrets/cross-project-secret}")
		if err != nil {
			t.Fatalf("ResolveValue failed: %v", err)
		}

		// Verify the provider was called without prefix (full path)
		if len(provider.calls) == 0 {
			t.Fatal("expected provider to be called")
		}
		if provider.calls[0].Name != "cross-project-secret" {
			t.Errorf("provider called with name %q, want cross-project-secret (no prefix)", provider.calls[0].Name)
		}
		if !provider.calls[0].IsFullPath {
			t.Error("expected IsFullPath to be true for full path reference")
		}
	})
}
