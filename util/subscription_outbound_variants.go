package util

import "strings"

// BuildMixedSubscriptionOutboundPair turns a manually configured mixed proxy
// endpoint into the SOCKS and HTTP client outbounds that subscriptions support.
func BuildMixedSubscriptionOutboundPair(outbound map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	if outbound == nil {
		return nil, nil
	}

	tag, _ := outbound["tag"].(string)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, nil
	}

	socks := cloneSubscriptionOutboundVariant(outbound)
	socks["type"] = "socks"
	socks["tag"] = tag + "-socks"
	if _, exists := socks["version"]; !exists {
		socks["version"] = "5"
	}

	http := cloneSubscriptionOutboundVariant(outbound)
	http["type"] = "http"
	http["tag"] = tag + "-http"
	delete(http, "version")
	delete(http, "network")
	delete(http, "udp_over_tcp")

	return socks, http
}

func cloneSubscriptionOutboundVariant(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
