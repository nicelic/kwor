package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func collectInboundBlockRanges(inbound *model.Inbound) []portRange {
	if inbound == nil {
		return nil
	}

	ranges := make([]portRange, 0, 4)
	listenPort := extractPort(inbound.Options)
	if listenPort > 0 {
		ranges = append(ranges, portRange{start: listenPort, end: listenPort})
	}

	if strings.TrimSpace(strings.ToLower(inbound.Type)) == "mieru" {
		if fromOptions := parseMieruPortRangesFromOptions(inbound.Options); len(fromOptions) > 0 {
			ranges = append(ranges, fromOptions...)
		} else if fromOutJSON := parseMieruPortRangesFromOutJSON(inbound.OutJson); len(fromOutJSON) > 0 {
			ranges = append(ranges, fromOutJSON...)
		}
	} else if hopRange := strings.TrimSpace(extractPortHopRange(inbound.Options)); hopRange != "" {
		ranges = append(ranges, parsePortRangeInput(hopRange)...)
	}

	return normalizeNftPortRanges(ranges)
}

func collectMihomoInboundBlockRanges(inbound *model.MihomoInbound) []portRange {
	if inbound == nil {
		return nil
	}

	ranges := make([]portRange, 0, 4)
	listenPort := extractPort(inbound.Options)
	if listenPort > 0 {
		ranges = append(ranges, portRange{start: listenPort, end: listenPort})
	}

	redirectRange, _ := resolveMihomoInboundRedirectSpec(inbound)
	if strings.TrimSpace(redirectRange) != "" {
		ranges = append(ranges, parsePortRangeInput(redirectRange)...)
	}

	return normalizeNftPortRanges(ranges)
}

func parseMieruPortRangesFromOptions(options json.RawMessage) []portRange {
	if len(options) == 0 {
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(options, &fields); err != nil {
		return nil
	}

	rawRange := firstRawString(fields["port_range"])
	if normalized, ok := normalizeMieruPortRangeForBlock(rawRange); ok {
		return parsePortRangeInput(normalized)
	}

	bindings := firstRawString(fields["port_bindings"])
	return parsePortRangeInput(strings.Join(parseMieruPortBindingsForBlock(bindings), ","))
}

func parseMieruPortRangesFromOutJSON(outJSON json.RawMessage) []portRange {
	if len(outJSON) == 0 {
		return nil
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(outJSON, &payload); err != nil {
		return nil
	}

	// Prefer explicit range/bindings in out_json.
	if normalized, ok := normalizeMieruPortRangeForBlock(firstString(payload["port_range"])); ok {
		return parsePortRangeInput(normalized)
	}
	if bindings := util.NormalizeMieruPortBindingsValue(payload["port_bindings"]); len(bindings) > 0 {
		return parsePortRangeInput(strings.Join(bindings, ","))
	}

	// Fallback: if only server_port exists, treat it as a single extra port.
	if value, ok := payload["server_port"]; ok {
		if port, parsed := toBlockPort(value); parsed {
			return []portRange{{start: port, end: port}}
		}
	}

	return nil
}

func firstRawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return ""
}

func parseMieruPortBindingsForBlock(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\uFF0C", ",")
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		normalized, ok := normalizeMieruPortBindingForBlock(part)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeMieruPortRangeForBlock(raw string) (string, bool) {
	normalized, ok := normalizeMieruPortBindingForBlock(raw)
	if !ok {
		return "", false
	}
	if !strings.Contains(normalized, "-") {
		return "", false
	}
	return normalized, true
}

func normalizeMieruPortBindingForBlock(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	value = strings.ReplaceAll(value, "\uFF1A", ":")
	value = strings.ReplaceAll(value, ":", "-")
	value = strings.ReplaceAll(value, " ", "")

	if strings.Contains(value, "-") {
		parts := strings.Split(value, "-")
		if len(parts) != 2 {
			return "", false
		}
		start, startErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, endErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if startErr != nil || endErr != nil {
			return "", false
		}
		if start < 1 || start > 65535 || end < 1 || end > 65535 {
			return "", false
		}
		if start > end {
			start, end = end, start
		}
		if start == end {
			return strconv.Itoa(start), true
		}
		return strconv.Itoa(start) + "-" + strconv.Itoa(end), true
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return "", false
	}
	return strconv.Itoa(port), true
}

func toBlockPort(value interface{}) (int, bool) {
	switch current := value.(type) {
	case float64:
		port := int(current)
		if float64(port) != current {
			return 0, false
		}
		if port < 1 || port > 65535 {
			return 0, false
		}
		return port, true
	case int:
		if current < 1 || current > 65535 {
			return 0, false
		}
		return current, true
	case int64:
		if current < 1 || current > 65535 {
			return 0, false
		}
		return int(current), true
	case string:
		text := strings.TrimSpace(current)
		if text == "" {
			return 0, false
		}
		port, err := strconv.Atoi(text)
		if err != nil || port < 1 || port > 65535 {
			return 0, false
		}
		return port, true
	default:
		return 0, false
	}
}
