package refrewrite

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// rewriteYAMLBytes parses src as YAML and rewrites $ref scalar values where
// applicable. The original document structure, comments, and key order are
// preserved by walking yaml.Node trees instead of decoding into maps.
func rewriteYAMLBytes(src []byte, dir string, allowed map[string]struct{}, fileName string, report *Report) ([]byte, bool, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, false, fmt.Errorf("parse yaml: %w", err)
	}

	changed := false
	walkYAML(&root, dir, allowed, fileName, report, &changed)

	if !changed {
		return src, false, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		_ = enc.Close()
		return nil, false, fmt.Errorf("marshal yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, false, fmt.Errorf("close yaml encoder: %w", err)
	}

	return buf.Bytes(), true, nil
}

// walkYAML descends into a yaml.Node tree. Whenever it finds a mapping whose
// key is exactly "$ref" and whose value is a scalar string, it applies
// evaluateRef and updates the scalar in place.
func walkYAML(node *yaml.Node, dir string, allowed map[string]struct{}, fileName string, report *Report, changed *bool) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			walkYAML(child, dir, allowed, fileName, report, changed)
		}
	case yaml.MappingNode:
		// Content is laid out as [key0, value0, key1, value1, ...].
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]

			if key != nil && key.Kind == yaml.ScalarNode && key.Value == "$ref" &&
				value != nil && value.Kind == yaml.ScalarNode {
				outcome := evaluateRef(value.Value, dir, allowed)
				switch {
				case outcome.Rewrote:
					value.Value = outcome.NewValue
					// Reset Style so the encoder picks an appropriate quoting.
					value.Tag = "!!str"
					value.Style = 0
					report.RefsRewritten++
					*changed = true
				case outcome.SkipReason != "":
					report.RefsSkipped++
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("%s: $ref %q skipped: %s", fileName, value.Value, outcome.SkipReason))
				}
				continue
			}

			walkYAML(value, dir, allowed, fileName, report, changed)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			walkYAML(child, dir, allowed, fileName, report, changed)
		}
	}
}
