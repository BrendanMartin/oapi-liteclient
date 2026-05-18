package generator

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

var reservedFilenames = map[string]bool{
	"client":   true,
	"_base":    true,
	"models":   true,
	"__init__": true,
	"errors":   true,
}

// groupEndpointsByTag groups endpoints by their first tag.
// Returns (tagMap, hasTags). If no endpoint has tags, hasTags is false.
func groupEndpointsByTag(endpoints []ir.Endpoint) (map[string][]ir.Endpoint, bool) {
	groups := make(map[string][]ir.Endpoint)
	hasTags := false
	for _, ep := range endpoints {
		if len(ep.Tags) > 0 {
			hasTags = true
			tag := ep.Tags[0]
			groups[tag] = append(groups[tag], ep)
		} else {
			groups[""] = append(groups[""], ep)
		}
	}
	return groups, hasTags
}

// tagToFilename converts a tag name to a safe filename stem.
func tagToFilename(tag string) string {
	if tag == "" {
		return "general"
	}
	var b strings.Builder
	prevUnderscore := false
	for i, r := range tag {
		if unicode.IsUpper(r) && i > 0 {
			prev := rune(tag[i-1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				if !prevUnderscore {
					b.WriteRune('_')
				}
			}
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			if !prevUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				prevUnderscore = true
			}
			continue
		}
		if r == '_' {
			if !prevUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				prevUnderscore = true
			}
		} else if r >= 'A' && r <= 'Z' {
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		} else {
			b.WriteRune(r)
			prevUnderscore = false
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// tagToClassName converts a tag name to a Python class name suffix.
func tagToClassName(tag string) string {
	if tag == "" {
		return "General"
	}
	var b strings.Builder
	upper := true
	for _, r := range tag {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// mergeTagsByPrefix merges tags that share a common prefix into a single group.
// Tags are sorted alphabetically first, so shorter prefixes establish groups
// before longer variants are encountered. A tag matches a prefix if it equals
// the prefix or starts with the prefix followed by a space.
func mergeTagsByPrefix(groups map[string][]ir.Endpoint) map[string][]ir.Endpoint {
	tags := sortedTags(groups)
	merged := make(map[string][]ir.Endpoint)
	var prefixes []string

	for _, tag := range tags {
		matched := false
		for _, prefix := range prefixes {
			if tag == prefix || strings.HasPrefix(tag, prefix+" ") {
				merged[prefix] = append(merged[prefix], groups[tag]...)
				matched = true
				break
			}
		}
		if !matched {
			prefixes = append(prefixes, tag)
			merged[tag] = append(merged[tag], groups[tag]...)
		}
	}

	return merged
}

// sortedTags returns tag names in sorted order for deterministic output.
func sortedTags(groups map[string][]ir.Endpoint) []string {
	tags := make([]string, 0, len(groups))
	for tag := range groups {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// validateTagFilenames checks for collisions after sanitization and conflicts with reserved names.
func validateTagFilenames(groups map[string][]ir.Endpoint) error {
	seen := make(map[string]string) // filename -> original tag
	for tag := range groups {
		fn := tagToFilename(tag)
		if reservedFilenames[fn] {
			return fmt.Errorf("tag %q produces reserved filename %q", tag, fn)
		}
		if prev, ok := seen[fn]; ok {
			return fmt.Errorf("tags %q and %q both produce filename %q", prev, tag, fn)
		}
		seen[fn] = tag
	}
	return nil
}
