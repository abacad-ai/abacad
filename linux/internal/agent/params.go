package agent

import "strconv"

// Loose accessors over a decoded JSON params object. JSON numbers arrive as
// float64, so ints are coerced; missing or wrong-typed keys fall back to the
// default — matching the forgiving param handling in the macOS dispatcher.

func paramInt(m map[string]any, key string, def int) int {
	switch n := m[key].(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		if v, err := strconv.Atoi(n); err == nil {
			return v
		}
	}
	return def
}

func paramBool(m map[string]any, key string, def bool) bool {
	switch b := m[key].(type) {
	case bool:
		return b
	case string:
		return b == "true"
	}
	return def
}

func paramStr(m map[string]any, key, def string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return def
}

func paramStrs(m map[string]any, key string) []string {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func paramObjs(m map[string]any, key string) []map[string]any {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if o, ok := v.(map[string]any); ok {
			out = append(out, o)
		}
	}
	return out
}
