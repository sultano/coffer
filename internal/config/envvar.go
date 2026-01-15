package config

import (
	"strings"
	"unicode"
)

// ToEnvVars converts flattened config to environment variables
// using explicit mappings from env_mapping, falling back to auto-convention
func ToEnvVars(flat map[string]string, mapping map[string]string) map[string]string {
	result := make(map[string]string)

	for key, value := range flat {
		envName := mapToEnvVar(key, mapping)
		result[envName] = value
	}

	return result
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
	for i, r := range name {
		if unicode.IsLetter(r) || r == '_' {
			result.WriteRune(r)
		} else if unicode.IsDigit(r) && i > 0 {
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
		sb.WriteString(quoteIfNeeded(value))
		sb.WriteString("\n")
	}
	return sb.String()
}

// quoteIfNeeded wraps value in quotes if it contains special characters
func quoteIfNeeded(value string) string {
	needsQuotes := false
	for _, r := range value {
		if r == ' ' || r == '\n' || r == '\t' || r == '"' || r == '\'' || r == '$' || r == '`' || r == '\\' {
			needsQuotes = true
			break
		}
	}

	if !needsQuotes {
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
