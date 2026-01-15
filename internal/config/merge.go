package config

import "strconv"

// DeepMerge merges src into dst recursively.
// Values in src override values in dst.
// Maps are merged recursively; other types are replaced.
func DeepMerge(dst, src map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy all keys from dst
	for k, v := range dst {
		result[k] = v
	}

	// Merge/override with src
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = srcVal
			continue
		}

		// If both are maps, merge recursively
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			result[k] = DeepMerge(dstMap, srcMap)
		} else {
			// Otherwise, src overrides dst
			result[k] = srcVal
		}
	}

	return result
}

// Flatten converts nested config to flat key-value pairs using dot notation
// e.g., {"database": {"host": "localhost"}} -> {"database.host": "localhost"}
func Flatten(m map[string]any) map[string]string {
	result := make(map[string]string)
	flattenRecursive(m, "", result)
	return result
}

func flattenRecursive(m map[string]any, prefix string, result map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]any:
			flattenRecursive(val, key, result)
		case string:
			result[key] = val
		case int, int64, float64, bool:
			result[key] = toString(val)
		case nil:
			result[key] = ""
		default:
			// For slices and other complex types, convert to string representation
			result[key] = toString(val)
		}
	}
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case nil:
		return ""
	default:
		return ""
	}
}
