package client

// mergeFields overlays fields onto target and deep-merges selected map fields.
// Omada commonly requires full-object updates, so this keeps unknown keys in
// both the root object and nested settings.
func mergeFields(target, fields map[string]any, deepKeys ...string) {
	deep := make(map[string]bool, len(deepKeys))
	for _, key := range deepKeys {
		deep[key] = true
	}
	for key, value := range fields {
		if !deep[key] {
			target[key] = value
			continue
		}
		overlay, ok := value.(map[string]any)
		if !ok {
			target[key] = value
			continue
		}
		base, _ := target[key].(map[string]any)
		if base == nil {
			base = map[string]any{}
		}
		for nestedKey, nestedValue := range overlay {
			base[nestedKey] = nestedValue
		}
		target[key] = base
	}
}
