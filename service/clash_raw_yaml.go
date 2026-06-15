package service

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func extractClashProxyRawYAMLByName(yamlData []byte) (map[string][]byte, error) {
	keyNode, sequenceNode, nextKeyLine, err := findTopLevelYAMLSequence(yamlData, "proxies")
	if err != nil {
		return nil, err
	}
	if sequenceNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("clash proxies field is not a sequence")
	}

	lineStarts := buildYAMLLineStarts(yamlData)
	totalLines := len(lineStarts) - 1
	blockEndLine := totalLines
	if nextKeyLine > 0 && nextKeyLine-1 < blockEndLine {
		blockEndLine = nextKeyLine - 1
	}

	parentIndent := keyNode.Column - 1
	itemStarts := findYAMLSequenceItemStarts(yamlData, lineStarts, maxInt(sequenceNode.Line, keyNode.Line+1), blockEndLine, parentIndent)
	if len(itemStarts) == 0 {
		return map[string][]byte{}, nil
	}

	rawByName := make(map[string][]byte, len(itemStarts))
	for index, itemStart := range itemStarts {
		endLine := blockEndLine
		if index+1 < len(itemStarts) {
			endLine = itemStarts[index+1] - 1
		}

		startOffset := lineStarts[itemStart-1]
		endOffset := lineStarts[endLine]
		if startOffset < 0 || endOffset < startOffset || endOffset > len(yamlData) {
			continue
		}

		chunk := append([]byte(nil), yamlData[startOffset:endOffset]...)
		name := extractClashProxyNameFromRawYAMLChunk(chunk)
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, exists := rawByName[name]; exists {
			continue
		}
		rawByName[name] = chunk
	}

	return rawByName, nil
}

func findTopLevelYAMLSequence(raw []byte, key string) (*yaml.Node, *yaml.Node, int, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to parse clash yaml: %v", err)
	}
	if len(doc.Content) == 0 || doc.Content[0] == nil {
		return nil, nil, 0, fmt.Errorf("invalid clash yaml document")
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, 0, fmt.Errorf("clash yaml root is not a mapping")
	}

	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		if keyNode == nil || valueNode == nil {
			continue
		}
		if strings.TrimSpace(keyNode.Value) != key {
			continue
		}

		nextKeyLine := 0
		for next := index + 2; next+1 < len(root.Content); next += 2 {
			nextKey := root.Content[next]
			if nextKey != nil && nextKey.Line > 0 {
				nextKeyLine = nextKey.Line
				break
			}
		}
		return keyNode, valueNode, nextKeyLine, nil
	}

	return nil, nil, 0, fmt.Errorf("clash yaml has no %s field", key)
}

func buildYAMLLineStarts(raw []byte) []int {
	starts := []int{0}
	for index, value := range raw {
		if value == '\n' {
			starts = append(starts, index+1)
		}
	}
	if starts[len(starts)-1] != len(raw) {
		starts = append(starts, len(raw))
	}
	return starts
}

func findYAMLSequenceItemStarts(raw []byte, lineStarts []int, startLine int, endLine int, parentIndent int) []int {
	if startLine <= 0 || endLine < startLine {
		return nil
	}

	itemIndent := -1
	for line := startLine; line <= endLine; line++ {
		indent, marker := yamlLineIndentAndMarker(raw, lineStarts, line)
		if marker != '-' || indent <= parentIndent {
			continue
		}
		if itemIndent < 0 || indent < itemIndent {
			itemIndent = indent
		}
	}
	if itemIndent < 0 {
		return nil
	}

	starts := make([]int, 0)
	for line := startLine; line <= endLine; line++ {
		indent, marker := yamlLineIndentAndMarker(raw, lineStarts, line)
		if marker == '-' && indent == itemIndent {
			starts = append(starts, line)
		}
	}
	return starts
}

func yamlLineIndentAndMarker(raw []byte, lineStarts []int, line int) (int, byte) {
	if line <= 0 || line >= len(lineStarts) {
		return 0, 0
	}

	start := lineStarts[line-1]
	end := lineStarts[line]
	if start < 0 || end < start || end > len(raw) {
		return 0, 0
	}

	segment := raw[start:end]
	indent := 0
	for _, value := range segment {
		if value == ' ' || value == '\t' {
			indent++
			continue
		}
		if value == '\r' || value == '\n' {
			return 0, 0
		}
		return indent, value
	}
	return 0, 0
}

func extractClashProxyNameFromRawYAMLChunk(chunk []byte) string {
	if len(chunk) == 0 {
		return ""
	}

	wrapped := append([]byte("proxies:\n"), chunk...)
	var doc map[string]interface{}
	if err := yaml.Unmarshal(wrapped, &doc); err != nil {
		return ""
	}

	rawProxies, ok := doc["proxies"].([]interface{})
	if !ok || len(rawProxies) == 0 {
		return ""
	}
	firstProxy, ok := rawProxies[0].(map[string]interface{})
	if !ok || firstProxy == nil {
		return ""
	}

	name, _ := firstProxy["name"].(string)
	return strings.TrimSpace(name)
}

func cloneRawYAMLBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return append([]byte(nil), raw...)
}

func ensureTrailingLineFeed(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	if bytes.HasSuffix(raw, []byte("\n")) {
		return cloneRawYAMLBytes(raw)
	}
	withLF := cloneRawYAMLBytes(raw)
	return append(withLF, '\n')
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
