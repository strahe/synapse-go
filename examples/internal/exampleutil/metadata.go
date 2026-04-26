package exampleutil

import (
	"fmt"
	"sort"
	"strings"
)

// MetadataFlag parses repeated key=value flags into a map.
type MetadataFlag map[string]string

func (m *MetadataFlag) Set(raw string) error {
	if *m == nil {
		*m = make(map[string]string)
	}
	key, value, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return fmt.Errorf("metadata must be key=value, got %q", raw)
	}
	(*m)[key] = value
	return nil
}

func (m *MetadataFlag) String() string {
	if m == nil || *m == nil {
		return ""
	}
	keys := make([]string, 0, len(*m))
	for key := range *m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+(*m)[key])
	}
	return strings.Join(parts, ",")
}

// Map returns a copy suitable for SDK option structs. Empty maps become nil.
func (m MetadataFlag) Map() map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}
