package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IPDetectService 用于检测本机可用的公网IP
type IPDetectService struct{}

// IPInfo 存储IP信息
type IPInfo struct {
	IP       string `json:"ip"`
	IsPublic bool   `json:"isPublic"`
	Version  int    `json:"version"` // 4 或 6
}

// 用于验证IP的远程API列表（按优先级排序）
var ipCheckAPIs = []string{
	"https://icanhazip.com",
	"https://api.ip.sb/ip",
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://ipinfo.io/ip",
}

// IPv4专用API列表（这些API只返回IPv4地址）
var ipv4CheckAPIs = []string{
	"https://api4.ipify.org",
	"https://ipv4.icanhazip.com",
	"https://v4.ident.me",
	"https://ipv4.ip.sb",
}

// IPv6专用API列表（这些API只返回IPv6地址）
var ipv6CheckAPIs = []string{
	"https://api6.ipify.org",
	"https://ipv6.icanhazip.com",
	"https://v6.ident.me",
	"https://ipv6.ip.sb",
}

// GetLocalIPs 获取本机所有网卡上的IP地址
func (s *IPDetectService) GetLocalIPs() []IPInfo {
	var ips []IPInfo

	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range interfaces {
		// 跳过未启用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		// 跳过回环接口
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil {
				continue
			}

			// 跳过回环地址
			if ip.IsLoopback() {
				continue
			}

			// 跳过链路本地地址
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}

			ipStr := ip.String()
			version := 4
			if ip.To4() == nil {
				version = 6
			}

			// 判断是否为公网IP
			isPublic := !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()

			ips = append(ips, IPInfo{
				IP:       ipStr,
				IsPublic: isPublic,
				Version:  version,
			})
		}
	}

	return ips
}

// VerifyPublicIP 通过绑定源IP请求远程API验证IP是否为有效公网出口
func (s *IPDetectService) VerifyPublicIP(sourceIP string) (string, bool) {
	// 解析IP以确定版本
	ip := net.ParseIP(sourceIP)
	if ip == nil {
		return "", false
	}

	// 创建一个自定义的 dialer，绑定源IP
	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: ip,
		},
		Timeout: 2 * time.Second,
	}

	// 创建自定义的 transport
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   2 * time.Second,
	}

	// 按顺序尝试各个API
	for _, apiURL := range ipCheckAPIs {
		outboundIP, ok := s.queryAPI(client, apiURL)
		if ok {
			return outboundIP, true
		}
	}

	return "", false
}

// queryAPI 查询单个API获取出口IP
func (s *IPDetectService) queryAPI(client *http.Client, apiURL string) (string, bool) {
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}

	ip := strings.TrimSpace(string(body))
	// 验证返回的是有效IP
	if net.ParseIP(ip) == nil {
		return "", false
	}

	return ip, true
}

// GetVerifiedPublicIPs 获取经过验证的公网IP列表
// 返回：IPv4列表在前，IPv6列表在后
func (s *IPDetectService) GetVerifiedPublicIPs() []string {
	localIPs := s.GetLocalIPs()

	var verifiedIPv4 []string
	var verifiedIPv6 []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 并发验证所有可能的公网IP
	for _, ipInfo := range localIPs {
		if !ipInfo.IsPublic {
			continue
		}

		wg.Add(1)
		go func(info IPInfo) {
			defer wg.Done()

			outboundIP, ok := s.VerifyPublicIP(info.IP)
			if ok && outboundIP == info.IP {
				mu.Lock()
				if info.Version == 4 {
					verifiedIPv4 = append(verifiedIPv4, info.IP)
				} else {
					verifiedIPv6 = append(verifiedIPv6, info.IP)
				}
				mu.Unlock()
			}
		}(ipInfo)
	}

	wg.Wait()

	// IPv4在前，IPv6在后
	result := append(verifiedIPv4, verifiedIPv6...)
	return result
}

// GetAllAvailableIPs 获取所有可用的IP（包括未验证的公网IP）
// 用于快速获取，不进行远程验证
// 返回：IPv4列表在前，IPv6列表在后
func (s *IPDetectService) GetAllAvailableIPs() []string {
	localIPs := s.GetLocalIPs()

	var ipv4List []string
	var ipv6List []string

	for _, ipInfo := range localIPs {
		// 只返回公网IP
		if !ipInfo.IsPublic {
			continue
		}

		if ipInfo.Version == 4 {
			ipv4List = append(ipv4List, ipInfo.IP)
		} else {
			ipv6List = append(ipv6List, ipInfo.IP)
		}
	}

	// IPv4在前，IPv6在后
	return append(ipv4List, ipv6List...)
}

// GetDefaultOutboundIP 获取默认出口IP（不绑定源IP）
func (s *IPDetectService) GetDefaultOutboundIP() (string, bool) {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	for _, apiURL := range ipCheckAPIs {
		ip, ok := s.queryAPI(client, apiURL)
		if ok {
			return ip, true
		}
	}

	return "", false
}

// GetOutboundIPs 通过外部API获取真实的出口IP（IPv4和IPv6）
// 这个方法不依赖本地网卡检测，直接查询外部API获取真实出口IP
// 返回：IPv4列表在前，IPv6列表在后
func (s *IPDetectService) GetOutboundIPs() []string {
	var ipv4List []string
	var ipv6List []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 增加超时时间，因为在中国大陆访问这些API可能较慢
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// 并发获取IPv4地址
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, apiURL := range ipv4CheckAPIs {
			ip, ok := s.queryAPI(client, apiURL)
			if ok {
				parsedIP := net.ParseIP(ip)
				// 确保返回的是IPv4地址
				if parsedIP != nil && parsedIP.To4() != nil {
					mu.Lock()
					// 避免重复
					found := false
					for _, existingIP := range ipv4List {
						if existingIP == ip {
							found = true
							break
						}
					}
					if !found {
						ipv4List = append(ipv4List, ip)
					}
					mu.Unlock()
					return // 成功获取一个就够了
				}
			}
		}
	}()

	// 并发获取IPv6地址
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, apiURL := range ipv6CheckAPIs {
			ip, ok := s.queryAPI(client, apiURL)
			if ok {
				parsedIP := net.ParseIP(ip)
				// 确保返回的是IPv6地址
				if parsedIP != nil && parsedIP.To4() == nil {
					mu.Lock()
					// 避免重复
					found := false
					for _, existingIP := range ipv6List {
						if existingIP == ip {
							found = true
							break
						}
					}
					if !found {
						ipv6List = append(ipv6List, ip)
					}
					mu.Unlock()
					return // 成功获取一个就够了
				}
			}
		}
	}()

	wg.Wait()

	// IPv4在前，IPv6在后
	return append(ipv4List, ipv6List...)
}
