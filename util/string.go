package util

func ToStringSlice(v any) []string {
	if v == nil {
		return nil
	}

	// v 为 []any → 逐个取出其中的string项，组成[]string返回
	if s, ok := v.([]any); ok {
		out := make([]string, 0, len(s))
		for _, it := range s {
			if str, ok := it.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}

	// v 为 []string → 直接返回
	if s, ok := v.([]string); ok {
		return s
	}

	// v 为string且非空 → 转为[]string返回
	if s, ok := v.(string); ok && s != "" {
		return []string{s}
	}

	return nil
}

func StrOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
