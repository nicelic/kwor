package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

const (
	mihomoPortHopMaxRangeTextBytes = 512
	mihomoPortHopMaxSegments       = 32
	mihomoPortHopMaxPorts          = 4096
	mihomoPortHopMinInterval       = 10 * time.Second
)

// sanitizeMihomoHysteria2PortHop normalizes the panel-only Hysteria2 port-hop
// fields before persistence. The nft REDIRECT rule uses one set expression, but
// both monitoring and policy reconciliation still need a bounded input.
func sanitizeMihomoHysteria2PortHop(inbound *model.MihomoInbound) error {
	if inbound == nil || !strings.EqualFold(strings.TrimSpace(inbound.Type), "hysteria2") {
		return nil
	}

	options := map[string]interface{}{}
	if len(inbound.Options) > 0 {
		if err := json.Unmarshal(inbound.Options, &options); err != nil {
			return err
		}
	}

	rawRange := firstString(options["port_hop_range"])
	normalizedRange, err := normalizeMihomoPortHopRange(rawRange)
	if err != nil {
		return err
	}
	if normalizedRange == "" {
		delete(options, "port_hop_range")
		delete(options, "port_hop_interval")
		delete(options, "port_hop_interval_max")
	} else {
		options["port_hop_range"] = normalizedRange
		if err := normalizeMihomoPortHopIntervals(options); err != nil {
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

func normalizeMihomoPortHopRange(raw string) (string, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", nil
	}
	if err := validateMihomoManagedPortRange(input, "mihomo hysteria2 port hop range"); err != nil {
		return "", err
	}
	_, normalized, err := parseStrictPortRanges(input)
	if err != nil {
		return "", fmt.Errorf("invalid mihomo hysteria2 port hop range: %w", err)
	}
	return strings.ReplaceAll(normalized, ":", "-"), nil
}

func validateMihomoManagedPortRange(input string, label string) error {
	input = strings.TrimSpace(input)
	if len([]byte(input)) > mihomoPortHopMaxRangeTextBytes {
		return fmt.Errorf("%s is too long (max %d bytes)", label, mihomoPortHopMaxRangeTextBytes)
	}

	ranges, normalized, err := parseStrictPortRanges(input)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", label, err)
	}
	_ = normalized
	if len(ranges) > mihomoPortHopMaxSegments {
		return fmt.Errorf("%s has too many segments (max %d)", label, mihomoPortHopMaxSegments)
	}
	if total := countPorts(ranges); total > mihomoPortHopMaxPorts {
		return fmt.Errorf("%s contains too many ports (max %d)", label, mihomoPortHopMaxPorts)
	}
	return nil
}

func normalizeMihomoPortHopIntervals(options map[string]interface{}) error {
	if options == nil {
		return nil
	}
	lower, lowerOK := parseMihomoPortHopDuration(firstString(options["port_hop_interval"]))
	upper, upperOK := parseMihomoPortHopDuration(firstString(options["port_hop_interval_max"]))
	if !lowerOK && !upperOK {
		delete(options, "port_hop_interval")
		delete(options, "port_hop_interval_max")
		return nil
	}
	if lowerOK && lower < mihomoPortHopMinInterval {
		return fmt.Errorf("mihomo hysteria2 port hop interval must be at least %s", mihomoPortHopMinInterval)
	}
	if upperOK && upper < mihomoPortHopMinInterval {
		return fmt.Errorf("mihomo hysteria2 port hop interval must be at least %s", mihomoPortHopMinInterval)
	}
	if !lowerOK {
		lower = upper
	}
	if !upperOK {
		upper = lower
	}
	if lower > upper {
		lower, upper = upper, lower
	}
	options["port_hop_interval"] = formatMihomoPortHopDuration(lower)
	if upper > lower {
		options["port_hop_interval_max"] = formatMihomoPortHopDuration(upper)
	} else {
		delete(options, "port_hop_interval_max")
	}
	return nil
}

func parseMihomoPortHopDuration(raw string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return 0, false
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil || duration <= 0 || duration%time.Second != 0 {
		return 0, false
	}
	return duration, true
}

func parseMihomoPortHopInterval(raw string) (time.Duration, bool) {
	duration, ok := parseMihomoPortHopDuration(raw)
	return duration, ok && duration >= mihomoPortHopMinInterval
}

func formatMihomoPortHopDuration(duration time.Duration) string {
	return fmt.Sprintf("%ds", int64(duration/time.Second))
}
