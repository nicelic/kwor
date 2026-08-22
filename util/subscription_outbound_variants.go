package util

import (
	"encoding/json"
	"strings"
)

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
	return CloneJSONMap(src)
}

// CloneJSONMap returns an isolated copy of a JSON-shaped object. Subscription
// rendering creates address-specific outbound variants, so nested TLS and
// transport values must not remain shared between those variants.
func CloneJSONMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = cloneJSONValue(value)
	}
	return dst
}

func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return CloneJSONMap(typed)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	case []map[string]interface{}:
		cloned := make([]map[string]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = CloneJSONMap(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return typed
	}
}
