package service

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/alireza0/s-ui/util/common"
)

const (
	maxSessionMaxAgeMinutes = 365 * 24 * 60
	maxTrafficAgeDays       = 36500
	maxSubUpdatesHours      = 365 * 24
)

func normalizePanelRoutePath(value string, generateWhenEmpty bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if generateWhenEmpty {
			return generateRandomSubPath(), nil
		}
		return "/", nil
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.ContainsAny(value, `\\?#%:*`) {
		return "", common.NewError("路径不能包含空白字符、\\、?、#、%、: 或 *")
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	parts := strings.Split(value, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			return "", common.NewError("路径不能包含 . 或 .. 段")
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return "/", nil
	}
	return "/" + strings.Join(cleaned, "/") + "/", nil
}

func normalizeListenAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.EqualFold(value, "localhost") {
		return "localhost", nil
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	address, err := netip.ParseAddr(value)
	if err != nil {
		return "", common.NewError("监听地址必须为空、localhost 或有效的 IPv4/IPv6 地址")
	}
	return address.String(), nil
}

func normalizeConfiguredHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.ContainsAny(value, "/?#@") {
		return "", common.NewError("域名不能包含协议、端口、路径、查询参数或空白字符")
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	if address, err := netip.ParseAddr(value); err == nil {
		return address.String(), nil
	}
	if strings.Contains(value, ":") {
		return "", common.NewError("域名不能包含端口；IPv6 地址请单独填写")
	}
	value = strings.ToLower(strings.TrimSuffix(value, "."))
	if value == "" || len(value) > 253 {
		return "", common.NewError("域名长度无效")
	}
	if value == "localhost" {
		return value, nil
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", common.NewError("域名标签无效")
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
				return "", common.NewError("域名只能使用 ASCII 字母、数字、连字符和点")
			}
		}
	}
	return value, nil
}

func NormalizeSubscriptionBaseURI(value string) (string, error) {
	return normalizeAbsoluteHTTPURI(value, true)
}

func normalizePanelURI(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return normalizeAbsoluteHTTPURI(value, false)
}

func normalizeAbsoluteHTTPURI(value string, trailingSlash bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", common.NewError("订阅 URI 不能为空")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return "", common.NewError("URI 必须是绝对 HTTP 或 HTTPS 地址")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", common.NewError("URI 不能包含查询参数或片段")
	}
	host, err := normalizeConfiguredHost(parsed.Hostname())
	if err != nil {
		return "", err
	}
	port := parsed.Port()
	if port != "" {
		parsedPort, parseErr := strconv.Atoi(port)
		if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", common.NewError("URI 端口必须在 1-65535 之间")
		}
	}
	if strings.Contains(host, ":") {
		parsed.Host = "[" + host + "]"
	} else {
		parsed.Host = host
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if trailingSlash {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/"
	}
	return parsed.String(), nil
}

func normalizeGenericSettingsNumber(key string, value string) (string, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return "", common.NewErrorf("%s 必须是非负整数", key)
	}
	switch key {
	case "sessionMaxAge":
		if parsed > maxSessionMaxAgeMinutes {
			return "", common.NewErrorf("sessionMaxAge 不能超过 %d 分钟", maxSessionMaxAgeMinutes)
		}
	case "trafficAge":
		if parsed > maxTrafficAgeDays {
			return "", common.NewErrorf("trafficAge 不能超过 %d 天", maxTrafficAgeDays)
		}
	case "subUpdates":
		if parsed == 0 {
			return "", common.NewError("subUpdates 必须大于 0")
		}
		if parsed > maxSubUpdatesHours {
			return "", common.NewErrorf("subUpdates 不能超过 %d 小时", maxSubUpdatesHours)
		}
	}
	return strconv.Itoa(parsed), nil
}
