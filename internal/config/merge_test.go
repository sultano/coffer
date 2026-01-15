package config

import (
	"reflect"
	"testing"
)

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]any
		src      map[string]any
		expected map[string]any
	}{
		{
			name:     "empty maps",
			dst:      map[string]any{},
			src:      map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "src overrides dst",
			dst: map[string]any{
				"key": "old",
			},
			src: map[string]any{
				"key": "new",
			},
			expected: map[string]any{
				"key": "new",
			},
		},
		{
			name: "nested merge",
			dst: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": 5432,
				},
			},
			src: map[string]any{
				"database": map[string]any{
					"host": "prod-db",
				},
			},
			expected: map[string]any{
				"database": map[string]any{
					"host": "prod-db",
					"port": 5432,
				},
			},
		},
		{
			name: "add new keys",
			dst: map[string]any{
				"existing": "value",
			},
			src: map[string]any{
				"new": "value",
			},
			expected: map[string]any{
				"existing": "value",
				"new":      "value",
			},
		},
		{
			name: "deeply nested",
			dst: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "original",
					},
				},
			},
			src: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "overridden",
					},
				},
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "overridden",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepMerge(tt.dst, tt.src)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("DeepMerge() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFlatten(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]string
	}{
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]string{},
		},
		{
			name: "flat map",
			input: map[string]any{
				"key": "value",
			},
			expected: map[string]string{
				"key": "value",
			},
		},
		{
			name: "nested map",
			input: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": 5432,
				},
			},
			expected: map[string]string{
				"database.host": "localhost",
				"database.port": "5432",
			},
		},
		{
			name: "deeply nested",
			input: map[string]any{
				"app": map[string]any{
					"config": map[string]any{
						"timeout": 30,
					},
				},
			},
			expected: map[string]string{
				"app.config.timeout": "30",
			},
		},
		{
			name: "boolean values",
			input: map[string]any{
				"enabled": true,
				"debug":   false,
			},
			expected: map[string]string{
				"enabled": "true",
				"debug":   "false",
			},
		},
		{
			name: "nil value",
			input: map[string]any{
				"empty": nil,
			},
			expected: map[string]string{
				"empty": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Flatten(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Flatten() = %v, want %v", result, tt.expected)
			}
		})
	}
}
