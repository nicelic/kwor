package util

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// SudokuCustomTablesFlowYAML keeps Sudoku custom-tables rendered in YAML flow style:
// ["ppvxvxvv","vxvpvpvx"].
type SudokuCustomTablesFlowYAML []string

func (s SudokuCustomTablesFlowYAML) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle,
	}
	for _, item := range s {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Style: yaml.DoubleQuotedStyle,
			Value: item,
		})
	}
	return node, nil
}

// ApplySudokuCustomTablesFlowYAML recursively walks a map/array structure and converts
// "custom-tables"/"custom_tables" slices into SudokuCustomTablesFlowYAML so YAML output
// keeps the inline array style required by documentation examples.
func ApplySudokuCustomTablesFlowYAML(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, item := range v {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if lowerKey == "custom-tables" || lowerKey == "custom_tables" {
				if normalized := NormalizeSudokuCustomTables(item); len(normalized) > 0 {
					v[key] = SudokuCustomTablesFlowYAML(normalized)
				}
				continue
			}
			v[key] = ApplySudokuCustomTablesFlowYAML(item)
		}
		return v
	case []interface{}:
		for index := range v {
			v[index] = ApplySudokuCustomTablesFlowYAML(v[index])
		}
		return v
	case []map[string]interface{}:
		for index := range v {
			v[index] = ApplySudokuCustomTablesFlowYAML(v[index]).(map[string]interface{})
		}
		return v
	default:
		return value
	}
}

// CompactSudokuCustomTablesFlowYAML rewrites
//
//	custom-tables: ["a", "b"]
//
// to
//
//	custom-tables: ["a","b"]
//
// to keep strict parser compatibility while preserving YAML sequence semantics.
func CompactSudokuCustomTablesFlowYAML(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}

	lines := strings.Split(string(raw), "\n")
	for index, line := range lines {
		keyIndex := strings.Index(line, "custom-tables:")
		if keyIndex < 0 {
			continue
		}

		leftBracket := strings.Index(line[keyIndex:], "[")
		rightBracket := strings.LastIndex(line, "]")
		if leftBracket < 0 || rightBracket < 0 {
			continue
		}
		leftBracket += keyIndex
		if rightBracket <= leftBracket {
			continue
		}

		inside := line[leftBracket+1 : rightBracket]
		inside = strings.ReplaceAll(inside, `", "`, `","`)
		lines[index] = line[:leftBracket+1] + inside + line[rightBracket:]
	}

	return []byte(strings.Join(lines, "\n"))
}
