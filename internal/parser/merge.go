package parser

import (
	"fmt"
	"maps"

	"go.yaml.in/yaml/v4"
)

// deepMerge recursively merges fragment into base. Maps are merged key by key;
// scalars, arrays, and type mismatches replace the base value.
func deepMerge(base, fragment map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(fragment))
	maps.Copy(out, base)
	for k, fragVal := range fragment {
		baseMap, baseOK := out[k].(map[string]any)
		fragMap, fragOK := fragVal.(map[string]any)
		if baseOK && fragOK {
			out[k] = deepMerge(baseMap, fragMap)
			continue
		}
		out[k] = fragVal
	}
	return out
}

func decodeSpec(data []byte) (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decoding spec: %w", err)
	}
	return doc, nil
}

func encodeSpec(doc map[string]any) ([]byte, error) {
	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encoding merged spec: %w", err)
	}
	return out, nil
}
