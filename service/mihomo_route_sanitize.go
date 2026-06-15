package service

import "encoding/json"

var mihomoDisabledRouteRuleKeys = []string{
	"clash_mode",
	"ip_version",
	"package_name",
	"protocol",
	"user",
}

var mihomoTransientRouteRuleKeys = []string{
	"type",
	"mode",
	"rules",
	"invert",
	"override_address",
	"override_port",
	"network_strategy",
	"fallback_delay",
	"udp_disable_domain_unmapping",
	"udp_connect",
	"udp_timeout",
	"sniffer",
	"timeout",
	"strategy",
	"server",
}

func sanitizeMihomoConfigJSON(config json.RawMessage) (json.RawMessage, error) {
	if len(config) == 0 {
		return config, nil
	}

	var document map[string]interface{}
	if err := json.Unmarshal(config, &document); err != nil {
		return nil, err
	}

	if ipv6, ok := toBool(document["ipv6"]); ok {
		document["ipv6"] = ipv6
	} else {
		delete(document, "ipv6")
	}

	if tcpConcurrent, ok := toBool(document["tcp-concurrent"]); ok {
		document["tcp-concurrent"] = tcpConcurrent
	} else {
		delete(document, "tcp-concurrent")
	}

	if dns := sanitizeMihomoDNSConfig(document["dns"]); len(dns) > 0 {
		document["dns"] = dns
	} else {
		delete(document, "dns")
	}

	route, ok := document["route"].(map[string]interface{})
	if !ok || route == nil {
		return json.Marshal(document)
	}

	route["no_resolve"] = sanitizeMihomoRouteNoResolve(route)
	delete(route, "no-resolve")
	delete(route, "noResolve")

	rawRules, ok := route["rules"].([]interface{})
	if !ok {
		return json.Marshal(document)
	}

	route["rules"] = sanitizeMihomoRouteRules(rawRules)
	return json.Marshal(document)
}

func sanitizeMihomoRouteNoResolve(route map[string]interface{}) bool {
	if route == nil {
		return true
	}
	if enabled, ok := toBool(route["no_resolve"]); ok {
		return enabled
	}
	if enabled, ok := toBool(route["no-resolve"]); ok {
		return enabled
	}
	if enabled, ok := toBool(route["noResolve"]); ok {
		return enabled
	}
	return true
}

func sanitizeMihomoRouteRules(rules []interface{}) []interface{} {
	sanitized := make([]interface{}, 0, len(rules))
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok || rule == nil {
			continue
		}

		if logicalType, _ := rule["type"].(string); logicalType == "logical" {
			continue
		}

		action, _ := rule["action"].(string)
		if action != "route" && action != "reject" {
			continue
		}

		for _, key := range mihomoDisabledRouteRuleKeys {
			delete(rule, key)
		}
		for _, key := range mihomoTransientRouteRuleKeys {
			delete(rule, key)
		}

		if action != "route" {
			delete(rule, "outbound")
		}
		delete(rule, "no_drop")
		if action != "reject" {
			delete(rule, "method")
		}

		sanitized = append(sanitized, rule)
	}
	return sanitized
}
