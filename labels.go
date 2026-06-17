package main

import (
	"fmt"
	"sort"
	"strings"
)

// parseLabels turns repeatable "key=value" flag values into a map. Keys must
// be non-empty and contain no "=" (the first "=" separates key from value, so
// values may contain "="). Duplicate keys take the last value. Returns nil for
// an empty input so the map is omitted from the wire message.
func parseLabels(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		key = strings.TrimSpace(key)
		if !ok {
			return nil, fmt.Errorf("label %q must be in key=value form", p)
		}
		if key == "" {
			return nil, fmt.Errorf("label %q has an empty key", p)
		}
		out[key] = value
	}
	return out, nil
}

// formatLabels renders a label map as a stable, sorted "k=v,k=v" string for
// display. Returns "" for an empty map.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}
