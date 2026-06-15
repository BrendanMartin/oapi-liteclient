package parser

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// deepMerge recursively merges the fragment spec into base and returns base.
// It operates on yaml.Node trees rather than map[string]any so the base
// document's key order is preserved — re-marshaling a Go map iterates keys in
// random order, which made generated output differ between regenerations.
// Mapping nodes merge key by key (base order kept, new fragment keys appended);
// scalars, sequences, and kind mismatches replace the base value.
func deepMerge(base, fragment *yaml.Node) *yaml.Node {
	mergeNode(mappingOf(base), mappingOf(fragment))
	return base
}

// mergeNode merges fragment into base. When both are mapping nodes it merges in
// place (preserving base order, appending unseen keys) and returns base;
// otherwise fragment replaces base.
func mergeNode(base, fragment *yaml.Node) *yaml.Node {
	if base == nil || fragment == nil || base.Kind != yaml.MappingNode || fragment.Kind != yaml.MappingNode {
		return fragment
	}
	for i := 0; i+1 < len(fragment.Content); i += 2 {
		key, val := fragment.Content[i], fragment.Content[i+1]
		if j := mapIndex(base, key.Value); j >= 0 {
			base.Content[j+1] = mergeNode(base.Content[j+1], val)
		} else {
			base.Content = append(base.Content, key, val)
		}
	}
	return base
}

// mappingOf returns the mapping node n represents, unwrapping a document node.
func mappingOf(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

// mapIndex returns the index of key within a mapping node's flat Content slice
// (the key position; its value is at index+1), or -1 if absent.
func mapIndex(node *yaml.Node, key string) int {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func decodeSpec(data []byte) (*yaml.Node, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("decoding spec: %w", err)
	}
	return &node, nil
}

func encodeSpec(node *yaml.Node) ([]byte, error) {
	// A spec parsed from JSON carries flow-style flags on every node, so
	// re-marshaling would reproduce JSON-ish flow output (which libopenapi can
	// then mis-detect and fail to parse). Clear the styles to emit clean block
	// YAML, matching the format libopenapi expects.
	clearFlowStyle(node)
	out, err := yaml.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("encoding merged spec: %w", err)
	}
	return out, nil
}

// clearFlowStyle recursively resets the flow-style bit so the tree marshals as
// block YAML rather than inheriting JSON flow style from the parsed input.
func clearFlowStyle(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style &^= yaml.FlowStyle
	for _, child := range node.Content {
		clearFlowStyle(child)
	}
}
