package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// BEHAVIOR: every emitted name must be a valid POSIX identifier. env_mapping
// targets are user input, and a name like "A=B" or "X;rm" breaks dotenv parsing
// and the shell, so it is rejected rather than passed through.
var validEnvVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ToEnvVars converts flattened config to environment variables
// using explicit mappings from env_mapping, falling back to auto-convention.
// Returns an error if a resulting name is not a valid environment variable name,
// or if two distinct config keys map to the same env var name.
func ToEnvVars(flat map[string]string, mapping map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(flat))
	sourceKey := make(map[string]string, len(flat)) // envName -> config key that produced it

	// BEHAVIOR: iterate keys in sorted order so a collision error is deterministic,
	// not dependent on Go's randomized map iteration order
	keys := make([]string, 0, len(flat))
	for key := range flat {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		envName := mapToEnvVar(key, mapping)
		if !validEnvVarName.MatchString(envName) {
			return nil, fmt.Errorf("config key %q maps to invalid environment variable name %q: names must match %s; set an ASCII name for it under env_mapping in %s", key, envName, validEnvVarName, ProjectFileName)
		}
		if existing, ok := sourceKey[envName]; ok {
			a, b := existing, key
			if a > b {
				a, b = b, a
			}
			return nil, fmt.Errorf("config keys %q and %q both map to environment variable %s", a, b, envName)
		}
		sourceKey[envName] = key
		result[envName] = flat[key]
	}

	return result, nil
}

// mapToEnvVar converts a dot-notation key to an environment variable name
// Uses explicit mapping if available, otherwise auto-converts
func mapToEnvVar(key string, mapping map[string]string) string {
	// Check explicit mapping first
	if envName, ok := mapping[key]; ok {
		return envName
	}

	// Auto-convert: database.host -> DATABASE_HOST
	return autoConvertToEnvVar(key)
}

// autoConvertToEnvVar converts a dot-notation key to an environment variable
// database.host -> DATABASE_HOST
// api.key_name -> API_KEY_NAME
func autoConvertToEnvVar(key string) string {
	// Replace dots with underscores
	result := strings.ReplaceAll(key, ".", "_")

	// Convert to uppercase
	result = strings.ToUpper(result)

	// Clean up any invalid characters
	result = cleanEnvVarName(result)

	return result
}

// cleanEnvVarName ensures the name is a valid environment variable name
func cleanEnvVarName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || r == '_' {
			result.WriteRune(r)
		} else if unicode.IsDigit(r) {
			// BEHAVIOR: a leading digit is prefixed with an underscore rather than
			// dropped, so "3rd.party" becomes "_3RD_PARTY" instead of losing the digit
			if result.Len() == 0 {
				result.WriteRune('_')
			}
			result.WriteRune(r)
		} else if r == '-' {
			result.WriteRune('_')
		}
		// Skip other invalid characters
	}
	return result.String()
}

// FormatAsDotenv formats environment variables as a .env file
func FormatAsDotenv(envVars map[string]string) string {
	var sb strings.Builder
	for key, value := range envVars {
		sb.WriteString(key)
		sb.WriteString("=")
		sb.WriteString(QuoteForDotenv(value))
		sb.WriteString("\n")
	}
	return sb.String()
}

// QuoteForDotenv wraps value in double quotes unless every character is in a
// small allowlist that is inert to both dotenv parsers and POSIX shells.
func QuoteForDotenv(value string) string {
	// BEHAVIOR: quote on anything outside the allowlist, never on a denylist.
	// Output may be eval'd by a shell, so an unquoted ; & | < > ( ) etc. would
	// run commands; a denylist silently misses characters like these.
	if strings.IndexFunc(value, func(r rune) bool { return !isDotenvSafe(r) }) == -1 {
		return value
	}

	// Use double quotes and escape special characters
	var sb strings.Builder
	sb.WriteRune('"')
	for _, r := range value {
		switch r {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\t':
			sb.WriteString("\\t")
		case '$':
			sb.WriteString("\\$")
		case '`':
			sb.WriteString("\\`")
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteRune('"')
	return sb.String()
}

func isDotenvSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		strings.ContainsRune("_./:@%+,=-", r)
}
