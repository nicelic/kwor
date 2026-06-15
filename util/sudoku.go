package util

import (
	"encoding/json"
	"strconv"
	"strings"
)

func UnquoteSudokuString(value string) string {
	normalized := strings.TrimSpace(value)
	for len(normalized) >= 2 {
		if (strings.HasPrefix(normalized, "\"") && strings.HasSuffix(normalized, "\"")) ||
			(strings.HasPrefix(normalized, "'") && strings.HasSuffix(normalized, "'")) {
			normalized = strings.TrimSpace(normalized[1 : len(normalized)-1])
			continue
		}
		break
	}
	return normalized
}

func NormalizeSudokuStringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return UnquoteSudokuString(value)
	case []string:
		for _, item := range value {
			normalized := UnquoteSudokuString(item)
			if normalized != "" {
				return normalized
			}
		}
	case []interface{}:
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			normalized := UnquoteSudokuString(itemStr)
			if normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func NormalizeSudokuKeyValue(raw interface{}) string {
	normalize := func(value string) string {
		value = strings.ReplaceAll(value, "\r\n", "\n")
		value = strings.ReplaceAll(value, "\r", "\n")
		value = UnquoteSudokuString(value)
		if value == "" {
			return ""
		}
		return strings.Join(strings.Fields(value), "")
	}

	switch value := raw.(type) {
	case string:
		return normalize(value)
	case []string:
		return normalize(strings.Join(value, ""))
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			parts = append(parts, itemStr)
		}
		return normalize(strings.Join(parts, ""))
	default:
		return ""
	}
}

func NormalizeSudokuAEADMethod(value string) string {
	switch strings.ToLower(UnquoteSudokuString(value)) {
	case "aes-128-gcm":
		return "aes-128-gcm"
	case "none":
		return "none"
	default:
		return "chacha20-poly1305"
	}
}

func NormalizeSudokuTableType(value string) string {
	switch strings.ToLower(UnquoteSudokuString(value)) {
	case "prefer-entropy", "prefer_entropy":
		return "prefer_entropy"
	default:
		return "prefer_ascii"
	}
}

func SudokuTableTypeSupportsCustom(tableType string) bool {
	return NormalizeSudokuTableType(tableType) == "prefer_entropy"
}

func NormalizeSudokuTableTypeForCustom(tableType string, hasCustom bool) string {
	normalized := NormalizeSudokuTableType(tableType)
	if hasCustom && !SudokuTableTypeSupportsCustom(normalized) {
		return "prefer_entropy"
	}
	return normalized
}

func NormalizeSudokuHTTPMaskMode(value string) string {
	switch strings.ToLower(UnquoteSudokuString(value)) {
	case "stream", "split-stream":
		return "stream"
	case "poll":
		return "poll"
	case "auto":
		return "auto"
	case "ws":
		return "ws"
	default:
		return "legacy"
	}
}

func NormalizeSudokuHTTPMaskMultiplex(value string) string {
	switch strings.ToLower(UnquoteSudokuString(value)) {
	case "auto":
		return "auto"
	case "on":
		return "on"
	default:
		return "off"
	}
}

func normalizeSudokuCustomTablePattern(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "\uFF0C", ",")
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.Join(strings.Fields(normalized), "")
	normalized = UnquoteSudokuString(normalized)
	normalized = strings.TrimSpace(normalized)
	normalized = strings.TrimPrefix(normalized, "[")
	normalized = strings.TrimSuffix(normalized, "]")
	normalized = UnquoteSudokuString(strings.TrimSpace(normalized))

	if len(normalized) != 8 {
		return ""
	}

	countX := 0
	countP := 0
	countV := 0
	for _, ch := range normalized {
		switch ch {
		case 'x':
			countX++
		case 'p':
			countP++
		case 'v':
			countV++
		default:
			return ""
		}
	}

	if countX != 2 || countP != 2 || countV != 4 {
		return ""
	}
	return normalized
}

func parseSudokuCustomTablesJSONString(raw string) ([]string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return nil, false
	}

	stringList := []string{}
	if err := json.Unmarshal([]byte(trimmed), &stringList); err == nil {
		return stringList, true
	}

	anyList := []interface{}{}
	if err := json.Unmarshal([]byte(trimmed), &anyList); err != nil {
		return nil, false
	}
	converted := make([]string, 0, len(anyList))
	for _, item := range anyList {
		itemStr, ok := item.(string)
		if !ok {
			continue
		}
		converted = append(converted, itemStr)
	}
	return converted, true
}

func NormalizeSudokuCustomTable(raw interface{}) string {
	customTables := NormalizeSudokuCustomTables(raw)
	if len(customTables) == 0 {
		return ""
	}
	return customTables[0]
}

func NormalizeSudokuCustomTables(raw interface{}) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(value string) {
		normalized := normalizeSudokuCustomTablePattern(value)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	switch value := raw.(type) {
	case string:
		if parsed, ok := parseSudokuCustomTablesJSONString(value); ok {
			for _, item := range parsed {
				add(item)
			}
			break
		}
		normalized := strings.ReplaceAll(value, "\uFF0C", ",")
		normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
		for _, item := range strings.FieldsFunc(normalized, func(r rune) bool {
			return r == '\n' || r == ','
		}) {
			add(item)
		}
	case []string:
		for _, item := range value {
			add(item)
		}
	case []interface{}:
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			add(itemStr)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func sudokuToBool(raw interface{}) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case int:
		return value != 0, true
	case int32:
		return value != 0, true
	case int64:
		return value != 0, true
	case float32:
		return value != 0, true
	case float64:
		return value != 0, true
	case string:
		switch strings.ToLower(UnquoteSudokuString(value)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	}
	return false, false
}

func sudokuToInt(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		normalized := UnquoteSudokuString(value)
		if normalized == "" {
			return 0, false
		}
		number, err := strconv.Atoi(normalized)
		if err == nil {
			return number, true
		}
	}
	return 0, false
}
