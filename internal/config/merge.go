package config

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
		return intToString(val)
	case int64:
		return int64ToString(val)
	case float64:
		return float64ToString(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return ""
	}
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func int64ToString(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func float64ToString(f float64) string {
	// Simple implementation - handles common cases
	// For production, consider using strconv.FormatFloat
	if f == float64(int64(f)) {
		return int64ToString(int64(f))
	}
	// For non-integers, use basic formatting
	// This is a simplified version - full implementation would use strconv
	return floatFormat(f)
}

func floatFormat(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}

	intPart := int64(f)
	fracPart := f - float64(intPart)

	result := int64ToString(intPart)

	if fracPart > 0 {
		result += "."
		// Up to 6 decimal places
		for i := 0; i < 6 && fracPart > 0.0000001; i++ {
			fracPart *= 10
			digit := int(fracPart)
			result += string(byte('0' + digit))
			fracPart -= float64(digit)
		}
	}

	if neg {
		return "-" + result
	}
	return result
}
