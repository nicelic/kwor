package service

import (
	"strings"

	"github.com/alireza0/s-ui/util"
)

// isSupportedMihomoInboundType is the single runtime allowlist for Mihomo
// listeners. In particular, Hysteria v1 and the old standalone ShadowTLS
// wrapper are not Mihomo listener types; they must not reach config
// generation, nftables, traffic accounting, or client policies.
func isSupportedMihomoInboundType(inboundType string) bool {
	return util.SupportsMihomoRuntimeListenerType(inboundType)
}

func isRemovedMihomoInboundType(inboundType string) bool {
	switch strings.ToLower(strings.TrimSpace(inboundType)) {
	case "hysteria", "shadowtls":
		return true
	default:
		return false
	}
}
