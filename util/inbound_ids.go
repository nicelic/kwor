package util

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// ParseInboundIDs accepts the historical JSON representations used for a
// client inbound selection and returns positive, de-duplicated IDs in order.
func ParseInboundIDs(raw json.RawMessage) ([]uint, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []uint{}, nil
	}

	var direct []uint
	if err := json.Unmarshal(raw, &direct); err == nil {
		return deduplicatePositiveInboundIDs(direct), nil
	}

	var mixed []interface{}
	if err := json.Unmarshal(raw, &mixed); err != nil {
		return nil, err
	}

	maxUint := uint64(^uint(0))
	parsed := make([]uint, 0, len(mixed))
	for _, item := range mixed {
		switch value := item.(type) {
		case float64:
			if value <= 0 || math.Trunc(value) != value || value > float64(maxUint) {
				continue
			}
			parsed = append(parsed, uint(value))
		case int:
			if value > 0 {
				parsed = append(parsed, uint(value))
			}
		case int64:
			if value > 0 && uint64(value) <= maxUint {
				parsed = append(parsed, uint(value))
			}
		case string:
			numeric, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err == nil && numeric > 0 && numeric <= maxUint {
				parsed = append(parsed, uint(numeric))
			}
		case json.Number:
			numeric, err := strconv.ParseUint(string(value), 10, 64)
			if err == nil && numeric > 0 && numeric <= maxUint {
				parsed = append(parsed, uint(numeric))
			}
		}
	}

	return deduplicatePositiveInboundIDs(parsed), nil
}

func deduplicatePositiveInboundIDs(values []uint) []uint {
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
