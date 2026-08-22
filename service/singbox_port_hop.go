package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

const (
	singboxPortHopMaxRangeTextBytes = 512
	singboxPortHopMaxSegments       = 32
	singboxPortHopMaxPorts          = 4096
	singboxPortHopMinInterval       = 10 * time.Second
)

// sanitizeSingboxHysteriaPortHop keeps the panel-managed Hysteria/Hysteria2
// port-hop fields aligned with the bounded occupancy monitor and nft renderer.
func sanitizeSingboxHysteriaPortHop(inbound *model.Inbound) error {
	if inbound == nil {
		return nil
	}
	inboundType := strings.ToLower(strings.TrimSpace(inbound.Type))
	if inboundType != "hysteria" && inboundType != "hysteria2" {
		return nil
	}

	options := map[string]interface{}{}
	if len(inbound.Options) > 0 {
		if err := json.Unmarshal(inbound.Options, &options); err != nil {
			return err
		}
	}

	normalizedRange, err := normalizeSingboxPortHopRange(portHopOptionString(options["port_hop_range"]))
	if err != nil {
		return err
	}
	if normalizedRange == "" {
		delete(options, "port_hop_range")
		delete(options, "port_hop_interval")
		delete(options, "port_hop_interval_max")
	} else {
		options["port_hop_range"] = normalizedRange
		if err := normalizeSingboxPortHopIntervals(options); err != nil {
			return err
		}
	}

	encoded, err := json.Marshal(options)
	if err != nil {
		return err
	}
	inbound.Options = encoded
	return nil
}

func normalizeSingboxPortHopRange(raw string) (string, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", nil
	}
	if len([]byte(input)) > singboxPortHopMaxRangeTextBytes {
		return "", fmt.Errorf("sing-box hysteria port hop range is too long (max %d bytes)", singboxPortHopMaxRangeTextBytes)
	}
	ranges, normalized, err := parseStrictPortRanges(input)
	if err != nil {
		return "", fmt.Errorf("invalid sing-box hysteria port hop range: %w", err)
	}
	if len(ranges) > singboxPortHopMaxSegments {
		return "", fmt.Errorf("sing-box hysteria port hop range has too many segments (max %d)", singboxPortHopMaxSegments)
	}
	if total := countPorts(ranges); total > singboxPortHopMaxPorts {
		return "", fmt.Errorf("sing-box hysteria port hop range contains too many ports (max %d)", singboxPortHopMaxPorts)
	}
	return strings.ReplaceAll(normalized, ":", "-"), nil
}

func normalizeSingboxPortHopIntervals(options map[string]interface{}) error {
	lower, lowerSet, err := parseSingboxPortHopDuration(portHopOptionString(options["port_hop_interval"]))
	if err != nil {
		return err
	}
	upper, upperSet, err := parseSingboxPortHopDuration(portHopOptionString(options["port_hop_interval_max"]))
	if err != nil {
		return err
	}
	if !lowerSet && !upperSet {
		delete(options, "port_hop_interval")
		delete(options, "port_hop_interval_max")
		return nil
	}
	if lowerSet && lower < singboxPortHopMinInterval {
		return fmt.Errorf("sing-box hysteria port hop interval must be at least %s", singboxPortHopMinInterval)
	}
	if upperSet && upper < singboxPortHopMinInterval {
		return fmt.Errorf("sing-box hysteria port hop interval must be at least %s", singboxPortHopMinInterval)
	}
	if !lowerSet {
		lower = upper
	}
	if !upperSet {
		upper = lower
	}
	if lower > upper {
		lower, upper = upper, lower
	}
	options["port_hop_interval"] = fmt.Sprintf("%ds", int64(lower/time.Second))
	if upper > lower {
		options["port_hop_interval_max"] = fmt.Sprintf("%ds", int64(upper/time.Second))
	} else {
		delete(options, "port_hop_interval_max")
	}
	return nil
}

func parseSingboxPortHopDuration(raw string) (time.Duration, bool, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 0, false, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 || duration%time.Second != 0 {
		return 0, false, fmt.Errorf("invalid sing-box hysteria port hop interval %q", raw)
	}
	return duration, true, nil
}

func portHopOptionString(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}
