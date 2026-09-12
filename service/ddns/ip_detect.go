package ddns

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type InterfaceInfo struct {
	Name  string   `json:"name"`
	IPs   []string `json:"ips"`
	Flags string   `json:"flags"`
}

var defaultIPv4URLs = []string{
	"https://api-ipv4.ip.sb/ip",
	"https://api4.ipify.org",
	"https://v4.ident.me",
}

var defaultIPv6URLs = []string{
	"https://api-ipv6.ip.sb/ip",
	"https://api64.ipify.org",
	"https://v6.ident.me",
	"https://speed.neu6.edu.cn/getIP.php",
}

func DetectPublicIP(ctx context.Context, ipType string, customURL string) (string, error) {
	urls := make([]string, 0, 8)
	seen := make(map[string]bool)

	addURL := func(u string) {
		u = strings.TrimSpace(u)
		if u != "" && !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	if strings.TrimSpace(customURL) != "" {
		rawUrls := strings.FieldsFunc(customURL, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r'
		})
		for _, u := range rawUrls {
			addURL(u)
		}
	}

	if ipType == "ipv4" {
		for _, u := range defaultIPv4URLs {
			addURL(u)
		}
	} else if ipType == "ipv6" {
		for _, u := range defaultIPv6URLs {
			addURL(u)
		}
	} else {
		return "", errors.New("unsupported ipType: " + ipType)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var lastErr error

	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "curl/7.88.1")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		ipStr := strings.TrimSpace(string(body))
		parsedIP := net.ParseIP(ipStr)
		if parsedIP == nil {
			lastErr = fmt.Errorf("invalid IP returned from %s: %s", u, ipStr)
			continue
		}

		if ipType == "ipv4" && parsedIP.To4() != nil {
			return parsedIP.String(), nil
		}
		if ipType == "ipv6" && parsedIP.To4() == nil && parsedIP.To16() != nil {
			// Ensure it's a global unicast address
			if parsedIP.IsGlobalUnicast() && !parsedIP.IsPrivate() {
				return parsedIP.String(), nil
			}
			return parsedIP.String(), nil
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("failed to detect %s: %w", ipType, lastErr)
	}
	return "", fmt.Errorf("failed to detect %s from all sources", ipType)
}

// IsPublicIPv4 判断是否为公网 IPv4 地址（排除私网 10/172.16/192.168、回环 127、链路本地 169.254、未指定 0.0.0.0 等）
func IsPublicIPv4(ip net.IP) bool {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return false
	}
	if ipv4.IsLoopback() || ipv4.IsLinkLocalUnicast() || ipv4.IsLinkLocalMulticast() || ipv4.IsPrivate() || ipv4.IsUnspecified() {
		return false
	}
	return true
}

// IsPublicIPv6 判断是否为公网 IPv6 地址（必须为 GlobalUnicast，排除 ULA fc00::/7、链路本地 fe80::/10 等）
func IsPublicIPv6(ip net.IP) bool {
	if ip.To4() != nil {
		return false
	}
	if ip.To16() == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return false
	}
	return ip.IsGlobalUnicast()
}

func DetectInterfaceIP(ifaceName string, ipType string, publicOnly bool) (string, error) {
	iface, err := net.InterfaceByName(strings.TrimSpace(ifaceName))
	if err != nil {
		return "", err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}

		if ipType == "ipv4" {
			ipv4 := ip.To4()
			if ipv4 != nil {
				if publicOnly {
					if IsPublicIPv4(ipv4) {
						return ipv4.String(), nil
					}
				} else {
					return ipv4.String(), nil
				}
			}
		} else if ipType == "ipv6" {
			if ip.To4() == nil && ip.To16() != nil {
				if publicOnly {
					if IsPublicIPv6(ip) {
						return ip.String(), nil
					}
				} else {
					if ip.IsGlobalUnicast() {
						return ip.String(), nil
					}
				}
			}
		}
	}

	if publicOnly {
		return "", fmt.Errorf("no public %s found on interface %s", ipType, ifaceName)
	}
	return "", fmt.Errorf("no valid %s found on interface %s", ipType, ifaceName)
}

func GetSystemInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	result := make([]InterfaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		ips := make([]string, 0)
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() {
				// Don't show fe80 link-local or 169.254.x.x link-local
				if ip.To4() != nil || ip.IsGlobalUnicast() {
					ips = append(ips, ip.String())
				}
			}
		}
		result = append(result, InterfaceInfo{
			Name:  iface.Name,
			IPs:   ips,
			Flags: iface.Flags.String(),
		})
	}
	return result, nil
}
