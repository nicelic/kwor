package service

import "github.com/alireza0/s-ui/database/model"

func collectInboundLimitRanges(inbound *model.Inbound) []portRange {
	return collectInboundBlockRanges(inbound)
}

func collectMihomoInboundLimitRanges(inbound *model.MihomoInbound) []portRange {
	return collectMihomoInboundBlockRanges(inbound)
}

func expandPortRangesToPorts(ranges []portRange) []int {
	normalized := normalizeNftPortRanges(ranges)
	if len(normalized) == 0 {
		return nil
	}

	ports := make([]int, 0)
	for _, current := range normalized {
		for port := current.start; port <= current.end; port++ {
			ports = append(ports, port)
		}
	}
	return normalizePortList(ports)
}
