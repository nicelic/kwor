package service

import (
	"encoding/json"
	"strings"

	"github.com/alireza0/s-ui/util"
)

func normalizeSingboxRuntimeOutbounds(outbounds []json.RawMessage) ([]json.RawMessage, string, error) {
	normalized := make([]json.RawMessage, 0, len(outbounds))
	tlsStore := ""

	for _, outboundRaw := range outbounds {
		outbound := map[string]interface{}{}
		if err := json.Unmarshal(outboundRaw, &outbound); err != nil {
			return nil, "", err
		}

		store := sanitizeSingboxRuntimeOutbound(outbound)
		if tlsStore == "" && store != "" {
			tlsStore = store
		}

		raw, err := json.Marshal(outbound)
		if err != nil {
			return nil, "", err
		}
		normalized = append(normalized, raw)
	}

	return normalized, tlsStore, nil
}

func sanitizeSingboxRuntimeOutbound(outbound map[string]interface{}) string {
	if outbound == nil {
		return ""
	}

	protocol := strings.ToLower(strings.TrimSpace(firstString(outbound["type"])))

	delete(outbound, "mihomo_common")
	delete(outbound, "mihomo_hy2")
	delete(outbound, "mihomo_fast_open")
	delete(outbound, "fast_open")

	util.SanitizeSingboxSubscriptionOutbound(outbound)

	if protocol == "trusttunnel" {
		util.SanitizeTrustTunnelOutbound(outbound)
	}
	if protocol == "hysteria2" {
		util.SanitizeOptionalNetworkField(outbound)
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		return ""
	}

	tlsStore := extractSingboxRuntimeTLSStore(tlsMap)
	delete(tlsMap, "mihomo_use_fingerprint")
	delete(tlsMap, "fingerprint")
	delete(tlsMap, "tls_store")
	delete(tlsMap, "store")

	if len(tlsMap) == 0 {
		delete(outbound, "tls")
	}

	return tlsStore
}

func extractSingboxRuntimeTLSStore(tlsMap map[string]interface{}) string {
	if tlsMap == nil {
		return ""
	}

	for _, key := range []string{"tls_store", "store"} {
		store, _ := tlsMap[key].(string)
		store = normalizeCertificateStoreValue(store)
		if store != "" {
			return store
		}
	}

	return ""
}
