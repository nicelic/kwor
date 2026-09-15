package ddns

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
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
	"https://ipv4.icanhazip.com",
}

var defaultIPv6URLs = []string{
	"https://api-ipv6.ip.sb/ip",
	"https://api64.ipify.org",
	"https://v6.ident.me",
	"https://speed.neu6.edu.cn/getIP.php",
	"https://ipv6.icanhazip.com",
}

func DetectPublicIPs(ctx context.Context, ipType string, customURL string) ([]string, error) {
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
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' '
		})
		for _, u := range rawUrls {
			addURL(u)
		}
	}

	if len(urls) == 0 {
		if ipType == "ipv4" {
			for _, u := range defaultIPv4URLs {
				addURL(u)
			}
		} else if ipType == "ipv6" {
			for _, u := range defaultIPv6URLs {
				addURL(u)
			}
		} else {
			return nil, errors.New("unsupported ipType: " + ipType)
		}
	}

	client := GetSharedDDNSHTTPClient(8 * time.Second)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ipMap := make(map[string]struct{})
	var errList []string

	for _, u := range urls {
		targetURL := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
			if err != nil {
				mu.Lock()
				errList = append(errList, fmt.Sprintf("%s: %v", targetURL, err))
				mu.Unlock()
				return
			}
			req.Header.Set("User-Agent", "curl/7.88.1")

			resp, err := client.Do(req)
			if err != nil {
				mu.Lock()
				errList = append(errList, fmt.Sprintf("%s: %v", targetURL, err))
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			if err != nil {
				mu.Lock()
				errList = append(errList, fmt.Sprintf("%s: %v", targetURL, err))
				mu.Unlock()
				return
			}

			rawStr := strings.TrimSpace(string(body))
			cleaned := strings.Trim(rawStr, "\"' \r\n\t")
			parsedIP := net.ParseIP(cleaned)
			if parsedIP == nil {
				tokens := strings.Fields(cleaned)
				if len(tokens) > 0 {
					parsedIP = net.ParseIP(tokens[0])
				}
			}
			if parsedIP == nil {
				mu.Lock()
				errList = append(errList, fmt.Sprintf("%s: invalid IP '%s'", targetURL, cleaned))
				mu.Unlock()
				return
			}

			if ipType == "ipv4" && parsedIP.To4() != nil {
				if IsPublicIPv4(parsedIP) {
					mu.Lock()
					ipMap[parsedIP.String()] = struct{}{}
					mu.Unlock()
				}
			} else if ipType == "ipv6" && parsedIP.To4() == nil && parsedIP.To16() != nil {
				if IsPublicIPv6(parsedIP) {
					mu.Lock()
					ipMap[parsedIP.String()] = struct{}{}
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	result := make([]string, 0, len(ipMap))
	for ip := range ipMap {
		result = append(result, ip)
	}
	sort.Strings(result)

	if len(result) == 0 {
		if len(errList) > 0 {
			return nil, fmt.Errorf("failed to detect %s: %s", ipType, strings.Join(errList, "; "))
		}
		return nil, fmt.Errorf("no public %s found from urls", ipType)
	}
	return result, nil
}

func DetectPublicIP(ctx context.Context, ipType string, customURL string) (string, error) {
	ips, err := DetectPublicIPs(ctx, ipType, customURL)
	if err != nil {
		return "", err
	}
	if len(ips) > 0 {
		return ips[0], nil
	}
	return "", fmt.Errorf("failed to detect %s", ipType)
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

func DetectInterfaceIPs(ifaceName string, ipType string, publicOnly bool) ([]string, error) {
	iface, err := net.InterfaceByName(strings.TrimSpace(ifaceName))
	if err != nil {
		return nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	ipMap := make(map[string]struct{})
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
						ipMap[ipv4.String()] = struct{}{}
					}
				} else {
					ipMap[ipv4.String()] = struct{}{}
				}
			}
		} else if ipType == "ipv6" {
			if ip.To4() == nil && ip.To16() != nil {
				if publicOnly {
					if IsPublicIPv6(ip) {
						ipMap[ip.String()] = struct{}{}
					}
				} else {
					if ip.IsGlobalUnicast() {
						ipMap[ip.String()] = struct{}{}
					}
				}
			}
		}
	}

	result := make([]string, 0, len(ipMap))
	for ip := range ipMap {
		result = append(result, ip)
	}
	sort.Strings(result)

	if len(result) == 0 {
		if publicOnly {
			return nil, fmt.Errorf("no public %s found on interface %s", ipType, ifaceName)
		}
		return nil, fmt.Errorf("no valid %s found on interface %s", ipType, ifaceName)
	}
	return result, nil
}

func DetectInterfaceIP(ifaceName string, ipType string, publicOnly bool) (string, error) {
	ips, err := DetectInterfaceIPs(ifaceName, ipType, publicOnly)
	if err != nil {
		return "", err
	}
	if len(ips) > 0 {
		return ips[0], nil
	}
	return "", fmt.Errorf("no %s found on interface %s", ipType, ifaceName)
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
